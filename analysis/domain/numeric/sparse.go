package numeric

import "sort"

func (algebra *Algebra) expand(value Value, work *scratch) (*denseValue, bool) {
	if !algebra.owns(value) || value.bottom || work == nil {
		return nil, false
	}
	for index := range work.masks {
		work.masks[index] = algebra.baseEligibility(index)
	}
	clearWords(work.equal)
	clearWords(work.unequal)
	clearWords(work.integral)
	for index := range work.bounds {
		work.bounds[index] = uint16(len(algebra.thresholds[index]))
	}
	for index, fact := range value.masks {
		if fact.slot == 0 || uint64(fact.slot) > uint64(len(algebra.atoms)) || !fact.mask.Valid() ||
			fact.mask == allEligibility || index > 0 && value.masks[index-1].slot >= fact.slot {
			return nil, false
		}
		mask := fact.mask & algebra.baseEligibility(int(fact.slot-1))
		if !mask.Valid() {
			return nil, false
		}
		work.masks[fact.slot-1] = mask
	}
	for index, slot := range value.equal {
		if slot == 0 || uint64(slot) > uint64(len(algebra.pairs)) || index > 0 && value.equal[index-1] >= slot {
			return nil, false
		}
		setBit(work.equal, int(slot-1))
	}
	for index, slot := range value.unequal {
		if slot == 0 || uint64(slot) > uint64(len(algebra.pairs)) || index > 0 && value.unequal[index-1] >= slot {
			return nil, false
		}
		setBit(work.unequal, int(slot-1))
	}
	for index, slot := range value.integral {
		if slot == 0 || uint64(slot) > uint64(len(algebra.pairs)) || index > 0 && value.integral[index-1] >= slot {
			return nil, false
		}
		setBit(work.integral, int(slot-1))
	}
	for index, fact := range value.bounds {
		if fact.slot == 0 || uint64(fact.slot) > uint64(len(algebra.pairs)) ||
			int(fact.level) >= len(algebra.thresholds[fact.slot-1]) ||
			index > 0 && value.bounds[index-1].slot >= fact.slot {
			return nil, false
		}
		work.bounds[fact.slot-1] = fact.level
	}
	return &denseValue{masks: work.masks, equal: work.equal, unequal: work.unequal, integral: work.integral, bounds: work.bounds}, true
}

func (algebra *Algebra) compact(image *denseValue) Value {
	maskCount, equalCount, unequalCount, integralCount, boundCount := 0, 0, 0, 0, 0
	for index, mask := range image.masks {
		if mask != algebra.baseEligibility(index) {
			maskCount++
		}
	}
	for index := range algebra.pairs {
		if bitAt(image.equal, index) != 0 {
			equalCount++
		}
		if bitAt(image.unequal, index) != 0 {
			unequalCount++
		}
		if bitAt(image.integral, index) != 0 {
			integralCount++
		}
		if int(image.bounds[index]) != len(algebra.thresholds[index]) {
			boundCount++
		}
	}
	value := Value{
		algebra:  algebra,
		masks:    make([]atomFact, 0, maskCount),
		equal:    make([]uint32, 0, equalCount),
		unequal:  make([]uint32, 0, unequalCount),
		integral: make([]uint32, 0, integralCount),
		bounds:   make([]boundFact, 0, boundCount),
	}
	for index, mask := range image.masks {
		if mask != algebra.baseEligibility(index) {
			value.masks = append(value.masks, atomFact{slot: uint32(index + 1), mask: mask})
		}
	}
	for index := range algebra.pairs {
		slot := uint32(index + 1)
		if bitAt(image.equal, index) != 0 {
			value.equal = append(value.equal, slot)
		}
		if bitAt(image.unequal, index) != 0 {
			value.unequal = append(value.unequal, slot)
		}
		if bitAt(image.integral, index) != 0 {
			value.integral = append(value.integral, slot)
		}
		if int(image.bounds[index]) != len(algebra.thresholds[index]) {
			value.bounds = append(value.bounds, boundFact{slot: slot, level: image.bounds[index]})
		}
	}
	return value
}

func (algebra *Algebra) mask(value Value, slot uint32) Eligibility {
	index := sort.Search(len(value.masks), func(index int) bool { return value.masks[index].slot >= slot })
	if index < len(value.masks) && value.masks[index].slot == slot {
		return value.masks[index].mask
	}
	return algebra.baseEligibility(int(slot - 1))
}

func boundLevel(values []boundFact, slot uint32) (uint16, bool) {
	index := sort.Search(len(values), func(index int) bool { return values[index].slot >= slot })
	if index < len(values) && values[index].slot == slot {
		return values[index].level, true
	}
	return 0, false
}

