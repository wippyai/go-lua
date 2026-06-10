package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// rewriteUnionMembers applies a cache-preserving rewrite to the members of
// an already normalized union. If the rewrite only drops members, the
// existing member hash vector is reused so path-sensitive filters do not rehash
// recursive products. Rewrites preserve only root union representation
// normalization: deduplication, nil/optional folding, literal subsumption, and
// deterministic member order.
func rewriteUnionMembers(u *Union, rewrite func(Type) Type) Type {
	if u == nil {
		return Never
	}
	if rewrite == nil {
		return u
	}
	kept := make([]Type, 0, len(u.Members))
	hashes := make([]uint64, 0, len(u.Members))
	changed := false
	filterOnly := true
	scalarRewriteOnly := true
	hasStoredHashes := len(u.memberHashes) == len(u.Members)
	for i, member := range u.Members {
		rewritten := rewrite(member)
		if rewritten == nil || rewritten.Kind().IsNever() {
			changed = true
			continue
		}
		kept = append(kept, rewritten)
		if rewritten == member {
			if hasStoredHashes && !knownContainsOpenRecursive(member) {
				hashes = append(hashes, u.memberHashes[i])
			} else {
				hashes = append(hashes, unionMemberHash(member))
			}
			continue
		}
		changed = true
		filterOnly = false
		if !rewrittenUnionMemberUsesStructuralDedupe(rewritten) {
			scalarRewriteOnly = false
		}
		hashes = append(hashes, unionMemberHash(rewritten))
	}
	if !changed {
		return u
	}
	if len(kept) == 0 {
		return Never
	}
	if filterOnly {
		return newRewrittenNormalizedUnion(kept, hashes)
	}
	if scalarRewriteOnly && rewrittenMembersStayFlatNormalized(kept) {
		return newScalarRewrittenUnion(kept, hashes)
	}
	return NewUnion(kept...)
}

func newRewrittenNormalizedUnion(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return members[0]
	}
	if len(members) == 2 {
		if members[0] != nil && members[0].Kind() == kind.Nil {
			return NewOptional(members[1])
		}
		if members[1] != nil && members[1].Kind() == kind.Nil {
			return NewOptional(members[0])
		}
	}
	return newNormalizedUnion(members, memberHashes)
}

func newScalarRewrittenUnion(members []Type, memberHashes []uint64) Type {
	unique, uniqueHashes := deduplicateRewrittenTypesWithKnownHashes(members, memberHashes)
	sortHashedTypes(unique, uniqueHashes)
	return newRewrittenNormalizedUnion(unique, uniqueHashes)
}

func deduplicateRewrittenTypesWithKnownHashes(types []Type, hashes []uint64) ([]Type, []uint64) {
	if len(types) == 0 {
		return nil, nil
	}
	if len(hashes) != len(types) {
		return deduplicateTypesWithHashes(types)
	}

	seen := make(map[uint64][]Type)
	result := make([]Type, 0, len(types))
	resultHashes := make([]uint64, 0, len(types))
	for i, t := range types {
		if t == nil {
			continue
		}
		h := hashes[i]
		if !rewrittenUnionMemberUsesStructuralDedupe(t) {
			result = append(result, t)
			resultHashes = append(resultHashes, h)
			continue
		}
		bucket := seen[h]
		duplicate := false
		for _, existing := range bucket {
			if TypeEquals(existing, t) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[h] = append(bucket, t)
		result = append(result, t)
		resultHashes = append(resultHashes, h)
	}
	return result, resultHashes
}

func rewrittenUnionMemberUsesStructuralDedupe(t Type) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Literal, kind.Self:
		return true
	default:
		return false
	}
}

func rewrittenMembersStayFlatNormalized(members []Type) bool {
	var baseMask uint8
	var literalMask uint8
	for _, member := range members {
		if member == nil {
			return false
		}
		unwrapped := UnwrapAnnotated(member)
		switch unwrapped.Kind() {
		case kind.Never, kind.Unknown, kind.Any, kind.Nil, kind.Union, kind.Optional:
			return false
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
		if lit, ok := unwrapped.(*Literal); ok {
			switch lit.Base {
			case kind.String:
				literalMask |= 1
			case kind.Number:
				literalMask |= 2
			case kind.Integer:
				literalMask |= 4
			case kind.Boolean:
				literalMask |= 8
			}
		}
	}
	if baseMask&2 != 0 && baseMask&4 != 0 {
		return false
	}
	if baseMask&1 != 0 && literalMask&1 != 0 {
		return false
	}
	if baseMask&2 != 0 && literalMask&(2|4) != 0 {
		return false
	}
	if baseMask&4 != 0 && literalMask&4 != 0 {
		return false
	}
	if baseMask&8 != 0 && literalMask&8 != 0 {
		return false
	}
	return true
}
