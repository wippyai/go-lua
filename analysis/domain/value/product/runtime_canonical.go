package product

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func (rt *registryRuntime) canonicalSlots(slots []slot) []slot {
	if len(slots) == 0 {
		return nil
	}
	if rt.slotsAlreadyCanonical(slots) {
		return slots
	}
	byKey := make(map[string]any, len(slots))
	for _, slot := range slots {
		if slot.key == presence.Key.ID() {
			panic("product: presence is a core lane, not a sparse axis")
		}
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.IsTopAny(slot.value) {
			delete(byKey, slot.key)
			continue
		}
		byKey[slot.key] = slot.value
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]slot, 0, len(keys))
	for _, key := range keys {
		out = append(out, slot{key: key, value: byKey[key]})
	}
	return out
}

func (rt *registryRuntime) slotsAlreadyCanonical(slots []slot) bool {
	var prev string
	for i, slot := range slots {
		if slot.key == presence.Key.ID() {
			panic("product: presence is a core lane, not a sparse axis")
		}
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.IsTopAny(slot.value) {
			return false
		}
		if i > 0 && prev >= slot.key {
			return false
		}
		prev = slot.key
	}
	return true
}
