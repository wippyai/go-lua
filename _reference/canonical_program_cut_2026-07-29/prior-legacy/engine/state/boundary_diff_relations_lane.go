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

func rebaseRelConstraint(ctx *boundaryRebaseContext, value RelConstraint) ([]RelConstraint, bool) {
	a, ok := rebaseRelOperands(ctx, value.A)
	if !ok {
		return nil, false
	}
	b, ok := rebaseRelOperands(ctx, value.B)
	if !ok {
		return nil, false
	}
	c, ok := rebaseRelOperands(ctx, value.C)
	if !ok {
		return nil, false
	}
	out := make([]RelConstraint, 0, len(a)*len(b)*len(c))
	for _, av := range a {
		for _, bv := range b {
			for _, cv := range c {
				next := value
				next.A, next.B, next.C = av, bv, cv
				if next, valid := canonicalRelConstraint(next); valid {
					out = append(out, next)
				}
			}
		}
	}
	return out, true
}
func projectDiffRelationsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	lane, _ := projectDiffRelationsBoundaryFactor(ctx, source.diffRelations)
	setStateDiffRelations(out, lane)
	return true
}
func projectDiffRelationsBoundaryFactor(ctx *boundaryProjectContext, source diffRelationLane) (diffRelationLane, bool) {
	if source.bottom {
		return source, true
	}
	values := projectFiniteSet(source.values, func(value RelConstraint) bool { return relationTouches(ctx.keys, ctx.closure, value) })
	return diffRelationLane{mustSetLane[RelConstraint]{values: values}}, true
}
func rebaseDiffRelationsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	lane, ok := rebaseDiffRelationsBoundaryFactor(ctx, source.diffRelations)
	if ok {
		setStateDiffRelations(out, lane)
	}
	return ok
}
func rebaseDiffRelationsBoundaryFactor(ctx *boundaryRebaseContext, source diffRelationLane) (diffRelationLane, bool) {
	if source.bottom {
		return source, true
	}
	values, ok := rebaseBoundaryMustSet(source.values, func(value RelConstraint) ([]RelConstraint, bool) {
		return rebaseRelConstraint(ctx, value)
	}, func(value RelConstraint) boundaryTriple[RelOperand, RelOperand, RelOperand] {
		return boundaryTriple[RelOperand, RelOperand, RelOperand]{first: value.A, second: value.B, third: value.C}
	}, func(value RelConstraint) ([]boundaryTriple[RelOperand, RelOperand, RelOperand], bool) {
		preimages := func(operand RelOperand) ([]RelOperand, bool) {
			if !operand.valid() {
				return []RelOperand{operand}, true
			}
			keys, valid := ctx.quotient.stateKeyPreimages(operand.Key)
			if !valid {
				return nil, false
			}
			out := make([]RelOperand, len(keys))
			for i, key := range keys {
				out[i] = operand
				out[i].Key = key
			}
			return out, true
		}
		a, valid := preimages(value.A)
		if !valid {
			return nil, false
		}
		b, valid := preimages(value.B)
		if !valid {
			return nil, false
		}
		c, valid := preimages(value.C)
		if !valid {
			return nil, false
		}
		fibers := make(map[boundaryTriple[RelOperand, RelOperand, RelOperand]]struct{})
		for _, operands := range boundaryTriples(a, b, c) {
			sourceCandidate := value
			sourceCandidate.A, sourceCandidate.B, sourceCandidate.C = operands.first, operands.second, operands.third
			sourceCandidate, valid = canonicalRelConstraint(sourceCandidate)
			if !valid {
				continue
			}
			mapped, valid := rebaseRelConstraint(ctx, sourceCandidate)
			if !valid {
				return nil, false
			}
			for _, candidate := range mapped {
				if candidate == value {
					fibers[boundaryTriple[RelOperand, RelOperand, RelOperand]{first: sourceCandidate.A, second: sourceCandidate.B, third: sourceCandidate.C}] = struct{}{}
					break
				}
			}
		}
		out := make([]boundaryTriple[RelOperand, RelOperand, RelOperand], 0, len(fibers))
		for fiber := range fibers {
			out = append(out, fiber)
		}
		return out, true
	})
	if !ok {
		return diffRelationLane{}, false
	}
	lane := diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{}}
	for value := range values {
		lane, _ = lane.add(value)
	}
	return lane, true
}
func applyDiffRelationsBoundaryLane(ctx *boundaryApplyContext, destination, fragment diffRelationLane) (diffRelationLane, bool) {
	if destination.bottom || fragment.bottom {
		return diffRelationLane{mustSetLane: mustSetLane[RelConstraint]{bottom: true}}, true
	}
	values := applyFiniteSet(destination.values, fragment.values, func(value RelConstraint) bool { return relationTouches(ctx.keys, ctx.closure, value) })
	return diffRelationLane{mustSetLane[RelConstraint]{values: values}}, true
}
func equalDiffRelationsBoundary(_ *axis.Registry, a, b State) bool {
	return diffRelationDomain().Equal(a.diffRelations, b.diffRelations)
}
