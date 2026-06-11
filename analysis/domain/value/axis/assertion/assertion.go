package assertion

import (
	"strings"

	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

type state uint8

const (
	bottom state = iota
	concrete
	top
)

// Flag identifies one user-written assertion marker attached to a value. These
// flags are claims, not proof; transfer must not lower them into other axes.
type Flag uint8

const (
	TypeAssertion Flag = 1 << iota
	AnyAssertion
	NonNilAssertion
)

const knownFlags = TypeAssertion | AnyAssertion | NonNilAssertion

var flagOrder = []Flag{TypeAssertion, AnyAssertion, NonNilAssertion}

// Value carries finite must-evidence for user-written assertions.
//
// Top means no assertion indicator is known on all paths. Bottom is
// unreachable. A concrete value contains the assertion flags common to every
// incoming path.
type Value struct {
	state state
	flags Flag
}

// Bottom is the unreachable assertion state.
func Bottom() Value { return Value{state: bottom} }

// Top carries no assertion indicator.
func Top() Value { return Value{state: top} }

// Of constructs a concrete assertion indicator from flags. No flags normalize
// to Top because absence is represented by the sparse-axis top value.
func Of(flags ...Flag) Value {
	var mask Flag
	for _, flag := range flags {
		mask |= flag & knownFlags
	}
	if mask == 0 {
		return Top()
	}
	return Value{state: concrete, flags: mask}
}

// Type returns the ordinary type assertion indicator.
func Type() Value { return Of(TypeAssertion) }

// Any returns the explicit any assertion indicator.
func Any() Value { return Of(AnyAssertion) }

// NonNil returns the non-nil assertion indicator.
func NonNil() Value { return Of(NonNilAssertion) }

// IsBottom reports whether the assertion state is unreachable.
func (v Value) IsBottom() bool { return v.state == bottom }

// IsTop reports whether the value carries no assertion indicator.
func (v Value) IsTop() bool { return v.state == top }

// Has reports whether flag is guaranteed on all paths represented by v.
func (v Value) Has(flag Flag) bool {
	return v.state == concrete && v.flags&flag != 0
}

// Flags returns the concrete assertion flags in stable order.
func (v Value) Flags() []Flag {
	if v.state != concrete {
		return nil
	}
	out := make([]Flag, 0, len(flagOrder))
	for _, flag := range flagOrder {
		if v.Has(flag) {
			out = append(out, flag)
		}
	}
	return out
}

// Join keeps only assertion indicators present on all incoming paths.
func Join(a, b Value) Value {
	if Equal(a, b) {
		return a
	}
	if a.state == bottom {
		return b
	}
	if b.state == bottom {
		return a
	}
	if a.state == top || b.state == top {
		return Top()
	}
	return Of(a.flags & b.flags)
}

// Meet combines compatible assertion evidence on the same path.
func Meet(a, b Value) Value {
	if a.state == bottom || b.state == bottom {
		return Bottom()
	}
	if a.state == top {
		return b
	}
	if b.state == top {
		return a
	}
	return Of(a.flags | b.flags)
}

// Combine adds same-path assertion indicators together. Unlike Join, this does
// not model control-flow merging; it records multiple claims made about the same
// value expression.
func Combine(a, b Value) Value {
	if a.state == bottom || b.state == bottom {
		return Bottom()
	}
	if a.state == top {
		return b
	}
	if b.state == top {
		return a
	}
	return Of(a.flags | b.flags)
}

// Widen equals Join because the assertion lattice is finite.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence.
func Equal(a, b Value) bool {
	return a.state == b.state && normalizedFlags(a) == normalizedFlags(b)
}

// Covers reports whether the receiver is at least as high as other.
func (v Value) Covers(other Value) bool {
	return Equal(Join(v, other), v)
}

// Hash is stable and consistent with Equal.
func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("assertion"), uint64(v.state))
	return internal.MixHash(h, uint64(normalizedFlags(v)))
}

func (f Flag) String() string {
	switch f {
	case TypeAssertion:
		return "type"
	case AnyAssertion:
		return "any"
	case NonNilAssertion:
		return "non-nil"
	default:
		return "assertion-flag(invalid)"
	}
}

func (v Value) String() string {
	switch v.state {
	case bottom:
		return "bottom"
	case top:
		return "top"
	case concrete:
		names := make([]string, 0, len(flagOrder))
		for _, flag := range v.Flags() {
			names = append(names, flag.String())
		}
		return "assertion(" + strings.Join(names, "|") + ")"
	default:
		return "assertion(invalid)"
	}
}

func normalizedFlags(v Value) Flag {
	if v.state != concrete {
		return 0
	}
	return v.flags & knownFlags
}
