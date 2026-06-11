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

// NewIntersection creates a normalized intersection type.
// Returns Any for empty intersections, the single type for one member,
// or a normalized Intersection for multiple distinct members.
func NewIntersection(members ...Type) Type {
	if len(members) == 0 {
		return Any
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))

	var addMember func(Type)
	addMember = func(m Type) {
		if m == nil {
			return
		}

		unwrapped := unwrapAnnotated(m)
		if unwrapped == nil {
			return
		}

		if unwrapped.Kind() == kind.Intersection {
			for _, member := range unwrapped.(*Intersection).Members {
				addMember(member)
			}
			return
		}

		flat = append(flat, m)
	}

	for _, m := range members {
		addMember(m)
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique, uniqueHashes := deduplicateTypesWithHashes(flat)
	sortHashedTypes(unique, uniqueHashes)

	if len(unique) == 0 {
		return Any
	}

	if len(unique) == 1 {
		return unique[0]
	}

	h := uint64(kind.Intersection)
	containsAny := false
	containsNever := false
	containsTypeParam := false
	containsInstantiated := false
	containsRecursive := false
	containsOpenRecursive := false
	for i, m := range unique {
		h = hash.MixHash(h, uniqueHashes[i])
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
		Members:               unique,
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
