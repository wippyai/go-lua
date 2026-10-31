package core

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// EffectRowOf returns the effect row a callable type declares: a function's
// effects, joined over union and intersection members and resolved through
// optional and instantiated wrappers.
func EffectRowOf(t typ.Type) (effect.Row, bool) {
	if t == nil {
		return effect.Row{}, false
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Function:
		row, ok := v.Effects.(effect.Row)
		return row, ok
	case *typ.Optional:
		return EffectRowOf(v.Inner)
	case *typ.Union:
		var merged effect.Row
		for _, m := range v.Members {
			if row, ok := EffectRowOf(m); ok {
				merged = merged.With(row.Labels...)
			}
		}
		if len(merged.Labels) > 0 {
			return merged, true
		}
		return effect.Row{}, false
	case *typ.Intersection:
		var merged effect.Row
		for _, m := range v.Members {
			if row, ok := EffectRowOf(m); ok {
				merged = merged.With(row.Labels...)
			}
		}
		if len(merged.Labels) > 0 {
			return merged, true
		}
		return effect.Row{}, false
	case *typ.Instantiated:
		if resolved, err := ResolveInstantiated(v); err == nil {
			return EffectRowOf(resolved)
		}
	}
	return effect.Row{}, false
}
