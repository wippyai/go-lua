package control

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
)

var (
	_ effect.Label = Throw{}
	_ effect.Label = IO{}
)

type (
	// Throw is reserved high-risk control metadata. Error behavior is currently
	// represented by Never, postconditions, and module-load behavior rather than
	// active control-effect lowering.
	Throw struct{}

	// IO is reserved high-risk control metadata. IO policy and enforcement are
	// not active, so stdlib signatures must not declare this label.
	IO struct{}
)

func (Throw) CapabilityID() string { return capability.ControlThrow }
func (Throw) String() string       { return "throw" }
func (Throw) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(Throw)
	return ok
}

func (IO) CapabilityID() string { return capability.ControlIO }
func (IO) String() string       { return "io" }
func (IO) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(IO)
	return ok
}
