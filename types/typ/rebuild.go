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
	paramsCopy := append([]Param(nil), params...)
	returnsCopy := append([]Type(nil), returns...)
	params, returns = paramsCopy, returnsCopy
	if len(typeParams) > 0 {
		for i := range params {
			params[i].Type = bindTypeParams(params[i].Type, typeParams)
		}
		variadic = bindTypeParams(variadic, typeParams)
		for i := range returns {
			returns[i] = bindTypeParams(returns[i], typeParams)
		}
	}
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
	softPrunable := softPruneParams(paramsCopy) || softPruneAny(variadic) || softPruneAny(returnsCopy...)

	return &Function{
		TypeParams:   typeParamsCopy,
		Params:       paramsCopy,
		Variadic:     variadic,
		Returns:      returnsCopy,
		Effects:      effects,
		Spec:         spec,
		Refinement:   refinement,
		hash:         h,
		softPrunable: softPrunable,
	}
}

func buildRecordType(fields []Field, metatable, mapKey, mapValue Type, open, complete bool, assumeSorted bool) *Record {
	return buildRecordTypeWithFlags(fields, metatable, mapKey, mapValue, open, false, assumeSorted, false, false, complete)
}

// buildRecordTypeDeclared rebuilds a record keeping the declared provenance
// and complete bits.
func buildRecordTypeDeclared(fields []Field, metatable, mapKey, mapValue Type, open, declared, complete bool, assumeSorted bool) *Record {
	return buildRecordTypeWithFlags(fields, metatable, mapKey, mapValue, open, declared, assumeSorted, false, false, complete)
}

func buildRecordTypeWithFlags(fields []Field, metatable, mapKey, mapValue Type, open, declared bool, assumeSorted, inferred, explicitNil, complete bool) *Record {
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
		if !sorted[i].Optional {
			sorted[i].InferredPresence = false
		}
	}

	if mapKey == nil && mapValue != nil {
		mapKey = Unknown
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
		if f.InferredPresence {
			h = internal.HashCombine(h, 4)
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
	if declared {
		h = internal.HashCombine(h, 16)
	}
	if inferred {
		h = internal.HashCombine(h, 4)
	}
	if explicitNil {
		h = internal.HashCombine(h, 8)
	}
	if complete {
		h = internal.HashCombine(h, 32)
	}
	softPrunable := softPruneFields(sorted) || softPruneAny(metatable, mapKey, mapValue)

	return &Record{
		Fields:              sorted,
		Metatable:           metatable,
		MapKey:              mapKey,
		MapValue:            mapValue,
		MapInferredPresence: inferred,
		MapExplicitNilWrite: explicitNil,
		Open:                open,
		Complete:            complete,
		Declared:            declared,
		sorted:              true,
		hash:                h,
		softPrunable:        softPrunable,
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
