package sourcevalue

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BoundDynamicRead is the single syntax-free request for concrete dynamic
// indexing. ValueInput/Visibility own the read value. When HasProofInput is
// set, ProofInput/ProofVisibility own every range coordinate in the same
// registered binder transaction. Form is the normalized index algebra from
// the factflow edge; no syntax is reconstructed from its abstract value.
type BoundDynamicRead struct {
	Registry        *axis.Registry
	TypeValues      *typevalue.Cache
	KeySpace        *keyspace.KeySpace
	Visibility      *visibility.Resolver
	ProofVisibility *visibility.Resolver
	Point           cfg.Point
	TablePath       pathdom.Path
	KeyPath         pathdom.Path
	TableValue      product.Value
	KeyValue        product.Value
	ValueInput      state.State
	ProofInput      state.State
	HasProofInput   bool
	ProjectPath     bool
	IndexForm       indexform.IndexForm
	ModuloInteger   bool
}

// ReadBoundDynamicValue resolves one BoundDynamicRead through the canonical
// registered factor binder and source-value algebra.
func ReadBoundDynamicValue(request BoundDynamicRead) (product.Value, bool) {
	query, evidence, ok := projectBoundDynamicReadEvidence(request)
	if !ok {
		return product.Value{}, false
	}
	return ResolveDynamicRead(query, evidence)
}

// BoundDynamicReadInRange reports the canonical binder's exact range proof
// for request. It is an observation of the same evidence transaction consumed
// by ReadBoundDynamicValue, not a second range solver.
func BoundDynamicReadInRange(request BoundDynamicRead) (bool, bool) {
	_, evidence, ok := projectBoundDynamicReadEvidence(request)
	if !ok {
		return false, false
	}
	return evidence.InRangeIndexEvidence(), true
}

