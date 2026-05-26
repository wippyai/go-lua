package numeric

import (
	"math"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint/theory"
	flownumeric "github.com/wippyai/go-lua/types/flow/numeric"
)

// Value is the Numeric/Interval axis abstraction of a runtime value's integer
// content: an interval [lower, upper] optionally refined by a modular residue.
//
// Value is immutable. The zero Value is Top (the unbounded interval with no
// residue). Bottom (the empty set) is the unique canonical contradiction; all
// empty Values normalize to it so Equal and Hash stay coherent.
type Value struct {
	interval flownumeric.Interval
	mod      theory.ModularFact
	hasMod   bool
	bottom   bool
}

// Top is the unbounded interval with no modular residue: every integer.
func Top() Value {
	return Value{interval: flownumeric.Interval{Lower: math.MinInt64, Upper: math.MaxInt64}}
}

// Bottom is the empty integer set (an unsatisfiable numeric constraint).
func Bottom() Value {
	return Value{bottom: true}
}

// Range constructs the numeric value bounded by the closed interval [lower, upper].
//
// An interval with lower > upper denotes the empty set and normalizes to Bottom.
func Range(lower, upper int64) Value {
	return normalize(Value{interval: flownumeric.Interval{Lower: lower, Upper: upper}})
}

// Exact constructs the numeric value known to equal a single integer.
func Exact(v int64) Value {
	return Range(v, v)
}

// WithModulus refines the value with the constraint v mod modulus == residue.
//
// The residue is normalized to [0, modulus). A non-positive modulus is ignored
// (no refinement). If the residue is inconsistent with the interval (no integer
// in range satisfies it), the result is Bottom.
func (v Value) WithModulus(modulus, residue int64) Value {
	if v.bottom || modulus <= 0 {
		return v
	}
	r := residue % modulus
	if r < 0 {
		r += modulus
	}
	fact := theory.ModularFact{Modulus: modulus, Residue: r}
	if v.hasMod {
		if v.mod.Modulus == fact.Modulus && v.mod.Residue == fact.Residue {
			return v
		}
		// Two distinct residue constraints on one value cannot both hold under a
		// single tracked residue; conservatively keep the stronger by leaving the
		// existing residue and tightening only when the new one subsumes nothing.
		// Distinct moduli or residues are not representable together, so Bottom is
		// the sound result only when they provably conflict on the whole range.
		if conflicting(v.interval, v.mod, fact) {
			return Bottom()
		}
		return v
	}
	out := Value{interval: v.interval, mod: fact, hasMod: true}
	return normalize(out)
}

// Interval returns the abstracted closed interval [lower, upper].
//
// For Bottom the interval is empty (lower > upper).
func (v Value) Interval() (lower, upper int64) {
	if v.bottom {
		return 1, 0
	}
	return v.interval.Lower, v.interval.Upper
}

// Modulus returns the tracked modular residue, if any.
func (v Value) Modulus() (modulus, residue int64, ok bool) {
	if v.bottom || !v.hasMod {
		return 0, 0, false
	}
	return v.mod.Modulus, v.mod.Residue, true
}

// IsBottom reports whether the value is the empty set.
func (v Value) IsBottom() bool {
	return v.bottom
}

// IsTop reports whether the value is the unbounded interval with no residue.
func (v Value) IsTop() bool {
	return !v.bottom && !v.hasMod &&
		v.interval.Lower == math.MinInt64 && v.interval.Upper == math.MaxInt64
}

// Join is the least upper bound: the convex interval hull, keeping the modular
// residue only when both operands carry the identical residue.
func Join(a, b Value) Value {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	lower := a.interval.Lower
	if b.interval.Lower < lower {
		lower = b.interval.Lower
	}
	upper := a.interval.Upper
	if b.interval.Upper > upper {
		upper = b.interval.Upper
	}
	out := Value{interval: flownumeric.Interval{Lower: lower, Upper: upper}}
	if a.hasMod && b.hasMod && a.mod.Modulus == b.mod.Modulus && a.mod.Residue == b.mod.Residue {
		out.mod = a.mod
		out.hasMod = true
	}
	return normalize(out)
}

