// Package sendsafety decides whether one cross-actor send may hand its payload
// over without copying it.
//
// Send-safety is a derived judgment, not a placement axis. Placement owns
// where a value lives and what it is reachable from; this package reads that
// published answer and states what it means for a transfer. Nothing here
// re-derives a placement, and nothing here reads an artifact: the whole input
// is one allocation row of the placement summary plus the shape of the payload
// expression the Program already knows.
//
// # The arms
//
// A send is answered on exactly one of three arms:
//
//   - Immutable. The payload graph is deeply frozen, so no observer can tell a
//     shared reference from a copy. Aliasing is irrelevant to this arm, which
//     is why it is decided first.
//   - Isolated. The payload is the allocation's own birth site, its graph
//     contains no second identity, and placement proves it never left the
//     sending frame. The reference handed to the send is then the only one
//     that exists.
//   - CopyRequired. Placement proves that a retaining boundary precedes this
//     send, so a mutable payload cannot be transferred in place. This is a
//     positive provenance judgment.
//
// # Abstention
//
// An unanswered placement is not an arm. When the summary carries no row for
// the payload, or carries an Unknown class, this package answers nothing at
// all. Unknown retain provenance never becomes a mutable-copy decision; an
// independent deep-freeze proof may still establish Immutable.
package sendsafety

