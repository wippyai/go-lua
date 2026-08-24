// Package subjectflow publishes Flow's neutral subject facts at suspension
// boundaries.
//
// The package is deliberately below Placement.  It does not decide a
// placement policy, allocate a heap key, or claim that a dynamic call has a
// known return/capture effect.  It only publishes relations that the sealed
// Source/Authored/Causal facts can prove: local Define/Use/Alias events and
// the exact causal routes that leave a call through Yield and re-enter it
// through a normal arm.  Missing interprocedural facts are represented by an
// Unknown event or boundary state rather than by a guessed alias.
package subjectflow

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectKind identifies the neutral subject plane.  A subject is always
// carried by an issued semantic path; it is never a heap Key or a Placement
// allocation handle.
type SubjectKind uint8

const (
	SubjectInvalid SubjectKind = iota
	SubjectRoot
	SubjectCell
	SubjectValue
	SubjectValues
)

func (kind SubjectKind) valid() bool {
	return kind >= SubjectRoot && kind <= SubjectValues
}

// EventKind is the closed local fact vocabulary.  Unknown intentionally sits
// in the same sum as the exact facts, allowing consumers to remain sound when
// a return, capture, or dynamic call crosses the current proof boundary.
type EventKind uint8

const (
	EventUnknown EventKind = iota + 1
	EventDefine
	EventUse
	EventAlias
)

func (kind EventKind) valid() bool {
	return kind >= EventUnknown && kind <= EventAlias
}

// EventRole identifies the authored slot which caused an event.  It is part
// of row identity so two uses of the same term in distinct operand slots do
// not collapse into one fact.
type EventRole uint8

const (
	RoleRoot EventRole = iota + 1
	RoleResult
	RoleOperand
	RoleLeft
	RoleRight
	RoleReceiver
	RoleCallee
	RoleActuals
	RoleMember
	RoleCell
	RoleTail
	RoleCapture
	RoleReturn
	RoleTarget
	RoleWrite
)

// Subject is one neutral, owner-issued subject identity. Term is retained as
// a provenance coordinate for Flow queries; ID is the stable semantic path
// consumed by downstream domains.
type Subject struct {
	Kind SubjectKind
	ID   identity.ContentID
	Term keyspace.Term
}

// Event is an exact local relation or a conservative Unknown marker. For an
// Alias, Subject is the source and Related is the destination. For a Use,
// Subject is the used value/cell and Related is empty. Path is the semantic
// path of the authored operation that observed the relation.
type Event struct {
	ID      identity.ContentID
	Kind    EventKind
	Role    EventRole
	Index   uint32
	Subject Subject
	Related Subject
	Term    keyspace.Term
	Path    identity.ContentID
}

// AliasRouteScope owns one canonical causal-route denominator. Body scopes
// contain every route with an endpoint in Body. The single global scope is
// the fail-closed denominator for candidates whose body is unavailable.
// The route list is stored once per scope, never once per candidate.
type AliasRouteScope struct {
	ID     identity.ContentID
	Kind   AliasRouteScopeKind
	Body   identity.ContentID
	routes []identity.ContentID
	proven bool
}

// AliasRouteScopeKind is the closed denominator-scope vocabulary.
type AliasRouteScopeKind uint8

const (
	AliasRouteScopeInvalid AliasRouteScopeKind = iota
	AliasRouteScopeBody
	AliasRouteScopeGlobal
)

func (kind AliasRouteScopeKind) valid() bool {
	return kind == AliasRouteScopeBody || kind == AliasRouteScopeGlobal
}

// AliasCandidate binds one candidate to its canonical route scope. Closed is
// never inferred from an empty scope: it is issued only after complete event
// coverage, a known body, and the alias-component Unknown fold succeed.
type AliasCandidate struct {
	ID        identity.ContentID
	Candidate Subject
	Scope     identity.ContentID
	Closed    bool
	proven    bool
}

// BoundaryState says whether a Yield route has an exact normal re-entry
// route. Unknown is used for terminal/tail or malformedly incomplete
// interprocedural cases; no synthetic re-entry path is issued.
type BoundaryState uint8

const (
	BoundaryUnknown BoundaryState = iota + 1
	BoundaryPaired
)

// Boundary pairs one exact Yield route with one exact normal re-entry arm.
// A Yield can have more than one normal arm after a Select; each alternative
// is published as its own row. Re-entry fields are zero only for Unknown.
type Boundary struct {
	ID              identity.ContentID
	State           BoundaryState
	Call            keyspace.Term
	CallPath        identity.ContentID
	YieldArm        causal.BoundaryArmKind
	YieldRoute      identity.ContentID
	YieldFromPath   identity.ContentID
	YieldToPath     identity.ContentID
	ReentryArm      causal.BoundaryArmKind
	ReentryRoute    identity.ContentID
	ReentryFromPath identity.ContentID
	ReentryToPath   identity.ContentID
}

