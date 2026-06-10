package variantorigin

import (
	"slices"

	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

type state uint8

const (
	bottom state = iota
	concrete
	top
)

// Value carries finite variant-origin evidence for one product value.
//
// A concrete value means this runtime value belongs to OriginFamily and may be
// any of Cases. Top means no usable origin evidence. Bottom is unreachable.
type Value struct {
	state  state
	family uint64
	cases  []int
}

func Bottom() Value { return Value{state: bottom} }
func Top() Value    { return Value{state: top} }

func Of(family uint64, cases []int) Value {
	if family == 0 || len(cases) == 0 {
		return Bottom()
	}
	out := append([]int(nil), cases...)
	slices.Sort(out)
	out = slices.Compact(out)
	return Value{state: concrete, family: family, cases: out}
}

func Singleton(family uint64, caseIndex int) Value {
	return Of(family, []int{caseIndex})
}

func (v Value) IsBottom() bool { return v.state == bottom }
func (v Value) IsTop() bool    { return v.state == top }
func (v Value) Family() uint64 { return v.family }
func (v Value) Cases() []int   { return append([]int(nil), v.cases...) }

func Join(a, b Value) Value {
	if a.state == bottom {
		return b
	}
	if b.state == bottom {
		return a
	}
	if a.state == top || b.state == top {
		return Top()
	}
	if a.family != b.family {
		return Top()
	}
	cases := append(append([]int(nil), a.cases...), b.cases...)
	return Of(a.family, cases)
}

func Widen(prev, next Value) Value {
	return Join(prev, next)
}

func Equal(a, b Value) bool {
	if a.state != b.state || a.family != b.family || len(a.cases) != len(b.cases) {
		return false
	}
	for i := range a.cases {
		if a.cases[i] != b.cases[i] {
			return false
		}
	}
	return true
}

func (v Value) Hash() uint64 {
	h := internal.HashCombine(internal.FnvString("variantorigin"), uint64(v.state))
	h = internal.HashCombine(h, v.family)
	for _, c := range v.cases {
		h = internal.HashCombine(h, uint64(c+1))
	}
	return h
}

func (v Value) Covers(other Value) bool {
	return Equal(Join(v, other), v)
}

func (v Value) NarrowCase(family uint64, caseIndex int, equal bool) Value {
	if family == 0 {
		return v
	}
	switch v.state {
	case bottom:
		return v
	case top:
		if equal {
			return Singleton(family, caseIndex)
		}
		return v
	case concrete:
		if v.family != family {
			if equal {
				return Bottom()
			}
			return v
		}
		if equal {
			if slices.Contains(v.cases, caseIndex) {
				return Singleton(family, caseIndex)
			}
			return Bottom()
		}
		out := make([]int, 0, len(v.cases))
		for _, c := range v.cases {
			if c != caseIndex {
				out = append(out, c)
			}
		}
		return Of(family, out)
	default:
		return Top()
	}
}