func hasAtomFact(values []atomFact, slot uint32) bool {
	index := sort.Search(len(values), func(index int) bool { return values[index].slot >= slot })
	return index < len(values) && values[index].slot == slot
}

func joinAtomFacts(left, right []atomFact) []atomFact {
	// Missing is Top, so only coordinates restricted by both sides survive.
	out := make([]atomFact, 0, minInt(len(left), len(right)))
	li, ri := 0, 0
	for li < len(left) && ri < len(right) {
		switch {
		case left[li].slot < right[ri].slot:
			li++
		case right[ri].slot < left[li].slot:
			ri++
		default:
			mask := left[li].mask | right[ri].mask
			if mask != allEligibility {
				out = append(out, atomFact{slot: left[li].slot, mask: mask})
			}
			li++
			ri++
		}
	}
	return out
}

func meetAtomFacts(left, right []atomFact) []atomFact {
	out := make([]atomFact, 0, len(left)+len(right))
	li, ri := 0, 0
	for li < len(left) || ri < len(right) {
		switch {
		case ri == len(right) || li < len(left) && left[li].slot < right[ri].slot:
			out = append(out, left[li])
			li++
		case li == len(left) || right[ri].slot < left[li].slot:
			out = append(out, right[ri])
			ri++
		default:
			out = append(out, atomFact{slot: left[li].slot, mask: left[li].mask & right[ri].mask})
			li++
			ri++
		}
	}
	return out
}

func joinBoundFacts(left, right []boundFact) []boundFact {
	// Missing is infinity, so only finite coordinates on both sides survive.
	out := make([]boundFact, 0, minInt(len(left), len(right)))
	li, ri := 0, 0
	for li < len(left) && ri < len(right) {
		switch {
		case left[li].slot < right[ri].slot:
			li++
		case right[ri].slot < left[li].slot:
			ri++
		default:
			out = append(out, boundFact{slot: left[li].slot, level: maxUint16(left[li].level, right[ri].level)})
			li++
			ri++
		}
	}
	return out
}

func meetBoundFacts(left, right []boundFact) []boundFact {
	out := make([]boundFact, 0, len(left)+len(right))
	li, ri := 0, 0
	for li < len(left) || ri < len(right) {
		switch {
		case ri == len(right) || li < len(left) && left[li].slot < right[ri].slot:
			out = append(out, left[li])
			li++
		case li == len(left) || right[ri].slot < left[li].slot:
			out = append(out, right[ri])
			ri++
		default:
			out = append(out, boundFact{slot: left[li].slot, level: minUint16(left[li].level, right[ri].level)})
			li++
			ri++
		}
	}
	return out
}

func intersectUint32(left, right []uint32) []uint32 {
	out := make([]uint32, 0, minInt(len(left), len(right)))
	li, ri := 0, 0
	for li < len(left) && ri < len(right) {
		switch {
		case left[li] < right[ri]:
			li++
		case right[ri] < left[li]:
			ri++
		default:
			out = append(out, left[li])
			li++
			ri++
		}
	}
	return out
}

func unionUint32(left, right []uint32) []uint32 {
	out := make([]uint32, 0, len(left)+len(right))
	li, ri := 0, 0
	for li < len(left) || ri < len(right) {
		switch {
		case ri == len(right) || li < len(left) && left[li] < right[ri]:
			out = append(out, left[li])
			li++
		case li == len(left) || right[ri] < left[li]:
			out = append(out, right[ri])
			ri++
		default:
			out = append(out, left[li])
			li++
			ri++
		}
	}
	return out
}

func containsUint32Set(container, contained []uint32) bool {
	ci := 0
	for _, wanted := range contained {
		for ci < len(container) && container[ci] < wanted {
			ci++
		}
		if ci == len(container) || container[ci] != wanted {
			return false
		}
	}
	return true
}

func uniqueAtomFacts(values []atomFact) bool {
	for index, fact := range values {
		if index > 0 && values[index-1].slot == fact.slot {
			return false
		}
	}
	return true
}

func uniqueBoundFacts(values []boundFact) bool {
	for index, fact := range values {
		if index > 0 && values[index-1].slot == fact.slot {
			return false
		}
	}
	return true
}

func strictUint32(values []uint32) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

func equalAtomFacts(left, right []atomFact) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalBoundFacts(left, right []boundFact) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameAtomFacts(left, right []atomFact) bool {
	return len(left) == 0 || &left[0] == &right[0]
}

func sameUint32(left, right []uint32) bool {
	return len(left) == 0 || &left[0] == &right[0]
}

func sameBoundFacts(left, right []boundFact) bool {
	return len(left) == 0 || &left[0] == &right[0]
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
