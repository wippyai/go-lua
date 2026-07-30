package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func projectFrozenBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.frozenTables, _ = projectFrozenBoundaryFactor(ctx, source.frozenTables)
	return true
}
func projectFrozenBoundaryFactor(ctx *boundaryProjectContext, source frozenTableLane) (frozenTableLane, bool) {
	if source.bottom {
		return source, true
	}
	return frozenTableLane{mustSetLane[identity.Term]{values: projectFiniteSet(source.values, ctx.closure.ContainsIdentityTerm)}}, true
}
func rebaseFrozenBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.frozenTables, ok = rebaseFrozenBoundaryFactor(ctx, source.frozenTables)
	return ok
}
func rebaseFrozenBoundaryFactor(ctx *boundaryRebaseContext, source frozenTableLane) (frozenTableLane, bool) {
	if source.bottom {
		return source, true
	}
	values, ok := rebaseBoundaryMustSet(source.values, func(term identity.Term) ([]identity.Term, bool) {
		image, ok := identityImage(ctx, term)
		if !ok {
			return nil, false
		}
		if image.IsBottom() {
			ctx.relationBottom = true
			return nil, true
		}
		if image.IsTop() {
			return nil, true
		}
		next, exact := image.Term()
		return []identity.Term{next}, exact
	}, func(term identity.Term) identity.Term { return term }, func(term identity.Term) ([]identity.Term, bool) {
		return ctx.quotient.identityPreimages(term)
	})
	if !ok {
		return frozenTableLane{}, false
	}
	return frozenTableLane{mustSetLane[identity.Term]{values: values}}, true
}
func applyFrozenBoundaryLane(ctx *boundaryApplyContext, destination, fragment frozenTableLane) (frozenTableLane, bool) {
	if destination.bottom || fragment.bottom {
		return frozenTableLane{mustSetLane: mustSetLane[identity.Term]{bottom: true}}, true
	}
	values := applyFiniteSet(destination.values, fragment.values, ctx.closure.ContainsIdentityTerm)
	return frozenTableLane{mustSetLane[identity.Term]{values: values}}, true
}
func equalFrozenBoundary(_ *axis.Registry, a, b State) bool {
	return frozenTableDomain().Equal(a.frozenTables, b.frozenTables)
}
