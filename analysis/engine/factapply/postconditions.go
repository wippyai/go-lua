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
	for _, refinement := range facts.Refinements() {
		out = applyValueRefinementAt(reg, resolver, projectPath, point, out, refinement.TargetPathRef(), refinement.Value())
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
		return applyPathEqualityAt(ctx.Registry, resolver, projectPath, ctx.Point, out, fact.LeftPath(), fact.RightPath())
	default:
		return out
	}
}
