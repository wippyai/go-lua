package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// MaterializeUnion builds the hash-stable union node for already-selected members.
//
// It performs only low-level node materialization owned by typ: nil Type
// interface filtering, duplicate removal, deterministic member ordering,
// hash/cache/contains flag computation, and empty/single cardinality collapse.
// It does not apply union semantics: nested unions are kept as members,
// Optional is not interpreted as nil plus inner, and no Any/Unknown/Never/nil
// or literal/base relation policy is applied.
func MaterializeUnion(members []Type) Type {
	filtered := filterNilTypes(members)
	unique, uniqueHashes := deduplicateTypesWithHashes(filtered)
	sortHashedTypes(unique, uniqueHashes)
	return newCanonicalUnion(unique, uniqueHashes)
}

func filterNilTypes(types []Type) []Type {
	if len(types) == 0 {
		return nil
	}
	filtered := make([]Type, 0, len(types))
	for _, t := range types {
		if t != nil {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// typ owns hash-stable node materialization once semantic normalization has already happened.
func newCanonicalUnion(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return members[0]
	}
	if len(memberHashes) != len(members) {
		memberHashes = make([]uint64, len(members))
		for i, m := range members {
			memberHashes[i] = unionMemberHash(m)
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
		h = hash.MixHash(h, hashesCopy[i])
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
	if rec, ok := unwrapAnnotated(t).(*Recursive); ok {
		return rec.Body == nil
	}
	return knownContainsOpenRecursive(t)
}
