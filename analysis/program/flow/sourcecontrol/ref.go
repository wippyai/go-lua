package sourcecontrol

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// NodeRef and ArcRef are opaque owner-fenced structural capabilities.  Their
// dense ordinals remain private; neutral route planning can retain/pass them
// but cannot inspect or manufacture graph coordinates.
type NodeRef struct {
	result *Result
	node   uint32
}

// PhaseRef is the opaque path handle used by synthetic causal routes. Unlike
// NodeRef it can name a parent-issued Outcome phase, which is not a CSR node.
// It contains no coordinate or authored term.
type PhaseRef struct {
	result *Result
	path   identity.ContentID
	class  phaseClass
	// node is retained only for an exact CSR phase. It is never exposed as an
	// ordinal; recurrence uses ResolveCSRPhaseNode to prove that a phase-only
	// Outcome chain rejoins the already-proven component at its final resume.
	node uint32
}

type phaseClass uint8

const (
	phaseInvalid phaseClass = iota
	phaseCSR
	phaseOutcome
)

type ArcRef struct {
	result *Result
	index  uint32
}

// Owner is an opaque exact sourcecontrol authority for one seal. It lets the
// neutral route-plan builder reject foreign ArcRef/NodeRef capabilities before
// recurrence, without exposing any graph identity or physical coordinate.
type Owner struct{ result *Result }

func (r *Result) Owner() Owner {
	if r == nil || !r.available() {
		return Owner{}
	}
	return Owner{result: r}
}
func (owner Owner) Available() bool { return owner.result != nil && owner.result.available() }
func (owner Owner) OwnsArcRef(ref ArcRef) bool {
	return owner.Available() && ref.Available() && owner.result == ref.result
}
func (owner Owner) OwnsNodeRef(ref NodeRef) bool {
	return owner.Available() && ref.Available() && owner.result == ref.result
}
func (owner Owner) OwnsPhaseRef(ref PhaseRef) bool {
	return owner.Available() && ref.Available() && owner.result == ref.result
}
func (r *Result) OwnsOwner(owner Owner) bool {
	return r != nil && r.available() && owner.Available() && owner.result == r
}

func (ref NodeRef) Available() bool {
	return ref.result != nil && ref.result.available() && ref.node < ref.result.NodeCount()
}
func (ref PhaseRef) Available() bool {
	return ref.result != nil && ref.result.available() && ref.path.Available() &&
		(ref.class == phaseCSR || ref.class == phaseOutcome)
}
func (ref PhaseRef) OutcomePhase() bool {
	return ref.Available() && ref.class == phaseOutcome
}
func (ref ArcRef) Available() bool {
	return ref.result != nil && ref.result.available() && uint64(ref.index) < uint64(len(ref.result.witnesses.rows))
}
func SameOwner(left, right NodeRef) bool {
	return left.Available() && right.Available() && left.result == right.result
}
func SamePhaseOwner(left, right PhaseRef) bool {
	return left.Available() && right.Available() && left.result == right.result
}

// SameArcRef is opaque exact-reference equality. It is available only to
// Flow's internal seal phases, so no physical Arc ordinal becomes public.
func SameArcRef(left, right ArcRef) bool {
	return left.Available() && right.Available() && left.result == right.result && left.index == right.index
}

// ArcRefAt issues the exact structural Arc witness at one seal-local ordinal.
func (r *Result) ArcRefAt(index int) (ArcRef, bool) {
	if !r.available() || index < 0 || index >= len(r.witnesses.rows) {
		return ArcRef{}, false
	}
	return ArcRef{result: r, index: uint32(index)}, true
}

// BodyEntryRef issues Cursor(body,0); BodyTailRef issues Tail(body).
func (r *Result) BodyEntryRef(body keyspace.Term) (NodeRef, bool) {
	node, ok := r.Cursor(body, 0)
	if !ok {
		return NodeRef{}, false
	}
	return NodeRef{result: r, node: node}, true
}
func (r *Result) BodyTailRef(body keyspace.Term) (NodeRef, bool) {
	node, ok := r.Tail(body)
	if !ok {
		return NodeRef{}, false
	}
	return NodeRef{result: r, node: node}, true
}

