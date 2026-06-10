package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/lua/access"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ApplyTypeProjection applies projection steps to source.
func ApplyTypeProjection(source typ.Type, projection effect.TypeProjection) (typ.Type, bool) {
	current := source
	for _, step := range projection.Steps {
		switch step.Kind {
		case effect.TypeProjectionField:
			next, ok := access.Field(current, step.Field)
			if !ok {
				return nil, false
			}
			current = next
		case effect.TypeProjectionCallableReturn:
			next, ok := access.CallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case effect.TypeProjectionGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		case effect.TypeProjectionInstantiateGeneric:
			g, ok := unwrap.Alias(step.Type).(*typ.Generic)
			if !ok || g == nil || len(g.TypeParams) != 1 || current == nil {
				return nil, false
			}

			payload := current
			if meta, ok := unwrap.Alias(payload).(*typ.Meta); ok && meta != nil && meta.Of != nil {
				payload = meta.Of
			}
			if payload == nil {
				return nil, false
			}
			if constraint := g.TypeParams[0].Constraint; constraint != nil && !subtype.IsSubtype(payload, constraint) {
				return nil, false
			}
			current = typ.Instantiate(g, payload)
		default:
			return nil, false
		}
	}
	return current, current != nil
}
