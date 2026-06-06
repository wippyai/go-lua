package callreturn

import (
	"github.com/wippyai/go-lua/types/typ"
)

// ResultTypes extracts the Lua return vector from a packed call result: the
// expression-adjusted return vector when present, else the packed tuple unpacked
// to slots, else the single packed return.
func ResultTypes(packed typ.Type, returns []typ.Type) []typ.Type {
	if len(returns) > 0 {
		out := make([]typ.Type, len(returns))
		copy(out, returns)
		return out
	}
	if tuple, ok := packed.(*typ.Tuple); ok {
		out := make([]typ.Type, len(tuple.Elements))
		copy(out, tuple.Elements)
		return out
	}
	if packed == nil {
		return nil
	}
	return []typ.Type{packed}
}
