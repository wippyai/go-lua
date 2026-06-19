package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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
		if !typeIncludesNil(t) {
			return typenormalize.Optional(t)
		}
	case presence.Equal(p, presence.Present()):
		if withoutNil := typetable.PresentReadonlyEntryValue(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func typeIncludesNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	normalized := unwrap.NormalizeNil(t)
	return (normalized != nil && normalized.Kind() == kind.Nil) || ProjectionHasNil(t)
}

// ProjectionHasNil reports whether a projected type can include nil.
func ProjectionHasNil(t typ.Type) bool {
	return projectionHasNilDepth(t, 0)
}

func projectionHasNilDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	t = unwrap.NormalizeNil(unwrap.Annotated(t))
	if t == nil {
		return false
	}
	if t.Kind() == kind.Nil {
		return true
	}
	switch tt := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range tt.Members {
			if projectionHasNilDepth(member, depth+1) {
				return true
			}
		}
	case *typ.Alias:
		return projectionHasNilDepth(tt.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		return expanded != nil && expanded != t && projectionHasNilDepth(expanded, depth+1)
	case *typ.Recursive:
		return tt.Body != nil && tt.Body != t && projectionHasNilDepth(tt.Body, depth+1)
	}
	return false
}
