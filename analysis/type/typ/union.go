package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Union represents a type that can be any of its member types: T1 | T2 | ...
//
// Unions are normalized during construction:
//   - Nested unions are flattened
//   - Duplicate members are removed
//   - Single member with nil becomes Optional representation sugar
//
// Members are sorted by hash for deterministic comparison and serialization.
type Union struct {
	Members               []Type
	memberHashes          []uint64
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewUnion creates a normalized union type from the given members.
// Returns Never for empty unions, the single type for one member,
// or a normalized Union for multiple distinct members.
func NewUnion(members ...Type) Type {
	if len(members) == 0 {
		return Never
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))
	hasNil := false

	var addMember func(Type)
	addMember = func(m Type) {
		if m == nil {
			return
		}

		// Unwrap Annotated to access structural type for flattening.
		// Annotations delegate Kind() to their inner type, so type
		// assertions on concrete wrappers (Union, Optional) require
		// operating on the unwrapped type.
		unwrapped := UnwrapAnnotated(m)
		if unwrapped == nil {
			return
		}

		switch unwrapped.Kind() {
		case kind.Nil:
			hasNil = true
		case kind.Union:
			for _, member := range unwrapped.(*Union).Members {
				addMember(member)
			}
		case kind.Optional:
			hasNil = true
			addMember(unwrapped.(*Optional).Inner)
		default:
			flat = append(flat, m)
		}
	}

	for _, m := range members {
		addMember(m)
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique, uniqueHashes := deduplicateTypesWithHashes(flat)
	sortHashedTypes(unique, uniqueHashes)

	// Add nil back if present
	if hasNil {
		// If single member + nil, return Optional
		if len(unique) == 1 {
			return newOptionalNode(unique[0])
		}

		unique = append([]Type{Nil}, unique...)
		uniqueHashes = append([]uint64{Nil.Hash()}, uniqueHashes...)
	}

	if len(unique) == 0 {
		if hasNil {
			return Nil
		}

		return Never
	}

	if len(unique) == 1 {
		return unique[0]
	}

	return newNormalizedUnion(unique, uniqueHashes)
}

func (u *Union) Kind() kind.Kind { return kind.Union }

func (u *Union) String() string {
	return u.strCache.get(func() string {
		parts := make([]string, len(u.Members))
		for i, m := range u.Members {
			if m == nil {
				parts[i] = "nil"
			} else {
				parts[i] = m.String()
			}
		}
		return strings.Join(parts, " | ")
	})
}

func (u *Union) Hash() uint64 { return u.hash }

func (u *Union) Equals(other Type) bool {
	return TypeEquals(u, other)
}

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := UnionMemberHash(t)
	for _, m := range u.Members {
		if UnionMemberHash(m) == h && SameUnionMember(m, t) {
			return true
		}
	}

	return false
}
