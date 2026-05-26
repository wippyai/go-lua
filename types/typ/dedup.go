package typ

type hashedType struct {
	typ  Type
	hash uint64
}

// deduplicateTypes removes duplicate types using hash-based bucketing
// with structural equality checks to handle hash collisions.
func deduplicateTypes(types []Type) []Type {
	deduped, _ := deduplicateTypesWithHashes(types)
	return deduped
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
			if unionMemberEquals(existing, t) {
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

func unionMemberHash(t Type) uint64 {
	return UnionMemberHash(t)
}

// UnionMemberHash returns the hash paired with SameUnionMember for normalized
// union/member-set construction. Recursive products use family hashes so
// fixed-point observations dedupe without unfolding by concrete node identity.
func UnionMemberHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	if ContainsRecursive(t) {
		return ProductFamilyHash(t)
	}
	return typeEqualityHash(t)
}

func unionMemberEquals(a, b Type) bool {
	return SameUnionMember(a, b)
}

// SameUnionMember is the canonical equality relation for generic union
// construction. It intentionally preserves distinct recursive product nodes;
// product-family coalescing belongs to explicit convergence/join policies.
func SameUnionMember(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return false
	}
	return TypeEquals(a, b)
}