// Widen accelerates an ascending chain. A bound that moves outward between prev
// and next is released to infinity so the chain stabilizes in finite steps. The
// residue is kept only when both iterates agree, matching Join.
func Widen(prev, next Value) Value {
	if prev.bottom {
		return next
	}
	if next.bottom {
		return prev
	}
	lower := next.interval.Lower
	if next.interval.Lower < prev.interval.Lower {
		lower = math.MinInt64
	} else if prev.interval.Lower < lower {
		lower = prev.interval.Lower
	}
	upper := next.interval.Upper
	if next.interval.Upper > prev.interval.Upper {
		upper = math.MaxInt64
	} else if prev.interval.Upper > upper {
		upper = prev.interval.Upper
	}
	out := Value{interval: flownumeric.Interval{Lower: lower, Upper: upper}}
	if prev.hasMod && next.hasMod && prev.mod == next.mod {
		out.mod = prev.mod
		out.hasMod = true
	}
	return normalize(out)
}

// Equal is lattice equivalence: identical normalized interval and residue.
func Equal(a, b Value) bool {
	if a.bottom || b.bottom {
		return a.bottom && b.bottom
	}
	if a.interval != b.interval {
		return false
	}
	if a.hasMod != b.hasMod {
		return false
	}
	if !a.hasMod {
		return true
	}
	return a.mod == b.mod
}

// Hash is a stable hash consistent with Equal.
func (v Value) Hash() uint64 {
	if v.bottom {
		return internal.FnvString("numeric.bottom")
	}
	h := internal.HashCombine(uint64(v.interval.Lower), uint64(v.interval.Upper))
	if v.hasMod {
		h = internal.HashCombine(h, uint64(v.mod.Modulus))
		h = internal.HashCombine(h, uint64(v.mod.Residue))
	}
	return h
}

// Covers reports whether the receiver's integer set contains other's.
//
// Defined through Join so it stays consistent with the lattice order:
// v covers other iff joining them does not raise v.
func (v Value) Covers(other Value) bool {
	return Equal(Join(v, other), v)
}

// normalize collapses an empty interval (lower > upper) or a residue that no
// integer in the interval can satisfy to the canonical Bottom.
func normalize(v Value) Value {
	if v.bottom {
		return Bottom()
	}
	if v.interval.Lower > v.interval.Upper {
		return Bottom()
	}
	if v.hasMod && !residueSatisfiable(v.interval, v.mod) {
		return Bottom()
	}
	return v
}

// residueSatisfiable reports whether at least one integer in the interval
// satisfies the modular residue.
func residueSatisfiable(iv flownumeric.Interval, fact theory.ModularFact) bool {
	if fact.Modulus <= 0 {
		return true
	}
	// An unbounded interval always contains a satisfying residue class member.
	if iv.Lower == math.MinInt64 || iv.Upper == math.MaxInt64 {
		return true
	}
	if iv.Upper-iv.Lower+1 >= fact.Modulus {
		return true
	}
	for x := iv.Lower; x <= iv.Upper; x++ {
		if fact.Check(x) {
			return true
		}
	}
	return false
}

// conflicting reports whether two distinct residue facts cannot both hold for
// any integer in the interval.
func conflicting(iv flownumeric.Interval, a, b theory.ModularFact) bool {
	if iv.Lower == math.MinInt64 || iv.Upper == math.MaxInt64 {
		// Over an unbounded range, distinct same-modulus residues never coincide;
		// distinct moduli may coincide somewhere, so only same-modulus differing
		// residues are a provable conflict.
		return a.Modulus == b.Modulus && a.Residue != b.Residue
	}
	for x := iv.Lower; x <= iv.Upper; x++ {
		if a.Check(x) && b.Check(x) {
			return false
		}
	}
	return true
}
