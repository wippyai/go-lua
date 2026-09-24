package assign

import (
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// InferErrorReturnConvention derives the canonical Lua (value, err) correlation
// from a function signature when no explicit effect labels are present.
//
// Rule:
//   - Two returns: the error slot is selected by conventional position with
//     type-based precedence:
//   - Prefer return[1] when it is Optional<LuaError> or Optional<string>.
//   - Otherwise allow return[0] only when return[1] is not error-like and
//     return[0] is Optional<LuaError>.
//   - The other position is treated as the value slot.
//   - More returns: the last slot is the error when it is Optional<LuaError>
//     or Optional<string>; every other optional slot is a value slot inversely
//     correlated with it and co-correlated with the other value slots, as in
//     `(a?, b?, c?, err?)`: the values are present together exactly when the
//     error is absent. A slot that is never nil carries its value on both
//     paths and takes no part.
//
// This encodes the conventional `(value?, err?)` API shape while keeping the
// policy centralized and deterministic.
func InferErrorReturnConvention(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	fn := unwrap.Function(fnType)
	if fn == nil || len(fn.Returns) < 2 {
		return nil, nil
	}
	if n := len(fn.Returns); n > 2 {
		if !isOptionalErrorLike(fn.Returns[n-1]) {
			return nil, nil
		}
		var values []int
		for v := 0; v < n-1; v++ {
			if unwrap.IsOptionalLike(fn.Returns[v]) {
				values = append(values, v)
			}
		}
		if len(values) == 0 {
			return nil, nil
		}
		inverse := make([]flow.ReturnCorrelation, 0, len(values))
		var co []flow.ReturnCorrelation
		for i, v := range values {
			inverse = append(inverse, flow.ReturnCorrelation{ValueIndex: v, ErrorIndex: n - 1})
			for _, w := range values[i+1:] {
				co = append(co, flow.ReturnCorrelation{ValueIndex: v, ErrorIndex: w})
			}
		}
		return inverse, co
	}

	errIdx := -1
	if isOptionalErrorLike(fn.Returns[1]) {
		errIdx = 1
	} else if isOptionalLuaError(fn.Returns[0]) && !isOptionalErrorLike(fn.Returns[1]) {
		errIdx = 0
	}
	if errIdx < 0 {
		return nil, nil
	}
	valIdx := 1 - errIdx
	return []flow.ReturnCorrelation{{ValueIndex: valIdx, ErrorIndex: errIdx}}, nil
}

func isOptionalErrorLike(t typ.Type) bool {
	if t == nil {
		return false
	}
	inner := unwrap.Optional(t)
	if inner == nil {
		return false
	}
	return isErrorLikeType(inner)
}

func isErrorLikeType(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, m := range v.Members {
			if m == nil || m.Kind() == kind.Nil {
				continue
			}
			if !isErrorLikeType(m) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, m := range v.Members {
			if isErrorLikeType(m) {
				return true
			}
		}
		return false
	}

	if subtype.IsSubtype(t, typ.LuaError) || subtype.IsSubtype(t, typ.String) {
		return true
	}
	// Structured error objects in Lua code often expose `message` as a field.
	// Treat Optional<{message: string}> as error-like for canonical (value, err)
	// correlation when explicit specs are absent.
	messageType, ok := core.Field(t, "message")
	if !ok || messageType == nil {
		return false
	}
	if subtype.IsSubtype(messageType, typ.String) {
		return true
	}
	messageInner := unwrap.Optional(messageType)
	return messageInner != nil && subtype.IsSubtype(messageInner, typ.String)
}

func isOptionalLuaError(t typ.Type) bool {
	if t == nil {
		return false
	}
	inner := unwrap.Optional(t)
	if inner == nil {
		return false
	}
	return subtype.IsSubtype(inner, typ.LuaError)
}
