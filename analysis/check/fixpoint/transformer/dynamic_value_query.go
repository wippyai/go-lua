package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/indexform"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// dynamicValueQuery is the one carrier-neutral request constructor for a
// canonical dynamic ValueTerm. Concrete and formal factor evaluators differ
// only in how they supply the resulting query's registered evidence.
func dynamicValueQuery(body *relationProgramBody, keys *keyspace.KeySpace, node valueNode, args []product.Value) (state.DynamicReadQuery, error) {
	if body == nil || body.relation.arena == nil || (node.op != valueDynamicRead && node.op != valueDynamicTableRead) || len(node.args) != 2 || len(args) < 2 {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: malformed dynamic value query")
	}
	tablePath, ok := guardedDynamicTermPath(body, node.path)
	if node.path != 0 && !ok {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: dynamic value path is not structurally bound")
	}
	keyPath, keyPathOK := guardedDynamicTermPath(body, node.keyPath)
	if node.keyPath != 0 && !keyPathOK {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: dynamic key path is not structurally bound")
	}
	rangePath, rangePathOK := guardedDynamicTermPath(body, node.rangePath)
	if node.rangePath != 0 && !rangePathOK {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: dynamic range path is not structurally bound")
	}
	tableAddress, _ := guardedDynamicPathAddress(body, node.point, tablePath)
	ownerAddress, _ := guardedDynamicPathAddress(body, node.point, tablePath.RootOnly())
	keyAddress, _ := guardedDynamicPathAddress(body, node.point, keyPath)
	rangeAddress, _ := guardedDynamicPathAddress(body, node.point, rangePath)
	typeValues := typevalue.NewCache()
	query := state.DynamicReadQuery{
		KeySpace: keys, TableKeys: tableAddress.StateKeys, KeyKeys: keyAddress.StateKeys,
		TableValue: args[0], TablePath: tableAddress.Coordinate, OwnerPath: ownerAddress.Coordinate,
		OwnerValue: args[0], HasOwnerValue: node.op == valueDynamicRead,
		ProjectPath: node.op == valueDynamicRead, KeyValue: args[1], TypeValues: typeValues,
	}
	if node.op == valueDynamicTableRead {
		query.RangeContainer, query.HasRangeContainer = args[0], true
	} else if projected, projectedOK := sourcevalue.ProjectDynamicTableTypePath(body.productDomain.Registry(), typeValues, args[0], tablePath.Segments); projectedOK {
		query.RangeContainer, query.HasRangeContainer = projected, true
	}
	if node.indexShape.Valid() && tableAddress.HasVisible {
		rangeQuery := state.DynamicReadRangeQuery{Shape: node.indexShape, ArrayStateKey: tableAddress.Visible}
		switch node.indexShape.Kind() {
		case indexform.IndexFormAffine:
			if rangeAddress.HasRootOrVisible {
				rangeQuery.IndexStateKey = rangeAddress.RootOrVisible
				if rangeAddress.HasVisible {
					rangeQuery.IndexProofStateKey = rangeAddress.Visible
					rangeQuery.ArrayProofStateKey = tableAddress.Visible
				}
				query.HasRange = true
			}
		case indexform.IndexFormModuloLength:
			if node.integerProof != 0 && len(args) >= 3 {
				rangeQuery.ModuloInteger = typevalue.HasIntegerType(body.productDomain.Registry(), args[2])
			}
			query.HasRange = true
		default:
			query.HasRange = true
		}
		if query.HasRange {
			query.Range = rangeQuery
		}
	}
	return query, nil
}