func (r *Result) phaseRefAt(node uint32) (PhaseRef, bool) {
	path, ok := r.VertexPathAt(node)
	if !ok {
		return PhaseRef{}, false
	}
	return PhaseRef{result: r, path: path, class: phaseCSR, node: node}, true
}

func (r *Result) BodyEntryPhase(body keyspace.Term) (PhaseRef, bool) {
	node, ok := r.Cursor(body, 0)
	if !ok {
		return PhaseRef{}, false
	}
	return r.phaseRefAt(node)
}

func (r *Result) BodyTailPhase(body keyspace.Term) (PhaseRef, bool) {
	node, ok := r.Tail(body)
	if !ok {
		return PhaseRef{}, false
	}
	return r.phaseRefAt(node)
}

// repeatEntryNode is the authored Loop cursor of a Repeat: the node a
// predecessor enters. The generic Loop coordinate denotes the post-body
// hidden decision, which a route entering the Repeat never reaches.
func (r *Result) repeatEntryNode(view source.View, term keyspace.Term) (uint32, bool) {
	if !r.available() || keyspace.TermFamily(term) != keyspace.FamilyLoop {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.coordinates.repeatLoop)) || !r.coordinates.repeatLoop[ordinal] {
		return 0, false
	}
	body, _, cursor, ok := view.Index().Position(term)
	if !ok || cursor < 0 {
		return 0, false
	}
	return r.Cursor(body, uint32(cursor))
}

// EntryPhase is the phase a route entering term reaches.
func (r *Result) EntryPhase(view source.View, term keyspace.Term) (PhaseRef, bool) {
	if node, ok := r.repeatEntryNode(view, term); ok {
		return r.phaseRefAt(node)
	}
	return r.CoordinatePhase(view, term)
}

// EntryRef is the CSR node a route entering term reaches.
func (r *Result) EntryRef(view source.View, term keyspace.Term) (NodeRef, bool) {
	if node, ok := r.repeatEntryNode(view, term); ok {
		return NodeRef{result: r, node: node}, true
	}
	return r.CoordinateRef(view, term)
}

func (r *Result) CoordinatePhase(view source.View, term keyspace.Term) (PhaseRef, bool) {
	node, ok := r.Coordinate(view, term)
	if !ok {
		return PhaseRef{}, false
	}
	return r.phaseRefAt(node)
}

// CoordinateRef issues the existing occurrence coordinate through Source's
// owner fence; callers cannot turn a raw term into a NodeRef themselves.
func (r *Result) CoordinateRef(view source.View, term keyspace.Term) (NodeRef, bool) {
	node, ok := r.Coordinate(view, term)
	if !ok {
		return NodeRef{}, false
	}
	return NodeRef{result: r, node: node}, true
}

