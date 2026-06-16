package control

import "github.com/wippyai/go-lua/analysis/domain/effect"

var (
	_ effect.Label = Throw{}
	_ effect.Label = IO{}
)

type (
	// Throw indicates that a function may raise an error.
	Throw struct{}

	// IO indicates that a function performs I/O.
	IO struct{}
)

func (Throw) EffectLabel()   {}
func (Throw) String() string { return "throw" }
func (Throw) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(Throw)
	return ok
}

func (IO) EffectLabel()   {}
func (IO) String() string { return "io" }
func (IO) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(IO)
	return ok
}
