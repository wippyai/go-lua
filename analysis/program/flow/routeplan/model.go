// Package routeplan owns the seal-local final-route declaration consumed by
// recurrence and Causal.  It deliberately contains no adjacency, SCC, reset,
// or domain data: each row names an already-derived final route and exactly
// one typed origin capability.
package routeplan

import (
	"errors"
	"sync"

	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Arm is the final causal arm vocabulary.  It intentionally mirrors semantic
// arms rather than physical Edge/CallBoundary storage, which has not been
// materialized while this plan is built.
type Arm uint8

const (
	ArmLocal Arm = iota + 1
	ArmResume
	ArmSelectTrue
	ArmSelectFalse
	ArmTail
	ArmThrow
	ArmYield
	ArmCancel
)

func (arm Arm) valid() bool { return arm >= ArmLocal && arm <= ArmCancel }

// Origin is one exact typed parent capability. It carries a mandatory endpoint
// proof and optional recurrence witness; no physical ordinal or raw endpoint
// term is present in the neutral plan.
type Origin struct {
	from        sourcecontrol.PhaseRef
	to          sourcecontrol.PhaseRef
	carrier     RecurrenceCarrier
	subdivision subdivisionKind
	segment     sourcecontrol.Segment
	resume      outcomeResume
}

// outcomeResume is the resolved runtime-entry resume endpoint pair. RoutePlan
// admits the four already-resolved values; the owner fence over the entry and
// SourceControl relations belongs to the caller that owns both.
type outcomeResume struct {
	from     sourcecontrol.PhaseRef
	to       sourcecontrol.PhaseRef
	fromTerm keyspace.Term
	toTerm   keyspace.Term
}

func (resume outcomeResume) Available() bool {
	return resume.fromTerm != 0 && resume.toTerm != 0 &&
		resume.from.Available() && resume.to.Available() &&
		resume.from.OutcomePhase() && !resume.to.OutcomePhase() &&
		sourcecontrol.SamePhaseOwner(resume.from, resume.to)
}

func (resume outcomeResume) MatchesRoute(from, to keyspace.Term) bool {
	return resume.Available() && from == resume.fromTerm && to == resume.toTerm
}

type subdivisionKind uint8

const (
	subdivisionNone subdivisionKind = iota
	subdivisionSourceControl
	subdivisionRuntimeEntry
)

func (origin Origin) endpoints() (sourcecontrol.PhaseRef, sourcecontrol.PhaseRef, bool) {
	return origin.from, origin.to, origin.from.Available() && origin.to.Available()
}

type RecurrenceCarrierKind uint8

const (
	CarrierNone RecurrenceCarrierKind = iota
	CarrierArc
	CarrierNodePair
)

// RecurrenceCarrier is independent of endpoint identity. Arc carries the
// existing recurrence annotation; NodePair carries only genuine CSR
// component membership; None deliberately contributes zero SCC/Mu/reset.
type RecurrenceCarrier struct {
	kind     RecurrenceCarrierKind
	arc      sourcecontrol.ArcRef
	fromNode sourcecontrol.NodeRef
	toNode   sourcecontrol.NodeRef
}

func noRecurrenceCarrier() RecurrenceCarrier { return RecurrenceCarrier{} }
func arcCarrier(ref sourcecontrol.ArcRef) RecurrenceCarrier {
	return RecurrenceCarrier{kind: CarrierArc, arc: ref}
}
func nodePairCarrier(from, to sourcecontrol.NodeRef) RecurrenceCarrier {
	return RecurrenceCarrier{kind: CarrierNodePair, fromNode: from, toNode: to}
}

func (carrier RecurrenceCarrier) Kind() RecurrenceCarrierKind { return carrier.kind }
func (carrier RecurrenceCarrier) ArcRef() (sourcecontrol.ArcRef, bool) {
	return carrier.arc, carrier.kind == CarrierArc && carrier.arc.Available()
}
func (carrier RecurrenceCarrier) NodePair() (sourcecontrol.NodeRef, sourcecontrol.NodeRef, bool) {
	return carrier.fromNode, carrier.toNode, carrier.kind == CarrierNodePair && sourcecontrol.SameOwner(carrier.fromNode, carrier.toNode)
}

// CSRPhasePair and CSRNodePair are the only generic ingress for ordinary
// catalog phases. Outcome phases are rejected by valid: their endpoint and
// carrier relation must instead arrive through OutcomeSubdivision below.
func CSRPhasePair(from, to sourcecontrol.PhaseRef) Origin {
	return Origin{from: from, to: to, carrier: noRecurrenceCarrier()}
}
func CSRArcPair(from, to sourcecontrol.PhaseRef, ref sourcecontrol.ArcRef) Origin {
	return Origin{from: from, to: to, carrier: arcCarrier(ref)}
}
func CSRNodePair(from, to sourcecontrol.PhaseRef, fromNode, toNode sourcecontrol.NodeRef) Origin {
	return Origin{from: from, to: to, carrier: nodePairCarrier(fromNode, toNode)}
}

// OutcomeSubdivision admits SourceControl's exact immutable route segment.
// The Builder owns one-shot publication; this boundary only checks the row's
// owner fence and never introduces a second per-row lifecycle.
func OutcomeSubdivision(graph *sourcecontrol.Result, segment sourcecontrol.Segment) (Origin, bool) {
	if !segment.Valid(graph) {
		return Origin{}, false
	}
	from, to, endpoints := segment.Endpoints()
	carrier, carrierOK := segment.Carrier()
	if !endpoints || !carrierOK {
		return Origin{}, false
	}
	origin := Origin{from: from, to: to, subdivision: subdivisionSourceControl, segment: segment}
	switch carrier.Kind() {
	case sourcecontrol.SegmentCarrierNone:
		origin.carrier = noRecurrenceCarrier()
	case sourcecontrol.SegmentCarrierArc:
		ref, ok := carrier.ArcRef()
		if !ok {
			return Origin{}, false
		}
		origin.carrier = arcCarrier(ref)
	case sourcecontrol.SegmentCarrierNodePair:
		left, right, ok := carrier.NodePair()
		if !ok {
			return Origin{}, false
		}
		origin.carrier = nodePairCarrier(left, right)
	default:
		return Origin{}, false
	}
	return origin, true
}

// OutcomeResumeSubdivision admits one already-resolved normalized endpoint
// pair. It is the only RoutePlan ingress for an Outcome→CSR runtime resume;
// the caller owns the entry and SourceControl relations the values came from
// and has already proven the row against them.
func OutcomeResumeSubdivision(from, to sourcecontrol.PhaseRef,
	fromTerm, toTerm keyspace.Term) (Origin, keyspace.Term, bool) {
	resume := outcomeResume{from: from, to: to, fromTerm: fromTerm, toTerm: toTerm}
	if !resume.Available() {
		return Origin{}, 0, false
	}
	return Origin{from: from, to: to, carrier: noRecurrenceCarrier(),
		subdivision: subdivisionRuntimeEntry, resume: resume}, toTerm, true
}

func (origin Origin) valid() bool {
	from, to, endpointOK := origin.endpoints()
	if endpointOK && sourcecontrol.SamePhaseOwner(from, to) {
		if (from.OutcomePhase() || to.OutcomePhase()) != (origin.subdivision != subdivisionNone) {
			return false
		}
		if origin.subdivision == subdivisionRuntimeEntry && (!from.OutcomePhase() || to.OutcomePhase() || !origin.resume.Available()) {
			return false
		}
		switch origin.carrier.kind {
		case CarrierNone:
			return true
		case CarrierArc:
			return origin.carrier.arc.Available()
		case CarrierNodePair:
			return sourcecontrol.SameOwner(origin.carrier.fromNode, origin.carrier.toNode)
		default:
			return false
		}
	}
	// A zero endpoint is intentionally invalid for all new rows. The legacy
	// kind check remains only so malformed old values fail closed rather than
	// becoming an accidental carrier.
	return false
}

// Route is an immutable final semantic route declaration.  Its Origin is
// unexported outside this package so no consumer can manufacture one.
type Route struct {
	From     keyspace.Term
	To       keyspace.Term
	Decision keyspace.Term
	Truth    bool
	Arm      Arm
	origin   Origin
}

// Builder is a one-shot mutable declaration capability.  It is intentionally
// neutral: it may be fed by the existing typed causal emitters but owns none
// of their sourcecontrol/evaluation/outcome inputs.
type Builder struct {
	state *builderState
}

type builderState struct {
	mu       sync.Mutex
	terminal bool
	owner    sourcecontrol.Owner
	routes   []Route
}

func New(owner sourcecontrol.Owner) (*Builder, error) {
	if !owner.Available() {
		return nil, errors.New("program/flow/routeplan: sourcecontrol owner is unavailable")
	}
	return &Builder{state: &builderState{owner: owner}}, nil
}

// Emit declares exactly one final local or boundary route with its exact
// origin.  A caller cannot omit the origin or append after sealing.
func (builder *Builder) Emit(route Route, origin Origin) error {
	if builder == nil || builder.state == nil {
		return errors.New("program/flow/routeplan: Builder is unavailable")
	}
	state := builder.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal || !validRoute(route, origin, state.owner) {
		return errors.New("program/flow/routeplan: final route declaration is invalid")
	}
	route.origin = origin
	state.routes = append(state.routes, route)
	return nil
}

// Seal transfers the immutable route list once.  It does not retain the
// builder's mutable slice, and Plan exposes rows only by ordinal to recurrence
// and Causal during the same assembly transaction.
func (builder *Builder) Seal() (*Plan, error) {
	if builder == nil || builder.state == nil {
		return nil, errors.New("program/flow/routeplan: Builder is unavailable")
	}
	state := builder.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		return nil, errors.New("program/flow/routeplan: Builder is consumed")
	}
	state.terminal = true
	if !state.owner.Available() {
		return nil, errors.New("program/flow/routeplan: owner became unavailable")
	}
	routes := append([]Route(nil), state.routes...)
	state.routes = nil
	return &Plan{owner: state.owner, routes: routes, token: &token{}}, nil
}

