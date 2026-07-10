package userlattice

import (
	"strings"
	"testing"
)

func TestVerifyTaintLatticeComputesJoin(t *testing.T) {
	verified, err := Verify(taintSpec())
	if err != nil {
		t.Fatalf("Verify(taint) error = %v", err)
	}
	rtAxis := Axis{spec: verified}
	sanitized, _ := rtAxis.Element("Sanitized")
	tainted, _ := rtAxis.Element("Tainted")
	if got := rtAxis.ElementName(rtAxis.Join(sanitized, tainted)); got != "Unknown" {
		t.Fatalf("Sanitized join Tainted = %s, want Unknown", got)
	}
}

func TestVerifyRejectsMissingLUB(t *testing.T) {
	spec := Spec{
		ID:       "test.missing-lub",
		Elements: []ElementID{"Bottom", "A", "B", "C", "D", "Top"},
		Bottom:   "Bottom",
		Top:      "Top",
		Order: []OrderPair{
			{"Bottom", "A"},
			{"Bottom", "B"},
			{"A", "C"},
			{"B", "C"},
			{"A", "D"},
			{"B", "D"},
			{"C", "Top"},
			{"D", "Top"},
		},
	}
	_, err := Verify(spec)
	if err == nil || !strings.Contains(err.Error(), "has no least upper bound") {
		t.Fatalf("Verify missing-lub error = %v, want missing lub", err)
	}
}

func TestVerifyRejectsCycle(t *testing.T) {
	spec := Spec{
		ID:       "test.cycle",
		Elements: []ElementID{"A", "B"},
		Bottom:   "A",
		Top:      "B",
		Order: []OrderPair{
			{"A", "B"},
			{"B", "A"},
		},
	}
	_, err := Verify(spec)
	if err == nil || !strings.Contains(err.Error(), "order cycle") {
		t.Fatalf("Verify cycle error = %v, want cycle", err)
	}
}

func TestVerifyRejectsNonMonotoneMap(t *testing.T) {
	spec := taintSpec()
	spec.ID = "test.nonmonotone"
	spec.Hooks.OnAssign.Map = []ElementMapEntry{
		{From: "Untainted", To: "Tainted"},
		{From: "Sanitized", To: "Sanitized"},
	}
	_, err := Verify(spec)
	if err == nil || !strings.Contains(err.Error(), "assign map is not monotone") {
		t.Fatalf("Verify nonmonotone map error = %v, want monotone rejection", err)
	}
}

func taintSpec() Spec {
	return Spec{
		ID:       "test.taint",
		Elements: []ElementID{"Untainted", "Sanitized", "Tainted", "Unknown"},
		Bottom:   "Untainted",
		Top:      "Unknown",
		Order: []OrderPair{
			{"Untainted", "Sanitized"},
			{"Untainted", "Tainted"},
			{"Sanitized", "Unknown"},
			{"Tainted", "Unknown"},
		},
		Hooks: Hooks{
			OnAssign:       AssignHook{Mode: AssignPropagate},
			OnCallBoundary: CallBoundaryHook{Mode: CallBoundaryKeep},
			OnClaim: []ClaimHook{
				{Claim: "tainted", Element: "Tainted"},
				{Claim: "sanitized", Element: "Sanitized"},
			},
		},
	}
}
