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
		return slots[i].typ.String() < slots[j].typ.String()
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
	return typeEqualityHash(t)
}

func sameUnionMember(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return false
	}
	return TypeEquals(a, b)
}
