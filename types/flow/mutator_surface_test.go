package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestRootMutationPoints_UnifiesMutatorLanes(t *testing.T) {
	const (
		state cfg.SymbolID = 1
		other cfg.SymbolID = 2
	)
	inputs := &Inputs{
		MapMutatorAssignments: []MapMutatorAssignment{
			{Point: 12, Target: constraint.NewPath(state, "state")},
			{Point: 12, Target: constraint.NewPath(state, "state")},
		},
		TableMutatorAssignments: []TableMutatorAssignment{
			{Point: 10, Target: constraint.NewPath(state, "state").Field("items")},
		},
	}

	got := inputs.RootMutationPoints(map[cfg.SymbolID]bool{state: true})
	if len(got) != 2 {
		t.Fatalf("RootMutationPoints returned %d points, want 2: %#v", len(got), got)
	}
	if got[0].Point != 10 || !got[0].Target.Equal(constraint.NewPath(state, "state").Field("items")) {
		t.Fatalf("first mutation = %#v, want state.items at point 10", got[0])
	}
	if got[1].Point != 12 || !got[1].Target.Equal(constraint.NewPath(state, "state")) {
		t.Fatalf("second mutation = %#v, want state at point 12", got[1])
	}
}

func TestTransferObservationPoints_IncludesAssignmentsAndMutators(t *testing.T) {
	inputs := &Inputs{
		Assignments: []UnifiedAssignment{
			{Point: 4, TargetPath: constraint.NewPath(1, "alias").Field("__index")},
			{Point: 7, TargetPath: constraint.NewPath(2, "Class").Field("run")},
			{Point: 7, TargetPath: constraint.NewPath(2, "Class").Field("stop")},
		},
		MapMutatorAssignments: []MapMutatorAssignment{
			{Point: 9, Target: constraint.NewPath(3, "state")},
		},
		TableMutatorAssignments: []TableMutatorAssignment{
			{Point: 5, Target: constraint.NewPath(3, "state").Field("items")},
		},
	}

	got := inputs.TransferObservationPoints()
	want := []cfg.Point{4, 5, 7, 9}
	if len(got) != len(want) {
		t.Fatalf("TransferObservationPoints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TransferObservationPoints = %v, want %v", got, want)
		}
	}
}
