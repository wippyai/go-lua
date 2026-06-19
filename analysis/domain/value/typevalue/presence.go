package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func presenceFromType(t typ.Type) (presence.Value, bool) {
	return presenceFromTypeDepth(t, 0)
}

func presenceFromTypeDepth(t typ.Type, depth int) (presence.Value, bool) {
	if depth > typ.DefaultRecursionDepth {
		return presence.Bottom(), false
	}
	t = normalize(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return presence.Bottom(), false
	}
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
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeDepth(expanded, depth+1)
	case *typ.Alias:
		target := tt.UnaliasedTarget()
		if target == nil || target == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeDepth(target, depth+1)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return presence.Bottom(), false
		}
		return presenceFromTypeDepth(tt.Body, depth+1)
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
