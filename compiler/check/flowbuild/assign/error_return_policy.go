package assign

import (
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// InferErrorReturnConvention derives the canonical Lua (value, err) correlation
// from a function signature when no explicit effect labels are present.
//
// Rule:
//   - Signature must have exactly two returns.
//   - Error slot is selected by conventional position with type-based precedence:
//   - Prefer return[1] when it is Optional<LuaError> or Optional<string>.
//   - Otherwise allow return[0] only when return[1] is not error-like and
//     return[0] is Optional<LuaError>.
//   - The other position is treated as the value slot.
//
// This encodes the conventional `(value?, err?)` API shape while keeping the
// policy centralized and deterministic.
func InferErrorReturnConvention(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	fn := unwrap.Function(fnType)
	if fn == nil || len(fn.Returns) != 2 {
		return nil, nil
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
	if subtype.IsSubtype(inner, typ.LuaError) || subtype.IsSubtype(inner, typ.String) {
		return true
	}
	// Structured error objects in Lua code often expose `message` as a field.
	// Treat Optional<{message: string}> as error-like for canonical (value, err)
	// correlation when explicit specs are absent.
	messageType, ok := core.Field(inner, "message")
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
