// Package runtimeentry owns the assembly-local normalization from an authored
// runtime occurrence to its exact executable Entry endpoint. It is sealed
// after executable membership and before Causal route emission.
package runtimeentry

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable dense normalized-entry directory. It retains exact
// immutable parent proof pointers only as replay fences; no authored View,
// Source View, or mutable construction callback survives sealing.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
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
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
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

// OutcomeResumeRow is the immutable exact join between SourceControl's
// structural Outcome continuation and this owner's normalized executable
// endpoint. The owner pointers are fences, not mutable capabilities; the
// route-plan Builder owns the one-shot publication transaction.
type OutcomeResumeRow struct {
	owner    *Result
	control  *sourcecontrol.Result
	fromTerm keyspace.Term
	toTerm   keyspace.Term
	from     sourcecontrol.PhaseRef
	to       sourcecontrol.PhaseRef
}

func (row OutcomeResumeRow) Available() bool {
	return row.owner != nil && row.owner.available() && row.control != nil && row.owner.control == row.control &&
		row.fromTerm != 0 && row.toTerm != 0 && row.from.Available() &&
		row.to.Available() && row.from.OutcomePhase() && !row.to.OutcomePhase() &&
		sourcecontrol.SamePhaseOwner(row.from, row.to)
}

func (row OutcomeResumeRow) OwnedBy(owner *Result, control *sourcecontrol.Result) bool {
	return row.Available() && owner != nil && control != nil && row.owner == owner && row.control == control
}

func (row OutcomeResumeRow) Endpoints(owner *Result, control *sourcecontrol.Result) (sourcecontrol.PhaseRef, sourcecontrol.PhaseRef, bool) {
	if !row.OwnedBy(owner, control) {
		return sourcecontrol.PhaseRef{}, sourcecontrol.PhaseRef{}, false
	}
	return row.from, row.to, true
}

func (row OutcomeResumeRow) MatchesRoute(from, to keyspace.Term) bool {
	return row.Available() &&
		from == row.fromTerm && to == row.toTerm
}

// RouteTerms returns the exact authored Outcome and executable Entry terms
// after the caller has checked OwnedBy. A zero term is never a valid row.
func (row OutcomeResumeRow) RouteTerms() (keyspace.Term, keyspace.Term) {
	return row.fromTerm, row.toTerm
}
