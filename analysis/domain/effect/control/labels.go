package control

import "github.com/wippyai/go-lua/analysis/domain/effect"

var (
	_ effect.Label = Throw{}
	_ effect.Label = Diverge{}
	_ effect.Label = IO{}
)

type (
	// Throw indicates that a function may raise an error.
	Throw struct{}

	// Diverge indicates that a function may not terminate.
	Diverge struct{}

	// IO indicates that a function performs I/O.
	IO struct{}
)

func (Throw) EffectLabel()   {}
func (Throw) String() string { return "throw" }
func (Throw) Equals(other effect.Label) bool {
	_, ok := other.(Throw)
	return ok
}

func (Diverge) EffectLabel()   {}
func (Diverge) String() string { return "diverge" }
func (Diverge) Equals(other effect.Label) bool {
	_, ok := other.(Diverge)
	return ok
}

func (IO) EffectLabel()   {}
func (IO) String() string { return "io" }
func (IO) Equals(other effect.Label) bool {
	_, ok := other.(IO)
	return ok
}
