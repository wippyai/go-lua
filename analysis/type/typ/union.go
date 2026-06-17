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

func (u *Union) Kind() kind.Kind { return kind.Union }

func (u *Union) String() string {
	return u.strCache.get(func() string { return joinMemberStrings(u.Members, " | ", "nil") })
}

// joinMemberStrings renders members joined by sep, substituting nilLabel for nil
// members. Shared by the Union and Intersection string forms.
func joinMemberStrings(members []Type, sep, nilLabel string) string {
	parts := make([]string, len(members))
	for i, m := range members {
		if m == nil {
			parts[i] = nilLabel
		} else {
			parts[i] = m.String()
		}
	}
	return strings.Join(parts, sep)
}

func (u *Union) Hash() uint64 { return u.hash }

func (u *Union) Equals(other Type) bool {
	return typeEquals(u, other)
}

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := unionMemberHash(t)
	for _, m := range u.Members {
		if unionMemberHash(m) == h && sameUnionMember(m, t) {
			return true
		}
	}

	return false
}
