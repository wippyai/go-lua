package table

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	keptCount := 0
	for _, member := range u.Members {
		normalized := unwrap.NormalizeNil(member)
		if normalized == nil {
			continue
		}
		nonNil, memberNilable := withoutNil(normalized, mode)
		if memberNilable {
			nilable = true
		}
		if nonNil == nil {
			nonNil = normalized
		}
		if nonNil.Kind().IsNever() {
			continue
		}
		keptCount++
	}
	if !nilable {
		return u, false
	}
	members := make([]typ.Type, 0, keptCount)
	for _, member := range u.Members {
		member = unwrap.NormalizeNil(member)
		if member == nil {
			continue
		}
		nonNil, _ := withoutNil(member, mode)
		if nonNil == nil {
			nonNil = member
		}
		if nonNil.Kind().IsNever() {
			continue
		}
		members = append(members, nonNil)
	}
	if len(members) == 0 {
		return typ.Never, true
	}
	if len(members) == 1 {
		return members[0], true
	}
	return typ.MaterializeUnion(members), true
}
