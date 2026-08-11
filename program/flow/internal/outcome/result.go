// Package outcome seals the canonical lexical outcome coordinates.  It owns
// no execution graph: normal continuation, loop transitions, label resume,
// and all analysis relations are later Flow passes.
package outcome

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Result is the immutable, dense Outcome relation.  Every slice has a
// sentinel at index zero; the remaining entries are indexed by the canonical
// one-based Outcome ordinal or by the corresponding authored occurrence
// ordinal.  In particular, propagation never stores a Loop or Label: those
// are typed targets of terminal Break/Goto coordinates, not Outcome edges.
type Result struct {
	// Provenance is the scalar fence for all four sealed-owner identities:
	// Source, authored Flow, Static, and Module. Outcome retains no Source,
	// Flow, Body, or Shape owners; any unavailable identity makes this result
	// unusable until final assembly matches the exact quartet.
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID

	bodies      []keyspace.Term
	kinds       []kind.OutcomeKind
	targets     []keyspace.Term
	propagation []keyspace.Term

	// base is indexed by the stable OutcomeKind ordinal.  Only the four
	// mandatory body planes are populated; indexing by the canonical ordinal
	// avoids a second local kind numbering scheme.
	base [8][]keyspace.Term

	returnExit []keyspace.Term
	breakExit  []keyspace.Term
	gotoExit   []keyspace.Term
}

// Matches reports whether r was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by final assembly. Unavailable
// identities never match, including a malformed Result carrying plausible
// outcome rows.
func Matches(r *Result, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Count reports the complete sealed Outcome denominator.
func (r *Result) Count() int {
	if !r.validRows() {
		return 0
	}
	return len(r.bodies) - 1
}

// At returns the canonical Outcome at one zero-based dense result index.
func (r *Result) At(index int) (keyspace.Term, bool) {
	if r == nil || index < 0 || index >= r.Count() {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(index+1)), true
}

// Get returns the owning Body, closed kind, and typed destination of one
// Outcome.  Invalid or foreign-family terms fail closed.
func (r *Result) Get(term keyspace.Term) (body keyspace.Term, outcomeKind kind.OutcomeKind, target keyspace.Term, ok bool) {
	ordinal, ok := r.outcomeOrdinal(term)
	if !ok {
		return 0, 0, 0, false
	}
	if !r.validStoredRow(ordinal) {
		return 0, 0, 0, false
	}
	return r.bodies[ordinal], r.kinds[ordinal], r.targets[ordinal], true
}

// BodyRange returns the half-open zero-based At range owned by body.  The
// range is derived from adjacent mandatory Normal coordinates, never kept as
// another semantic index.
func (r *Result) BodyRange(body keyspace.Term) (start, end int, ok bool) {
	if r == nil || r.Count() == 0 || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, 0, false
	}
	bodyOrdinal := keyspace.TermOrdinal(body)
	if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(r.base[kind.OutcomeNormal])) {
		return 0, 0, false
	}
	normal := r.base[kind.OutcomeNormal][bodyOrdinal]
	normalOrdinal, valid := r.outcomeOrdinal(normal)
	if !valid || r.kinds[normalOrdinal] != kind.OutcomeNormal || r.bodies[normalOrdinal] != body {
		return 0, 0, false
	}
	start = int(normalOrdinal - 1)
	if nextOrdinal := bodyOrdinal + 1; uint64(nextOrdinal) < uint64(len(r.base[kind.OutcomeNormal])) {
		next := r.base[kind.OutcomeNormal][nextOrdinal]
		nextResultOrdinal, nextValid := r.outcomeOrdinal(next)
		if !nextValid || r.kinds[nextResultOrdinal] != kind.OutcomeNormal ||
			keyspace.TermFamily(r.bodies[nextResultOrdinal]) != keyspace.FamilyBody ||
			keyspace.TermOrdinal(r.bodies[nextResultOrdinal]) != nextOrdinal {
			return 0, 0, false
		}
		end = int(nextResultOrdinal - 1)
	} else {
		end = r.Count()
	}
	if start < 0 || end < start || end > r.Count() {
		return 0, 0, false
	}
	return start, end, true
}

