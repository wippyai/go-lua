package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// FunctionParts carries the structural pieces needed to rebuild a function.
type FunctionParts struct {
	TypeParams []*TypeParam
	Params     []Param
	Variadic   Type
	Returns    []Type
	Effects    EffectInfo
	Spec       SpecInfo
	Refinement RefinementInfo
}

// RebuildFunction rebuilds a function from already-computed structural parts.
func RebuildFunction(parts FunctionParts) *Function {
	return buildFunctionType(
		parts.TypeParams,
		parts.Params,
		parts.Variadic,
		parts.Returns,
		parts.Effects,
		parts.Spec,
		parts.Refinement,
	)
}

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
		h = hash.HashCombine(h, tp.Hash())
	}

	for _, p := range params {
		h = hash.HashCombine(h, p.Type.Hash())
		if p.Optional {
			h = hash.HashCombine(h, 1)
		}
	}

	if variadic != nil {
		h = hash.HashCombine(h, variadic.Hash())
	}

	for _, r := range returns {
		if r == nil {
			panic("FunctionBuilder.Build: nil entry in returns; normalize before building")
		}
		h = hash.HashCombine(h, r.Hash())
	}

	typeParamsCopy := make([]*TypeParam, len(typeParams))
	copy(typeParamsCopy, typeParams)
	paramsCopy := make([]Param, len(params))
	copy(paramsCopy, params)
	returnsCopy := make([]Type, len(returns))
	copy(returnsCopy, returns)
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
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}