// LivenessState is the neutral result of asking whether one subject is
// needed after a particular suspension route.  DiesBefore is a must result:
// it is issued only when every normal re-entry arm proves that the subject is
// not used/aliased after re-entry.  Live means every arm proves a post-
// re-entry use/alias.  Unknown is the fail-closed result for a missing route,
// an unresolved event, or mixed arm answers.
type LivenessState uint8

const (
	LivenessUnknown LivenessState = iota + 1
	LivenessLive
	LivenessDiesBefore
)

func (state LivenessState) valid() bool {
	return state >= LivenessUnknown && state <= LivenessDiesBefore
}

// AggregateLiveness applies the all-normal-arms (must) law.  In particular,
// a mixture of Live and DiesBefore is not evidence for either result, and an
// empty set is a missing route rather than a proof of death.
func AggregateLiveness(states []LivenessState) LivenessState {
	if len(states) == 0 {
		return LivenessUnknown
	}
	first := states[0]
	if !first.valid() || first == LivenessUnknown {
		return LivenessUnknown
	}
	for _, state := range states[1:] {
		if !state.valid() || state == LivenessUnknown || state != first {
			return LivenessUnknown
		}
	}
	return first
}

// Liveness is a per-yield-route, per-subject neutral result.  YieldFromPath
// and YieldToPath are retained as route provenance; YieldRoute is the stable
// key used by mounted consumers and by the all-path fold.
type Liveness struct {
	ID            identity.ContentID
	Call          keyspace.Term
	YieldRoute    identity.ContentID
	YieldFromPath identity.ContentID
	YieldToPath   identity.ContentID
	Subject       Subject
	State         LivenessState
}

// Result is the immutable neutral subject-flow authority for one complete
// Source/Flow/Static/Module quartet. Its rows own no source graph and no
// Placement state.
type Result struct {
	sourceID    identity.ContentID
	flowID      identity.ContentID
	staticID    identity.ContentID
	moduleID    identity.ContentID
	events      []Event
	routeScopes []AliasRouteScope
	candidates  []AliasCandidate
	boundaries  []Boundary
	liveness    []Liveness
	// yieldOrder is the program-ordered yield boundary sequence the Program
	// plane publishes its liveness as ranges over. Each body occupies a
	// contiguous block of ordinals, so a run inside a body is a run inside
	// the sequence and no consumer needs to name the body.
	yieldOrder []YieldOrdinal
}

// YieldOrdinal is one distinct yield route and its position in the ordered
// sequence. The order is program order: the owning body first, then the call
// term ordinal, then the route identity for a call that yields more than one
// route.
type YieldOrdinal struct {
	Call          keyspace.Term
	YieldRoute    identity.ContentID
	YieldFromPath identity.ContentID
	YieldToPath   identity.ContentID
	Ordinal       uint32
}

var (
	ErrOwnerMismatch = errors.New("program/flow/subjectflow: owner identities disagree")
	ErrUnavailable   = errors.New("program/flow/subjectflow: required owner proof is unavailable")
	ErrMalformed     = errors.New("program/flow/subjectflow: authored or causal row is malformed")
)

// Matches is the exact owner fence used by Flow's public projection.
func Matches(result *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return result != nil && result.sourceID == sourceID && result.flowID == flowID &&
		result.staticID == staticID && result.moduleID == moduleID &&
		sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available()
}

// Available reports whether the sealed owner tuple is present.
func (result *Result) Available() bool {
	if result == nil {
		return false
	}
	return Matches(result, result.sourceID, result.flowID, result.staticID, result.moduleID)
}

func (result *Result) EventCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.events)
}

func (result *Result) EventAt(index int) (Event, bool) {
	if !result.Available() || index < 0 || index >= len(result.events) {
		return Event{}, false
	}
	return result.events[index], true
}

// AliasRouteScopeCount is the sealed width of the canonical denominator
// plane.
func (result *Result) AliasRouteScopeCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.routeScopes)
}

// AliasRouteScopeAt returns one canonical route denominator. The row remains
// owner-borrowed; callers enumerate routes through RouteAt without receiving
// a mutable slice.
func (result *Result) AliasRouteScopeAt(index int) (AliasRouteScope, bool) {
	if !result.Available() || index < 0 || index >= len(result.routeScopes) {
		return AliasRouteScope{}, false
	}
	return result.routeScopes[index], true
}