// BodyExit returns one of the four mandatory body coordinates.  Return,
// Break, and Goto are occurrence/target coordinates and are intentionally not
// available through this query.
func (r *Result) BodyExit(body keyspace.Term, outcomeKind kind.OutcomeKind) (keyspace.Term, bool) {
	if outcomeKind != kind.OutcomeNormal && outcomeKind != kind.OutcomeThrow &&
		outcomeKind != kind.OutcomeYield && outcomeKind != kind.OutcomeCancel {
		return 0, false
	}
	if r == nil || r.Count() == 0 || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	bodyOrdinal := keyspace.TermOrdinal(body)
	plane := r.base[outcomeKind]
	if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(plane)) {
		return 0, false
	}
	exit := plane[bodyOrdinal]
	exitOrdinal, ok := r.outcomeOrdinal(exit)
	if !ok || r.bodies[exitOrdinal] != body || r.kinds[exitOrdinal] != outcomeKind || r.targets[exitOrdinal] != 0 {
		return 0, false
	}
	return exit, true
}

// Find resolves one semantic key in the sorted Outcome relation.  It is a
// binary search over immutable rows, so the hot query allocates nothing and
// no map becomes a second identity authority.
func (r *Result) Find(body keyspace.Term, outcomeKind kind.OutcomeKind, target keyspace.Term) (keyspace.Term, bool) {
	// Validate the row storage before the search touches any parallel plane.
	// Result is immutable after sealing, but malformed internal values must
	// still fail closed rather than letting a short plane panic the query.
	if !r.validRows() || !validBody(body) || !validKind(outcomeKind) || !validTarget(outcomeKind, target) {
		return 0, false
	}
	lo, hi := 1, len(r.bodies)
	for lo < hi {
		middle := lo + (hi-lo)/2
		candidate := outcomeKey{body: r.bodies[middle], kind: r.kinds[middle], target: r.targets[middle]}
		wanted := outcomeKey{body: body, kind: outcomeKind, target: target}
		if compareKey(candidate, wanted) < 0 {
			lo = middle + 1
			continue
		}
		hi = middle
	}
	if lo >= len(r.bodies) {
		return 0, false
	}
	if !r.validStoredRow(uint32(lo)) || r.bodies[lo] != body || r.kinds[lo] != outcomeKind || r.targets[lo] != target {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(lo)), true
}

// Propagation returns the next Outcome on a non-local lexical path.  A
// terminal path deliberately returns false: its typed Loop or Label target is
// retained by Get, while no non-Outcome edge is smuggled into this relation.
func (r *Result) Propagation(term keyspace.Term) (keyspace.Term, bool) {
	ordinal, ok := r.outcomeOrdinal(term)
	if !ok {
		return 0, false
	}
	next := r.propagation[ordinal]
	if next == 0 {
		return 0, false
	}
	nextOrdinal, nextOK := r.outcomeOrdinal(next)
	if !nextOK || next == term || r.kinds[nextOrdinal] != r.kinds[ordinal] ||
		r.targets[nextOrdinal] != r.targets[ordinal] || !r.validStoredRow(ordinal) || !r.validStoredRow(nextOrdinal) {
		return 0, false
	}
	return next, true
}

// ReturnExit resolves one authored Return occurrence to its first Return
// Outcome in the current activation.
func (r *Result) ReturnExit(term keyspace.Term) (keyspace.Term, bool) {
	if r == nil {
		return 0, false
	}
	return r.occurrenceExit(r.returnExit, term, keyspace.FamilyReturn, kind.OutcomeReturn)
}

// BreakExit resolves one authored Break occurrence to its first typed Break
// Outcome.  The returned Outcome retains the Loop target in Get.
func (r *Result) BreakExit(term keyspace.Term) (keyspace.Term, bool) {
	if r == nil {
		return 0, false
	}
	return r.occurrenceExit(r.breakExit, term, keyspace.FamilyBreak, kind.OutcomeBreak)
}

// GotoExit resolves one authored Goto occurrence.  A same-Body Goto returns
// its Label directly; an outward Goto returns its first typed Goto Outcome.
func (r *Result) GotoExit(term keyspace.Term) (keyspace.Term, bool) {
	if r == nil || r.Count() == 0 || keyspace.TermFamily(term) != keyspace.FamilyGoto {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.gotoExit)) {
		return 0, false
	}
	exit := r.gotoExit[ordinal]
	if exit == 0 {
		return 0, false
	}
	if keyspace.TermFamily(exit) == keyspace.FamilyLabel {
		return exit, true
	}
	if keyspace.TermFamily(exit) != keyspace.FamilyOutcome {
		return 0, false
	}
	exitOrdinal, ok := r.outcomeOrdinal(exit)
	if !ok || !r.validStoredRow(exitOrdinal) || r.kinds[exitOrdinal] != kind.OutcomeGoto {
		return 0, false
	}
	return exit, true
}

