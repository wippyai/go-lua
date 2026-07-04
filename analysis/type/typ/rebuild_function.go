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
	returns = normalizeFunctionReturns(returns)
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

	props := typePropertiesOfTypeParams(typeParamsCopy)
	props.includeParams(paramsCopy)
	props.include(variadic)
	props.includeTypes(returnsCopy...)

	return &Function{
		TypeParams:     typeParamsCopy,
		Params:         paramsCopy,
		Variadic:       variadic,
		Returns:        returnsCopy,
		hash:           h,
		typeProperties: props,
	}
}

func normalizeFunctionReturns(returns []Type) []Type {
	if len(returns) == 0 {
		return nil
	}
	var out []Type
	for i, r := range returns {
		r = requiredFunctionSlotType("returns", r)
		if tuple, ok := r.(*Tuple); ok {
			if out == nil {
				out = make([]Type, 0, len(returns)+len(tuple.Elements))
				for _, prefix := range returns[:i] {
					out = append(out, requiredFunctionSlotType("returns", prefix))
				}
			}
			for _, elem := range tuple.Elements {
				out = append(out, requiredFunctionSlotType("returns", elem))
			}
			continue
		}
		if out != nil {
			out = append(out, r)
		}
	}
	if out != nil {
		return out
	}
	return returns
}

func requiredFunctionSlotType(slot string, t Type) Type {
	t = NormalizeNil(t)
	if t == nil {
		panic("typ.RebuildFunction: nil entry in " + slot + "; normalize before building")
	}
	return t
}
