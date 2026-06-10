package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Union represents a type that can be any of its member types: T1 | T2 | ...
//
// Unions are normalized during construction:
//   - Nested unions are flattened
//   - Duplicate members are removed
//   - Never members are dropped (Never is identity for union)
//   - Any absorbs all other members (union with Any = Any)
//   - Literals are subsumed by their base types (string | "foo" = string)
//   - Single non-nil member with nil becomes Optional
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
	containsCallableSurf  bool
	strCache              stringCache
}

// NewUnion creates a normalized union type from the given members.
// Returns Never for empty unions, the single type for one member,
// or a normalized Union for multiple distinct members.
func NewUnion(members ...Type) Type {
	if len(members) == 0 {
		return Never
	}

	if len(members) == 1 {
		return members[0]
	}

	// Flatten and collect
	flat := make([]Type, 0, len(members))
	hasNil := false
	hasAny := false
	hasUnknown := false

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

		switch unwrapped.Kind() {
		case kind.Never:
			return // Never is identity for union
		case kind.Unknown:
			hasUnknown = true
			return // Unknown doesn't contribute information to union
		case kind.Any:
			hasAny = true
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

	if hasAny {
		return Any
	}

	// Deduplicate by hash + structural equality (collision-safe).
	unique, uniqueHashes := deduplicateTypesWithHashes(flat)

	// If nothing concrete remains, preserve Unknown (and Optional<Unknown> when nil present).
	if len(unique) == 0 && hasUnknown {
		if hasNil {
			return NewOptional(Unknown)
		}
		return Unknown
	}

	// Primitive subsumption:
	// number subsumes integer in the lattice, so keep number only.
	hasNumberType := false
	for _, m := range unique {
		if m.Kind() == kind.Number {
			hasNumberType = true
			break
		}
	}
	if hasNumberType {
		filtered := unique[:0]
		for _, m := range unique {
			if m.Kind() == kind.Integer {
				continue
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Subsume literals: if a base type is present, drop literal members with matching base.
	// e.g. string | "" => string, number | 42 => number
	var baseMask uint8
	for _, m := range unique {
		switch m.Kind() {
		case kind.String:
			baseMask |= 1
		case kind.Number:
			baseMask |= 2
		case kind.Integer:
			baseMask |= 4
		case kind.Boolean:
			baseMask |= 8
		}
	}
	if baseMask != 0 {
		filtered := unique[:0]
		for _, m := range unique {
			if lit, ok := m.(*Literal); ok {
				drop := false
				switch lit.Base {
				case kind.String:
					drop = baseMask&1 != 0
				case kind.Number:
					drop = baseMask&2 != 0
				case kind.Integer:
					drop = baseMask&4 != 0 || baseMask&2 != 0
				case kind.Boolean:
					drop = baseMask&8 != 0
				}
				if drop {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		unique = filtered
	}

	// Sort by hash then string for deterministic order. Keep the precomputed
	// hashes paired with their members because recursive products are expensive
	// to hash.
	slots := make([]hashedType, len(unique))
	for i, m := range unique {
		slots[i] = hashedType{typ: m, hash: uniqueHashes[i]}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		return slots[i].typ.String() < slots[j].typ.String()
	})
	for i, slot := range slots {
		unique[i] = slot.typ
		uniqueHashes[i] = slot.hash
	}

	// Add nil back if present
	if hasNil {
		// If single member + nil, return Optional
		if len(unique) == 1 {
			return NewOptional(unique[0])
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
