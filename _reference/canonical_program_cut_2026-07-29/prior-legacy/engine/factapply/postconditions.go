package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ApplyExpressionConditionFacts applies the path facts selected by a boolean
// expression value. It is used both for call-parameter postconditions and for
// temporary short-circuit operand states.
func ApplyExpressionConditionFacts(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	facts factflow.ExpressionConditionFacts,
) state.State {
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	domain := state.RegisteredProductDomain(reg)
	for _, refinement := range facts.Refinements() {
		if next, _, err := applyValueRefinementFactorState(
			domain, nil, resolver, projectPath, point, out, refinement.TargetPathRef(), refinement.Value(), false,
		); err == nil {
			out = next
		}
	}
	for _, relation := range facts.PathRelations() {
		out = applyPostconditionPathRelation(ctx, resolver, projectPath, out, relation)
	}
	return out
}

func applyPostconditionPathRelation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	fact factflow.PostconditionPathRelation,
) state.State {
	switch fact.Kind() {
	case factflow.PostconditionPathRelationEqual:
		next, _, err := applyPathEqualityFactorState(
			state.RegisteredProductDomain(ctx.Registry), nil, resolver, ctx.Point, out, fact.LeftPath(), fact.RightPath(),
		)
		if err == nil {
			return next
		}
		return out
	default:
		return out
	}
}
