package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// DefinitelyNonEmptyIndexContainer reports whether a type proves that at least
// one positional/index value exists. Every union arm must carry the proof.
func DefinitelyNonEmptyIndexContainer(t typ.Type) bool {
	ok, productive := definitelyNonEmptyIndexContainer(t, &typegraph.Path{})
	return ok && productive
}

func definitelyNonEmptyIndexContainer(t typ.Type, active *typegraph.Path) (bool, bool) {
	if t == nil {
		return false, true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return true, false
	}
	defer active.Leave(t, 0)
	switch tt := t.(type) {
	case *typ.Tuple:
		return len(tt.Elements) > 0, true
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false, true
		}
		productive := false
		for _, member := range tt.Members {
			ok, memberProductive := definitelyNonEmptyIndexContainer(member, active)
			if !ok {
				return false, true
			}
			productive = productive || memberProductive
		}
		return true, productive
	case *typ.Alias:
		return definitelyNonEmptyIndexContainer(tt.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return false, false
		}
		return definitelyNonEmptyIndexContainer(expanded, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false, false
		}
		return definitelyNonEmptyIndexContainer(tt.Body, active)
	default:
		return false, true
	}
}
