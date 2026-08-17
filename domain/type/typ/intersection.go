package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
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
	Members []Type
	hash    uint64
	typeProperties
	strCache stringCache
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
	for i := range membersCopy {
		h = hash.MixHash(h, hashesCopy[i])
	}
	props := typePropertiesOf(membersCopy...)

	return &Intersection{
		Members:        membersCopy,
		hash:           h,
		typeProperties: props,
	}
}

func (i *Intersection) Kind() kind.Kind { return kind.Intersection }

func (i *Intersection) String() string {
	return i.strCache.get(func() string { return renderTypeString(i) })
}

func (i *Intersection) Hash() uint64 { return i.hash }

func (i *Intersection) Equals(other Type) bool {
	return typeEquals(i, other)
}
