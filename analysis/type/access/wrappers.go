package access

import (
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func descendAccessWrappers[T any](
	t typ.Type,
	depth int,
	descend func(typ.Type, int) T,
	optionalize func(T) T,
) T {
	if stopDepth(t, depth) {
		var zero T
		return zero
	}

	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return optionalize(descendAccessWrappers(v.Inner, depth+1, descend, optionalize))
	case *typ.Alias:
		return descendAccessWrappers(v.UnaliasedTarget(), depth+1, descend, optionalize)
	case *typ.TypeParam:
		if v.Constraint == nil {
			var zero T
			return zero
		}
		return descendAccessWrappers(v.Constraint, depth+1, descend, optionalize)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			var zero T
			return zero
		}
		return descendAccessWrappers(v.Body, depth+1, descend, optionalize)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			var zero T
			return zero
		}
		return descendAccessWrappers(expanded, depth+1, descend, optionalize)
	default:
		return descend(t, depth)
	}
}
