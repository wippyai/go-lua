package ssa

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Phi nodes are kept as a planned IR surface even when no higher layer wires
// them in yet.
func TestPlannedPhiNodeSurfaceRemainsAvailable(t *testing.T) {
	phi := PhiNode{
		Point:  cfg.Point(1),
		Target: Version{Root: "x", ID: 2},
		Operands: []PhiOperand{
			{From: cfg.Point(0), Version: Version{Root: "x", ID: 1}},
		},
	}

	if phi.Point != 1 {
		t.Fatalf("Point = %d, want 1", phi.Point)
	}
	if phi.Target.ID != 2 {
		t.Fatalf("Target.ID = %d, want 2", phi.Target.ID)
	}
	if len(phi.Operands) != 1 {
		t.Fatalf("Operands length = %d, want 1", len(phi.Operands))
	}
}
