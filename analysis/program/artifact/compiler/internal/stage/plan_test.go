package stage

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func stageID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func TestSealOrdersComputationDependenciesAndKeepsPredecessorAxes(t *testing.T) {
	builder := New(1)
	base, firstOccurrence, secondOccurrence := stageID(1), stageID(2), stageID(3)
	input, right := stageID(4), stageID(5)
	first, firstOK := builder.Computation(base, "computation", schema.Key("first"), firstOccurrence, input, right)
	second, secondOK := builder.Computation(base, "computation", schema.Key("second"), secondOccurrence, firstOccurrence, right)
	if !firstOK || !secondOK {
		t.Fatal("computation requests")
	}
	if _, duplicate := builder.Computation(base, "computation", schema.Key("again"), firstOccurrence, input, right); duplicate {
		t.Fatal("duplicate computation occurrence accepted")
	}
	if _, predecessorOK := builder.Predecessor(base, "predecessor", schema.Key("write-b")); !predecessorOK {
		t.Fatal("first predecessor request")
	}
	if _, predecessorOK := builder.Predecessor(base, "predecessor", schema.Key("write-a")); !predecessorOK {
		t.Fatal("second predecessor request")
	}

	plan, fault := builder.Seal()
	if fault.Failed() || plan.Count() != 1 {
		t.Fatalf("sealed plan fault/count=%v/%d, want clear/1", fault, plan.Count())
	}
	placement, placementOK := plan.At(0)
	if !placementOK || placement.Base() != base || placement.ComputationCount() != 2 {
		t.Fatalf("placement=%v/%v", placement, placementOK)
	}
	orderedFirst, firstPresent := placement.ComputationAt(0)
	orderedSecond, secondPresent := placement.ComputationAt(1)
	if !firstPresent || !secondPresent || orderedFirst.Point() != first || orderedSecond.Point() != second {
		t.Fatalf("computation order=%v/%v, want %v/%v", orderedFirst.Point(), orderedSecond.Point(), first, second)
	}
	if placement.PredecessorWriteCount() != 2 {
		t.Fatalf("predecessor write count=%d, want 2", placement.PredecessorWriteCount())
	}
	firstWrite, firstWriteOK := placement.PredecessorWriteAt(0)
	secondWrite, secondWriteOK := placement.PredecessorWriteAt(1)
	if !firstWriteOK || !secondWriteOK || firstWrite != schema.Key("write-a") || secondWrite != schema.Key("write-b") {
		t.Fatalf("predecessor writes=%q/%q", firstWrite, secondWrite)
	}
}

func TestLocalSuccessorRequiresAndFollowsLocalCut(t *testing.T) {
	base := stageID(11)
	builder := New(1)
	local, localOK := builder.Local(base, "local")
	successor, successorOK := builder.Successor(base, "successor")
	plan, fault := builder.Seal()
	placement, placementOK := plan.At(0)
	gotLocal, gotLocalOK := placement.Local()
	gotSuccessor, gotSuccessorOK := placement.Successor()
	if !localOK || !successorOK || local == successor || fault.Failed() || plan.Count() != 1 || !placementOK ||
		!gotLocalOK || gotLocal != local || !gotSuccessorOK || gotSuccessor != successor {
		t.Fatalf("local/successor plan = %v/%v fault=%v count=%d", gotLocal, gotSuccessor, fault, plan.Count())
	}

	orphan := New(1)
	if _, ok := orphan.Successor(base, "successor"); !ok {
		t.Fatal("successor request was rejected before plan closure")
	}
	if orphanPlan, orphanFault := orphan.Seal(); !orphanFault.Failed() || orphanPlan.Count() != 0 || orphanFault.Base() != base {
		t.Fatalf("orphan successor = fault=%v count=%d base=%x", orphanFault, orphanPlan.Count(), orphanFault.Base())
	}
}
