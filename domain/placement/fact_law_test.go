package placement

import (
	"fmt"
	"testing"
)

func TestFactStringCoversValidProductAndInvalidComponents(t *testing.T) {
	classes := []Placement{Bottom, Stack, OwnedHeap, SharedHeap, Unknown}
	evidence := []EvidenceState{EvidenceAbsent, EvidenceRefuted, EvidenceUnknown, EvidenceProven}
	evidenceNames := map[EvidenceState]string{
		EvidenceAbsent:  "absent",
		EvidenceRefuted: "refuted",
		EvidenceUnknown: "unknown",
		EvidenceProven:  "proven",
	}
	seen := make(map[string]Fact, len(classes)*len(evidence))

	for _, class := range classes {
		for _, retain := range evidence {
			fact := Fact{Class: class, RetainEscape: retain}
			if !fact.Valid() {
				t.Fatalf("valid product fact rejected: %#v", fact)
			}
			got := fmt.Sprintf("%s", fact)
			want := "class=" + class.String() + "/retain=" + evidenceNames[retain]
			if got != want {
				t.Errorf("Fact.String() = %q for %#v, want %q", got, fact, want)
			}
			if previous, duplicate := seen[got]; duplicate && previous != fact {
				t.Errorf("distinct valid facts share diagnostic spelling %q: %#v and %#v", got, previous, fact)
			}
			seen[got] = fact
		}
	}

	invalid := []struct {
		fact Fact
		want string
	}{
		{Fact{Class: invalidPlacementResult, RetainEscape: EvidenceUnknown}, "class=" + invalidPlacementResult.String() + "/retain=unknown"},
		{Fact{Class: Stack, RetainEscape: InvalidEvidenceState()}, "class=stack/retain=invalid"},
		{Fact{Class: invalidPlacementResult, RetainEscape: InvalidEvidenceState()}, "class=" + invalidPlacementResult.String() + "/retain=invalid"},
	}
	for _, test := range invalid {
		if got := fmt.Sprintf("%s", test.fact); got != test.want {
			t.Errorf("invalid Fact.String() = %q for %#v, want %q", got, test.fact, test.want)
		}
	}
}

func TestFactRetainEscapeIsPathSensitiveAndOwnedByDisplacement(t *testing.T) {
	baseline := DefaultFact()
	if baseline.Class != Stack || baseline.RetainEscape != EvidenceRefuted || !baseline.Valid() {
		t.Fatalf("default fact = %#v, want Stack with an authenticated no-prior-retain proof", baseline)
	}

	retained, ok := DisplaceFactChecked(baseline, Retain)
	if !ok || retained.Class != OwnedHeap || retained.RetainEscape != EvidenceProven {
		t.Fatalf("retained fact = %#v/%t, want OwnedHeap with proven retain escape", retained, ok)
	}

	sent, ok := DisplaceFactChecked(baseline, Send)
	if !ok || sent.Class != SharedHeap || sent.RetainEscape != EvidenceProven {
		t.Fatalf("sent fact = %#v/%t, want SharedHeap with proven retain escape after the effect", sent, ok)
	}

	joined, ok := JoinFactChecked(baseline, sent)
	if !ok || joined.Class != SharedHeap || joined.RetainEscape != EvidenceUnknown {
		t.Fatalf("branch join = %#v/%t, want SharedHeap with path-ambiguous retain escape", joined, ok)
	}

	// Sampling the predecessor is deliberately just a read. Applying the
	// current send creates a successor fact and must not mutate or compensate
	// the pre-effect answer.
	if baseline.RetainEscape != EvidenceRefuted {
		t.Fatalf("pre-effect fact changed after send displacement: %#v", baseline)
	}
}

func TestFactLatticeIsTheProductOfPlacementAndRetainEvidence(t *testing.T) {
	bottom := BottomFact()
	top := UnknownFact()
	baseline := DefaultFact()
	retained, ok := DisplaceFactChecked(baseline, Retain)
	if !ok {
		t.Fatal("retain displacement")
	}

	domain := FactLattice()
	if !domain.Equal(domain.Join(bottom, baseline), baseline) {
		t.Fatalf("bottom join baseline = %#v, want %#v", domain.Join(bottom, baseline), baseline)
	}
	if !domain.Equal(domain.Join(baseline, retained), Fact{Class: OwnedHeap, RetainEscape: EvidenceUnknown}) {
		t.Fatalf("mixed-path join = %#v", domain.Join(baseline, retained))
	}
	if !domain.Equal(domain.Join(top, baseline), top) {
		t.Fatalf("top join baseline = %#v, want %#v", domain.Join(top, baseline), top)
	}
}
