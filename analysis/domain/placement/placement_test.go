package placement

import (
	"fmt"
	"testing"

	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestPlacementLatticeLaws(t *testing.T) {
	suite := latticelaws.LawSuite[Value]{
		Name:   "placement",
		Domain: Lattice(),
		Sample: []Value{
			Bottom,
			Stack,
			OwnedHeap,
			SharedHeap,
			Unknown,
		},
		Format: func(v Value) string {
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
		left, right Value
		join        Value
		meet        Value
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
