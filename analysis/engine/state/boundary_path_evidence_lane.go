package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func projectPathEvidenceBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.pathEvidence, _ = projectPathEvidenceBoundaryFactor(ctx, source.pathEvidence)
	return true
}
func projectPathEvidenceBoundaryFactor(ctx *boundaryProjectContext, source pathevidence.Lane) (pathevidence.Lane, bool) {
	return source.ProjectBoundary(ctx.closure.ContainsPath, func(value product.Value) product.Value {
		return product.ProjectBoundary(ctx.reg, value)
	}), true
}
func rebasePathEvidenceBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var valid bool
	out.pathEvidence, valid = rebasePathEvidenceBoundaryFactor(ctx, source.pathEvidence)
	return valid
}
func rebasePathEvidenceBoundaryFactor(ctx *boundaryRebaseContext, source pathevidence.Lane) (pathevidence.Lane, bool) {
	return source.RebaseBoundary(func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	}, func(path keyspace.Key) ([]keyspace.Key, bool) {
		return ctx.quotient.pathPreimages(path)
	}, func(value product.Value) (product.Value, bool) {
		return rebaseBoundaryProduct(ctx, value)
	}, func(a, b product.Value) product.Value { return product.Join(ctx.reg, a, b) })
}
func applyPathEvidenceBoundaryLane(ctx *boundaryApplyContext, destination, fragment pathevidence.Lane) (pathevidence.Lane, bool) {
	return destination.ApplyBoundary(fragment, ctx.closure.ContainsPath), true
}
func applyPathEvidenceBoundaryRoots(ctx *boundaryApplyContext, lane pathevidence.Lane, roots boundaryRootPlan) (pathevidence.Lane, bool) {
	for _, root := range roots.paths {
		lane, _ = lane.WritePathKey(ctx.reg, root.Path, root.Value)
	}
	if roots.establishesReachability {
		lane = lane.Reachable()
	}
	return lane, true
}
func postRebasePathEvidenceBoundary(_ *boundaryRebaseContext, aliases [][2]keyspace.Key, out *State) bool {
	var ok bool
	out.pathEvidence, ok = postRebasePathEvidenceBoundaryFactor(nil, aliases, out.pathEvidence)
	return ok
}
func postRebasePathEvidenceBoundaryFactor(_ *boundaryRebaseContext, aliases [][2]keyspace.Key, lane pathevidence.Lane) (pathevidence.Lane, bool) {
	lane, _ = lane.AddBranchProofs(pathevidence.BoundaryAliasPathEqualProofs(aliases))
	return lane, true
}
func equalPathEvidenceBoundary(reg *axis.Registry, a, b State) bool {
	return pathevidence.Domain(reg).Equal(a.pathEvidence, b.pathEvidence)
}
