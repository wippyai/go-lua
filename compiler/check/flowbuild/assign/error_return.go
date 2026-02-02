package assign

import (
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// InferErrorReturnCorrelations infers ErrorReturn-style correlations from a function signature
// when no explicit spec effect is present. This is a conservative heuristic:
// it only applies to two-value returns (value, err) where err is optional LuaError.
func InferErrorReturnCorrelations(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if fnType == nil {
		return nil, nil
	}
	u := unwrap.Alias(fnType)
	fn, ok := u.(*typ.Function)
	if !ok || fn == nil {
		return nil, nil
	}
	if len(fn.Returns) != 2 {
		return nil, nil
	}
	errIdx := -1
	for i, ret := range fn.Returns {
		if isErrorLike(ret) {
			errIdx = i
			break
		}
	}
	if errIdx < 0 {
		return nil, nil
	}
	valIdx := 1 - errIdx
	return []flow.ReturnCorrelation{{ValueIndex: valIdx, ErrorIndex: errIdx}}, nil
}

func isErrorLike(t typ.Type) bool {
	if t == nil {
		return false
	}
	inner := unwrap.Optional(t)
	if inner == nil {
		return false
	}
	if subtype.IsSubtype(inner, typ.LuaError) {
		return true
	}
	// Lua code often uses (T?, string?) for error returns.
	return subtype.IsSubtype(inner, typ.String)
}
