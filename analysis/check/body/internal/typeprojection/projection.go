package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Apply applies projection steps to source.
func Apply(source typ.Type, p projection.Projection) (typ.Type, bool) {
	current := source
	for _, step := range p.Steps {
		switch step.Kind {
		case projection.StepField:
			next, ok := typeaccess.Field(current, step.Field)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepCallableReturn:
			next, ok := typecall.CallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		case projection.StepInstantiateGeneric:
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

// ElementOf projects the element/value type read from Lua container shapes.
func ElementOf(t typ.Type) (typ.Type, bool) {
	return elementOfDepth(t, 0)
}

func elementOfDepth(t typ.Type, depth int) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return elementOfDepth(tt.Inner, depth+1)
	case *typ.Alias:
		return elementOfDepth(tt.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return elementOfDepth(tt.Inner, depth+1)
	case *typ.Array:
		if unwrap.NormalizeNil(tt.Element) == nil {
			return nil, false
		}
		return tt.Element, true
	case *typ.Map:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.ReadonlyMap:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		if len(tt.Elements) == 1 {
			if unwrap.NormalizeNil(tt.Elements[0]) == nil {
				return nil, false
			}
			return tt.Elements[0], true
		}
		return typ.NewUnion(tt.Elements...), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			member = unwrap.NormalizeNil(member)
			if member == nil {
				continue
			}
			if member.Kind() == kind.Nil {
				continue
			}
			elem, ok := elementOfDepth(member, depth+1)
			if !ok {
				return nil, false
			}
			members = append(members, elem)
		}
		if len(members) == 0 {
			return nil, false
		}
		return typ.NewUnion(members...), true
	default:
		return nil, false
	}
}
