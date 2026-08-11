package footprint

// BoundKind records the precision of one non-negative symbolic bound.
// Overflow is a proved loss to the arithmetic-overflow class; Unknown means
// no bound theorem is available. They remain distinct so evidence can explain
// a lost bound without inventing a physical allocation policy.
type BoundKind uint8

const (
	BoundInvalid BoundKind = iota
	BoundExact
	BoundRange
	BoundOverflow
	BoundUnbounded
	BoundUnknown
)

// Bound is an immutable non-negative element or capacity bound. Lower and
// Upper are meaningful only for Exact and Range.
type Bound struct {
	kind  BoundKind
	lower uint64
	upper uint64
}

// Exact constructs the singleton bound n.
func Exact(n uint64) Bound { return Bound{kind: BoundExact, lower: n, upper: n} }

// Range constructs the closed finite interval [lower, upper].
func Range(lower, upper uint64) (Bound, bool) {
	if lower > upper {
		return Bound{}, false
	}
	if lower == upper {
		return Exact(lower), true
	}
	return Bound{kind: BoundRange, lower: lower, upper: upper}, true
}

// Overflow constructs the explicit arithmetic-overflow class.
func Overflow() Bound { return Bound{kind: BoundOverflow} }

// Unbounded records ordinary recurrence widening. Unlike Overflow, it carries
// no claim that arithmetic overflow occurred; it means only that the finite
// numeric theorem was abstracted at a Mu boundary.
func Unbounded() Bound { return Bound{kind: BoundUnbounded} }

// Unknown constructs the unconstrained-bound class.
func Unknown() Bound { return Bound{kind: BoundUnknown} }

// Kind reports the precision class.
func (b Bound) Kind() BoundKind { return b.kind }

// Interval returns the finite interval when one is known.
func (b Bound) Interval() (lower, upper uint64, ok bool) {
	return b.lower, b.upper, b.kind == BoundExact || b.kind == BoundRange
}

func validBound(b Bound) bool {
	switch b.kind {
	case BoundExact:
		return b.lower == b.upper
	case BoundRange:
		return b.lower < b.upper
	case BoundOverflow, BoundUnbounded, BoundUnknown:
		return b.lower == 0 && b.upper == 0
	default:
		return false
	}
}

func equalBound(left, right Bound) bool { return left == right }

// lessBound is the information order used by Footprint. Overflow is a
// conservative arithmetic class and Unknown is the final no-theorem class.
func lessBound(left, right Bound) bool {
	if !validBound(left) || !validBound(right) {
		return false
	}
	if left == right || right.kind == BoundUnknown {
		return true
	}
	if right.kind == BoundUnbounded {
		return left.kind != BoundUnknown
	}
	if right.kind == BoundOverflow {
		return left.kind == BoundExact || left.kind == BoundRange || left.kind == BoundOverflow
	}
	if left.kind != BoundExact && left.kind != BoundRange {
		return false
	}
	if right.kind != BoundExact && right.kind != BoundRange {
		return false
	}
	return right.lower <= left.lower && left.upper <= right.upper
}

func joinBound(left, right Bound) Bound {
	if left == right {
		return left
	}
	if !validBound(left) || !validBound(right) || left.kind == BoundUnknown || right.kind == BoundUnknown {
		return Unknown()
	}
	if left.kind == BoundUnbounded || right.kind == BoundUnbounded {
		return Unbounded()
	}
	if left.kind == BoundOverflow || right.kind == BoundOverflow {
		return Overflow()
	}
	lower, upper := left.lower, left.upper
	if right.lower < lower {
		lower = right.lower
	}
	if right.upper > upper {
		upper = right.upper
	}
	bound, _ := Range(lower, upper)
	return bound
}

func meetBound(left, right Bound) (Bound, bool) {
	if !validBound(left) || !validBound(right) {
		return Bound{}, false
	}
	if left.kind == BoundUnknown {
		return right, true
	}
	if right.kind == BoundUnknown {
		return left, true
	}
	if left.kind == BoundUnbounded || right.kind == BoundUnbounded {
		if left.kind == BoundUnbounded && right.kind == BoundUnbounded {
			return Unbounded(), true
		}
		if left.kind == BoundUnbounded {
			return right, true
		}
		return left, true
	}
	if left.kind == BoundOverflow || right.kind == BoundOverflow {
		if left.kind == BoundOverflow && right.kind == BoundOverflow {
			return Overflow(), true
		}
		if left.kind == BoundOverflow {
			return right, true
		}
		return left, true
	}
	lower, upper := left.lower, right.upper
	if right.lower > lower {
		lower = right.lower
	}
	if left.upper < upper {
		upper = left.upper
	}
	if lower > upper {
		return Bound{}, false
	}
	return Range(lower, upper)
}

// widenBound forces every changing numeric recurrence into Unbounded. This is
// a true widening: no capacity/budget cutoff is used, and once unbounded the
// finite numeric component cannot grow again. Arithmetic Overflow is retained
// only when supplied as semantic input; Widen never manufactures it.
func widenBound(previous, next Bound) Bound {
	joined := joinBound(previous, next)
	if equalBound(previous, joined) {
		return previous
	}
	if previous.kind == BoundUnknown || joined.kind == BoundUnknown {
		return Unknown()
	}
	if joined.kind == BoundOverflow || joined.kind == BoundUnbounded {
		return joined
	}
	return Unbounded()
}