// Plan is immutable and assembly-local.  It is not a durable topology or a
// public Flow authority; Binding and Causal must consume the same ordinal list.
type Plan struct {
	owner  sourcecontrol.Owner
	routes []Route
	token  *token
}

type token struct{}

func (plan *Plan) Owner() sourcecontrol.Owner {
	if plan == nil {
		return sourcecontrol.Owner{}
	}
	return plan.owner
}

func (plan *Plan) Count() int {
	if plan == nil || !plan.owner.Available() || plan.token == nil {
		return 0
	}
	return len(plan.routes)
}
func (plan *Plan) At(index int) (Route, Origin, bool) {
	if plan == nil || !plan.owner.Available() || plan.token == nil || index < 0 || index >= len(plan.routes) {
		return Route{}, Origin{}, false
	}
	route := plan.routes[index]
	return route, route.origin, validRoute(route, route.origin, plan.owner)
}

// Endpoints exposes only opaque endpoint phases to recurrence. The phases are
// resolved once while SourceControl is live and retained on this route row.
func (origin Origin) Endpoints() (sourcecontrol.PhaseRef, sourcecontrol.PhaseRef, bool) {
	from, to, ok := origin.endpoints()
	if !ok || !sourcecontrol.SamePhaseOwner(from, to) || !origin.valid() {
		return sourcecontrol.PhaseRef{}, sourcecontrol.PhaseRef{}, false
	}
	return from, to, true
}

