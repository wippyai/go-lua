package signature

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SubstituteEffectTypes returns a copy of row whose embedded type payloads have
// been substituted with the provided generic bindings.
func SubstituteEffectTypes(row effect.Row, params []*typ.TypeParam, args []typ.Type) effect.Row {
	if len(params) == 0 || len(params) != len(args) {
		return row.Clone()
	}
	out := row.Clone()
	for i, label := range out.Labels {
		out.Labels[i] = substituteEffectLabelTypes(label, params, args)
	}
	return out
}

func substituteEffectLabelTypes(label effect.Label, params []*typ.TypeParam, args []typ.Type) effect.Label {
	switch v := effect.NormalizeLabel(label).(type) {
	case returns.Return:
		v.Transform = substituteReturnTypeTypes(v.Transform, params, args)
		return v
	default:
		return v
	}
}

func substituteReturnTypeTypes(transform returns.ReturnType, params []*typ.TypeParam, args []typ.Type) returns.ReturnType {
	if returns.IsNilReturnType(transform) {
		return transform
	}
	switch v := transform.(type) {
	case returns.TypeProjection:
		v.Projection = substituteProjectionTypes(v.Projection, params, args)
		return v
	case *returns.TypeProjection:
		if v == nil {
			return transform
		}
		out := *v
		out.Projection = substituteProjectionTypes(out.Projection, params, args)
		return out
	case returns.ConditionalType:
		v.Projection = substituteProjectionTypes(v.Projection, params, args)
		v.When = subst.Params(v.When, params, args)
		v.Then = subst.Params(v.Then, params, args)
		return v
	case *returns.ConditionalType:
		if v == nil {
			return transform
		}
		out := *v
		out.Projection = substituteProjectionTypes(out.Projection, params, args)
		out.When = subst.Params(out.When, params, args)
		out.Then = subst.Params(out.Then, params, args)
		return out
	default:
		return transform
	}
}

func substituteProjectionTypes(p projection.Projection, params []*typ.TypeParam, args []typ.Type) projection.Projection {
	if len(p.Steps) == 0 {
		return p
	}
	out := projection.Projection{Steps: make([]projection.Step, len(p.Steps))}
	copy(out.Steps, p.Steps)
	for i := range out.Steps {
		if out.Steps[i].Type != nil {
			out.Steps[i].Type = subst.Params(out.Steps[i].Type, params, args)
		}
	}
	return out
}
