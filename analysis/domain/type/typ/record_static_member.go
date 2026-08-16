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

// CompareStaticMembers compares static members by their canonical record key.
func CompareStaticMembers(left, right StaticMember) int {
	return compareStaticMemberKey(left, right.Kind, right.Name, right.Index)
}

func compareStaticMemberKey(left StaticMember, kind StaticMemberKind, name string, index int64) int {
	if left.Kind != kind {
		if left.Kind < kind {
			return -1
		}
		return 1
	}
	switch left.Kind {
	case StaticMemberStringIndex:
		if left.Name < name {
			return -1
		}
		if left.Name > name {
			return 1
		}
	case StaticMemberIntIndex:
		if left.Index < index {
			return -1
		}
		if left.Index > index {
			return 1
		}
	}
	return 0
}

func staticMembersSorted(members []StaticMember) bool {
	for i := 1; i < len(members); i++ {
		if CompareStaticMembers(members[i-1], members[i]) > 0 {
			return false
		}
	}
	return true
}

// WriteStaticMemberKey renders a static member's bracketed key into sb.
func WriteStaticMemberKey(sb *strings.Builder, member StaticMember) {
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
