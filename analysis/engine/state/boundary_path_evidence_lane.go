package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func projectPathEvidenceBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.pathEvidence = source.pathEvidence.ProjectBoundary(ctx.closure.ContainsPath, func(value product.Value) product.Value {
		return product.ProjectBoundary(ctx.reg, value)
	})
	return true
}
func rebasePathEvidenceBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, valid := source.pathEvidence.RebaseBoundary(func(path keyspace.Key) ([]keyspace.Key, bool) {
		return boundaryRebasePaths(ctx, path)
	}, func(value product.Value) (product.Value, bool) {
		return rebaseBoundaryProduct(ctx, value)
	}, func(a, b product.Value) product.Value { return product.Join(ctx.reg, a, b) })
	out.pathEvidence = lane
	return valid
}
func applyPathEvidenceBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	out.pathEvidence = destination.pathEvidence.ApplyBoundary(fragment.pathEvidence, ctx.closure.ContainsPath)
	return true
}
func equalPathEvidenceBoundary(reg *axis.Registry, a, b State) bool {
	return pathevidence.Domain(reg).Equal(a.pathEvidence, b.pathEvidence)
}
