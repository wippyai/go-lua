package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// addCovariantExposureType appends a covariant-exposure fact for sourcePath
// toward a resolved contract type, when the contract is a mutable container view
// (array or record). A non-container contract is not a covariant exposure and is
// skipped.
func (l *lowerer) addCovariantExposureType(input *factflow.FactsInput, point cfg.Point, sourcePath path.Path, contract typ.Type) {
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return
	}
	kind, ok := covariantExposureKind(contract)
	if !ok {
		return
	}
	wide := l.valueFromTypeWithWitness(contract)
	if input.CovariantExposures == nil {
		input.CovariantExposures = make(map[cfg.Point][]factflow.CovariantExposure)
	}
	input.CovariantExposures[point] = append(input.CovariantExposures[point], factflow.NewCovariantExposure(sourcePath, wide, kind))
}

// covariantExposureKind selects the widening kind for a mutable container
// contract: an opaque-array element widen for an array, a record field rebuild
// for a record. Any other shape is not a mutable container view and emits no
// exposure. The call-boundary twin callresult.covariantExposureKind must
// classify identically; the layered architecture keeps factflow type-independent,
// so the two cannot share one helper.
func covariantExposureKind(contract typ.Type) (factflow.CovariantExposureKind, bool) {
	switch unwrap.Alias(contract).(type) {
	case *typ.Array:
		return factflow.CovariantExposureArray, true
	case *typ.Record:
		return factflow.CovariantExposureRecord, true
	default:
		return 0, false
	}
}

// aliasStrictlyWidens reports whether the target array element or record field(s)
// strictly supertype the source's, which is the covariant alias that needs an
// eager source widen.
func aliasStrictlyWidens(sourceType, targetType typ.Type) bool {
	if sourceElement, ok := arrayElementType(sourceType); ok {
		if targetElement, ok := arrayElementType(targetType); ok {
			return strictlyWidens(sourceElement, targetElement)
		}
		return false
	}
	sourceRecord, ok := unwrap.Alias(sourceType).(*typ.Record)
	if !ok || sourceRecord == nil {
		return false
	}
	targetRecord, ok := unwrap.Alias(targetType).(*typ.Record)
	if !ok || targetRecord == nil {
		return false
	}
	return recordHasStrictlyWiderField(sourceRecord, targetRecord, make(map[[2]typ.Type]bool))
}

// recordHasStrictlyWiderField reports whether any shared field of target
// strictly widens the same-named source field, recursing into nested records.
func recordHasStrictlyWiderField(source, target *typ.Record, visited map[[2]typ.Type]bool) bool {
	key := [2]typ.Type{source, target}
	if visited[key] {
		return false
	}
	visited[key] = true
	for i := range target.Fields {
		tf := target.Fields[i]
		sf, ok := recordField(source, tf.Name)
		if !ok {
			continue
		}
		if sf.Type == nil || tf.Type == nil {
			continue
		}
		if typ.IsAny(sf.Type) || typ.IsUnknown(sf.Type) || typ.IsAny(tf.Type) || typ.IsUnknown(tf.Type) {
			continue
		}
		if strictlyWidens(sf.Type, tf.Type) {
			return true
		}
		if sr, ok := unwrap.Alias(sf.Type).(*typ.Record); ok && sr != nil {
			if tr, ok := unwrap.Alias(tf.Type).(*typ.Record); ok && tr != nil {
				if recordHasStrictlyWiderField(sr, tr, visited) {
					return true
				}
			}
		}
	}
	return false
}

func recordField(r *typ.Record, name string) (typ.Field, bool) {
	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return r.Fields[i], true
		}
	}
	return typ.Field{}, false
}

func strictlyWidens(narrow, wide typ.Type) bool {
	return subtype.IsSubtype(narrow, wide) && !subtype.IsSubtype(wide, narrow)
}

// exposureContractElement strips a container-slot contract's outer optionality
// (array element / optional-field presence) to the element record when the
// stored object cannot itself be nil. The widen rebuilds the stored object's
// record structure, so the element record is the contract to widen toward.
func exposureContractElement(sourceType, contract typ.Type) typ.Type {
	if _, ok := unwrap.Alias(sourceType).(*typ.Record); !ok {
		return contract
	}
	inner := unwrap.Optional(contract)
	if inner == nil {
		return contract
	}
	if _, ok := unwrap.Alias(inner).(*typ.Record); ok {
		return inner
	}
	return contract
}

func arrayElementType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	array, ok := unwrap.Alias(t).(*typ.Array)
	if !ok || array == nil || array.Element == nil {
		return nil, false
	}
	return array.Element, true
}

func reachesArray(t typ.Type) bool {
	return reachesArrayDepth(t, 0)
}

func reachesArrayDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Array:
		return true
	case *typ.Alias:
		return reachesArrayDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesArrayDepth(v.Body, depth+1)
	default:
		return false
	}
}

func (l *lowerer) declaredReturnLocalContractForSymbol(id symbol.ID) (typ.Type, bool) {
	t, ok := l.returnLocalTypes[id]
	if !ok || !declaredReturnLocalContractType(t) {
		return nil, false
	}
	return t, true
}

func recordWithCallableField(t typ.Type) bool {
	return recordWithCallableFieldDepth(t, 0)
}

func recordWithCallableFieldDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return recordWithCallableFieldDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return recordWithCallableFieldDepth(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return recordWithCallableFieldDepth(v.Body, depth+1)
	case *typ.Record:
		for i := range v.Fields {
			if _, ok := typecall.Callable(v.Fields[i].Type); ok {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (l *lowerer) addAliasExposureValueSourceToContractType(input *factflow.FactsInput, point cfg.Point, source factflow.ValueSource, contract typ.Type) {
	sourcePath, sourceType, ok := l.aliasValueSource(source)
	if !ok {
		return
	}
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return
	}
	contract = exposureContractElement(sourceType, contract)
	if !aliasStrictlyWidens(sourceType, contract) {
		return
	}
	l.addCovariantExposureType(input, point, sourcePath, contract)
}

func (l *lowerer) aliasValueSource(source factflow.ValueSource) (path.Path, typ.Type, bool) {
	if l == nil {
		return path.Path{}, nil, false
	}
	var sourcePath path.Path
	var ok bool
	switch source.Kind {
	case factflow.ValueSourcePath:
		sourcePath, ok = pathFromRootSymbolKey(source.PathKey)
	case factflow.ValueSourceExpression:
		if !source.HasExpr || l.expressionPaths == nil {
			return path.Path{}, nil, false
		}
		sourcePath, ok = l.expressionPaths[source.ExprRef]
	default:
		return path.Path{}, nil, false
	}
	if !ok || sourcePath.Symbol == 0 {
		return path.Path{}, nil, false
	}
	sourceType, ok := l.aliasPathType(sourcePath)
	if !ok {
		return path.Path{}, nil, false
	}
	return sourcePath, sourceType, true
}

func dynamicIndexReadbackIntent(readKey bool, readValue bool) factflow.DynamicIndexReadbackIntent {
	switch {
	case readKey && readValue:
		return factflow.DynamicIndexReadbackKeyAndValue
	case readKey:
		return factflow.DynamicIndexReadbackKey
	case readValue:
		return factflow.DynamicIndexReadbackValue
	default:
		return factflow.DynamicIndexReadbackNone
	}
}
