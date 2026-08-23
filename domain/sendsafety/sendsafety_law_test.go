package sendsafety

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
)

func allocationID(t *testing.T, seed string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("domain/sendsafety/law/allocation", []byte(seed))
	if !ok || !id.Available() {
		t.Fatalf("allocation identity %q unavailable", seed)
	}
	return id
}

// ownedSubject is the fully proven isolated payload every law below perturbs:
// a literal born at the send, frame-local, with a graph that reaches no second
// identity.
func ownedSubject(t *testing.T) Subject {
	t.Helper()
	id := allocationID(t, "owned")
	return Subject{
		Allocation: id, Answered: true,
		Fact:  placement.DefaultFact(),
		Owner: id,
		Depth: 0, DepthKnown: true,
		FrameLocal: placement.EvidenceProven,
		DeepFrozen: placement.EvidenceUnknown,
		Shape:      PayloadShapeLiteralBirth,
	}
}

// TestOwnedAdmissiblePayloadIsIsolated is the positive law: when every clause
// of the isolation proof is published, the judgment admits the zero-copy
// transfer.
func TestOwnedAdmissiblePayloadIsIsolated(t *testing.T) {
	if verdict := Derive(ownedSubject(t)); verdict != VerdictIsolated {
		t.Fatalf("fully owned frame-local literal birth = %d, want isolated", verdict)
	}
}

// TestUnknownPlacementNeverYieldsSendSafe is the central soundness law:
// unknown is never laundered into a proof. Every way placement can fail to
// answer must produce no verdict at all and never an admission.
func TestUnknownPlacementNeverYieldsSendSafe(t *testing.T) {
	for _, testcase := range []struct {
		name   string
		mutate func(Subject) Subject
	}{
		{"class unknown is the lattice top", func(s Subject) Subject { s.Fact = placement.UnknownFact(); return s }},
		{"class bottom is unreachable", func(s Subject) Subject { s.Fact = placement.BottomFact(); return s }},
		{"no row published for the allocation", func(s Subject) Subject { s.Answered = false; return s }},
		{"row carries no placement fact", func(s Subject) Subject { s.Fact = placement.Fact{}; return s }},
		{"allocation identity unavailable", func(s Subject) Subject { s.Allocation = identity.ContentID{}; return s }},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			subject := testcase.mutate(ownedSubject(t))
			if verdict := Derive(subject); verdict != VerdictNone {
				t.Fatalf("unanswered placement = %d, want no verdict", verdict)
			}
		})
	}
}

// TestUnknownPlacementAbstainsEvenWhenFrozen states that abstention outranks
// the immutability arm. A deep-freeze proof on an allocation placement never
// answered for is evidence about nothing.
func TestUnknownPlacementAbstainsEvenWhenFrozen(t *testing.T) {
	subject := ownedSubject(t)
	subject.Fact = placement.UnknownFact()
	subject.DeepFrozen = placement.EvidenceProven
	if verdict := Derive(subject); verdict != VerdictNone {
		t.Fatalf("frozen payload with unknown placement = %d, want no verdict", verdict)
	}
}

// TestUnprovenIsolationAbstains is the refusal law. Each case removes exactly
// one clause of the isolation proof, and none may invent a copy decision.
func TestUnprovenIsolationAbstains(t *testing.T) {
	for _, testcase := range []struct {
		name   string
		mutate func(Subject) Subject
	}{
		{"payload named before the send may keep a reader", func(s Subject) Subject { s.Shape = PayloadShapeReference; return s }},
		{"unclassified payload shape proves no birth site", func(s Subject) Subject { s.Shape = PayloadShapeUnknown; return s }},
		{"graph reaches a second identity", func(s Subject) Subject { s.Depth = 1; return s }},
		{"containment depth unpublished", func(s Subject) Subject { s.DepthKnown = false; return s }},
		{"frame locality refuted", func(s Subject) Subject {
			s.FrameLocal, s.Fact.Class = placement.EvidenceRefuted, placement.OwnedHeap
			return s
		}},
		{"frame locality unknown", func(s Subject) Subject { s.FrameLocal = placement.EvidenceUnknown; return s }},
		{"owner identity is not the allocation", func(s Subject) Subject {
			s.Owner = allocationID(t, "other")
			return s
		}},
		{"owner identity absent", func(s Subject) Subject { s.Owner = identity.ContentID{}; return s }},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			subject := testcase.mutate(ownedSubject(t))
			verdict := Derive(subject)
			if verdict == VerdictIsolated {
				t.Fatalf("%s still admitted a zero-copy transfer", testcase.name)
			}
			if verdict != VerdictNone {
				t.Fatalf("%s = %d, want no verdict", testcase.name, verdict)
			}
		})
	}
}

// TestSharedPlacementIsNeverReadAsRetainProvenance pins the canonical split:
// the class alone decides nothing, while the Fact's retain component can
// positively require a copy.
func TestSharedPlacementIsNeverReadAsRetainProvenance(t *testing.T) {
	subject := ownedSubject(t)
	subject.Fact.Class = placement.SharedHeap
	subject.FrameLocal = placement.EvidenceRefuted
	if verdict := Derive(subject); verdict != VerdictNone {
		t.Fatalf("shared class without retain proof = %d, want no verdict", verdict)
	}
	subject.Fact.RetainEscape = placement.EvidenceProven
	subject.DeepFrozen = placement.EvidenceRefuted
	if verdict := Derive(subject); verdict != VerdictCopyRequired {
		t.Fatalf("shared class with prior retain = %d, want copy_required", verdict)
	}
}

func TestUnknownMutabilityDoesNotInventCopyRequired(t *testing.T) {
	subject := ownedSubject(t)
	subject.Fact = placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	subject.Shape = PayloadShapeReference
	if verdict := Derive(subject); verdict != VerdictNone {
		t.Fatalf("unknown mutability with prior retain = %d, want no verdict", verdict)
	}
}

// TestFrozenPayloadIsImmutableRegardlessOfAliasing states the arm order. A
// deeply frozen graph is admissible however it is named and however many
// identities it reaches, so the immutability proof outranks every clause the
// isolation arm requires.
func TestFrozenPayloadIsImmutableRegardlessOfAliasing(t *testing.T) {
	subject := ownedSubject(t)
	subject.DeepFrozen = placement.EvidenceProven
	subject.Shape = PayloadShapeReference
	subject.Depth, subject.DepthKnown = 3, true
	subject.Fact = placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	subject.FrameLocal = placement.EvidenceRefuted
	if verdict := Derive(subject); verdict != VerdictImmutable {
		t.Fatalf("deeply frozen aliased payload = %d, want immutable", verdict)
	}
}

// TestVerdictVocabularyIsClosed holds the catalog against the decidable arms.
func TestVerdictVocabularyIsClosed(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 3 {
		t.Fatalf("catalog holds %d arms, want the three decidable ones", len(catalog))
	}
	for index, verdict := range catalog {
		if !verdict.Available() || verdict.Ordinal() != uint16(index+1) {
			t.Fatalf("catalog[%d] = %d/%d", index, verdict, verdict.Ordinal())
		}
	}
	if VerdictNone.Available() || VerdictNone.Ordinal() != 0 {
		t.Fatal("the absence of an answer must not be a decided arm")
	}
}
