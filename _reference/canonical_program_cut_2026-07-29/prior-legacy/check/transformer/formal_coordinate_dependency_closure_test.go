package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFormalCoordinateInventoryEqualityDoesNotUseLength(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	leftPath := keys.FromPath(pathdom.NewPath(symbol.ID(9401), "left"))
	rightPath := keys.FromPath(pathdom.NewPath(symbol.ID(9402), "right"))
	leftSlots, err := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{leftPath})
	if err != nil {
		t.Fatal(err)
	}
	rightSlots, err := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{rightPath})
	if err != nil {
		t.Fatal(err)
	}
	left, err := domain.SealCoordinateFactorInventory(keys, leftSlots)
	if err != nil {
		t.Fatal(err)
	}
	right, err := domain.SealCoordinateFactorInventory(keys, rightSlots)
	if err != nil {
		t.Fatal(err)
	}
	if left.Len() == 0 || left.Len() != right.Len() {
		t.Fatalf("fixture inventory widths = %d/%d", left.Len(), right.Len())
	}
	equal, err := formalCoordinateInventoriesEqual(domain, left, right)
	if err != nil {
		t.Fatal(err)
	}
	if equal {
		t.Fatal("same-width coordinate inventories with different roots compared equal")
	}
}

func TestFormalCoordinateDependencyClosurePropagatesCellGrowthWithoutBodyGrowth(t *testing.T) {
	targetCell := formalRelationCell{Variable: 1, Outcome: 1, Kind: formalRelationCellOutcome}
	applyCell := formalRelationCell{Variable: 2, Root: 1, Step: 1, Kind: formalRelationCellStep}
	presenceCell := formalRelationCell{Variable: 2, Root: 1, Step: 2, Kind: formalRelationCellStep}
	assignmentCell := formalRelationCell{Variable: 2, Root: 1, Step: 3, Kind: formalRelationCellStep}
	callerCode := &relationCode{nodes: make([]relationNode, 2)}
	callerCode.nodes[1].steps = []boundaryStep{
		{kind: boundaryStepApply},
		{kind: boundaryStepPresenceImplications},
		{kind: boundaryStepRootAssignment},
	}
	program := &RelationProgram{bodies: []relationProgramBody{
		{variable: 1, relation: Relation{code: &relationCode{nodes: make([]relationNode, 2)}}},
		{variable: 2, relation: Relation{code: callerCode}},
	}}
	closure := &formalCoordinateDependencyClosure{
		program:        program,
		region:         &formalRelationRegionInventory{outcomes: [][]formalRelationCell{{targetCell}, nil}},
		bodies:         make([]state.CoordinateFactorInventory, 2),
		cells:          []formalRelationCell{targetCell, applyCell, presenceCell, assignmentCell},
		cellIndex:      map[formalRelationCell]int{targetCell: 0, applyCell: 1, presenceCell: 2, assignmentCell: 3},
		cellBody:       [][]int{{0}, {1, 2, 3}},
		selectors:      make([]state.CoordinateFactorInventory, 2),
		selectorMember: []map[int]struct{}{{0: {}}, {}},
		frames:         []formalStaticApplyCoordinateFrame{{caller: 1, target: 0, cells: []int{1}}},
	}
	closure.sealDependencies()

	targetCellNode := closure.cellNodeFirst
	targetSelectorNode := closure.selectorNodeFirst
	frameNode := closure.frameNodeFirst
	callerApplyNode := closure.cellNodeFirst + 1
	callerBodyNode := closure.bodyNodeFirst + 1
	presenceNode := closure.cellNodeFirst + 2
	assignmentNode := closure.cellNodeFirst + 3

	// A changed per-cell footprint reaches the target selector directly. It
	// does not depend on the body's union changing size (or changing at all).
	assertFormalCoordinateDependent(t, closure, targetCellNode, targetSelectorNode)
	assertFormalCoordinateDependent(t, closure, targetSelectorNode, frameNode)
	// Identity-only growth does not necessarily change either body inventory
	// or target selector. The frame reads both its caller-site input support and
	// target outcome support directly, so both cells must wake it directly.
	assertFormalCoordinateDependent(t, closure, targetCellNode, frameNode)
	assertFormalCoordinateDependent(t, closure, callerApplyNode, frameNode)
	assertFormalCoordinateDependent(t, closure, frameNode, callerApplyNode)

	// The imported image grows the caller Apply footprint, which grows the body
	// inventory and re-enqueues both inventory-sensitive operator laws.
	assertFormalCoordinateDependent(t, closure, callerApplyNode, callerBodyNode)
	assertFormalCoordinateDependent(t, closure, callerBodyNode, presenceNode)
	assertFormalCoordinateDependent(t, closure, callerBodyNode, assignmentNode)
}

func assertFormalCoordinateDependent(t *testing.T, closure *formalCoordinateDependencyClosure, from, want int) {
	t.Helper()
	for _, got := range closure.dependents[from] {
		if got == want {
			return
		}
	}
	t.Fatalf("node %d dependents %v do not contain %d", from, closure.dependents[from], want)
}

// refreezeFormalTestStaticTopology is the test-only equivalent of the
// production freeze order after a fixture replaces sealed relationCode.
func refreezeFormalTestStaticTopology(t *testing.T, program *RelationProgram) {
	t.Helper()
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	fibers, err := freezeFormalFiberInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalFibers, program.formalSlots = fibers, fibers.slots
}
