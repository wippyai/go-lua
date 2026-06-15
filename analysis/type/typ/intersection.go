package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Intersection represents a type satisfying all member constraints: T1 & T2 & ...
//
// Intersections require a value to satisfy every member type simultaneously.
// They are normalized during construction:
//   - Nested intersections are flattened
//   - Duplicate members are removed
//
// Members are sorted by hash for deterministic comparison.
type Intersection struct {
	Members               []Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// MaterializeIntersection builds the hash-stable intersection node for
// already-selected members.
//
// It performs only low-level node materialization owned by typ: nil Type
// interface filtering, duplicate removal, deterministic member ordering,
// hash/cache/contains flag computation, and empty/single cardinality collapse.
// It does not apply intersection semantics: nested intersections are kept as
// members, Optional is not interpreted as nil plus inner, and no
// Any/Unknown/Never/nil or literal/base relation policy is applied.
func MaterializeIntersection(members []Type) Type {
	filtered := filterNilTypes(members)
	unique, uniqueHashes := deduplicateTypesWithHashes(filtered)
	sortHashedTypes(unique, uniqueHashes)
	return newCanonicalIntersection(unique, uniqueHashes)
}

func newCanonicalIntersection(members []Type, memberHashes []uint64) Type {
	if len(members) == 0 {
		return Any
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

	h := uint64(kind.Intersection)
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
		if !containsOpenRecursive && knownContainsOpenRecursive(m) {
			containsOpenRecursive = true
		}
	}

	return &Intersection{
		Members:               membersCopy,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func (i *Intersection) Kind() kind.Kind { return kind.Intersection }

func (i *Intersection) String() string {
	return i.strCache.get(func() string {
		parts := make([]string, len(i.Members))
		for j, m := range i.Members {
			if m == nil {
				parts[j] = "unknown"
			} else {
				parts[j] = m.String()
			}
		}
		return strings.Join(parts, " & ")
	})
}

func (i *Intersection) Hash() uint64 { return i.hash }

func (i *Intersection) Equals(other Type) bool {
	return typeEquals(i, other)
}
