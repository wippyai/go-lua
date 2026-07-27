package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func typeWithPresence(t typ.Type, p presence.Value) typ.Type {
	switch {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Maybe()):
		if !TypeIncludesNil(t) {
			return typenormalize.Optional(t)
		}
	case presence.Equal(p, presence.Present()):
		if withoutNil := typetable.PresentReadonlyEntryValue(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

// TypeWithPresence applies a value-presence lane to a type witness using the
// same projection TypeOf uses when reconstructing a value type.
func TypeWithPresence(t typ.Type, p presence.Value) typ.Type {
	return typeWithPresence(t, p)
}

// TypeIncludesNil reports whether t admits nil, by direct nil evidence or a
// nil-bearing projection.
func TypeIncludesNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	normalized := unwrap.NormalizeNil(t)
	return (normalized != nil && normalized.Kind() == kind.Nil) || ProjectionHasNil(t)
}

// ProjectionHasNil reports whether a projected type can include nil.
func ProjectionHasNil(t typ.Type) bool {
	hasNil, productive := projectionHasNilSeen(t, &typegraph.Path{})
	return hasNil && productive
}

func projectionHasNilSeen(t typ.Type, active *typegraph.Path) (bool, bool) {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false, true
	}
	t = unwrap.NormalizeNil(unwrap.Annotated(t))
	if t == nil {
		return false, true
	}
	if !active.Enter(t, 0) {
		return false, false
	}
	defer active.Leave(t, 0)
	if t.Kind() == kind.Nil {
		return true, true
	}
	switch tt := t.(type) {
	case *typ.Optional:
		return true, true
	case *typ.Union:
		productive := false
		for _, member := range tt.Members {
			hasNil, memberProductive := projectionHasNilSeen(member, active)
			productive = productive || memberProductive
			if hasNil && memberProductive {
				return true, true
			}
		}
		return false, productive
	case *typ.Intersection:
		if len(tt.Members) == 0 {
			return false, true
		}
		productive := false
		for _, member := range tt.Members {
			hasNil, memberProductive := projectionHasNilSeen(member, active)
			if !memberProductive {
				continue
			}
			productive = true
			if !hasNil {
				return false, true
			}
		}
		return productive, productive
	case *typ.Alias:
		return projectionHasNilSeen(tt.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return false, false
		}
		return projectionHasNilSeen(expanded, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false, false
		}
		return projectionHasNilSeen(tt.Body, active)
	}
	return false, true
}
