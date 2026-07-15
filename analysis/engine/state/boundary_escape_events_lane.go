package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
)

func projectEscapeEventsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.escapeEvents.Snapshot().Bottom {
		out.escapeEvents = source.escapeEvents
		return true
	}
	lane := escapeevent.Top()
	for _, fact := range source.escapeEvents.Snapshot().Facts {
		if boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Target) {
			lane, _ = lane.Add(fact)
		}
	}
	out.escapeEvents = lane
	return true
}
func rebaseEscapeEventsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.escapeEvents.Snapshot().Bottom {
		out.escapeEvents = source.escapeEvents
		return true
	}
	lane := escapeevent.Top()
	for _, fact := range source.escapeEvents.Snapshot().Facts {
		targets, ok := rebaseBoundaryStateKeys(ctx, fact.Target)
		if !ok {
			return false
		}
		for _, target := range targets {
			next := fact
			next.Target = target
			lane, _ = lane.Add(next)
		}
	}
	out.escapeEvents = lane
	return true
}
func applyEscapeEventsBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.escapeEvents.Snapshot().Bottom || fragment.escapeEvents.Snapshot().Bottom {
		out.escapeEvents = escapeevent.Bottom()
		return true
	}
	lane := escapeevent.Top()
	for _, fact := range destination.escapeEvents.Snapshot().Facts {
		if !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Target) {
			lane, _ = lane.Add(fact)
		}
	}
	for _, fact := range fragment.escapeEvents.Snapshot().Facts {
		lane, _ = lane.Add(fact)
	}
	out.escapeEvents = lane
	return true
}
func equalEscapeEventsBoundary(_ *axis.Registry, a, b State) bool {
	return escapeevent.Domain().Equal(a.escapeEvents, b.escapeEvents)
}