// AliasCandidateCount is the sealed width of the candidate-to-scope plane.
func (result *Result) AliasCandidateCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.candidates)
}

func (result *Result) AliasCandidateAt(index int) (AliasCandidate, bool) {
	if !result.Available() || index < 0 || index >= len(result.candidates) {
		return AliasCandidate{}, false
	}
	return result.candidates[index], true
}

// AliasCandidateFor resolves a candidate by its authenticated semantic path.
// It returns false for an ambiguous duplicate rather than selecting physical
// row order.
func (result *Result) AliasCandidateFor(candidate Subject) (AliasCandidate, bool) {
	if !result.Available() || !candidate.Kind.valid() || !candidate.ID.Available() {
		return AliasCandidate{}, false
	}
	var found AliasCandidate
	for _, row := range result.candidates {
		if row.Candidate.Kind != candidate.Kind || row.Candidate.ID != candidate.ID {
			continue
		}
		if found.ID.Available() {
			return AliasCandidate{}, false
		}
		found = row
	}
	if !found.ID.Available() {
		return AliasCandidate{}, false
	}
	return found, true
}

func (result *Result) BoundaryCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.boundaries)
}

func (result *Result) BoundaryAt(index int) (Boundary, bool) {
	if !result.Available() || index < 0 || index >= len(result.boundaries) {
		return Boundary{}, false
	}
	return result.boundaries[index], true
}

// YieldOrdinalCount is the width of the ordered yield boundary sequence.
func (result *Result) YieldOrdinalCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.yieldOrder)
}

// YieldOrdinalAt returns one ordered boundary by its position.
func (result *Result) YieldOrdinalAt(index int) (YieldOrdinal, bool) {
	if !result.Available() || index < 0 || index >= len(result.yieldOrder) {
		return YieldOrdinal{}, false
	}
	return result.yieldOrder[index], true
}

func (result *Result) LivenessCount() int {
	if !result.Available() {
		return 0
	}
	return len(result.liveness)
}

func (result *Result) LivenessAt(index int) (Liveness, bool) {
	if !result.Available() || index < 0 || index >= len(result.liveness) {
		return Liveness{}, false
	}
	return result.liveness[index], true
}

// LivenessFor resolves the aggregate answer for one exact yield route and
// subject.  It deliberately does not derive a default: absent rows mean the
// producer could not establish the route/subject relation and therefore
// return Unknown.
func (result *Result) LivenessFor(yieldRoute identity.ContentID, subject Subject) (LivenessState, bool) {
	if !result.Available() || !yieldRoute.Available() || !subject.Kind.valid() || !subject.ID.Available() {
		return LivenessUnknown, false
	}
	for _, row := range result.liveness {
		if row.YieldRoute == yieldRoute && row.Subject.Kind == subject.Kind && row.Subject.ID == subject.ID {
			return row.State, true
		}
	}
	return LivenessUnknown, false
}

// UnknownCount counts unresolved local rows. Unknown is not an error: it is
// the explicit proof boundary which keeps consumers from treating a dynamic
// return, capture, or call effect as a proven alias.
func (result *Result) UnknownCount() int {
	if !result.Available() {
		return 0
	}
	count := 0
	for _, event := range result.events {
		if event.Kind == EventUnknown {
			count++
		}
	}
	for _, boundary := range result.boundaries {
		if boundary.State == BoundaryUnknown {
			count++
		}
	}
	for _, row := range result.liveness {
		if row.State == LivenessUnknown {
			count++
		}
	}
	return count
}

func (result *Result) HasUnknown() bool { return result.UnknownCount() != 0 }

func (scope AliasRouteScope) Available() bool {
	return scope.proven && scope.ID.Available() && scope.Kind.valid() &&
		(scope.Kind == AliasRouteScopeGlobal && !scope.Body.Available() || scope.Kind == AliasRouteScopeBody && scope.Body.Available())
}

func (scope AliasRouteScope) RouteCount() int {
	if !scope.Available() {
		return 0
	}
	return len(scope.routes)
}

func (scope AliasRouteScope) RouteAt(index int) (identity.ContentID, bool) {
	if !scope.Available() || index < 0 || index >= len(scope.routes) {
		return identity.ContentID{}, false
	}
	return scope.routes[index], true
}

func (candidate AliasCandidate) Available() bool {
	return candidate.proven && candidate.ID.Available() && candidate.Candidate.Kind.valid() &&
		candidate.Candidate.ID.Available() && candidate.Scope.Available()
}

