package signature

import (
	"fmt"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// Function carries a pure function type together with the effects produced by
// calling that function. Effect rows intentionally live here instead of inside
// typ.Function so core type identity remains effect-free.
type Function struct {
	Type   *typ.Function
	Effect effect.Row
}

// Clone copies the signature carrier and the mutable slices owned by its row
// and function type.
func (f Function) Clone() Function {
	return Function{
		Type:   cloneFunctionType(f.Type),
		Effect: f.Effect.Clone(),
	}
}

// Equals reports whether both the pure function type and effect row match.
func (f Function) Equals(other Function) bool {
	if (f.Type == nil) != (other.Type == nil) {
		return false
	}
	if f.Type != nil && !typ.TypeEquals(f.Type, other.Type) {
		return false
	}
	if !f.Effect.Equals(other.Effect) {
		return false
	}
	return true
}

func (f Function) String() string {
	typeString := "<nil>"
	if f.Type != nil {
		typeString = f.Type.String()
	}
	if f.Effect.Pure() {
		return typeString
	}
	return fmt.Sprintf("%s ! %s", typeString, f.Effect.String())
}

func cloneFunctionType(fn *typ.Function) *typ.Function {
	return typ.CloneFunction(fn)
}
