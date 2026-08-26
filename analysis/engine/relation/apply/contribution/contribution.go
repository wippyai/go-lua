// Package contribution derives the schema-declared contribution subset of an
// authenticated Apply result. It is the sole semantic-to-state classifier,
// consumed at the publication door.
package contribution

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
)

// TransitionsForApplication transports the exact After side of every
// schema-declared contribution output in one authenticated Apply result.
//
// Ordinary outputs are deliberately left in the ordinary proposal path. A
// contribution output is recognized only by its sealed mounted output-cell
// descriptor; neither evaluator nor publication classify columns by name,
// shape, or a local enumeration.
//
// A removal on a declared contribution port is refused here: a contribution
// removal needs the exact signed Before side and travels through the signed
// invocation path, not through an invented sparse value here. A removal on an
// undeclared port is ordinary state transport and is deliberately left for
// the ordinary proposal path.
func TransitionsForApplication(mounted witness.Mounted, application apply.Application) ([]invocation.ContributionTransition, bool) {
	if !mounted.Available() || !application.Available() || !application.Fence().Same(mounted.RuntimeFence()) || !application.Invocation().ValidFor(mounted.RuntimeFence()) {
		return nil, false
	}
	result := make([]invocation.ContributionTransition, 0)
	proposals, hasProposals := application.Proposals()
	if !hasProposals {
		return result, true
	}
	if !proposals.Available() {
		return nil, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() {
		return nil, false
	}
	for index := 0; index < proposals.Len(); index++ {
		proposal, ok := proposals.At(index)
		if !ok || !proposal.Available() {
			return nil, false
		}
		cell := proposal.Destination()
		if !cell.Available() || !cell.ValidFor(mounted.RuntimeFence()) {
			return nil, false
		}
		descriptor, declared := plan.ContributionCell(application.Operation(), cell)
		if !declared {
			continue
		}
		if proposal.Removal() {
			return nil, false
		}
		spec := descriptor.Spec()
		if !descriptor.Available() || !descriptor.ValidFor(plan.Fence()) || descriptor.Operation() != application.Operation() || descriptor.Column() != cell.Column() || !spec.Available() || spec.Column() != cell.Column() {
			return nil, false
		}
		side, ok := binding.NewContributionSide(proposal.Value(), proposal.Presence(), application.Lineage())
		if !ok {
			return nil, false
		}
		transition, ok := invocation.NewContributionTransition(spec, application.Invocation(), cell, application.Fence(), binding.NoContributionSide(), side)
		if !ok || !transition.ValidFor(mounted.RuntimeFence()) {
			return nil, false
		}
		result = append(result, transition)
	}
	return result, true
}
