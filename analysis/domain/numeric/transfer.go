package numeric

import "math"

// MustRawEqual constructs one primitive/raw equality contribution after the
// caller has proved the exact raw equality route and both operands are
// non-NaN. It does not select an operation, coerce an operand, or add an
// observation: the supplied atoms must already share a sealed pair template.
func (algebra *Algebra) MustRawEqual(key Key, left, right Atom) (Value, bool) {
	pair, ok := algebra.Pair(left, right)
	if !ok {
		return Value{}, false
	}
	// A primitive equality fact is valid only on its non-NaN branch. This
	// refinement is part of the equality fact itself, not a second kind rule.
	return algebra.admitImage(key, Value{
		algebra: algebra,
		masks:   exactMasks(left, allEligibility&^MayNaN, right, allEligibility&^MayNaN),
		equal:   []uint32{pair.slot},
	})
}

// MustRawUnequal constructs one primitive/raw disequality contribution after
// the caller has proved the exact raw inequality route. A self pair is
// intentionally admitted only through the carrier's NaN normalization.
func (algebra *Algebra) MustRawUnequal(key Key, left, right Atom) (Value, bool) {
	pair, ok := algebra.Pair(left, right)
	if !ok {
		return Value{}, false
	}
	return algebra.admitImage(key, Value{algebra: algebra, unequal: []uint32{pair.slot}})
}

// MustIntegralEqual constructs the numeric-integral equality projection of a
// proven integer equality route. The relation permits integer and exact
// integral-float representations; it does not collapse their runtime kinds.
func (algebra *Algebra) MustIntegralEqual(key Key, left, right Atom) (Value, bool) {
	pair, ok := algebra.Pair(left, right)
	if !ok {
		return Value{}, false
	}
	return algebra.admitImage(key, Value{algebra: algebra, integral: []uint32{pair.slot}})
}

// IntegerDifference constructs x-y<=bound for a sealed Numeric integer route.
// The two endpoints are restricted to the exact integer representation before
// the bound enters the finite difference carrier. A missing sealed pair or
// threshold rejects the transfer instead of inventing topology or rounding a
// source-level constant.
func (algebra *Algebra) IntegerDifference(key Key, left, right Atom, bound int64) (Value, bool) {
	pair, ok := algebra.Pair(left, right)
	if !ok {
		return Value{}, false
	}
	index, ok := algebra.index(pair)
	if !ok || !algebra.validKey(key) {
		return Value{}, false
	}
	level, ok := algebra.exactLevel(index, bound)
	if !ok {
		return Value{}, false
	}
	return algebra.admitImage(key, Value{
		algebra: algebra,
		masks:   exactMasks(left, MayInteger, right, MayInteger),
		bounds:  []boundFact{{slot: pair.slot, level: level}},
	})
}

// IntegerTranslation constructs result=input+constant for a sealed Numeric
// no-overflow integer transfer. Both oriented pair templates and both exact
// finite thresholds must be present in the sealed Numeric topology.
func (algebra *Algebra) IntegerTranslation(key Key, result, input Atom, constant int64) (Value, bool) {
	if constant == math.MinInt64 {
		return Value{}, false
	}
	forward, ok := algebra.Pair(result, input)
	if !ok {
		return Value{}, false
	}
	reverse, ok := algebra.Pair(input, result)
	if !ok {
		return Value{}, false
	}
	forwardIndex, forwardOK := algebra.index(forward)
	reverseIndex, reverseOK := algebra.index(reverse)
	if !forwardOK || !reverseOK || !algebra.validKey(key) || forward == reverse && constant != 0 {
		return Value{}, false
	}
	forwardLevel, forwardOK := algebra.exactLevel(forwardIndex, constant)
	reverseLevel, reverseOK := algebra.exactLevel(reverseIndex, -constant)
	if !forwardOK || !reverseOK {
		return Value{}, false
	}
	bounds := []boundFact{{slot: forward.slot, level: forwardLevel}}
	if reverse != forward {
		bounds = append(bounds, boundFact{slot: reverse.slot, level: reverseLevel})
		if bounds[1].slot < bounds[0].slot {
			bounds[0], bounds[1] = bounds[1], bounds[0]
		}
	}
	return algebra.admitImage(key, Value{
		algebra: algebra,
		masks:   exactMasks(result, MayInteger, input, MayInteger),
		bounds:  bounds,
	})
}

// exactMasks produces canonical, duplicate-free fixed-arity eligibility
// facts. The only repeated-atom case has one identical intended mask.
func exactMasks(left Atom, leftMask Eligibility, right Atom, rightMask Eligibility) []atomFact {
	if left == right {
		return []atomFact{{slot: left.slot, mask: leftMask & rightMask}}
	}
	if left.slot < right.slot {
		return []atomFact{{slot: left.slot, mask: leftMask}, {slot: right.slot, mask: rightMask}}
	}
	return []atomFact{{slot: right.slot, mask: rightMask}, {slot: left.slot, mask: leftMask}}
}
