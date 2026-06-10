package typ

import (
	"strconv"
	"strings"
)

// StaticMemberKind classifies an exact non-dot table member stored on a record.
type StaticMemberKind uint8

const (
	StaticMemberStringIndex StaticMemberKind = iota + 1
	StaticMemberIntIndex
)

// StaticMember represents a provably-present bracket member such as t["k"] or
// t[1]. It is separate from Field so dot-field shape and bracket-key facts do
// not collapse into one raw string namespace.
type StaticMember struct {
	Kind     StaticMemberKind
	Name     string
	Index    int64
	Type     Type
	Optional bool
	Readonly bool
}

// StaticStringIndex adds a required bracket-string member.
func (b *RecordBuilder) StaticStringIndex(name string, t Type) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, StaticMember{Kind: StaticMemberStringIndex, Name: name, Type: t})
	return b
}

// StaticIntIndex adds a required bracket-integer member.
func (b *RecordBuilder) StaticIntIndex(index int64, t Type) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, StaticMember{Kind: StaticMemberIntIndex, Index: index, Type: t})
	return b
}

// AddStaticMember adds a pre-built exact bracket member.
func (b *RecordBuilder) AddStaticMember(member StaticMember) *RecordBuilder {
	b.staticMembers = append(b.staticMembers, member)
	return b
}

func writeStaticMemberKey(sb *strings.Builder, member StaticMember) {
	switch member.Kind {
	case StaticMemberStringIndex:
		sb.WriteString("[\"")
		sb.WriteString(strings.ReplaceAll(member.Name, `"`, `\"`))
		sb.WriteString("\"]")
	case StaticMemberIntIndex:
		sb.WriteString("[")
		sb.WriteString(strconv.FormatInt(member.Index, 10))
		sb.WriteString("]")
	default:
		sb.WriteString("[unknown]")
	}
}
