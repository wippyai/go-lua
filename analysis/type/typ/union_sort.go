package typ

import "sort"

func sortHashedTypes(types []Type, hashes []uint64) {
	if len(types) != len(hashes) || len(types) < 2 {
		return
	}
	slots := make([]hashedType, len(types))
	for i, t := range types {
		slots[i] = hashedType{typ: t, hash: hashes[i]}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		return slots[i].typ.String() < slots[j].typ.String()
	})
	for i, slot := range slots {
		types[i] = slot.typ
		hashes[i] = slot.hash
	}
}