func projectBoundDynamicReadEvidence(request BoundDynamicRead) (state.DynamicReadQuery, state.DynamicReadEvidence, bool) {
	reg, typeValues, ks := request.Registry, request.TypeValues, request.KeySpace
	resolver, point := request.Visibility, request.Point
	tablePath, keyPath := request.TablePath, request.KeyPath
	tableValue, keyValue, in := request.TableValue, request.KeyValue, request.ValueInput
	projectPath := request.ProjectPath
	if reg == nil || ks == nil || projectPath && tablePath.IsEmpty() {
		return state.DynamicReadQuery{}, state.DynamicReadEvidence{}, false
	}
	domain := state.RegisteredProductDomain(reg)
	pathKey := keyspace.Key{}
	ownerKey := keyspace.Key{}
	var tableKeys []pathaddr.StateKey
	var tableAddress visibility.DynamicReadAddress
	var hasTableAddress bool
	if !tablePath.IsEmpty() {
		tableAddress, hasTableAddress = visibility.FreezeDynamicReadAddress(ks, resolver, point, tablePath)
		if hasTableAddress {
			pathKey = tableAddress.Coordinate
			tableKeys = append([]pathaddr.StateKey(nil), tableAddress.StateKeys...)
		}
		if ownerAddress, ok := visibility.FreezeDynamicReadAddress(ks, resolver, point, tablePath.RootOnly()); ok {
			ownerKey = ownerAddress.Coordinate
		}
	}
	var keyKeys []pathaddr.StateKey
	var keyAddress visibility.DynamicReadAddress
	var hasKeyAddress bool
	if !keyPath.IsEmpty() {
		keyAddress, hasKeyAddress = visibility.FreezeDynamicReadAddress(ks, resolver, point, keyPath)
		if hasKeyAddress {
			keyKeys = append([]pathaddr.StateKey(nil), keyAddress.StateKeys...)
		}
	}
	ownerValue, hasOwnerValue := product.Value{}, false
	if projectPath {
		ownerValue = in.ReadValue(reg, statekey.SymbolValue(tablePath.Symbol))
		hasOwnerValue = !product.Equal(reg, ownerValue, product.Bottom(reg))
		if !hasOwnerValue && !product.Equal(reg, tableValue, product.Bottom(reg)) {
			// A ProjectPath request's table operand is, by contract, the
			// lexical root owner. Carry it when point-local State legitimately
			// omits the equivalent root storage spelling; Top is a valid abstract
			// owner and yields a conservative result rather than a closed hole.
			ownerValue, hasOwnerValue = tableValue, true
		}
	}
	query := state.DynamicReadQuery{
		KeySpace: ks, TypeValues: typeValues,
		TableValue: tableValue, KeyValue: keyValue, TablePath: pathKey, OwnerPath: ownerKey,
		OwnerValue: ownerValue, HasOwnerValue: hasOwnerValue,
		TableKeys: tableKeys, KeyKeys: keyKeys, ProjectPath: projectPath,
	}
	if !projectPath {
		query.RangeContainer, query.HasRangeContainer = tableValue, true
	} else if projected, ok := ProjectDynamicTableTypePath(reg, typeValues, tableValue, tablePath.Segments); ok {
		query.RangeContainer, query.HasRangeContainer = projected, true
	}
	proofResolver := request.ProofVisibility
	if proofResolver == nil {
		proofResolver = resolver
	}
	if request.IndexForm.Valid() && proofResolver != nil {
		shape, shapeOK := request.IndexForm.Shape()
		proofTableAddress, tableOK := visibility.FreezeDynamicReadAddress(ks, proofResolver, point, tablePath)
		if shapeOK && tableOK && proofTableAddress.HasVisible {
			rangeQuery := state.DynamicReadRangeQuery{
				Shape: shape, ArrayStateKey: proofTableAddress.Visible, ModuloInteger: request.ModuloInteger,
			}
			if affine, affineOK := request.IndexForm.Affine(); affineOK {
				if affinePath, pathOK := affine.Path(); pathOK {
					proofKeyAddress, keyOK := visibility.FreezeDynamicReadAddress(ks, proofResolver, point, affinePath)
					if keyOK && proofKeyAddress.HasRootOrVisible {
						rangeQuery.IndexStateKey = proofKeyAddress.RootOrVisible
						if proofKeyAddress.HasVisible {
							rangeQuery.IndexProofStateKey = proofKeyAddress.Visible
							rangeQuery.ArrayProofStateKey = proofTableAddress.Visible
						}
					}
				}
			}
			if request.IndexForm.Kind() != indexform.IndexFormAffine || rangeQuery.IndexStateKey != "" {
				query.Range, query.HasRange = rangeQuery, true
			}
		}
	}
	var evidence state.DynamicReadEvidence
	var err error
	if request.HasProofInput {
		evidence, err = domain.ProjectDynamicReadEvidenceWithProof(query, in, request.ProofInput)
	} else {
		evidence, err = domain.ProjectDynamicReadEvidence(query, in)
	}
	if err != nil {
		return state.DynamicReadQuery{}, state.DynamicReadEvidence{}, false
	}
	return query, evidence, true
}

// ProjectDynamicTableTypePath applies the canonical runtime index relation to
// a statically named suffix. It is used only to obtain a sound container type
// witness for range proofs; flow-sensitive path/heap values remain owned by
// the registered binder.
func ProjectDynamicTableTypePath(reg *axis.Registry, typeValues *typevalue.Cache, owner product.Value, segments []segment.Segment) (product.Value, bool) {
	current := owner
	for _, suffix := range segments {
		keyValue, ok := scalarSegmentValue(reg, suffix)
		if !ok {
			return product.Value{}, false
		}
		current, ok = runtimeDynamicIndexValue(reg, typeValues, current, keyValue, true)
		if !ok {
			return product.Value{}, false
		}
	}
	return current, true
}

