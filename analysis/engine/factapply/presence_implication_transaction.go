package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ConcretePresenceImplicationCancellation selects the state published when a
// presence-implication barrier observes cancellation. Node transfers are one
// transaction and roll back to their immutable input. Edge transfers are
// already evolving a predecessor state and retain the last completed closure
// round.
type ConcretePresenceImplicationCancellation uint8

const (
	ConcretePresenceImplicationKeepEvolving ConcretePresenceImplicationCancellation = iota
	ConcretePresenceImplicationRollbackNode
)

// ConcretePresenceImplicationBarriers describes where a publication operation
// closes its facts. The default matches bulk boundary-fact publication. Direct
// path facts use the invalidation-aware form because making a root absent must
// not overtake consequences pending on one of its descendants.
type ConcretePresenceImplicationBarriers uint8

const (
	ConcretePresenceImplicationTrailingBarrier ConcretePresenceImplicationBarriers = iota
	ConcretePresenceImplicationDescendantInvalidationBarriers
)

// ConcretePresenceImplicationRequest describes one atomic publication phase.
// Publications are concrete implication facts, not individual consequence
// firings: Apply closes the whole set to a fixed point at the same barriers as
// the legacy applicator.
//
// Publications are batched behind one trailing barrier by default. With
// ConcretePresenceImplicationDescendantInvalidationBarriers, an implication
// that can make its target absent first closes all pending facts, then publishes
// and closes itself before the next fact. This preserves the invalidation
// ordering relied on by direct path evidence.
type ConcretePresenceImplicationRequest struct {
	Registry     *axis.Registry
	Resolver     *visibility.Resolver
	Point        cfg.Point
	Input        state.State
	Output       state.State
	Publications []pathevidence.PathPresenceImplication
	Token        *cancellation.Token
	Cancellation ConcretePresenceImplicationCancellation
	Barriers     ConcretePresenceImplicationBarriers
}

// ConcretePresenceImplicationResult is the state at the completed barrier, or
// the request's rollback state when Canceled is true.
type ConcretePresenceImplicationResult struct {
	Output   state.State
	Canceled bool
}

// ConcretePresenceImplicationExecutor publishes implication facts and closes
// their consequences. Its zero value is ready to use and carries no mutable
// semantic state, so a future operation-plan interpreter can share the same
// concrete kernel as the current applicator.
type ConcretePresenceImplicationExecutor struct{}

// Apply publishes the request and executes every required closure barrier.
func (*ConcretePresenceImplicationExecutor) Apply(req ConcretePresenceImplicationRequest) ConcretePresenceImplicationResult {
	out := req.Output
	pending := false
	poll := cancellation.NewPoller(req.Token, cancellation.EveryCheap)

	barrier := func() bool {
		if !pending {
			return false
		}
		var canceled bool
		out, canceled = closeConcretePresenceImplications(req.Registry, req.Resolver, req.Point, out, req.Token)
		pending = false
		return canceled
	}
	canceledResult := func() ConcretePresenceImplicationResult {
		if req.Cancellation == ConcretePresenceImplicationRollbackNode {
			return ConcretePresenceImplicationResult{Output: req.Input, Canceled: true}
		}
		return ConcretePresenceImplicationResult{Output: out, Canceled: true}
	}

	for _, implication := range req.Publications {
		if poll.Poll() {
			return canceledResult()
		}
		if req.Barriers == ConcretePresenceImplicationDescendantInvalidationBarriers &&
			presenceImplicationTargetInvalidatesDescendants(implication) {
			if barrier() {
				return canceledResult()
			}
			out = out.AddPathPresenceImplication(implication)
			pending = true
			if barrier() {
				return canceledResult()
			}
			continue
		}
		out = out.AddPathPresenceImplication(implication)
		pending = true
	}
	if len(req.Publications) == 0 {
		pending = true
	}
	if barrier() {
		return canceledResult()
	}
	return ConcretePresenceImplicationResult{Output: out}
}

// ApplyConcretePresenceImplications is the stateless convenience form.
func ApplyConcretePresenceImplications(req ConcretePresenceImplicationRequest) ConcretePresenceImplicationResult {
	return new(ConcretePresenceImplicationExecutor).Apply(req)
}

// closeConcretePresenceImplications closes already-published implications to
// a local fixed point. On cancellation it returns the last fully completed
// round, never the partially accumulated next round.
func closeConcretePresenceImplications(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	token *cancellation.Token,
) (state.State, bool) {
	stateDomain := state.Domain(reg)
	poll := cancellation.NewPoller(token, cancellation.EveryExpensive)
	for {
		if poll.Poll() {
			return out, true
		}
		next := out
		snapshot := next.PathPresenceImplicationsSnapshot(resolver.KeySpace())
		if snapshot.Bottom || len(snapshot.Implications) == 0 {
			return next, false
		}
		for _, implication := range snapshot.Implications {
			if poll.Poll() {
				return out, true
			}
			if !pathPresenceImplicationTriggered(reg, resolver, point, next, implication) {
				continue
			}
			next = applyPathPresenceImplicationTarget(reg, resolver, point, next, implication)
		}
		if stateDomain.Equal(next, out) {
			return out, false
		}
		out = next
	}
}
