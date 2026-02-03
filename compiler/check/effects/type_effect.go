package effects

import (
	"github.com/wippyai/go-lua/types/effect"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// HasEffectInType checks whether any reachable function type inside t has an
// effect row that satisfies the predicate.
func HasEffectInType(t typ.Type, check func(effect.Row) bool) bool {
	if t == nil {
		return false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Function:
		if row, ok := v.Effects.(effect.Row); ok {
			return check(row)
		}
	case *typ.Optional:
		return HasEffectInType(v.Inner, check)
	case *typ.Union:
		for _, m := range v.Members {
			if HasEffectInType(m, check) {
				return true
			}
		}
	case *typ.Intersection:
		for _, m := range v.Members {
			if HasEffectInType(m, check) {
				return true
			}
		}
	case *typ.Instantiated:
		if resolved, err := querycore.ResolveInstantiated(v); err == nil {
			return HasEffectInType(resolved, check)
		}
	}
	return false
}
