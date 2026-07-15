package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
)

func projectChannelSelectBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.channelSelect.Snapshot().Bottom {
		out.channelSelect = source.channelSelect
		return true
	}
	lane := channelselectfact.Top()
	for _, fact := range source.channelSelect.Snapshot().Facts {
		if boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Result) || boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Case) {
			if fact.HasPayload {
				fact.Payload = product.ProjectBoundary(ctx.reg, fact.Payload)
			}
			lane = lane.Add(fact)
		}
	}
	out.channelSelect = lane
	return true
}
func rebaseChannelSelectBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.channelSelect.Snapshot().Bottom {
		out.channelSelect = source.channelSelect
		return true
	}
	lane := channelselectfact.Top()
	for _, fact := range source.channelSelect.Snapshot().Facts {
		results, ok := rebaseBoundaryStateKeys(ctx, fact.Result)
		if !ok {
			return false
		}
		cases, ok := rebaseBoundaryStateKeys(ctx, fact.Case)
		if !ok {
			return false
		}
		if fact.HasPayload {
			fact.Payload, ok = rebaseBoundaryProduct(ctx, fact.Payload)
			if !ok {
				return false
			}
		}
		for _, result := range results {
			for _, caseKey := range cases {
				next := fact
				next.Result, next.Case = result, caseKey
				lane = lane.Add(next)
			}
		}
	}
	out.channelSelect = lane
	return true
}
func applyChannelSelectBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.channelSelect.Snapshot().Bottom || fragment.channelSelect.Snapshot().Bottom {
		out.channelSelect = channelselectfact.Bottom()
		return true
	}
	lane := channelselectfact.Top()
	for _, fact := range destination.channelSelect.Snapshot().Facts {
		if !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Result) && !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Case) {
			lane = lane.Add(fact)
		}
	}
	for _, fact := range fragment.channelSelect.Snapshot().Facts {
		lane = lane.Add(fact)
	}
	out.channelSelect = lane
	return true
}
func equalChannelSelectBoundary(_ *axis.Registry, a, b State) bool {
	return channelselectfact.Domain().Equal(a.channelSelect, b.channelSelect)
}
