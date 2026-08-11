package recurrence

import "github.com/wippyai/go-lua/program/keyspace"

// Annotation is the recurrence projection for one sourcecontrol Arc. The
// slice is aligned with sourcecontrol.Result.ArcAt: an ordinary Arc has a
// zero Head, while a recurrent Arc has an existing dynamic Label or Loop
// Head. First and Past are local indexes in that Head's decision stream; the
// half-open range may be empty and is still a real recurrence.
type Annotation struct {
	Head  keyspace.Term
	First uint32
	Past  uint32
}

// Result is the immutable, assembly-local recurrence proof. The arrays are
// private so no caller can retain a second SCC, stream, or reset-set
// authority. All query methods fail closed and perform no allocation.
type Result struct {
	annotations []Annotation

	// The owner identities are the narrow splice fence for this projection.
	// No Source, Flow, or sourcecontrol authority is retained here.
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID

	// streams is the concatenation of one semantic decision stream per Mu
	// head. headSlots are dense by existing Label/Loop identity and are the
	// only lookup index retained for stream queries.
	streams   []keyspace.Term
	headSlots [keyspace.FamilyCount][]headSlot

	// A decision slot is installed exactly once while sealing the stream.
	// These are dense typed projections, not a generic key map or a second
	// graph. A zero head means the decision is not in a reachable cyclic
	// component.
	decisionSlots [keyspace.FamilyCount][]decisionSlot
}

// Matches reports whether r was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by the final assembly.
func Matches(r *Result, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

type headSlot struct {
	start, end uint32
	live       bool
}

type decisionSlot struct {
	head keyspace.Term
	rank uint32
}

// ArcCount reports the exact sourcecontrol Arc denominator consumed by the
// recurrence pass.
func (r *Result) ArcCount() int {
	if !available(r) {
		return 0
	}
	return len(r.annotations)
}

// ArcAt returns the annotation aligned with one sourcecontrol Arc ordinal.
func (r *Result) ArcAt(index int) (Annotation, bool) {
	if !available(r) || index < 0 || index >= len(r.annotations) {
		return Annotation{}, false
	}
	return r.annotations[index], true
}

// ResetCount reports the exact number of decision slots discharged by one
// recurrent Arc. An empty range is reported as zero with ok=true.
func (r *Result) ResetCount(index int) (int, bool) {
	annotation, ok := r.ArcAt(index)
	if !ok || annotation.Head == 0 || annotation.Past < annotation.First {
		return 0, false
	}
	start, end, ok := r.headRange(annotation.Head)
	if !ok || annotation.Past > end-start {
		return 0, false
	}
	return int(annotation.Past - annotation.First), true
}

// ResetAt returns one decision from a recurrent Arc's local stream range.
func (r *Result) ResetAt(index, offset int) (keyspace.Term, bool) {
	annotation, ok := r.ArcAt(index)
	if !ok || annotation.Head == 0 || annotation.First > annotation.Past || offset < 0 ||
		uint64(offset) >= uint64(annotation.Past-annotation.First) {
		return 0, false
	}
	start, end, ok := r.headRange(annotation.Head)
	if !ok || uint64(end-start) < uint64(annotation.Past) {
		return 0, false
	}
	position := start + annotation.First + uint32(offset)
	if position >= end || uint64(position) >= uint64(len(r.streams)) {
		return 0, false
	}
	term := r.streams[position]
	if !r.validDecision(term) {
		return 0, false
	}
	return term, true
}

// ResetContains answers the exact edge-local reset predicate in O(1). It
// does not enumerate absent decisions and does not allocate.
func (r *Result) ResetContains(index int, decision keyspace.Term) bool {
	annotation, ok := r.ArcAt(index)
	if !ok || annotation.Head == 0 || annotation.First > annotation.Past || !r.validDecision(decision) {
		return false
	}
	family, ordinal := keyspace.TermFamily(decision), keyspace.TermOrdinal(decision)
	if uint64(ordinal) >= uint64(len(r.decisionSlots[family])) ||
		r.decisionSlots[family][ordinal].head != annotation.Head {
		return false
	}
	rank := r.decisionSlots[family][ordinal].rank
	start, end, ok := r.headRange(annotation.Head)
	if !ok || uint64(rank) >= uint64(end-start) {
		return false
	}
	return annotation.First <= rank && rank < annotation.Past
}

// DecisionCount reports the sealed semantic stream for an existing Mu head.
func (r *Result) DecisionCount(head keyspace.Term) (int, bool) {
	if !available(r) {
		return 0, false
	}
	start, end, ok := r.headRange(head)
	if !ok || end < start || uint64(end) > uint64(len(r.streams)) {
		return 0, false
	}
	return int(end - start), true
}

// DecisionAt returns one decision from an existing Mu head's semantic stream.
func (r *Result) DecisionAt(head keyspace.Term, index int) (keyspace.Term, bool) {
	if !available(r) {
		return 0, false
	}
	start, end, ok := r.headRange(head)
	if !ok || index < 0 || uint64(index) >= uint64(end-start) {
		return 0, false
	}
	term := r.streams[start+uint32(index)]
	if !r.validDecision(term) {
		return 0, false
	}
	return term, true
}

func (r *Result) headRange(head keyspace.Term) (uint32, uint32, bool) {
	if !available(r) {
		return 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(head), keyspace.TermOrdinal(head)
	if (family != keyspace.FamilyLabel && family != keyspace.FamilyLoop) || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(r.headSlots[family])) || !r.headSlots[family][ordinal].live {
		return 0, 0, false
	}
	slot := r.headSlots[family][ordinal]
	start, end := slot.start, slot.end
	if end < start || uint64(end) > uint64(len(r.streams)) {
		return 0, 0, false
	}
	return start, end, true
}

func (r *Result) validDecision(term keyspace.Term) bool {
	if !available(r) {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family != keyspace.FamilySelect && family != keyspace.FamilyBranch && family != keyspace.FamilyLoop || ordinal == 0 ||
		uint64(ordinal) >= uint64(len(r.decisionSlots[family])) {
		return false
	}
	return r.decisionSlots[family][ordinal].head != 0
}

func available(r *Result) bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}
