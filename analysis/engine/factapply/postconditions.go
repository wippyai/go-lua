package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func applyPostconditionRefinement(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.PostconditionRefinement,
) state.State {
	return applyValueRefinementAt(ctx.Registry, resolver, ctx.Point, out, fact.TargetPath(), fact.Value())
}
