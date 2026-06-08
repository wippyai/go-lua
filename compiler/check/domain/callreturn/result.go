package callreturn

import (
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnVector is the canonical type-level Lua return vector at call boundaries.
// It owns the transition from a packed call result (single type or tuple) to the
// expression-adjusted return slots consumed by synth, observation, and canonical
// call projection.
type ReturnVector struct {
	slots []typ.Type
}

// ReturnVectorOfCallResult normalizes a packed call result plus any explicit
// expression-adjusted return slots.
func ReturnVectorOfCallResult(packed typ.Type, returns []typ.Type) ReturnVector {
	if len(returns) > 0 {
		return ReturnVector{slots: cloneTypes(returns)}
	}
	if tuple, ok := packed.(*typ.Tuple); ok {
		return ReturnVector{slots: cloneTypes(tuple.Elements)}
	}
	if packed == nil {
		return ReturnVector{}
	}
	return ReturnVector{slots: []typ.Type{packed}}
}

// Types returns a defensive copy of the normalized return slots.
func (v ReturnVector) Types() []typ.Type {
	return cloneTypes(v.slots)
}

// ResultTypes extracts the Lua return vector from a packed call result. Prefer
// ReturnVectorOfCallResult at new call-boundary code; this compatibility helper
// keeps older callers on the same normalization.
func ResultTypes(packed typ.Type, returns []typ.Type) []typ.Type {
	return ReturnVectorOfCallResult(packed, returns).Types()
}

func cloneTypes(types []typ.Type) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	out := make([]typ.Type, len(types))
	copy(out, types)
	return out
}
