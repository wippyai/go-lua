package artifact

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/schema"
)

func TestPointRowsUseFlowBoundaryArmsAndKeepTransferSeparate(t *testing.T) {
	if causal.BoundaryLocal != 1 || causal.BoundaryCancel != 8 {
		t.Fatalf("Flow boundary arm ordinals changed: local=%d cancel=%d", causal.BoundaryLocal, causal.BoundaryCancel)
	}
	edge := EnvironmentEdge{
		id: valuesLawID(3), from: valuesLawID(4), to: valuesLawID(5), route: valuesLawID(6),
		arm: causal.BoundaryLocal,
	}
	for arm := causal.BoundaryLocal; arm <= causal.BoundaryCancel; arm++ {
		edge.arm = arm
		if !edge.Available() || edge.Arm() != arm {
			t.Fatalf("Flow boundary arm %d is not accepted by EnvironmentEdge", arm)
		}
	}
	for _, arm := range []causal.BoundaryArmKind{causal.BoundaryLocal - 1, causal.BoundaryCancel + 1} {
		edge.arm = arm
		if edge.Available() {
			t.Fatalf("out-of-range Flow boundary arm %d was accepted", arm)
		}
	}
	point := Point{id: valuesLawID(1), decisions: []identity.ContentID{valuesLawID(2)}, initial: true}
	if !point.Available() || point.DecisionCount() != 1 {
		t.Fatal("valid point row unavailable")
	}
	transfer := localTransferDraft{id: valuesLawID(3), from: valuesLawID(4), to: valuesLawID(5), full: true}
	if !transfer.Available() || !transfer.full || len(transfer.writes) != 0 {
		t.Fatal("full local transfer row unavailable")
	}
}

func TestLocalTransferWritesAreStrictlyAscendingAndSetIdenticalEmissionsShareAnID(t *testing.T) {
	from, to := valuesLawID(4), valuesLawID(5)
	left, leftOK := orderedWrites([]schema.Key{"value-source", "pack-source", "heap-ingress", "call-dispatch"})
	right, rightOK := orderedWrites([]schema.Key{"call-dispatch", "heap-ingress", "pack-source", "value-source"})
	if !leftOK || !rightOK || len(left) != 4 || !slices.Equal(left, right) {
		t.Fatalf("ordered writes drifted: left=%v right=%v", left, right)
	}
	if _, ok := orderedWrites([]schema.Key{"value-source", "value-source"}); ok {
		t.Fatal("duplicate write key was accepted")
	}
	fields := func(writes []schema.Key) []field {
		out := []field{bytesField(from), bytesField(to), boolField(false), uintField(uint64(len(writes)))}
		for _, write := range writes {
			out = append(out, keyField(write))
		}
		return out
	}
	leftID := digest("analysis/program-artifact/call-base-dispatch-transfer", artifactFormat, fields(left)...)
	rightID := digest("analysis/program-artifact/call-base-dispatch-transfer", artifactFormat, fields(right)...)
	if !leftID.Available() || leftID != rightID {
		t.Fatal("set-identical write emissions produced different transfer identities")
	}
	unsorted := localTransferDraft{id: leftID, from: from, to: to, writes: []schema.Key{"value-source", "pack-source"}}
	if unsorted.Available() {
		t.Fatal("descending write keys were accepted")
	}
	sorted := localTransferDraft{id: leftID, from: from, to: to, writes: left}
	if !sorted.Available() || len(sorted.writes) != 4 {
		t.Fatal("ascending write keys were rejected")
	}
}
