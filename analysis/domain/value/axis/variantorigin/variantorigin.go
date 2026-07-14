package variantorigin

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

var Key = axis.NewKey[Value]("variantorigin")

func Spec() axis.Spec[Value] {
	return axis.Spec[Value]{
		Key:       Key,
		Bottom:    Bottom,
		Top:       Top,
		Equal:     Equal,
		LessOrEq:  func(a, b Value) bool { return b.Covers(a) },
		Join:      Join,
		Meet:      Meet,
		Widen:     Widen,
		Hash:      Value.Hash,
		Retention: axis.ImmutableRetention[Value](),
		Canonical: canonicalDescriptor(),
		Boundary:  axis.PortableIdentity,
	}
}

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
	cases  caseset.Set
}

func Bottom() Value { return Value{state: bottom} }
func Top() Value    { return Value{state: top} }

func Of(family uint64, cases []int) Value {
	if family == 0 || len(cases) == 0 {
		return Bottom()
	}
	return Value{state: concrete, family: family, cases: caseset.New(cases)}
}

func Singleton(family uint64, caseIndex int) Value {
	return Of(family, []int{caseIndex})
}

func (v Value) IsBottom() bool { return v.state == bottom }
func (v Value) IsTop() bool    { return v.state == top }
func (v Value) Family() uint64 { return v.family }

// Cases returns an owned copy of the cases in canonical ascending order.
func (v Value) Cases() []int {
	if v.cases.Len() == 0 {
		return nil
	}
	out := make([]int, v.cases.Len())
	for i := 0; i < v.cases.Len(); i++ {
		out[i] = v.cases.At(i)
	}
	return out
}

// CasesLen reports the number of variant cases without allocating.
func (v Value) CasesLen() int { return v.cases.Len() }

// CaseAt returns the i-th variant case without allocating.
func (v Value) CaseAt(i int) int { return v.cases.At(i) }

// CasesView returns an allocation-free immutable view in canonical order.
func (v Value) CasesView() caseset.View { return v.cases.View() }

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
	cases := make([]int, 0, a.cases.Len()+b.cases.Len())
	for i := 0; i < a.cases.Len(); i++ {
		cases = append(cases, a.cases.At(i))
	}
	for i := 0; i < b.cases.Len(); i++ {
		cases = append(cases, b.cases.At(i))
	}
	return Of(a.family, cases)
}

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
	if a.family != b.family {
		return Bottom()
	}
	cases := make([]int, 0, min(a.cases.Len(), b.cases.Len()))
	i, j := 0, 0
	for i < a.cases.Len() && j < b.cases.Len() {
		switch {
		case a.cases.At(i) == b.cases.At(j):
			cases = append(cases, a.cases.At(i))
			i++
			j++
		case a.cases.At(i) < b.cases.At(j):
			i++
		default:
			j++
		}
	}
	return Of(a.family, cases)
}

func Widen(prev, next Value) Value {
	return Join(prev, next)
}

func Equal(a, b Value) bool {
	if a.state != b.state || a.family != b.family || a.cases.Len() != b.cases.Len() {
		return false
	}
	for i := 0; i < a.cases.Len(); i++ {
		if a.cases.At(i) != b.cases.At(i) {
			return false
		}
	}
	return true
}

func (v Value) Hash() uint64 {
	h := internal.MixHash(internal.FnvString("variantorigin"), uint64(v.state))
	h = internal.MixHash(h, v.family)
	for i := 0; i < v.cases.Len(); i++ {
		h = internal.MixHash(h, uint64(v.cases.At(i)+1))
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
			if v.containsCase(caseIndex) {
				return Singleton(family, caseIndex)
			}
			return Bottom()
		}
		out := make([]int, 0, v.cases.Len())
		for i := 0; i < v.cases.Len(); i++ {
			c := v.cases.At(i)
			if c != caseIndex {
				out = append(out, c)
			}
		}
		return Of(family, out)
	default:
		return Top()
	}
}

func (v Value) containsCase(caseIndex int) bool {
	low, high := 0, v.cases.Len()
	for low < high {
		middle := int(uint(low+high) >> 1)
		candidate := v.cases.At(middle)
		if candidate < caseIndex {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low < v.cases.Len() && v.cases.At(low) == caseIndex
}