func (r *Result) occurrenceExit(plane []keyspace.Term, term keyspace.Term, family keyspace.Family, expected kind.OutcomeKind) (keyspace.Term, bool) {
	if r == nil || r.Count() == 0 || keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(plane)) {
		return 0, false
	}
	exit := plane[ordinal]
	exitOrdinal, ok := r.outcomeOrdinal(exit)
	if !ok || !r.validStoredRow(exitOrdinal) || r.kinds[exitOrdinal] != expected {
		return 0, false
	}
	return exit, true
}

func (r *Result) outcomeOrdinal(term keyspace.Term) (uint32, bool) {
	if r == nil || !r.sourceID.Available() || !r.flowID.Available() || !r.staticID.Available() || !r.moduleID.Available() || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.bodies)) ||
		len(r.kinds) != len(r.bodies) || len(r.targets) != len(r.bodies) || len(r.propagation) != len(r.bodies) {
		return 0, false
	}
	if r.bodies[0] != 0 || r.kinds[0] != 0 || r.targets[0] != 0 || r.propagation[0] != 0 {
		return 0, false
	}
	return ordinal, true
}

func (r *Result) validRows() bool {
	if r == nil || len(r.bodies) == 0 || len(r.bodies) != len(r.kinds) ||
		len(r.bodies) != len(r.targets) || len(r.bodies) != len(r.propagation) {
		return false
	}
	// A published relation is not queryable until all four scalar provenance
	// fences are present. Without this check a zero-provenance value with
	// plausible rows would answer Count/At/Get/Find as if it were sealed;
	// callers could bypass Matches simply by retaining the value itself.
	return r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available() &&
		r.bodies[0] == 0 && r.kinds[0] == 0 && r.targets[0] == 0 && r.propagation[0] == 0
}

func (r *Result) validStoredRow(ordinal uint32) bool {
	if r == nil || ordinal == 0 || uint64(ordinal) >= uint64(len(r.bodies)) ||
		len(r.base[kind.OutcomeNormal]) < 2 ||
		uint64(keyspace.TermOrdinal(r.bodies[ordinal])) >= uint64(len(r.base[kind.OutcomeNormal])) ||
		!validBody(r.bodies[ordinal]) || !validKind(r.kinds[ordinal]) || !validTarget(r.kinds[ordinal], r.targets[ordinal]) {
		return false
	}
	return true
}

func validBody(body keyspace.Term) bool {
	return keyspace.TermFamily(body) == keyspace.FamilyBody && keyspace.TermOrdinal(body) != 0
}

func validKind(outcomeKind kind.OutcomeKind) bool {
	return outcomeKind >= kind.OutcomeNormal && outcomeKind <= kind.OutcomeCancel
}

func validTarget(outcomeKind kind.OutcomeKind, target keyspace.Term) bool {
	switch outcomeKind {
	case kind.OutcomeBreak:
		return keyspace.TermFamily(target) == keyspace.FamilyLoop
	case kind.OutcomeGoto:
		return keyspace.TermFamily(target) == keyspace.FamilyLabel
	default:
		return target == 0
	}
}

type outcomeKey struct {
	body   keyspace.Term
	kind   kind.OutcomeKind
	target keyspace.Term
}

func compareKey(left, right outcomeKey) int {
	if leftBody, rightBody := keyspace.TermOrdinal(left.body), keyspace.TermOrdinal(right.body); leftBody != rightBody {
		if leftBody < rightBody {
			return -1
		}
		return 1
	}
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	leftFamily, rightFamily := keyspace.TermFamily(left.target), keyspace.TermFamily(right.target)
	if leftFamily != rightFamily {
		if leftFamily < rightFamily {
			return -1
		}
		return 1
	}
	leftOrdinal, rightOrdinal := keyspace.TermOrdinal(left.target), keyspace.TermOrdinal(right.target)
	if leftOrdinal < rightOrdinal {
		return -1
	}
	if leftOrdinal > rightOrdinal {
		return 1
	}
	return 0
}
