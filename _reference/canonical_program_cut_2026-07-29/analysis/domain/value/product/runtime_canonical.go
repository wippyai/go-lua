package product

import "sort"

func (rt *registryRuntime) canonicalSlots(slots []slot) []slot {
	if len(slots) == 0 {
		return nil
	}
	if rt.slotsAlreadyCanonical(slots) {
		return slots
	}
	byOrdinal := make(map[uint16]any, len(slots))
	for _, slot := range slots {
		info := rt.axisOrdinal(slot.ordinal)
		if info.spec.IsTopAny(slot.value) {
			delete(byOrdinal, slot.ordinal)
			continue
		}
		byOrdinal[slot.ordinal] = slot.value
	}
	if len(byOrdinal) == 0 {
		return nil
	}
	ordinals := make([]uint16, 0, len(byOrdinal))
	for ordinal := range byOrdinal {
		ordinals = append(ordinals, ordinal)
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	out := make([]slot, 0, len(ordinals))
	for _, ordinal := range ordinals {
		out = append(out, slot{ordinal: ordinal, value: byOrdinal[ordinal]})
	}
	return out
}

func (rt *registryRuntime) slotsAlreadyCanonical(slots []slot) bool {
	var prev uint16
	for i, slot := range slots {
		info := rt.axisOrdinal(slot.ordinal)
		if info.spec.IsTopAny(slot.value) {
			return false
		}
		if i > 0 && prev >= slot.ordinal {
			return false
		}
		prev = slot.ordinal
	}
	return true
}
