package typeprojection

import (
	"github.com/wippyai/go-lua/types/effect"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FromArgs resolves a TypeProjection against a call's argument types.
func FromArgs(args []typ.Type, projection effect.TypeProjection) typ.Type {
	idx, ok := effect.ResolveParamIndex(projection.Source, len(args))
	if !ok || idx < 0 || idx >= len(args) {
		return nil
	}
	return ApplySteps(args[idx], projection.Steps)
}

// ApplySteps walks an already-selected type witness through TypeProjection steps.
func ApplySteps(current typ.Type, steps []effect.TypeProjectionStep) typ.Type {
	if current == nil {
		return nil
	}
	for _, step := range steps {
		next := applyStep(current, step)
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func applyStep(t typ.Type, step effect.TypeProjectionStep) typ.Type {
	switch step.Kind {
	case effect.TypeProjectionField:
		ft, ok := querycore.Field(t, step.Field)
		if !ok {
			return nil
		}
		return ft
	case effect.TypeProjectionCallableReturn:
		return CallableReturn(t)
	case effect.TypeProjectionGenericArg:
		inst, ok := unwrap.Alias(t).(*typ.Instantiated)
		if !ok || step.Index < 0 || step.Index >= len(inst.TypeArgs) {
			return nil
		}
		return inst.TypeArgs[step.Index]
	case effect.TypeProjectionInstantiateGeneric:
		generic, ok := unwrap.Alias(step.Type).(*typ.Generic)
		if !ok || len(generic.TypeParams) != 1 {
			return nil
		}
		return typ.Instantiate(generic, payload(t))
	default:
		return nil
	}
}

// CallableReturn projects a callable witness to its first return type.
func CallableReturn(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Function:
		if len(v.Returns) == 0 || v.Returns[0] == nil {
			return nil
		}
		return v.Returns[0]
	case *typ.Optional:
		return CallableReturn(v.Inner)
	case *typ.Union:
		var members []typ.Type
		for _, m := range v.Members {
			if rt := CallableReturn(m); rt != nil {
				members = append(members, rt)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	default:
		if typ.IsAny(t) {
			return typ.Any
		}
		if typ.IsUnknown(t) {
			return typ.Unknown
		}
		return nil
	}
}

func payload(t typ.Type) typ.Type {
	if meta, ok := unwrap.Alias(t).(*typ.Meta); ok && meta != nil && meta.Of != nil {
		return meta.Of
	}
	return t
}
