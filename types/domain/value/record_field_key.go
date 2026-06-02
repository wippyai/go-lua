package value

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

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
// string-keyed Lua table accesses at runtime. That lets product/domain code keep
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

// MemberFromSegment lowers a constraint path segment into a structural member key.
func MemberFromSegment(seg constraint.Segment) (MemberKey, bool) {
	switch seg.Kind {
	case constraint.SegmentField:
		key := MemberField(seg.Name)
		return key, key.IsValid()
	case constraint.SegmentIndexString:
		return MemberStringIndex(seg.Name), true
	case constraint.SegmentIndexInt:
		return MemberIntIndex(seg.Index), true
	default:
		return MemberKey{}, false
	}
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

// recordFieldKey is the value-domain's structural identity for a typ.Record dot
// field. Exact bracket members use recordStaticMemberKey below so `.x` and
// `["x"]` remain different lattice coordinates.
type recordFieldKey = MemberKey

type recordStaticMemberKey = MemberKey

func recordFieldKeyFromName(name string) recordFieldKey {
	return MemberField(name)
}

func recordFieldKeyName(key recordFieldKey) string {
	return key.Name()
}

func recordFieldsByKey(r *typ.Record) map[recordFieldKey]typ.Field {
	out := make(map[recordFieldKey]typ.Field, len(r.Fields))
	for _, field := range r.Fields {
		out[recordFieldKeyFromName(field.Name)] = field
	}
	return out
}

func recordStaticMemberKeyFromMember(member typ.StaticMember) recordStaticMemberKey {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return MemberStringIndex(member.Name)
	case typ.StaticMemberIntIndex:
		return MemberIntIndex(int(member.Index))
	default:
		return MemberKey{}
	}
}

func recordStaticMembersByKey(r *typ.Record) map[recordStaticMemberKey]typ.StaticMember {
	out := make(map[recordStaticMemberKey]typ.StaticMember, len(r.StaticMembers))
	for _, member := range r.StaticMembers {
		out[recordStaticMemberKeyFromMember(member)] = member
	}
	return out
}

func recordStaticMemberByKey(r *typ.Record, key recordStaticMemberKey) *typ.StaticMember {
	if r == nil {
		return nil
	}
	switch key.Kind() {
	case MemberKindStringIndex:
		return r.GetStaticStringIndex(key.Name())
	case MemberKindIntIndex:
		return r.GetStaticIntIndex(int64(key.Index()))
	default:
		return nil
	}
}

func sortedRecordStaticMemberKeys[T any](m map[recordStaticMemberKey]T) []recordStaticMemberKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]recordStaticMemberKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	SortMemberKeys(keys)
	return keys
}

func staticMemberKeyType(member typ.StaticMember) typ.Type {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return typ.LiteralString(member.Name)
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index)
	default:
		return typ.Unknown
	}
}

func sortedRecordFieldKeys[T any](m map[recordFieldKey]T) []recordFieldKey {
	if len(m) == 0 {
		return nil
	}
	keys := make([]recordFieldKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sortRecordFieldKeys(keys)
	return keys
}

func sortRecordFieldKeys(keys []recordFieldKey) {
	SortMemberKeys(keys)
}

func sortRecordFieldsByKey(fields []typ.Field) {
	sort.Slice(fields, func(i, j int) bool {
		left := recordFieldKeyFromName(fields[i].Name)
		right := recordFieldKeyFromName(fields[j].Name)
		return CompareMemberKey(left, right) < 0
	})
}
