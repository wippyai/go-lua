package placement

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lattice"
)

func TestPlacementLatticeLaws(t *testing.T) {
	suite := lattice.LawSuite[Placement]{
		Name:   "placement",
		Domain: Lattice(),
		Sample: []Placement{
			Bottom,
			Stack,
			OwnedHeap,
			SharedHeap,
			Unknown,
		},
		Format: func(v Placement) string {
			return fmt.Sprintf("%v", v)
		},
	}
	suite.Run(t)
}

func TestPlacementOrderJoinMeetAndWiden(t *testing.T) {
	domain := Lattice()

	if got := domain.Bottom(); got != Bottom {
		t.Fatalf("bottom = %v, want %v", got, Bottom)
	}
	if got := domain.Top(); got != Unknown {
		t.Fatalf("top = %v, want %v", got, Unknown)
	}

	cases := []struct {
		left, right Placement
		join        Placement
		meet        Placement
	}{
		{Bottom, Stack, Stack, Bottom},
		{Stack, OwnedHeap, OwnedHeap, Stack},
		{OwnedHeap, SharedHeap, SharedHeap, OwnedHeap},
		{SharedHeap, Unknown, Unknown, SharedHeap},
	}
	for _, tc := range cases {
		if got := domain.Join(tc.left, tc.right); got != tc.join {
			t.Fatalf("join(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.join)
		}
		if got := domain.Meet(tc.left, tc.right); got != tc.meet {
			t.Fatalf("meet(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.meet)
		}
		if got := domain.Widen(tc.left, tc.right); got != tc.join {
			t.Fatalf("widen(%v, %v) = %v, want %v", tc.left, tc.right, got, tc.join)
		}
		if !domain.LessOrEq(tc.left, tc.join) || !domain.LessOrEq(tc.right, tc.join) {
			t.Fatalf("order should place %v and %v below %v", tc.left, tc.right, tc.join)
		}
	}
	if !LessOrEq(Bottom, Stack) || !LessOrEq(Stack, OwnedHeap) || !LessOrEq(OwnedHeap, SharedHeap) || !LessOrEq(SharedHeap, Unknown) {
		t.Fatalf("expected total order bottom < stack < owned-heap < shared-heap < unknown")
	}
}

// TestPlacementCoversIsStrictAnalysisBoundary separates the partial semantic
// placement order from Lattice's conservative totalization. Runtime/JIT
// realizations and invalid values are not analysis placement evidence.
func TestPlacementCoversIsStrictAnalysisBoundary(t *testing.T) {
	analysis := []Placement{Bottom, Stack, OwnedHeap, SharedHeap, Unknown}
	for _, covering := range analysis {
		for _, covered := range analysis {
			if got, want := covering.Covers(covered), LessOrEq(covered, covering); got != want {
				t.Fatalf("%v covers %v = %t, want analysis order %t", covering, covered, got, want)
			}
		}
	}

	outside := []Placement{Interpreter, Register, Placement(255)}
	for _, value := range outside {
		for _, analysisValue := range analysis {
			if value.Covers(analysisValue) || analysisValue.Covers(value) {
				t.Fatalf("out-of-domain placement %v entered the analysis order with %v", value, analysisValue)
			}
		}
		if value.Covers(value) {
			t.Fatalf("out-of-domain placement %v covers itself", value)
		}
	}
}

func TestPlacementLatticeRejectsOutOfDomainValues(t *testing.T) {
	values := []Placement{Interpreter, Register, Placement(255)}
	valid := []Placement{Bottom, Stack, OwnedHeap, SharedHeap, Unknown}
	for _, outside := range values {
		if Equal(outside, outside) {
			t.Fatalf("invalid placement %v became lattice-equivalent to itself", outside)
		}
		for _, inside := range valid {
			if LessOrEq(outside, inside) || LessOrEq(inside, outside) {
				t.Fatalf("invalid placement %v entered order with %v", outside, inside)
			}
			for name, got := range map[string]Placement{
				"join-left":   Join(outside, inside),
				"join-right":  Join(inside, outside),
				"meet-left":   Meet(outside, inside),
				"meet-right":  Meet(inside, outside),
				"widen-left":  Widen(outside, inside),
				"widen-right": Widen(inside, outside),
			} {
				if got == Unknown || validAnalysisPlacement(got) {
					t.Fatalf("%s(%v,%v) = %v, want invalid refusal sentinel", name, outside, inside, got)
				}
			}
		}
	}
}

func TestPlacementCheckedLatticeRefusesOutOfDomainValues(t *testing.T) {
	for _, outside := range []Placement{Interpreter, Register, Placement(255)} {
		if got, ok := JoinChecked(outside, Stack); ok || got != invalidPlacementResult {
			t.Fatalf("JoinChecked(%v,stack) = %v/%t, want refusal", outside, got, ok)
		}
		if got, ok := MeetChecked(Stack, outside); ok || got != invalidPlacementResult {
			t.Fatalf("MeetChecked(stack,%v) = %v/%t, want refusal", outside, got, ok)
		}
		if got, ok := WidenChecked(Stack, outside); ok || got != invalidPlacementResult {
			t.Fatalf("WidenChecked(stack,%v) = %v/%t, want refusal", outside, got, ok)
		}
	}
}
