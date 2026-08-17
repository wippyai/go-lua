package typ

import (
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/internal/hash"
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
		p.Receiver = p.Receiver || p.Name == "self"
		paramsCopy[i] = p
		h = hash.MixHash(h, p.Type.Hash())
		if p.Receiver {
			h = hash.MixHash(h, 2)
		}
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

	fn := &Function{
		TypeParams:        typeParamsCopy,
		Params:            paramsCopy,
		Variadic:          variadic,
		Returns:           returnsCopy,
		hash:              h,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    props,
	}
	if functionSemanticNamesCanonical(paramsCopy) {
		fn.semantic.Store(fn)
	}
	return fn
}

func functionSemanticNamesCanonical(params []Param) bool {
	for i := range params {
		semanticName := ""
		if params[i].Receiver {
			semanticName = "self"
		}
		if params[i].Name != semanticName {
			return false
		}
	}
	return true
}

func newSemanticFunction(source *Function) *Function {
	if source == nil {
		return nil
	}
	semanticParams := append([]Param(nil), source.Params...)
	for i := range semanticParams {
		semanticParams[i].Name = ""
		if semanticParams[i].Receiver {
			semanticParams[i].Name = "self"
		}
	}
	semantic := &Function{
		TypeParams:        source.TypeParams,
		Params:            semanticParams,
		Variadic:          source.Variadic,
		Returns:           source.Returns,
		hash:              source.hash,
		equalityHashCache: &equalityHashCache{},
		typeProperties:    source.typeProperties.copyStatic(),
	}
	semantic.semantic.Store(semantic)
	return semantic
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
