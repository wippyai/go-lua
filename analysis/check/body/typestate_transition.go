package body

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// TypestateInvalidTransitionProof is a solved, proven violation of a declared
// lifecycle transition precondition. It is intentionally protocol-generic:
// database connections, transactions, files, locks, and ambient resources all
// use the same proof once their lifecycle facts reach the typestate store.
type TypestateInvalidTransitionProof struct {
	Point    cfg.Point
	Resource string
	Protocol string
	Expected string
	Found    string
	Target   string
	Span     SourceSpan
}

// TypestateInvalidTransitionProofs returns every invalid transition recorded
// by normal-return fact application. The state store retains these may-facts
// across joins, while the site marker lets this projection report each fact at
// the exact call that proved its precondition false.
func (r *Result) TypestateInvalidTransitionProofs() []TypestateInvalidTransitionProof {
	if r == nil || r.Graph() == nil {
		return nil
	}
	var out []TypestateInvalidTransitionProof
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		stateAtBoundary, ok := r.StateAtBoundary(point)
		if !ok {
			continue
		}
		for _, invalid := range stateAtBoundary.TypestateInvalidTransitions() {
			if invalid.Site != uint32(point) {
				continue
			}
			out = append(out, r.typestateInvalidTransitionProof(point, invalid))
		}
	}
	return out
}

func (r *Result) typestateInvalidTransitionProof(point cfg.Point, invalid typestate.InvalidTransition) TypestateInvalidTransitionProof {
	return TypestateInvalidTransitionProof{
		Point:    point,
		Resource: invalid.Resource.ID.String(),
		Protocol: string(invalid.Resource.Protocol),
		Expected: string(invalid.Expected),
		Found:    string(invalid.Found),
		Target:   r.typestateInvalidTransitionTarget(point, invalid),
		Span:     r.callSpanAt(point),
	}
}

func (r *Result) typestateInvalidTransitionTarget(point cfg.Point, invalid typestate.InvalidTransition) string {
	outcome, ok := r.CallOutcomeAt(point)
	if !ok {
		return invalid.Resource.ID.String()
	}
	bindings := r.callGuardCallBindingsAt(point)
	for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
		if fact.Kind != callboundary.LifecycleTransition || fact.Protocol != invalid.Resource.Protocol || fact.From != invalid.Expected {
			continue
		}
		target, ok := fact.Target.Substitute(bindings)
		if !ok || target.IsEmpty() {
			continue
		}
		resource, ok := r.TypestateResourceAtCallEntry(point, target, fact.Protocol)
		if ok && resource == invalid.Resource {
			return r.DisplayPath(target)
		}
	}
	return invalid.Resource.ID.String()
}
