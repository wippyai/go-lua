package discriminant

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func staticMemberPath(member typ.StaticMember) string {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return "[" + strconv.Quote(member.Name) + "]"
	case typ.StaticMemberIntIndex:
		return "[" + strconv.FormatInt(member.Index, 10) + "]"
	default:
		return "[]"
	}
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