func (origin Origin) RecurrenceCarrier() (RecurrenceCarrier, bool) {
	if !origin.valid() {
		return RecurrenceCarrier{}, false
	}
	return origin.carrier, true
}

func validRoute(route Route, origin Origin, owner sourcecontrol.Owner) bool {
	if !origin.valid() || !route.Arm.valid() || route.From == 0 || route.To == 0 || route.Decision == 0 && route.Truth {
		return false
	}
	from, to, endpointOK := origin.endpoints()
	if !endpointOK || !owner.OwnsPhaseRef(from) || !owner.OwnsPhaseRef(to) {
		return false
	}
	if from.OutcomePhase() || to.OutcomePhase() {
		switch origin.subdivision {
		case subdivisionSourceControl:
			if !origin.segment.MatchesRoute(route.From, route.To) {
				return false
			}
		case subdivisionRuntimeEntry:
			if !origin.resume.MatchesRoute(route.From, route.To) {
				return false
			}
		default:
			return false
		}
	}
	switch origin.carrier.kind {
	case CarrierNone:
		return true
	case CarrierArc:
		return owner.OwnsArcRef(origin.carrier.arc)
	case CarrierNodePair:
		return owner.OwnsNodeRef(origin.carrier.fromNode) && owner.OwnsNodeRef(origin.carrier.toNode)
	default:
		return false
	}
}
