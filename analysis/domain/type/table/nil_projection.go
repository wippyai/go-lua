package table

import (
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

type nilProjectionMode uint8

const (
	nilProjectionStructural nilProjectionMode = iota
	nilProjectionPreserveAliases
)

func withoutNil(t typ.Type, mode nilProjectionMode) (nonNil typ.Type, nilable bool) {
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}

	switch v := t.(type) {
	case *typ.Annotated:
		inner, nilable := withoutNil(v.Inner, mode)
		if !nilable {
			return t, false
		}
		if inner == nil {
			inner = typ.Never
		}
		return typ.NewAnnotated(inner, v.Annotations), true
	case *typ.Alias:
		target, nilable := withoutNil(v.Target, mode)
		if !nilable {
			return t, false
		}
		if target == nil {
			target = typ.Never
		}
		if mode == nilProjectionPreserveAliases {
			return typ.NewAlias(v.Name, target), true
		}
		return target, true
	case *typ.Optional:
		if v.Inner == nil {
			return typ.Never, true
		}
		inner, _ := withoutNil(v.Inner, mode)
		if inner == nil {
			inner = typ.Never
		}
		return inner, true
	case *typ.Union:
		rewritten, nilable := projectUnionWithoutNil(v, mode)
		if !nilable {
			return t, false
		}
		return rewritten, true
	default:
		if t.Kind() == kind.Nil {
			return typ.Never, true
		}
		return t, false
	}
}

func projectUnionWithoutNil(u *typ.Union, mode nilProjectionMode) (typ.Type, bool) {
	if u == nil {
		return typ.Never, false
	}
	nilable := false
	var members []typ.Type
	for i, member := range u.Members {
		normalized := unwrap.NormalizeNil(member)
		if normalized == nil {
			continue
		}
		nonNil, memberNilable := withoutNil(normalized, mode)
		if !memberNilable {
			if nilable {
				members = appendProjectedUnionMember(members, normalized, mode)
			}
			continue
		}
		if !nilable {
			nilable = true
			members = make([]typ.Type, 0, len(u.Members)-1)
			for _, prefix := range u.Members[:i] {
				members = appendProjectedUnionMember(members, prefix, mode)
			}
		}
		members = appendNonNeverUnionMember(members, nonNil)
	}
	if !nilable {
		return u, false
	}
	if len(members) == 0 {
		return typ.Never, true
	}
	if len(members) == 1 {
		return members[0], true
	}
	return typ.MaterializeUnion(members), true
}

func appendProjectedUnionMember(members []typ.Type, member typ.Type, mode nilProjectionMode) []typ.Type {
	member = unwrap.NormalizeNil(member)
	if member == nil {
		return members
	}
	nonNil, _ := withoutNil(member, mode)
	if nonNil == nil {
		nonNil = member
	}
	return appendNonNeverUnionMember(members, nonNil)
}

func appendNonNeverUnionMember(members []typ.Type, member typ.Type) []typ.Type {
	if member == nil || member.Kind().IsNever() {
		return members
	}
	return append(members, member)
}
