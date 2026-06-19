package typ

import "sort"

type hashedType struct {
	typ  Type
	hash uint64
}

func deduplicateTypesWithHashes(types []Type) ([]Type, []uint64) {
	if len(types) == 0 {
		return nil, nil
	}

	seen := make(map[uint64][]Type)
	result := make([]Type, 0, len(types))
	hashes := make([]uint64, 0, len(types))

	for _, t := range types {
		h := unionMemberHash(t)
		bucket := seen[h]
		duplicate := false

		for _, existing := range bucket {
			if sameUnionMember(existing, t) {
				duplicate = true
				break
			}
		}

		if !duplicate {
			seen[h] = append(bucket, t)
			result = append(result, t)
			hashes = append(hashes, h)
		}
	}

	return result, hashes
}

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
		if slots[i].typ.String() != slots[j].typ.String() {
			return slots[i].typ.String() < slots[j].typ.String()
		}
		return typePointer(slots[i].typ) < typePointer(slots[j].typ)
	})
	for i, slot := range slots {
		types[i] = slot.typ
		hashes[i] = slot.hash
	}
}

func unionMemberHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	return EqualityHash(t)
}

func sameUnionMember(a, b Type) bool {
	if sameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if knownContainsRecursive(a) || knownContainsRecursive(b) {
		if !sameRecursiveIdentityGraph(a, b) {
			return false
		}
	}
	return typeEquals(a, b)
}

func sameRecursiveIdentityGraph(a, b Type) bool {
	left := make(map[uint64]bool)
	right := make(map[uint64]bool)
	collectRecursiveIdentities(a, left, nil)
	collectRecursiveIdentities(b, right, nil)
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if !right[id] {
			return false
		}
	}
	return true
}

func collectRecursiveIdentities(t Type, ids map[uint64]bool, seen map[uintptr]bool) {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return
	}
	ptr := typePointer(t)
	if ptr != 0 {
		if seen == nil {
			seen = make(map[uintptr]bool)
		}
		if seen[ptr] {
			return
		}
		seen[ptr] = true
	}
	if rec, ok := t.(*Recursive); ok {
		ids[rec.ID] = true
	}
	walkChildren(t, func(child Type) bool {
		collectRecursiveIdentities(child, ids, seen)
		return false
	})
}
