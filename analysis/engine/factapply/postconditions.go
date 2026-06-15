package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

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
