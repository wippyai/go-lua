package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func pointDecision(value byte) pointDecisionDraft {
	atom, _ := region.NewAtom(valuesLawID(value))
	return pointDecisionDraft{semantic: valuesLawID(value), atom: atom}
}

func TestPointRowsUseFlowBoundaryArmsAndKeepTransferSeparate(t *testing.T) {
	if causal.BoundaryLocal != 1 || causal.BoundaryCancel != 8 {
		t.Fatalf("Flow boundary arm ordinals changed: local=%d cancel=%d", causal.BoundaryLocal, causal.BoundaryCancel)
	}
	edge := environmentEdgeDraft{
		id: valuesLawID(3), from: valuesLawID(4), to: valuesLawID(5), route: valuesLawID(6),
		arm: causal.BoundaryLocal,
	}
	for arm := causal.BoundaryLocal; arm <= causal.BoundaryCancel; arm++ {
		edge.arm = arm
		if !edge.Available() || edge.Arm() != arm {
			t.Fatalf("Flow boundary arm %d is not accepted by environmentEdgeDraft", arm)
		}
	}
	for _, arm := range []causal.BoundaryArmKind{causal.BoundaryLocal - 1, causal.BoundaryCancel + 1} {
		edge.arm = arm
		if edge.Available() {
			t.Fatalf("out-of-range Flow boundary arm %d was accepted", arm)
		}
	}
	pointID := valuesLawID(1)
	point := pointDraft{id: pointID, decisionScope: pointID, decisions: []pointDecisionDraft{pointDecision(2)}, initial: true}
	if !point.Available() || point.DecisionCount() != 1 {
		t.Fatal("valid point row unavailable")
	}
}
