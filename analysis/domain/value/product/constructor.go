package product

import "github.com/wippyai/go-lua/analysis/domain/value/axis/presence"

// inlineSlotCount is the common product shape. A candidate is deliberately
// stack-backed through hashing and interner probing; only a novel canonical
// node receives a persistent slot slice.
const inlineSlotCount = 8

// setAxis is the schema-owned single-axis constructor. Its input is already
// canonical, so it can form an ordinal-ordered candidate without copying the
// source node's slice. The overflow fallback preserves the same behaviour for
// unusually wide products.
func (rt *registryRuntime) setAxis(v Value, info axisRuntimeAxis, value any) Value {
	if info.spec.EqualAny(value, info.bottomAny) {
		return rt.bottomValue()
	}
	if PresenceOf(v).IsBottom() {
		return v
	}
	source := v.slotsView()
	drop := info.spec.IsTopAny(value)
	index, found := slotIndex(source, info.ordinal)
	count := len(source)
	if found && drop {
		count--
	} else if !found && !drop {
		count++
	}

	if count <= inlineSlotCount {
		var candidate [inlineSlotCount]slot
		if found {
			copy(candidate[:], source)
			if drop {
				copy(candidate[index:], candidate[index+1:len(source)])
			} else {
				candidate[index].value = value
			}
		} else if !drop {
			copy(candidate[:index], source[:index])
			candidate[index] = slot{ordinal: info.ordinal, value: value}
			copy(candidate[index+1:], source[index:])
		}
		if anyReducerApplicable(rt, rt.reducers, candidate[:count]) {
			owned := make([]slot, count)
			copy(owned, candidate[:count])
			return internRuntime(rt, ShapeOf(v), PresenceOf(v), owned)
		}
		return internCanonicalNoBottom(rt, ShapeOf(v), PresenceOf(v), candidate[:count])
	}

	// Overflow is intentionally uncommon. It retains the old slice path while
	// keeping the small candidate path allocation-free before an interner hit.
	slots := copySlots(v)
	if drop {
		slots = deleteSlot(slots, info.ordinal)
	} else {
		slots = upsertSlot(slots, info.ordinal, value)
	}
	return internConstructedRuntime(rt, ShapeOf(v), PresenceOf(v), slots)
}

func slotIndex(slots []slot, ordinal uint16) (int, bool) {
	for i := range slots {
		if slots[i].ordinal >= ordinal {
			return i, slots[i].ordinal == ordinal
		}
	}
	return len(slots), false
}

// internBorrowedRuntime admits an immutable node slice as a candidate. A
// reducer is the only construction phase that may write slots, so copy only
// when a reducer can actually run. This lets presence-only edits probe the
// interner without a speculative slice allocation.
func internBorrowedRuntime(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) Value {
	if anyReducerApplicable(rt, rt.reducers, slots) {
		owned := make([]slot, len(slots))
		copy(owned, slots)
		return internRuntime(rt, shape, p, owned)
	}
	return internCanonicalNoReducer(rt, shape, p, slots)
}

// internConstructedRuntime accepts a private work candidate. A reducer gets an
// owned candidate; otherwise the interner sees the caller's stack candidate
// directly and allocates only after a miss.
func internConstructedRuntime(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) Value {
	if anyReducerApplicable(rt, rt.reducers, slots) {
		return internRuntime(rt, shape, p, slots)
	}
	return internCanonicalNoReducer(rt, shape, p, slots)
}
