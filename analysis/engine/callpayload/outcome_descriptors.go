package callpayload

import (
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// CallOutcomeLaneOps is the CallOutcome family's per-kind behavior payload. A
// call-outcome lane has a much simpler shape than the NormalReturnFacts family:
// it exposes a field-presence predicate and a post-return classification flag,
// with no append/merge or lattice ops of its own (CallOutcome merge laws reside in
// the supplemental-fact binding). The descriptor spine still applies: a kind is
// the field name, WireRef links the field to its manifest wire lane, and Ops
// carries the presence predicate and post-return flag.
type CallOutcomeLaneOps struct {
	postReturn bool
	has        func(CallOutcome) bool
}

// PostReturn reports whether the lane carries caller-visible post-return
// evidence.
func (o CallOutcomeLaneOps) PostReturn() bool { return o.postReturn }

func callOutcomeLaneDescriptor(fieldName string, wireRef []string, postReturn bool, has func(CallOutcome) bool) callboundary.BoundaryFactDescriptor[CallOutcomeLaneOps] {
	return callboundary.BoundaryFactDescriptor[CallOutcomeLaneOps]{
		Kind:    callboundary.BoundaryFactKind(fieldName),
		WireRef: wireRef,
		Ops: CallOutcomeLaneOps{
			postReturn: postReturn,
			has:        has,
		},
	}
}

// deriveCallOutcomeLane rebuilds the storage lane a descriptor describes.
func deriveCallOutcomeLane(d callboundary.BoundaryFactDescriptor[CallOutcomeLaneOps]) callOutcomeLane {
	return callOutcomeLane{
		fieldName:  string(d.Kind),
		postReturn: d.Ops.postReturn,
		has:        d.Ops.has,
	}
}

// callOutcomeDescriptors registers CallOutcome fields in canonical struct
// order. WireRef identifies manifest OperationalEffects lanes. MaySuspend and
// ReturnPresenceRelations map directly to wire lanes; NormalReturnFacts owns
// nested refs, and the remaining lanes stay caller-relative or local.
var callOutcomeDescriptors = func() callboundary.BoundaryFactTable[CallOutcomeLaneOps] {
	t := callboundary.BoundaryFactTable[CallOutcomeLaneOps]{
		callOutcomeLaneDescriptor("Results", nil, false,
			func(o CallOutcome) bool { return len(o.Results) != 0 }),
		callOutcomeLaneDescriptor("PostReturnAuthority", nil, false,
			func(o CallOutcome) bool { return o.PostReturnAuthority }),
		callOutcomeLaneDescriptor("SuspensionKnown", nil, false,
			func(o CallOutcome) bool { return o.SuspensionKnown }),
		callOutcomeLaneDescriptor("MaySuspend", []string{"MaySuspend"}, false,
			func(o CallOutcome) bool { return o.MaySuspend }),
		callOutcomeLaneDescriptor("NormalReturnFacts", nil, true,
			func(o CallOutcome) bool { return !o.NormalReturnFacts.Empty() }),
		callOutcomeLaneDescriptor("HeapTableObjects", nil, true,
			func(o CallOutcome) bool { return len(o.HeapTableObjects) != 0 }),
		callOutcomeLaneDescriptor("Placements", nil, true,
			func(o CallOutcome) bool { return len(o.Placements) != 0 }),
		callOutcomeLaneDescriptor("ParamObligations", nil, false,
			func(o CallOutcome) bool { return len(o.ParamObligations) != 0 }),
		callOutcomeLaneDescriptor("PathObligations", nil, false,
			func(o CallOutcome) bool { return len(o.PathObligations) != 0 }),
		callOutcomeLaneDescriptor("ParamPathRefinements", nil, true,
			func(o CallOutcome) bool { return len(o.ParamPathRefinements) != 0 }),
		callOutcomeLaneDescriptor("ParamPathWrites", nil, true,
			func(o CallOutcome) bool { return len(o.ParamPathWrites) != 0 }),
		callOutcomeLaneDescriptor("ParamLengthFloors", nil, true,
			func(o CallOutcome) bool { return len(o.ParamLengthFloors) != 0 }),
		callOutcomeLaneDescriptor("ParamPathInvalidations", nil, true,
			func(o CallOutcome) bool { return len(o.ParamPathInvalidations) != 0 }),
		callOutcomeLaneDescriptor("ParamConditions", nil, true,
			func(o CallOutcome) bool { return len(o.ParamConditions) != 0 }),
		callOutcomeLaneDescriptor("ParamPathRelations", nil, true,
			func(o CallOutcome) bool { return len(o.ParamPathRelations) != 0 }),
		callOutcomeLaneDescriptor("ReturnConditionRefinements", nil, true,
			func(o CallOutcome) bool { return len(o.ReturnConditionRefinements) != 0 }),
		callOutcomeLaneDescriptor("ReturnConditionSlots", nil, true,
			func(o CallOutcome) bool { return len(o.ReturnConditionSlots) != 0 }),
		callOutcomeLaneDescriptor("ReturnPresenceRelations", []string{"ReturnPresenceRelations"}, true,
			func(o CallOutcome) bool { return len(o.ReturnPresenceRelations) != 0 }),
		callOutcomeLaneDescriptor("ParamExposures", nil, false,
			func(o CallOutcome) bool { return len(o.ParamExposures) != 0 }),
	}
	t.Validate("call-outcome")
	return t
}()

// CallOutcomeDescriptors returns the descriptor-driven CallOutcome lane table.
// The returned slice is a copy.
func CallOutcomeDescriptors() callboundary.BoundaryFactTable[CallOutcomeLaneOps] {
	out := make(callboundary.BoundaryFactTable[CallOutcomeLaneOps], len(callOutcomeDescriptors))
	copy(out, callOutcomeDescriptors)
	return out
}

// derivedCallOutcomeLanes is the lane slice used by callOutcomeLanes.
func derivedCallOutcomeLanes() []callOutcomeLane {
	return callboundary.DeriveBoundaryLanes(callOutcomeDescriptors, deriveCallOutcomeLane)
}
