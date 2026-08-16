// Package runtimeentry owns the assembly-local normalization from an authored
// runtime occurrence to its exact executable Entry endpoint. It is sealed
// after executable membership and before Causal route emission.
package runtimeentry

import (
	"sync"

	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Result is the immutable dense normalized-entry directory. It retains exact
// immutable parent proof pointers only as replay fences; no authored View,
// Source View, or mutable construction callback survives sealing.
type Result struct {
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID
	control  *sourcecontrol.Result
	ports    *evaluation.Ports
	exec     *executable.Result
	entries  [keyspace.FamilyCount][]keyspace.Term
}

func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available() &&
		r.control != nil && r.ports != nil && r.exec != nil && sourcecontrol.Matches(r.control, r.sourceID, r.flowID, r.staticID, r.moduleID) &&
		evaluation.Matches(r.ports, r.sourceID, r.flowID, r.staticID, r.moduleID) &&
		executable.Matches(r.exec, r.sourceID, r.flowID, r.staticID, r.moduleID)
}

// OwnsParents is the exact assembly-local splice fence. Equal replay owners
// with the same content identities do not own this directory.
func OwnsParents(r *Result, control *sourcecontrol.Result, ports *evaluation.Ports, exec *executable.Result) bool {
	return r != nil && r.available() && r.control == control && r.ports == ports && r.exec == exec
}

// Matches is the scalar provenance fence for the sealed directory.
func Matches(r *Result, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return r != nil && r.available() && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Entry resolves one authored occurrence in O(1). A dead/static wrapper may
// resolve through its typed child to an executable endpoint, exactly as the
// former Causal normalization did; a dead leaf, foreign, unsupported, or
// malformed term has no entry.
func (r *Result) Entry(term keyspace.Term) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return 0, false
	}
	plane := r.entries[family]
	if uint64(ordinal) >= uint64(len(plane)) {
		return 0, false
	}
	entry := plane[ordinal]
	return entry, entry != 0 && r.exec.Executable(entry)
}

type projectionState struct {
	mu       sync.Mutex
	used     bool
	owner    *Result
	control  *sourcecontrol.Result
	fromTerm keyspace.Term
	toTerm   keyspace.Term
	from     sourcecontrol.PhaseRef
	to       sourcecontrol.PhaseRef
}

// OutcomeResumeProjection is the one-shot exact join between SourceControl's
// structural resume anchor and this owner's normalized runtime endpoint.
type OutcomeResumeProjection struct{ state *projectionState }

// OutcomeResumeSegment is the consumed immutable route proof retained by the
// seal-local RoutePlan. It exposes only opaque phases and exact route matching.
type OutcomeResumeSegment struct {
	owner    *Result
	control  *sourcecontrol.Result
	fromTerm keyspace.Term
	toTerm   keyspace.Term
	from     sourcecontrol.PhaseRef
	to       sourcecontrol.PhaseRef
}

func (segment OutcomeResumeSegment) Available() bool {
	return segment.owner != nil && segment.owner.available() && segment.control != nil && segment.owner.control == segment.control &&
		segment.fromTerm != 0 && segment.toTerm != 0 && segment.from.Available() &&
		segment.to.Available() && segment.from.OutcomePhase() && !segment.to.OutcomePhase() &&
		sourcecontrol.SamePhaseOwner(segment.from, segment.to)
}

func (segment OutcomeResumeSegment) OwnedBy(owner *Result, control *sourcecontrol.Result) bool {
	return segment.Available() && owner != nil && control != nil && segment.owner == owner && segment.control == control
}

func (segment OutcomeResumeSegment) Endpoints(owner *Result, control *sourcecontrol.Result) (sourcecontrol.PhaseRef, sourcecontrol.PhaseRef, bool) {
	if !segment.OwnedBy(owner, control) {
		return sourcecontrol.PhaseRef{}, sourcecontrol.PhaseRef{}, false
	}
	return segment.from, segment.to, true
}

func (segment OutcomeResumeSegment) MatchesRoute(from, to keyspace.Term) bool {
	return segment.Available() &&
		from == segment.fromTerm && to == segment.toTerm
}
