package member

import "sort"

// MemberKind classifies a statically-known table member access.
type MemberKind uint8

const (
	MemberKindInvalid MemberKind = iota
	MemberKindField
	MemberKindStringIndex
	MemberKindIntIndex
)

// MemberKey is the value-domain boundary key for a statically-known table
// member. It intentionally distinguishes dot fields from static string indexes:
// `.x` and `["x"]` are different abstract member identities even though both are
// string-keyed Lua table accesses at runtime. That lets downstream code keep
// syntax/shape evidence structural and project to typ.Record source-name fields
// only at the current record boundary.
type MemberKey struct {
	kind  MemberKind
	name  string
	index int
	valid bool
}

// MemberField returns the structural key for a dot-field access. Empty field
// names are invalid.
func MemberField(name string) MemberKey {
	if name == "" {
		return MemberKey{}
	}
	return MemberKey{kind: MemberKindField, name: name, valid: true}
}

// MemberStringIndex returns the structural key for a static string-index access.
// Empty string indexes are valid Lua table keys.
func MemberStringIndex(name string) MemberKey {
	return MemberKey{kind: MemberKindStringIndex, name: name, valid: true}
}

// MemberIntIndex returns the structural key for a static integer-index access.
func MemberIntIndex(index int) MemberKey {
	return MemberKey{kind: MemberKindIntIndex, index: index, valid: true}
}

func (k MemberKey) IsValid() bool    { return k.valid }
func (k MemberKey) Kind() MemberKind { return k.kind }
func (k MemberKey) Name() string     { return k.name }
func (k MemberKey) Index() int       { return k.index }

// CompareMemberKey returns deterministic structural order for member-key maps.
func CompareMemberKey(left, right MemberKey) int {
	switch {
	case left.valid != right.valid:
		if !left.valid {
			return -1
		}
		return 1
	case left.kind != right.kind:
		if left.kind < right.kind {
			return -1
		}
		return 1
	case left.name != right.name:
		if left.name < right.name {
			return -1
		}
		return 1
	case left.index != right.index:
		if left.index < right.index {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// SortMemberKeys sorts member keys in deterministic structural order.
func SortMemberKeys(keys []MemberKey) {
	sort.Slice(keys, func(i, j int) bool {
		return CompareMemberKey(keys[i], keys[j]) < 0
	})
}

// SortedMemberKeys returns map keys in deterministic structural order.
func SortedMemberKeys[T any](m map[MemberKey]T) []MemberKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]MemberKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	SortMemberKeys(keys)
	return keys
}
