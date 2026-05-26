package typ

import (
	"sort"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

var (
	recordMapKeyHash   = internal.FnvString("$mapKey")
	recordMapValueHash = internal.FnvString("$mapValue")
)

func buildFunctionType(
	typeParams []*TypeParam,
	params []Param,
	variadic Type,
	returns []Type,
	effects EffectInfo,
	spec SpecInfo,
	refinement RefinementInfo,
) *Function {
	h := uint64(kind.Function)
	for _, tp := range typeParams {
		h = internal.HashCombine(h, tp.Hash())
	}

	for _, p := range params {
		h = internal.HashCombine(h, p.Type.Hash())
		if p.Optional {
			h = internal.HashCombine(h, 1)
		}
	}

	if variadic != nil {
		h = internal.HashCombine(h, variadic.Hash())
	}

	for _, r := range returns {
		if r == nil {
			panic("FunctionBuilder.Build: nil entry in returns; normalize before building")
		}
		h = internal.HashCombine(h, r.Hash())
	}

	typeParamsCopy := make([]*TypeParam, len(typeParams))
	copy(typeParamsCopy, typeParams)
	paramsCopy := make([]Param, len(params))
	copy(paramsCopy, params)
	returnsCopy := make([]Type, len(returns))
	copy(returnsCopy, returns)
	softPrunable := softPruneParams(paramsCopy) || softPruneAny(variadic) || softPruneAny(returnsCopy...)
	containsAny := knownAnyTypeParams(typeParamsCopy) ||
		knownAnyParams(paramsCopy) ||
		knownContainsAny(variadic) ||
		knownAny(returnsCopy...)
	containsNever := knownNeverTypeParams(typeParamsCopy) ||
		knownNeverParams(paramsCopy) ||
		knownContainsNever(variadic) ||
		knownNever(returnsCopy...)
	containsTypeParam := knownTypeParamTypeParams(typeParamsCopy) ||
		knownTypeParamParams(paramsCopy) ||
		knownContainsTypeParam(variadic) ||
		knownTypeParam(returnsCopy...)
	containsInstantiated := knownInstantiatedTypeParams(typeParamsCopy) ||
		knownInstantiatedParams(paramsCopy) ||
		knownContainsInstantiated(variadic) ||
		knownInstantiated(returnsCopy...)
	containsRecursive := knownRecursiveTypeParams(typeParamsCopy) ||
		knownRecursiveParams(paramsCopy) ||
		knownContainsRecursive(variadic) ||
		knownRecursive(returnsCopy...)
	containsOpenRecursive := knownOpenRecursiveTypeParams(typeParamsCopy) ||
		knownOpenRecursiveParams(paramsCopy) ||
		knownContainsOpenRecursive(variadic) ||
		knownOpenRecursive(returnsCopy...)

	return &Function{
		TypeParams:            typeParamsCopy,
		Params:                paramsCopy,
		Variadic:              variadic,
		Returns:               returnsCopy,
		Effects:               effects,
		Spec:                  spec,
		Refinement:            refinement,
		hash:                  h,
		softPrunable:          softPrunable,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func buildRecordType(fields []Field, metatable, mapKey, mapValue Type, open bool, assumeSorted bool) *Record {
	sorted := make([]Field, len(fields))
	copy(sorted, fields)
	if !assumeSorted || !fieldsSortedByName(sorted) {
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
	}
	for i := range sorted {
		if sorted[i].Type == nil {
			sorted[i].Type = Unknown
		}
		if sorted[i].Optional {
			sorted[i].Type = normalizeOptionalFieldType(sorted[i].Type)
		}
	}

	if mapKey == nil && mapValue != nil {
		mapKey = Unknown
	}
	if mapKey != nil {
		mapKey = NormalizeTableKey(mapKey)
	}
	if mapValue == nil && mapKey != nil {
		mapValue = Unknown
	}

	h := uint64(kind.Record)
	for _, f := range sorted {
		h = internal.HashCombine(h, internal.FnvString(f.Name))
		h = internal.HashCombine(h, f.Type.Hash())
		if f.Optional {
			h = internal.HashCombine(h, 1)
		}
		if f.Readonly {
			h = internal.HashCombine(h, 2)
		}
	}

	if metatable != nil {
		h = internal.HashCombine(h, metatable.Hash())
	}
	if open {
		h = internal.HashCombine(h, 3)
	}
	if mapKey != nil {
		h = internal.HashCombine(h, recordMapKeyHash)
		h = internal.HashCombine(h, mapKey.Hash())
	}
	if mapValue != nil {
		h = internal.HashCombine(h, recordMapValueHash)
		h = internal.HashCombine(h, mapValue.Hash())
	}
	softPrunable := softPruneFields(sorted) || softPruneAny(metatable, mapKey, mapValue)
	containsAny := knownAnyFields(sorted) || knownAny(metatable, mapKey, mapValue)
	containsNever := knownNeverFields(sorted) || knownNever(metatable, mapKey, mapValue)
	containsTypeParam := knownTypeParamFields(sorted) || knownTypeParam(metatable, mapKey, mapValue)
	containsInstantiated := knownInstantiatedFields(sorted) || knownInstantiated(metatable, mapKey, mapValue)
	containsRecursive := knownRecursiveFields(sorted) || knownRecursive(metatable, mapKey, mapValue)
	containsOpenRecursive := knownOpenRecursiveFields(sorted) || knownOpenRecursive(metatable, mapKey, mapValue)
	containsCallableSurf := knownCallableSurfaceFields(sorted) || HasCallableSurface(metatable) || HasCallableSurface(mapValue)

	return &Record{
		Fields:                sorted,
		Metatable:             metatable,
		MapKey:                mapKey,
		MapValue:              mapValue,
		Open:                  open,
		sorted:                true,
		hash:                  h,
		softPrunable:          softPrunable,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
		containsCallableSurf:  containsCallableSurf,
	}
}

func knownCallableSurfaceFields(fields []Field) bool {
	for _, f := range fields {
		if HasCallableSurface(f.Type) {
			return true
		}
	}
	return false
}

func normalizeOptionalFieldType(t Type) Type {
	if t == nil {
		return Unknown
	}
	switch v := t.(type) {
	case *Annotated:
		inner := normalizeOptionalFieldType(v.Inner)
		if inner == v.Inner {
			return t
		}
		return NewAnnotated(inner, v.Annotations)
	case *Alias:
		return t
	case *Optional:
		if v.Inner == nil || v.Inner.Kind() == kind.Never || v.Inner.Kind() == kind.Nil {
			return t
		}
		return v.Inner
	case *Union:
		nonNil := optionalFieldUnionWithoutNil(v)
		if nonNil == nil || nonNil.Kind() == kind.Never {
			return t
		}
		return nonNil
	default:
		return t
	}
}

func optionalFieldUnionWithoutNil(u *Union) Type {
	if u == nil {
		return nil
	}
	kept := make([]Type, 0, len(u.Members))
	for _, member := range u.Members {
		kept = appendOptionalFieldNonNilMember(kept, member)
	}
	if len(kept) == 0 {
		return Never
	}
	return NewUnion(kept...)
}

func appendOptionalFieldNonNilMember(out []Type, t Type) []Type {
	if t == nil {
		return out
	}
	switch v := UnwrapAnnotated(t).(type) {
	case nil:
		return out
	case *Optional:
		return appendOptionalFieldNonNilMember(out, v.Inner)
	case *Union:
		for _, member := range v.Members {
			out = appendOptionalFieldNonNilMember(out, member)
		}
		return out
	default:
		if v.Kind() == kind.Nil || v.Kind() == kind.Never {
			return out
		}
		return append(out, t)
	}
}

func fieldsSortedByName(fields []Field) bool {
	for i := 1; i < len(fields); i++ {
		if fields[i-1].Name > fields[i].Name {
			return false
		}
	}
	return true
}
