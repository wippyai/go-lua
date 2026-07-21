package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

func projectChannelSelectBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.channelSelect, _ = projectChannelSelectBoundaryFactor(ctx, source.channelSelect)
	return true
}
func projectChannelSelectBoundaryFactor(ctx *boundaryProjectContext, source channelselectfact.Lane) (channelselectfact.Lane, bool) {
	if source.Snapshot().Bottom {
		return source, true
	}
	lane := channelselectfact.Top()
	for _, fact := range source.Snapshot().Facts {
		if boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Result) || boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Case) {
			if fact.HasPayload {
				fact.Payload = product.ProjectBoundary(ctx.reg, fact.Payload)
			}
			lane = lane.Add(fact)
		}
	}
	return lane, true
}
func rebaseChannelSelectBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.channelSelect, ok = rebaseChannelSelectBoundaryFactor(ctx, source.channelSelect)
	return ok
}
func rebaseChannelSelectBoundaryFactor(ctx *boundaryRebaseContext, source channelselectfact.Lane) (channelselectfact.Lane, bool) {
	snapshot := source.Snapshot()
	if snapshot.Bottom {
		return source, true
	}
	facts := make(map[channelselectfact.Fact]struct{}, len(snapshot.Facts))
	for _, fact := range snapshot.Facts {
		facts[fact] = struct{}{}
	}
	values, ok := rebaseBoundaryMustSet(facts, func(fact channelselectfact.Fact) ([]channelselectfact.Fact, bool) {
		results, ok := rebaseBoundaryStateKeys(ctx, fact.Result)
		if !ok {
			return nil, false
		}
		cases, ok := rebaseBoundaryStateKeys(ctx, fact.Case)
		if !ok {
			return nil, false
		}
		if fact.HasPayload {
			fact.Payload, ok = rebaseBoundaryProduct(ctx, fact.Payload)
			if !ok {
				return nil, false
			}
		}
		out := make([]channelselectfact.Fact, 0, len(results)*len(cases))
		for _, result := range results {
			for _, caseKey := range cases {
				next := fact
				next.Result, next.Case = result, caseKey
				out = append(out, next)
			}
		}
		return out, true
	}, func(fact channelselectfact.Fact) boundaryPair[pathaddr.StateKey, pathaddr.StateKey] {
		return boundaryPair[pathaddr.StateKey, pathaddr.StateKey]{first: fact.Result, second: fact.Case}
	}, func(fact channelselectfact.Fact) ([]boundaryPair[pathaddr.StateKey, pathaddr.StateKey], bool) {
		results, valid := ctx.quotient.optionalStateKeyPreimages(fact.Result)
		if !valid {
			return nil, false
		}
		cases, valid := ctx.quotient.optionalStateKeyPreimages(fact.Case)
		if !valid {
			return nil, false
		}
		return boundaryPairs(results, cases), true
	})
	if !ok {
		return channelselectfact.Lane{}, false
	}
	lane := channelselectfact.Top()
	for fact := range values {
		lane = lane.Add(fact)
	}
	return lane, true
}
func applyChannelSelectBoundaryLane(ctx *boundaryApplyContext, destination, fragment channelselectfact.Lane) (channelselectfact.Lane, bool) {
	if destination.Snapshot().Bottom || fragment.Snapshot().Bottom {
		return channelselectfact.Bottom(), true
	}
	lane := channelselectfact.Top()
	for _, fact := range destination.Snapshot().Facts {
		if !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Result) && !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Case) {
			lane = lane.Add(fact)
		}
	}
	for _, fact := range fragment.Snapshot().Facts {
		lane = lane.Add(fact)
	}
	return lane, true
}
func equalChannelSelectBoundary(_ *axis.Registry, a, b State) bool {
	return channelselectfact.Domain().Equal(a.channelSelect, b.channelSelect)
}