import (
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// Verdict is the closed set of answers this judgment publishes. A verdict this
// package cannot prove is not spelled here, so no default arm can invent it.
type Verdict uint8

const (
	// VerdictNone is the absence of an answer, not an answer. It is what an
	// unknown or unpublished placement produces.
	VerdictNone Verdict = iota
	// VerdictImmutable is a deeply frozen payload: zero-copy sharing is
	// admissible because no observer can distinguish sharing from copying.
	VerdictImmutable
	// VerdictIsolated is a solely owned payload: zero-copy transfer is
	// admissible because the sent reference is the only one that exists.
	VerdictIsolated
	// VerdictCopyRequired is a mutable payload with a proven prior retaining
	// escape. The runtime must copy before sealing/publication.
	VerdictCopyRequired
)

// Available reports whether the verdict is one this judgment decided.
func (verdict Verdict) Available() bool {
	return verdict >= VerdictImmutable && verdict <= VerdictCopyRequired
}

// Ordinal is the verdict's position in the declared vocabulary. It is the
// ordinal a diagnostic variant selects on, so it is the verdict's own number
// rather than a spelling a table restates.
func (verdict Verdict) Ordinal() uint16 { return uint16(verdict) }

// Catalog is the declared verdict vocabulary in ordinal order.
func Catalog() []Verdict {
	return []Verdict{VerdictImmutable, VerdictIsolated, VerdictCopyRequired}
}

// PayloadShape is what the Program knows about the expression in the payload
// position, which placement does not publish because it is not a placement
// question.
//
// The distinction it carries is the one between a value that is born at the
// send and a value that is named before it: an object literal written into the
// argument has no other reader by construction, while a local read may have
// aliases the sending frame keeps. Both can be frame-local, so placement alone
// cannot separate them.
type PayloadShape uint8

const (
	// PayloadShapeUnknown is a payload whose expression the Program did not
	// classify. It proves nothing.
	PayloadShapeUnknown PayloadShape = iota
	// PayloadShapeLiteralBirth is an object literal written directly into the
	// payload position: the send is the allocation's birth site.
	PayloadShapeLiteralBirth
	// PayloadShapeReference is a payload named by a read of something bound
	// earlier. The sending frame may keep a reader.
	PayloadShapeReference
)

// Subject is one send site as this judgment needs it: the allocation the
// payload names, the placement row published for that allocation, and the
// shape of the payload expression.
//
// The complete placement Fact is copied from
// placement.SummaryResultAllocation by NewSubject. A caller never rebuilds it
// from detached columns.
type Subject struct {
	// Allocation is the identity of the payload's allocation root.
	Allocation identity.ContentID
	// Answered reports that the summary published a row for Allocation. A
	// query miss leaves it false, which is abstention rather than a verdict.
	Answered bool
	// Fact is the complete canonical Placement value at the pre-effect point.
	// Class and retain provenance remain inseparable so this judgment cannot
	// observe a combination the Placement owner never wrote.
	Fact placement.Fact
	// Owner is the owner identity the placement row carries. A row whose owner
	// is not its own allocation is malformed, not an isolation proof.
	Owner identity.ContentID
	// Depth is the static containment depth of the payload graph. A depth
	// above zero means the graph reaches a second identity, which the send
	// would carry along and which this judgment cannot prove unaliased.
	Depth uint32
	// DepthKnown reports that Depth was published. An unpublished depth is not
	// a depth of zero.
	DepthKnown bool
	// FrameLocal is placement's proof that the allocation never left the
	// sending frame.
	FrameLocal placement.EvidenceState
	// DeepFrozen is placement's transitive immutability proof.
	DeepFrozen placement.EvidenceState
	// Shape is the payload expression's classification.
	Shape PayloadShape
}

// NewSubject projects one published placement allocation row into a subject.
// A row that does not decode, or whose identity is not the allocation asked
// for, yields an unanswered subject rather than a partially filled one.
func NewSubject(row placement.SummaryResultAllocation, shape PayloadShape) (Subject, bool) {
	if !row.Available() {
		return Subject{}, false
	}
	allocation := row.AllocationID()
	if !allocation.Available() {
		return Subject{}, false
	}
	fact, factOK := row.Fact()
	if !factOK {
		return Subject{}, false
	}
	subject := Subject{Allocation: allocation, Answered: true, Fact: fact, Shape: shape}
	if owner, ok := row.OwnerIdentity(); ok {
		subject.Owner = owner
	}
	if depth, ok := row.Depth(); ok {
		subject.Depth, subject.DepthKnown = depth, true
	}
	if state, ok := row.FrameLocal(); ok {
		subject.FrameLocal = state
	}
	if state, ok := row.DeepFrozen(); ok {
		subject.DeepFrozen = state
	}
	return subject, true
}

// NewObservedSubject projects one allocation directly from the transient
// Placement observation produced at a send's pre-effect point. Placement owns
// the complete row composition; send-safety only copies that authenticated
// result into its judgment input.
func NewObservedSubject(schema placement.Schema, observation placement.PlacementSummaryObservation, key heapdomain.Key, shape PayloadShape) (Subject, bool) {
	fact, evidence, rowOK := placement.PlacementSummaryAllocation(schema, observation, key)
	allocation, allocationOK := key.ContentID()
	if !rowOK || !allocationOK || !allocation.Available() || !evidence.Valid() || !evidence.HasOwnerIdentity || evidence.OwnerIdentity != allocation {
		return Subject{}, false
	}
	subject := Subject{
		Allocation: allocation,
		Answered:   true,
		Fact:       fact,
		Owner:      evidence.OwnerIdentity,
		Shape:      shape,
		FrameLocal: evidence.FrameLocal,
		DeepFrozen: evidence.DeepFrozen,
	}
	if evidence.HasDepth {
		subject.Depth, subject.DepthKnown = evidence.Depth, true
	}
	return subject, true
}

// answered reports that placement gave a usable answer for this allocation.
// An unpublished row, an absent class, and a class of Unknown are all the
// absence of an answer: Unknown is placement's semantic top, so it states that
// every placement remains possible, which proves nothing about a transfer.
func (subject Subject) answered() bool {
	return subject.Answered && subject.Allocation.Available() && subject.Fact.Valid() &&
		subject.Fact.RetainEscape != placement.EvidenceAbsent && analysisPlacement(subject.Fact.Class)
}

// analysisPlacement is the analysis half of the placement vocabulary: the
// three classes an allocation of a running program can carry. Bottom is
// unreachable, Unknown is the lattice top, and the JIT-only classes are not
// answers about a heap allocation at all, so none of them is an answer this
// judgment may read.
func analysisPlacement(class placement.Placement) bool {
	switch class {
	case placement.Stack, placement.OwnedHeap, placement.SharedHeap:
		return true
	default:
		return false
	}
}

// wellFormed rejects a row whose owner identity is not the allocation it
// describes. Such a row is malformed evidence, and malformed evidence is never
// read as a proof.
func (subject Subject) wellFormed() bool {
	return subject.Owner.Available() && subject.Owner == subject.Allocation
}

// immutable is the deep-freeze arm. It is decided before isolation because a
// frozen graph is admissible however many aliases it has.
func (subject Subject) immutable() bool {
	return subject.DeepFrozen.Proven()
}

// isolated is the sole-ownership arm. Every clause is a published proof:
// placement proves the allocation never left the frame, the containment depth
// proves the graph carries no second identity, and the payload shape proves
// the send is the allocation's birth site so no earlier binding can hold it.
//
// A missing depth is not a depth of zero and an unclassified payload shape is
// not a literal birth, so either one leaves this arm undecided.
func (subject Subject) isolated() bool {
	return subject.Fact.RetainEscape.Refuted() &&
		subject.FrameLocal.Proven() &&
		subject.DepthKnown && subject.Depth == 0 &&
		subject.Shape == PayloadShapeLiteralBirth
}

// copyRequired is the retaining-escape arm. It is published only when the
// graph is proven mutable and Placement proves a prior retaining boundary.
func (subject Subject) copyRequired() bool {
	return subject.DeepFrozen.Refuted() && subject.Fact.RetainEscape.Proven()
}

// Derive answers one send site.
//
// The order is the soundness order. Immutability is checked first because it
// is independent of aliasing; isolation second because it is the stronger
// claim about ownership; CopyRequired follows only from proven prior-retain
// provenance. Missing or ambiguous proof returns VerdictNone.
func Derive(subject Subject) Verdict {
	if !subject.answered() || !subject.wellFormed() {
		return VerdictNone
	}
	if subject.immutable() {
		return VerdictImmutable
	}
	if subject.isolated() {
		return VerdictIsolated
	}
	if subject.copyRequired() {
		return VerdictCopyRequired
	}
	return VerdictNone
}
