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
		Allocation: id, Answered: true, Present: true,
		Class: placement.Stack, Owner: id,
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
		t.Fatalf("fully owned frame-local literal birth = %v, want isolated", verdict.Spelling())
	}
}

// TestUnknownPlacementNeverYieldsSendSafe is the central soundness law:
// unknown is never laundered into a proof. Every way placement can fail to
// answer must produce no verdict at all, not a copy fallback and never an
// admission.
func TestUnknownPlacementNeverYieldsSendSafe(t *testing.T) {
	for _, testcase := range []struct {
		name   string
		mutate func(Subject) Subject
	}{
		{"class unknown is the lattice top", func(s Subject) Subject { s.Class = placement.Unknown; return s }},
		{"class bottom is unreachable", func(s Subject) Subject { s.Class = placement.Bottom; return s }},
		{"no row published for the allocation", func(s Subject) Subject { s.Answered = false; return s }},
		{"row carries no placement class", func(s Subject) Subject { s.Present = false; return s }},
		{"allocation identity unavailable", func(s Subject) Subject { s.Allocation = identity.ContentID{}; return s }},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			subject := testcase.mutate(ownedSubject(t))
			if verdict := Derive(subject); verdict != VerdictNone {
				t.Fatalf("unanswered placement = %v, want no verdict", verdict.Spelling())
			}
		})
	}
}

// TestUnknownPlacementAbstainsEvenWhenFrozen states that abstention outranks
// the immutability arm. A deep-freeze proof on an allocation placement never
// answered for is evidence about nothing.
func TestUnknownPlacementAbstainsEvenWhenFrozen(t *testing.T) {
	subject := ownedSubject(t)
	subject.Class = placement.Unknown
	subject.DeepFrozen = placement.EvidenceProven
	if verdict := Derive(subject); verdict != VerdictNone {
		t.Fatalf("frozen payload with unknown placement = %v, want no verdict", verdict.Spelling())
	}
}

// TestAliasedOrEscapingSubgraphRefusesIsolation is the refusal law. Each case
// removes exactly one clause of the isolation proof, and each must fall back
// to the copy verdict rather than admitting the transfer.
func TestAliasedOrEscapingSubgraphRefusesIsolation(t *testing.T) {
	for _, testcase := range []struct {
		name   string
		mutate func(Subject) Subject
	}{
		{"payload named before the send may keep a reader", func(s Subject) Subject { s.Shape = PayloadShapeReference; return s }},
		{"unclassified payload shape proves no birth site", func(s Subject) Subject { s.Shape = PayloadShapeUnknown; return s }},
		{"graph reaches a second identity", func(s Subject) Subject { s.Depth = 1; return s }},
		{"containment depth unpublished", func(s Subject) Subject { s.DepthKnown = false; return s }},
		{"frame locality refuted", func(s Subject) Subject {
			s.FrameLocal, s.Class = placement.EvidenceRefuted, placement.OwnedHeap
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
			if verdict != VerdictCopyFallback {
				t.Fatalf("%s = %v, want the copy fallback", testcase.name, verdict.Spelling())
			}
		})
	}
}

// TestSharedPlacementIsNeverReadAsAnEscapeProof pins the arm this package
// deliberately does not decide. A SharedHeap class is what an ordinary send
// produces, so it must not refuse the transfer and must not be mistaken for a
// proven escaping alias; the answered-but-unproven arm is the only honest one.
func TestSharedPlacementIsNeverReadAsAnEscapeProof(t *testing.T) {
	subject := ownedSubject(t)
	subject.Class = placement.SharedHeap
	subject.FrameLocal = placement.EvidenceRefuted
	if verdict := Derive(subject); verdict != VerdictCopyFallback {
		t.Fatalf("shared placement = %v, want the copy fallback", verdict.Spelling())
	}
	for _, verdict := range Catalog() {
		if verdict.Spelling() == "escaped_refuted" {
			t.Fatal("the escaping arm is declared-not-composed and must not be in the catalog")
		}
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
	subject.Class, subject.FrameLocal = placement.SharedHeap, placement.EvidenceRefuted
	if verdict := Derive(subject); verdict != VerdictImmutable {
		t.Fatalf("deeply frozen aliased payload = %v, want immutable", verdict.Spelling())
	}
}

// TestVerdictVocabularyIsClosed holds the catalog against the decidable arms.
func TestVerdictVocabularyIsClosed(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 3 {
		t.Fatalf("catalog holds %d arms, want the three decidable ones", len(catalog))
	}
	seen := make(map[string]struct{}, len(catalog))
	for _, verdict := range catalog {
		if !verdict.Available() || verdict.Spelling() == "" {
			t.Fatalf("catalog member %d is not a decided arm", verdict.Ordinal())
		}
		if _, duplicate := seen[verdict.Spelling()]; duplicate {
			t.Fatalf("catalog spells %q twice", verdict.Spelling())
		}
		seen[verdict.Spelling()] = struct{}{}
	}
	if VerdictNone.Available() || VerdictNone.Spelling() != "" {
		t.Fatal("the absence of an answer must not be a decided arm")
	}
}
