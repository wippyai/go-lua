package state

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
)

func projectEscapeEventsBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.escapeEvents, _ = projectEscapeEventsBoundaryFactor(ctx, source.escapeEvents)
	return true
}
func projectEscapeEventsBoundaryFactor(ctx *boundaryProjectContext, source escapeevent.Lane) (escapeevent.Lane, bool) {
	if source.Snapshot().Bottom {
		return source, true
	}
	lane := escapeevent.Top()
	for _, fact := range source.Snapshot().Facts {
		if boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Target) {
			lane, _ = lane.Add(fact)
		}
	}
	return lane, true
}
func rebaseEscapeEventsBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.escapeEvents, ok = rebaseEscapeEventsBoundaryFactor(ctx, source.escapeEvents)
	return ok
}
func rebaseEscapeEventsBoundaryFactor(ctx *boundaryRebaseContext, source escapeevent.Lane) (escapeevent.Lane, bool) {
	snapshot := source.Snapshot()
	if snapshot.Bottom {
		return source, true
	}
	facts := make(map[escapeevent.Fact]struct{}, len(snapshot.Facts))
	for _, fact := range snapshot.Facts {
		facts[fact] = struct{}{}
	}
	values, ok := rebaseBoundaryMustSet(facts, func(fact escapeevent.Fact) ([]escapeevent.Fact, bool) {
		targets, ok := rebaseBoundaryStateKeys(ctx, fact.Target)
		if !ok {
			return nil, false
		}
		next := make([]escapeevent.Fact, len(targets))
		for i, target := range targets {
			next[i] = fact
			next[i].Target = target
		}
		return next, true
	}, func(fact escapeevent.Fact) pathaddr.StateKey {
		return fact.Target
	}, func(fact escapeevent.Fact) ([]pathaddr.StateKey, bool) {
		return ctx.quotient.stateKeyPreimages(fact.Target)
	})
	if !ok {
		return escapeevent.Lane{}, false
	}
	lane := escapeevent.Top()
	for fact := range values {
		lane, _ = lane.Add(fact)
	}
	return lane, true
}
func applyEscapeEventsBoundaryLane(ctx *boundaryApplyContext, destination, fragment escapeevent.Lane) (escapeevent.Lane, bool) {
	if destination.Snapshot().Bottom || fragment.Snapshot().Bottom {
		return escapeevent.Bottom(), true
	}
	lane := escapeevent.Top()
	for _, fact := range destination.Snapshot().Facts {
		if !boundaryContainsStateKey(ctx.keys, ctx.closure, fact.Target) {
			lane, _ = lane.Add(fact)
		}
	}
	for _, fact := range fragment.Snapshot().Facts {
		lane, _ = lane.Add(fact)
	}
	return lane, true
}
func equalEscapeEventsBoundary(_ *axis.Registry, a, b State) bool {
	return escapeevent.Domain().Equal(a.escapeEvents, b.escapeEvents)
}
