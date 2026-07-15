package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func relationTouches(keys *keyspace.KeySpace, closure BoundaryClosure, value RelConstraint) bool {
	return boundaryContainsStateKey(keys, closure, value.A.Key) || value.B.valid() && boundaryContainsStateKey(keys, closure, value.B.Key) || boundaryContainsStateKey(keys, closure, value.C.Key)
}
func rebaseRelOperands(ctx *boundaryRebaseContext, value RelOperand) ([]RelOperand, bool) {
	if !value.valid() {
		return []RelOperand{value}, true
	}
	next, ok := rebaseBoundaryStateKeys(ctx, value.Key)
	if !ok {
		return nil, false
	}
	out := make([]RelOperand, len(next))
	for i, key := range next {
		out[i] = value
		out[i].Key = key
	}
	return out, true
}
func projectDiffRelationsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.diffRelations.bottom {
		out.diffRelations = source.diffRelations
		return true
	}
	values := projectFiniteSet(source.diffRelations.values, func(value RelConstraint) bool { return relationTouches(ctx.keys, ctx.closure, value) })
	out.diffRelations = diffRelationLane{mustSetLane[RelConstraint]{values: values}}
	return true
}
func rebaseDiffRelationsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.diffRelations.bottom {
		out.diffRelations = source.diffRelations
		return true
	}
	lane := diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{}}
	for value := range source.diffRelations.values {
		a, ok := rebaseRelOperands(ctx, value.A)
		if !ok {
			return false
		}
		b, ok := rebaseRelOperands(ctx, value.B)
		if !ok {
			return false
		}
		c, ok := rebaseRelOperands(ctx, value.C)
		if !ok {
			return false
		}
		for _, av := range a {
			for _, bv := range b {
				for _, cv := range c {
					next := value
					next.A, next.B, next.C = av, bv, cv
					lane, _ = lane.add(next)
				}
			}
		}
	}
	out.diffRelations = lane
	return true
}
func applyDiffRelationsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.diffRelations.bottom || fragment.diffRelations.bottom {
		out.diffRelations = diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{bottom: true}}
		return true
	}
	values := applyFiniteSet(destination.diffRelations.values, fragment.diffRelations.values, func(value RelConstraint) bool { return relationTouches(ctx.keys, ctx.closure, value) })
	out.diffRelations = diffRelationLane{mustSetLane[RelConstraint]{values: values}}
	return true
}
func equalDiffRelationsBoundary(_ *axis.Registry, a, b State) bool {
	return diffRelationDomain().Equal(a.diffRelations, b.diffRelations)
}
