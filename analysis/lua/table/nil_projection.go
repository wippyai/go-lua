package table

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type nilProjectionMode uint8

const (
	nilProjectionStructural nilProjectionMode = iota
	nilProjectionPreserveAliases
)

func withoutNil(t typ.Type, mode nilProjectionMode) (nonNil typ.Type, nilable bool) {
	t = typ.NormalizeNilType(t)
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
	members := make([]typ.Type, 0, len(u.Members))
	nilable := false
	for _, member := range u.Members {
		member = typ.NormalizeNilType(member)
		if member == nil {
			continue
		}
		nonNil, memberNilable := withoutNil(member, mode)
		if memberNilable {
			nilable = true
		}
		if nonNil == nil {
			nonNil = member
		}
		if nonNil.Kind().IsNever() {
			continue
		}
		members = append(members, nonNil)
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
	return typ.NewUnion(members...), true
}