// ResolveArcRoutePhases binds one ordinary logical route to the exact CSR
// endpoints of its structural Arc carrier. Most logical endpoints resolve
// directly to those vertices. Dynamic Loop geometry has two closed typed
// exceptions: numeric/generic iteration denotes the hidden decision node,
// while Repeat's initial Loop->Body edge starts at the authored root cursor
// even though subsequent references to the Loop denote its post-body hidden
// decision.
func (r *Result) ResolveArcRoutePhases(view source.View, outcomes *outcome.Result, fromTerm, toTerm keyspace.Term, ref ArcRef) (PhaseRef, PhaseRef, bool) {
	if r == nil || !r.available() || view.Identity().ContentID() != r.sourceID ||
		!outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return PhaseRef{}, PhaseRef{}, false
	}
	_, arc, arcOK := r.ResolveArcRef(ref)
	logicalFrom, fromOK := r.ResolveRouteEndpoint(view, outcomes, fromTerm, true)
	logicalTo, toOK := r.ResolveRouteEndpoint(view, outcomes, toTerm, false)
	physicalFrom, physicalFromOK := r.phaseRefAt(arc.From)
	physicalTo, physicalToOK := r.phaseRefAt(arc.To)
	if !arcOK || !fromOK || !toOK || logicalFrom.OutcomePhase() || logicalTo.OutcomePhase() ||
		!physicalFromOK || !physicalToOK {
		return PhaseRef{}, PhaseRef{}, false
	}
	if logicalFrom.path != physicalFrom.path {
		decision, decisionOK := r.Decision(fromTerm)
		repeatInitial := false
		if decisionOK && keyspace.TermFamily(toTerm) == keyspace.FamilyBody {
			bodyOrdinal := keyspace.TermOrdinal(toTerm)
			repeatInitial = bodyOrdinal != 0 && uint64(bodyOrdinal) < uint64(len(r.coordinates.repeatBody)) &&
				r.coordinates.repeatBody[bodyOrdinal] == decision && arc.Source == fromTerm && arc.Target == toTerm &&
				arc.Decision == 0 && !arc.Truth && arc.From != decision
		}
		loopDecision := decisionOK && decision == arc.From && arc.Source == fromTerm && arc.Decision == fromTerm
		directBodyAnchor, anchorOK := r.Coordinate(view, fromTerm)
		directBodyEntry := keyspace.TermFamily(fromTerm) == keyspace.FamilyBody && anchorOK && arc.From == directBodyAnchor &&
			arc.Source == fromTerm && arc.Target == fromTerm && arc.Decision == 0 && !arc.Truth
		if keyspace.TermFamily(fromTerm) != keyspace.FamilyLoop && !directBodyEntry ||
			keyspace.TermFamily(fromTerm) == keyspace.FamilyLoop && !loopDecision && !repeatInitial {
			return PhaseRef{}, PhaseRef{}, false
		}
	}
	if logicalTo.path != physicalTo.path {
		decision, decisionOK := r.Decision(toTerm)
		directBodyAnchor, anchorOK := r.Coordinate(view, toTerm)
		directBodyTarget := keyspace.TermFamily(toTerm) == keyspace.FamilyBody && anchorOK && arc.To == directBodyAnchor && arc.Target == toTerm && arc.Decision == 0 && !arc.Truth
		loopDecision := keyspace.TermFamily(toTerm) == keyspace.FamilyLoop && decisionOK && decision == arc.To && arc.Target == toTerm
		// A predecessor entering Repeat targets the authored Loop cursor. The
		// generic Loop coordinate denotes Repeat's post-body hidden decision,
		// so distinguish an external entry Arc from the Body feedback Arc
		// (whose physical target is that decision). The entry may be guarded;
		// for example, a preceding Loop/Branch can select this Repeat arm.
		repeatEntry := keyspace.TermFamily(toTerm) == keyspace.FamilyLoop && decisionOK && arc.Target == toTerm &&
			arc.To != decision
		if !directBodyTarget && !loopDecision && !repeatEntry {
			return PhaseRef{}, PhaseRef{}, false
		}
	}
	return physicalFrom, physicalTo, true
}

// ResolveNodeRef and ResolveArcRef are internal-owner validation gates for
// recurrence. They reject a capability issued by another sourcecontrol Result.
func (r *Result) ResolveNodeRef(ref NodeRef) (uint32, bool) {
	if !r.available() || !ref.Available() || ref.result != r {
		return 0, false
	}
	return ref.node, true
}

// ResolvePhaseRef is recurrence's exact-owner gate for a synthetic route
// endpoint. It intentionally returns only the sealed semantic path.
func (r *Result) ResolvePhaseRef(ref PhaseRef) (identity.ContentID, bool) {
	if !r.available() || !ref.Available() || ref.result != r {
		return identity.ContentID{}, false
	}
	return ref.path, true
}

// ResolveCSRPhaseNode proves that ref is an exact existing CSR phase and
// returns its private coordinate only to recurrence while SourceControl is
// live. Outcome phases deliberately have no coordinate and fail closed.
func (r *Result) ResolveCSRPhaseNode(ref PhaseRef) (uint32, bool) {
	if !r.available() || !ref.Available() || ref.result != r || ref.class != phaseCSR || ref.node >= r.NodeCount() {
		return 0, false
	}
	path, ok := r.VertexPathAt(ref.node)
	if !ok || path != ref.path {
		return 0, false
	}
	return ref.node, true
}
func (r *Result) ResolveArcRef(ref ArcRef) (int, Arc, bool) {
	if !r.available() || !ref.Available() || ref.result != r {
		return 0, Arc{}, false
	}
	arc, ok := r.ArcAt(int(ref.index))
	return int(ref.index), arc, ok
}