func runtimeDynamicIndexValue(reg *axis.Registry, typeValues *typevalue.Cache, tableValue, keyValue product.Value, allowUnconstrained bool) (product.Value, bool) {
	if reg == nil || product.Equal(reg, tableValue, product.Bottom(reg)) {
		return product.Value{}, false
	}
	var value product.Value
	var ok bool
	if typeValues != nil {
		value, ok = typeValues.RuntimeIndex(reg, tableValue, keyValue)
	} else {
		value, ok = typevalue.RuntimeIndex(reg, tableValue, keyValue)
	}
	if !ok {
		// An exact table identity is not completed from type information: its
		// heap object is the authoritative finite producer. Missing or
		// incompatible heap state must remain fail-closed even when operand
		// types alone would prove a nonreturning index.
		if _, identified := product.Get(reg, tableValue, identity.Key).ID(); identified {
			return product.Value{}, false
		}
		if !allowUnconstrained {
			return product.Value{}, false
		}
		var tableTyped, keyTyped bool
		if typeValues != nil {
			_, tableTyped = typeValues.TypeOf(reg, tableValue)
			_, keyTyped = typeValues.TypeOf(reg, keyValue)
		} else {
			_, tableTyped = typevalue.TypeOf(reg, tableValue)
			_, keyTyped = typevalue.TypeOf(reg, keyValue)
		}
		if tableTyped && keyTyped {
			// Both operands are exact enough for the canonical type index
			// relation, and that relation has no normal result. Bottom is the
			// complete normal-flow answer; the operation boundary separately
			// retains the invalid-index diagnostic.
			return product.Bottom(reg), true
		}
		// Registered product axes are sparse: an omitted type witness and
		// omitted runtime-kind lane denote Top, not missing semantic authority.
		// Indexing such an unconstrained Lua value has the exact abstract result
		// Top. A concrete non-indexable type still fails above as a nonreturning
		// operation; only the genuinely unconstrained table reaches this case.
		if tableTyped {
			return product.Value{}, false
		}
		return InheritTopOriginEvidence(reg, product.Top(), tableValue), true
	}
	return InheritTopOriginEvidence(reg, value, tableValue), true
}

func scalarSegmentValue(reg *axis.Registry, seg segment.Segment) (product.Value, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typevalue.LiteralString(reg, seg.Name), true
	case segment.SegmentIndexInt:
		return typevalue.LiteralInt(reg, int64(seg.Index)), true
	default:
		return product.Value{}, false
	}
}

// StaticPathSegmentValue returns the canonical scalar key value for one
// statically named path segment. Symbolic relation construction uses this
// helper rather than maintaining a second field/index encoding beside the
// concrete dynamic-read kernel above.
func StaticPathSegmentValue(reg *axis.Registry, seg segment.Segment) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	return scalarSegmentValue(reg, seg)
}

// BindStaticPathRead freezes one descendant path read into the same registered
// factor query used by dynamic indexing. The target's final segment is the key
// operand; its parent is projected from the lexical root through exact path,
// heap and dynamic-index evidence. No State is needed to build the query, so
// concrete and formal carriers cannot diverge in path-resolution semantics.
func BindStaticPathRead(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	resolver *visibility.Resolver,
	point cfg.Point,
	target pathdom.Path,
	root product.Value,
) (state.DynamicReadQuery, bool) {
	if reg == nil || ks == nil || !ks.Valid() || resolver == nil || target.Symbol == 0 || len(target.Segments) == 0 ||
		!product.BelongsToRegistry(reg, root) {
		return state.DynamicReadQuery{}, false
	}
	parent := target.ParentView()
	parentAddress, ok := visibility.FreezeDynamicReadAddress(ks, resolver, point, parent)
	if !ok || parentAddress.Coordinate.Kind == keyspace.KindInvalid {
		return state.DynamicReadQuery{}, false
	}
	ownerAddress, ok := visibility.FreezeDynamicReadAddress(ks, resolver, point, target.RootOnly())
	if !ok || ownerAddress.Coordinate.Kind == keyspace.KindInvalid {
		return state.DynamicReadQuery{}, false
	}
	keyValue, ok := StaticPathSegmentValue(reg, target.Segments[len(target.Segments)-1])
	if !ok {
		return state.DynamicReadQuery{}, false
	}
	return state.DynamicReadQuery{
		KeySpace: ks, TypeValues: typeValues,
		TableKeys:  append([]pathaddr.StateKey(nil), parentAddress.StateKeys...),
		TableValue: root, TablePath: parentAddress.Coordinate,
		OwnerPath: ownerAddress.Coordinate, OwnerValue: root, HasOwnerValue: true,
		KeyValue: keyValue, ProjectPath: true,
	}, true
}
