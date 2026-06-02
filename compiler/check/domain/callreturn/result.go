package callreturn

import (
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/typ"
)

// ResultTypes extracts the Lua return vector from a completed call pipeline
// result: the expression-adjusted return vector when present, else the packed
// tuple unpacked to slots, else the single packed return.
func ResultTypes(result ops.CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		out := make([]typ.Type, len(result.Returns))
		copy(out, result.Returns)
		return out
	}
	if tuple, ok := result.Type.(*typ.Tuple); ok {
		out := make([]typ.Type, len(tuple.Elements))
		copy(out, tuple.Elements)
		return out
	}
	if result.Type == nil {
		return nil
	}
	return []typ.Type{result.Type}
}
