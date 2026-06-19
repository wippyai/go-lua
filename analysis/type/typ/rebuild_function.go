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
}

// RebuildFunction rebuilds a function from already-computed structural parts.
func RebuildFunction(parts FunctionParts) *Function {
	return newCanonicalFunction(
		parts.TypeParams,
		parts.Params,
		parts.Variadic,
		parts.Returns,
	)
}

// typ owns hash-stable node materialization; higher-level builders decide
// function semantics.
func newCanonicalFunction(
	typeParams []*TypeParam,
	params []Param,
	variadic Type,
	returns []Type,
) *Function {
	h := uint64(kind.Function)
	typeParamsCopy := make([]*TypeParam, len(typeParams))
	for i, tp := range typeParams {
		if tp == nil {
			panic("typ.RebuildFunction: nil entry in type params; normalize before building")
		}
		typeParamsCopy[i] = tp
		h = hash.MixHash(h, tp.Hash())
	}

	paramsCopy := make([]Param, len(params))
	for i, p := range params {
		p.Type = requiredFunctionSlotType("params", p.Type)
		paramsCopy[i] = p
		h = hash.MixHash(h, p.Type.Hash())
		if p.Optional {
			h = hash.MixHash(h, 1)
		}
	}

	variadic = NormalizeNil(variadic)
	if variadic != nil {
		h = hash.MixHash(h, variadic.Hash())
	}

	returnsCopy := make([]Type, len(returns))
	for i, r := range returns {
		r = requiredFunctionSlotType("returns", r)
		returnsCopy[i] = r
		h = hash.MixHash(h, r.Hash())
	}

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
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func requiredFunctionSlotType(slot string, t Type) Type {
	t = NormalizeNil(t)
	if t == nil {
		panic("typ.RebuildFunction: nil entry in " + slot + "; normalize before building")
	}
	return t
}
