package engine

import (
	"encoding/json"
	"testing"
)

// encodeDifference produces the closed descriptor encoding the front publishes
// for one normalized difference-logic branch relation.
func encodeDifference(t *testing.T, wire branchDiffWire) []byte {
	t.Helper()
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("encoding difference descriptor: %v", err)
	}
	return append([]byte(branchDiffPrefix), encoded...)
}

func TestDecodeBranchDifferenceRejectsAnEmptyOperand(t *testing.T) {
	if _, ok := decodeBranchDiff(encodeDifference(t, branchDiffWire{CoHi: 1, HiPath: "i", Edge: true})); ok {
		t.Fatal("a descriptor without a low operand names no relation and must be dropped")
	}
	if _, ok := decodeBranchDiff([]byte("front/branch-predicate/v1/{}")); ok {
		t.Fatal("a predicate encoding must not decode as a difference descriptor")
	}
}

func TestArtifactTrueEdgeLengthRelationAdmitsOnlyLengthBearingTrueEdgeRelations(t *testing.T) {
	lengthRelation := encodeDifference(t, branchDiffWire{CoHi: 1, HiPath: "i", LoPath: "xs", LoIsLen: true, C: -1, Edge: true})
	if !artifactTrueEdgeLengthRelation("difference-00000000", lengthRelation) {
		t.Fatal("i + 1 <= #xs relates an index to an array length on the true edge")
	}
	if artifactTrueEdgeLengthRelation("predicate", lengthRelation) {
		t.Fatal("only a difference operand carries a relation")
	}
	falseEdge := encodeDifference(t, branchDiffWire{CoHi: 1, HiPath: "i", LoPath: "xs", LoIsLen: true, C: -1})
	if artifactTrueEdgeLengthRelation("difference-00000000", falseEdge) {
		t.Fatal("a false-edge relation proves nothing on the guarded arm")
	}
	valuesOnly := encodeDifference(t, branchDiffWire{CoHi: 1, HiPath: "q", LoPath: "r", Edge: true})
	if artifactTrueEdgeLengthRelation("difference-00000000", valuesOnly) {
		t.Fatal("a relation between two values bounds no array")
	}
}

func TestRelationalIndexUpperBoundsProvesTransitiveOrdering(t *testing.T) {
	// i >= 1 and i < j and j <= #xs prove i <= #xs.
	predicates := []branchPredicateWire{
		{Kind: "num-ge", Path: "i", NumFloor: 1},
		{Kind: "index-in-range", Path: "j", OtherPath: "xs"},
	}
	differences := []branchDiffWire{
		{CoHi: 1, HiPath: "i", LoPath: "j", C: -1, Edge: true},
		{CoHi: 1, HiPath: "j", LoPath: "xs", LoIsLen: true, Edge: true},
	}
	proven := relationalIndexUpperBounds(predicates, differences, []string{"i"})
	if len(proven) != 1 || proven[0] != (relationalIndexPair{index: "i", container: "xs"}) {
		t.Fatalf("expected i in range of xs, got %v", proven)
	}
}

func TestRelationalIndexUpperBoundsCarriesACrossVariableLengthEquality(t *testing.T) {
	// #a == #b and i >= 1 and i <= #a prove i <= #b.
	predicates := []branchPredicateWire{
		{Kind: "num-ge", Path: "i", NumFloor: 1},
		{Kind: "index-in-range", Path: "i", OtherPath: "a"},
	}
	differences := []branchDiffWire{
		{CoHi: 1, HiPath: "a", HiIsLen: true, LoPath: "b", LoIsLen: true, Edge: true},
		{CoHi: 1, HiPath: "b", HiIsLen: true, LoPath: "a", LoIsLen: true, Edge: true},
		{CoHi: 1, HiPath: "i", LoPath: "a", LoIsLen: true, Edge: true},
	}
	proven := relationalIndexUpperBounds(predicates, differences, []string{"i"})
	found := false
	for _, pair := range proven {
		found = found || pair == relationalIndexPair{index: "i", container: "b"}
	}
	if !found {
		t.Fatalf("expected i in range of b through the length equality, got %v", proven)
	}
}

func TestRelationalIndexUpperBoundsDischargesABoundedSum(t *testing.T) {
	// i >= 1 and j >= 0 and i + j <= #xs prove i <= #xs. The bound side is a
	// variable length, so this leaves difference logic for the linear backend.
	predicates := []branchPredicateWire{
		{Kind: "num-ge", Path: "i", NumFloor: 1},
		{Kind: "num-ge", Path: "j", NumFloor: 0},
	}
	differences := []branchDiffWire{
		{CoHi: 1, HiPath: "i", CoHi2: 1, Hi2Path: "j", HasHi2: true, LoPath: "xs", LoIsLen: true, Edge: true},
	}
	proven := relationalIndexUpperBounds(predicates, differences, []string{"i"})
	if len(proven) != 1 || proven[0] != (relationalIndexPair{index: "i", container: "xs"}) {
		t.Fatalf("expected i in range of xs through the bounded sum, got %v", proven)
	}
}

func TestRelationalIndexUpperBoundsWithholdsASumWhoseOtherOperandHasNoFloor(t *testing.T) {
	// Without j >= 0 the sum i + j <= #xs does not bound i: j may be negative.
	predicates := []branchPredicateWire{{Kind: "num-ge", Path: "i", NumFloor: 1}}
	differences := []branchDiffWire{
		{CoHi: 1, HiPath: "i", CoHi2: 1, Hi2Path: "j", HasHi2: true, LoPath: "xs", LoIsLen: true, Edge: true},
	}
	if proven := relationalIndexUpperBounds(predicates, differences, []string{"i"}); len(proven) != 0 {
		t.Fatalf("an unbounded second operand proves no upper bound, got %v", proven)
	}
}

func TestRelationalIndexUpperBoundsSeparatesValueFromLength(t *testing.T) {
	// i <= a as values says nothing about #a, so no in-range conclusion exists.
	predicates := []branchPredicateWire{{Kind: "num-ge", Path: "i", NumFloor: 1}}
	differences := []branchDiffWire{{CoHi: 1, HiPath: "i", LoPath: "a", Edge: true}}
	if proven := relationalIndexUpperBounds(predicates, differences, []string{"i"}); len(proven) != 0 {
		t.Fatalf("a value relation must not be read as a length bound, got %v", proven)
	}
}

func TestProvenFloorPathsReadsTheEncodedLowerRelations(t *testing.T) {
	lower := map[string][]byte{"one": []byte("path/i"), "two": []byte("path/j"), "three": []byte("temp/1")}
	got := provenFloorPaths(lower)
	if len(got) != 2 || got[0] != "i" || got[1] != "j" {
		t.Fatalf("expected the sorted path indexes i and j, got %v", got)
	}
}
