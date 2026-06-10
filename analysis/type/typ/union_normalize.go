package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

func newNormalizedUnion(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return members[0]
	}
	if len(memberHashes) != len(members) {
		memberHashes = make([]uint64, len(members))
		for i, m := range members {
			memberHashes[i] = UnionMemberHash(m)
		}
	}

	membersCopy := make([]Type, len(members))
	copy(membersCopy, members)
	hashesCopy := make([]uint64, len(memberHashes))
	copy(hashesCopy, memberHashes)

	// Compute hash and cached structural flags.
	h := uint64(kind.Union)
	containsAny := false
	containsNever := false
	containsTypeParam := false
	containsInstantiated := false
	containsRecursive := false
	containsOpenRecursive := false
	for i, m := range membersCopy {
		h = hash.HashCombine(h, hashesCopy[i])
		if !containsAny && knownContainsAny(m) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(m) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(m) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(m) {
			containsInstantiated = true
		}
		if !containsRecursive && knownContainsRecursive(m) {
			containsRecursive = true
		}
		if !containsOpenRecursive && unionMemberContainsOpenRecursive(m) {
			containsOpenRecursive = true
		}
	}

	return &Union{
		Members:               membersCopy,
		memberHashes:          hashesCopy,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func unionMemberContainsOpenRecursive(t Type) bool {
	if rec, ok := UnwrapAnnotated(t).(*Recursive); ok {
		return rec.Body == nil
	}
	return knownContainsOpenRecursive(t)
}
