package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func presenceFromType(t typ.Type) (presence.Value, bool) {
	return presenceFromTypeSeen(t, &typegraph.Path{})
}

func presenceFromTypeSeen(t typ.Type, active *typegraph.Path) (presence.Value, bool) {
	t = normalize(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return presence.Bottom(), false
	}
	if !active.Enter(t, 0) {
		return presence.Bottom(), false
	}
	defer active.Leave(t, 0)
	switch tt := t.(type) {
	case *typ.Optional:
		return presence.Maybe(), true
	case *typ.Union:
		seenNil := false
		for _, member := range tt.Members {
			member = normalize(member)
			if member == nil || typ.IsAny(member) || typ.IsUnknown(member) {
				return presence.Bottom(), false
			}
			if member.Kind() == kind.Nil {
				seenNil = true
				continue
			}
			if ProjectionHasNil(member) {
				seenNil = true
			}
		}
		if seenNil {
			return presence.Maybe(), true
		}
		return presence.Present(), true
	case *typ.Intersection:
		out := presence.Bottom()
		seen := false
		for _, member := range tt.Members {
			memberPresence, ok := presenceFromTypeSeen(member, active)
			if !ok || presence.Equal(memberPresence, presence.Bottom()) {
				continue
			}
			if !seen {
				out = memberPresence
				seen = true
				continue
			}
			out = presence.Meet(out, memberPresence)
			if presence.Equal(out, presence.Bottom()) {
				return presence.Bottom(), false
			}
		}
		return out, seen
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeSeen(expanded, active)
	case *typ.Alias:
		target := tt.UnaliasedTarget()
		if target == nil || target == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeSeen(target, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeSeen(tt.Body, active)
	default:
		if t.Kind() == kind.Nil {
			return presence.Absent(), true
		}
		if _, ok := RuntimeKindFromType(t); ok {
			return presence.Present(), true
		}
		if t.Kind() == kind.Interface {
			return presence.Present(), true
		}
		return presence.Bottom(), false
	}
}