func (candidate AliasCandidate) CandidateSubject() Subject { return candidate.Candidate }
func (candidate AliasCandidate) ScopeID() identity.ContentID {
	if !candidate.Available() {
		return identity.ContentID{}
	}
	return candidate.Scope
}

// rowID and boundaryID are intentionally private identity equations. They
// bind all semantic row coordinates and use framed fields so adding a new
// role cannot create an ambiguous concatenation.
func rowID(kind EventKind, role EventRole, index uint32, subject, related Subject, term keyspace.Term, path identity.ContentID) identity.ContentID {
	return digest("program/flow/subjectflow-event-v1", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(kind)) == nil && writer.Uint(uint64(role)) == nil && writer.Uint(uint64(index)) == nil &&
			writer.Uint(uint64(subject.Kind)) == nil && writer.Bytes(subject.ID[:]) == nil && writer.Uint(uint64(subject.Term)) == nil &&
			writer.Uint(uint64(related.Kind)) == nil && writer.Bytes(related.ID[:]) == nil && writer.Uint(uint64(related.Term)) == nil &&
			writer.Uint(uint64(term)) == nil && writer.Bytes(path[:]) == nil
	})
}

func boundaryID(call keyspace.Term, callPath identity.ContentID, state BoundaryState, yieldArm, reentryArm causal.BoundaryArmKind, yieldRoute, reentryRoute, yieldFrom, yieldTo, reentryFrom, reentryTo identity.ContentID) identity.ContentID {
	return digest("program/flow/subjectflow-boundary-v1", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(call)) == nil && writer.Bytes(callPath[:]) == nil && writer.Uint(uint64(state)) == nil && writer.Uint(uint64(yieldArm)) == nil && writer.Uint(uint64(reentryArm)) == nil &&
			writer.Bytes(yieldRoute[:]) == nil && writer.Bytes(reentryRoute[:]) == nil && writer.Bytes(yieldFrom[:]) == nil && writer.Bytes(yieldTo[:]) == nil &&
			writer.Bytes(reentryFrom[:]) == nil && writer.Bytes(reentryTo[:]) == nil
	})
}

func livenessID(yieldRoute, yieldFrom, yieldTo identity.ContentID, subject Subject) identity.ContentID {
	return digest("program/flow/subjectflow-liveness-v1", func(writer *framing.Writer) bool {
		return writer.Bytes(yieldRoute[:]) == nil && writer.Bytes(yieldFrom[:]) == nil && writer.Bytes(yieldTo[:]) == nil &&
			writer.Uint(uint64(subject.Kind)) == nil && writer.Bytes(subject.ID[:]) == nil && writer.Uint(uint64(subject.Term)) == nil
	})
}

func newAliasRouteScope(kind AliasRouteScopeKind, body identity.ContentID, routes []identity.ContentID) AliasRouteScope {
	routes = append([]identity.ContentID(nil), routes...)
	sort.Slice(routes, func(left, right int) bool { return bytes.Compare(routes[left][:], routes[right][:]) < 0 })
	valid := kind.valid() && (kind == AliasRouteScopeGlobal && !body.Available() || kind == AliasRouteScopeBody && body.Available())
	for index, route := range routes {
		valid = valid && route.Available() && (index == 0 || bytes.Compare(routes[index-1][:], route[:]) < 0)
	}
	id := digest("program/flow/subjectflow-alias-route-scope-v1", func(writer *framing.Writer) bool {
		if writer.Uint(uint64(kind)) != nil || writer.Bool(body.Available()) != nil || writer.Bytes(body[:]) != nil || writer.Uint(uint64(len(routes))) != nil {
			return false
		}
		for _, route := range routes {
			if writer.Bytes(route[:]) != nil {
				return false
			}
		}
		return true
	})
	return AliasRouteScope{ID: id, Kind: kind, Body: body, routes: routes, proven: valid && id.Available()}
}

func newAliasCandidate(candidate Subject, scope identity.ContentID, closed bool) AliasCandidate {
	id := digest("program/flow/subjectflow-alias-candidate-v1", func(writer *framing.Writer) bool {
		return writer.Uint(uint64(candidate.Kind)) == nil && writer.Bytes(candidate.ID[:]) == nil &&
			writer.Uint(uint64(candidate.Term)) == nil && writer.Bytes(scope[:]) == nil && writer.Bool(closed) == nil
	})
	return AliasCandidate{ID: id, Candidate: candidate, Scope: scope, Closed: closed, proven: candidate.Kind.valid() && candidate.ID.Available() && scope.Available() && id.Available()}
}

func digest(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