// formalDynamicValueQuery is the sole formal address morphism for a canonical
// dynamic ValueTerm. The term remains owned by the lexical body; every path
// address is rekeyed once into the destination fiber keyspace before the
// registered dynamic-read binder observes it.
func formalDynamicValueQuery(body *relationProgramBody, span formalFiberDescriptorSpan, node valueNode, args []product.Value) (state.DynamicReadQuery, error) {
	if body == nil || span.keys == nil || !span.keys.Valid() ||
		(node.op != valueDynamicRead && node.op != valueDynamicTableRead) || len(node.args) != 2 || len(args) < 2 {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: malformed formal dynamic value query")
	}
	tablePath, tableOK := guardedDynamicTermPath(body, node.path)
	if node.path != 0 && !tableOK {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: formal dynamic value path is not structurally bound")
	}
	keyPath, keyOK := guardedDynamicTermPath(body, node.keyPath)
	if node.keyPath != 0 && !keyOK {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: formal dynamic key path is not structurally bound")
	}
	rangePath, rangeOK := guardedDynamicTermPath(body, node.rangePath)
	if node.rangePath != 0 && !rangeOK {
		return state.DynamicReadQuery{}, fmt.Errorf("transformer: formal dynamic range path is not structurally bound")
	}
	rekeyKey := func(source keyspace.Key) (keyspace.Key, bool) {
		if source.Kind == keyspace.KindInvalid {
			return keyspace.Key{}, true
		}
		mapped, err := body.productDomain.RekeyStructuralKeyFormal(span.rekey, source)
		return mapped, err == nil
	}
	rekeyStateKey := func(source pathaddr.StateKey) (pathaddr.StateKey, bool) {
		if source == "" {
			return "", true
		}
		key, ok := body.keys.FromStateKey(source.PathKey())
		if !ok {
			return "", false
		}
		mapped, ok := rekeyKey(key)
		if !ok {
			return "", false
		}
		return pathaddr.StateKeyFromPathKey(span.keys.FormatReadOnly(mapped))
	}
	rekeyAddress := func(address visibility.DynamicReadAddress, present bool) (visibility.DynamicReadAddress, bool) {
		if !present {
			return visibility.DynamicReadAddress{}, false
		}
		var ok bool
		if address.Coordinate, ok = rekeyKey(address.Coordinate); !ok {
			return visibility.DynamicReadAddress{}, false
		}
		for index := range address.StateKeys {
			if address.StateKeys[index], ok = rekeyStateKey(address.StateKeys[index]); !ok {
				return visibility.DynamicReadAddress{}, false
			}
		}
		for _, raw := range []struct {
			value *pathaddr.StateKey
			has   bool
		}{{&address.Visible, address.HasVisible}, {&address.RootOrVisible, address.HasRootOrVisible}} {
			if raw.has {
				if *raw.value, ok = rekeyStateKey(*raw.value); !ok {
					return visibility.DynamicReadAddress{}, false
				}
			}
		}
		return address, true
	}
	tableAddress, tableAddressOK := guardedDynamicPathAddress(body, node.point, tablePath)
	tableAddress, tableAddressOK = rekeyAddress(tableAddress, tableAddressOK)
	ownerAddress, ownerOK := guardedDynamicPathAddress(body, node.point, tablePath.RootOnly())
	ownerAddress, ownerOK = rekeyAddress(ownerAddress, ownerOK)
	keyAddress, keyAddressOK := guardedDynamicPathAddress(body, node.point, keyPath)
	keyAddress, keyAddressOK = rekeyAddress(keyAddress, keyAddressOK)
	rangeAddress, rangeAddressOK := guardedDynamicPathAddress(body, node.point, rangePath)
	rangeAddress, rangeAddressOK = rekeyAddress(rangeAddress, rangeAddressOK)
	typeValues := typevalue.NewCache()
	query := state.DynamicReadQuery{
		KeySpace: span.keys, TableValue: args[0], KeyValue: args[1], TypeValues: typeValues,
		OwnerValue: args[0], HasOwnerValue: node.op == valueDynamicRead, ProjectPath: node.op == valueDynamicRead,
	}
	if tableAddressOK {
		query.TablePath, query.TableKeys = tableAddress.Coordinate, append([]pathaddr.StateKey(nil), tableAddress.StateKeys...)
	}
	if ownerOK {
		query.OwnerPath = ownerAddress.Coordinate
	}
	if keyAddressOK {
		query.KeyKeys = append([]pathaddr.StateKey(nil), keyAddress.StateKeys...)
	}
	if node.op == valueDynamicTableRead {
		query.RangeContainer, query.HasRangeContainer = args[0], true
	} else if projected, ok := sourcevalue.ProjectDynamicTableTypePath(body.productDomain.Registry(), typeValues, args[0], tablePath.Segments); ok {
		query.RangeContainer, query.HasRangeContainer = projected, true
	}
	if node.indexShape.Valid() && tableAddressOK && tableAddress.HasVisible {
		rangeQuery := state.DynamicReadRangeQuery{Shape: node.indexShape, ArrayStateKey: tableAddress.Visible}
		switch node.indexShape.Kind() {
		case indexform.IndexFormAffine:
			if rangeAddressOK && rangeAddress.HasRootOrVisible {
				rangeQuery.IndexStateKey = rangeAddress.RootOrVisible
				if rangeAddress.HasVisible {
					rangeQuery.IndexProofStateKey, rangeQuery.ArrayProofStateKey = rangeAddress.Visible, tableAddress.Visible
				}
				query.HasRange = true
			}
		case indexform.IndexFormModuloLength:
			if node.integerProof != 0 && len(args) >= 3 {
				rangeQuery.ModuloInteger = typevalue.HasIntegerType(body.productDomain.Registry(), args[2])
			}
			query.HasRange = true
		default:
			query.HasRange = true
		}
		if query.HasRange {
			query.Range = rangeQuery
		}
	}
	return query, nil
}

// resolveFormalDynamicValue is the single registered-factor observation for
// formal dynamic ValueTerms. Callers provide only their already-declared
// factors and recursive term evaluator; query construction and semantics stay
// shared.
func resolveFormalDynamicValue(
	body *relationProgramBody,
	span formalFiberDescriptorSpan,
	node valueNode,
	args []product.Value,
	factors []state.LaneFactor,
	evaluate func(ValueTerm) (product.Value, bool),
) (product.Value, bool) {
	queryArgs := append([]product.Value(nil), args...)
	if node.integerProof != 0 {
		if evaluate == nil {
			return product.Value{}, false
		}
		proof, exact := evaluate(node.integerProof)
		if !exact {
			return product.Value{}, false
		}
		queryArgs = append(queryArgs, proof)
	}
	query, err := formalDynamicValueQuery(body, span, node, queryArgs)
	if err != nil {
		return product.Value{}, false
	}
	evidence, err := body.productDomain.ProjectDynamicReadEvidenceFactors(query, factors)
	if err != nil {
		return product.Value{}, false
	}
	return sourcevalue.ResolveDynamicRead(query, evidence)
}
