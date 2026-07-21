package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestFormalCoordinateDependencyClosureKeepsClosureDefinitionSelectorSupportProducerExact(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	terminalPath := keys.FromPath(pathdom.NewPath(symbol.ID(9411), "terminal"))
	definitionPath := keys.FromPath(pathdom.NewPath(symbol.ID(9412), "definition"))
	inventory := func(path keyspace.Key) state.CoordinateFactorInventory {
		t.Helper()
		slots, err := domain.BoundaryRootCoordinateSlots(keys, []keyspace.Key{path})
		if err != nil {
			t.Fatal(err)
		}
		value, err := domain.SealCoordinateFactorInventory(keys, slots)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	terminal, definition := inventory(terminalPath), inventory(definitionPath)
	definitionCell := formalRelationCell{Variable: 1, Definition: 1, Kind: formalRelationCellDefinition}
	consumerCell := formalRelationCell{Variable: 1, Root: 1, Step: 2, Kind: formalRelationCellStep}
	code := &relationCode{nodes: make([]relationNode, 2)}
	code.nodes[1].steps = []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{frame: 1}}, {kind: boundaryStepApply, apply: relationApplyRef{frame: 2}}}
	program := &RelationProgram{bodies: []relationProgramBody{{variable: 1, productDomain: domain, relation: Relation{code: code}}}}
	closure := &formalCoordinateDependencyClosure{
		program: program,
		region: &formalRelationRegionInventory{incoming: map[formalRelationCell][]formalRelationInfluence{
			consumerCell: {{Source: definitionCell, Target: consumerCell, Kind: formalRelationInfluenceClosureDefinition}},
		}},
		keys:      []*keyspace.KeySpace{keys},
		bodies:    []state.CoordinateFactorInventory{terminal},
		cells:     []formalRelationCell{definitionCell, consumerCell},
		cellIndex: map[formalRelationCell]int{definitionCell: 0, consumerCell: 1},
		cellValue: []formalOperatorCoordinateFootprint{{cell: definitionCell, inventory: definition}, {cell: consumerCell}},
		cellBody:  [][]int{{0, 1}},
		selectors: []state.CoordinateFactorInventory{terminal},
		selectorMember: []map[int]struct{}{
			{},
		},
		frames: []formalStaticApplyCoordinateFrame{
			{caller: 0, target: 0, frame: &linkedRelationFrame{term: 1}},
			{caller: 0, target: 0, frame: &linkedRelationFrame{term: 2, closureProducer: 1}},
			{caller: 0, target: 0, frame: &linkedRelationFrame{term: 3}},
		},
		frameByOwnerTerm: map[formalFrameFootprintKey]int{
			{variable: 1, frame: 1}: 0,
			{variable: 1, frame: 2}: 1,
			{variable: 1, frame: 3}: 2,
		},
	}
	if err := closure.deriveClosureDefinitionSelectorSupport(); err != nil {
		t.Fatal(err)
	}
	if got := closure.frames[0].selectorSupport; len(got) != 1 || got[0] != 0 {
		t.Fatalf("producer selector support = %v, want definition cell 0", got)
	}
	if got := closure.frames[2].selectorSupport; len(got) != 0 {
		t.Fatalf("sibling selector support = %v, want none", got)
	}
	closure.frameSelectorFolds = make([]formalCoordinateInventoryFold, len(closure.frames))
	for frameIndex := range closure.frames {
		fold, err := newFormalCoordinateInventoryFold(domain, keys, 1+len(closure.frames[frameIndex].selectorSupport))
		if err != nil {
			t.Fatal(err)
		}
		closure.frameSelectorFolds[frameIndex] = fold
	}
	producerSelector, err := closure.evaluateFrameSelector(0)
	if err != nil {
		t.Fatal(err)
	}
	wantProducer, err := domain.UnionCoordinateFactorInventories(keys, terminal, definition)
	if err != nil {
		t.Fatal(err)
	}
	wantProducer, err = domain.CloseCoordinateFactorInventory(keys, wantProducer)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.CoordinateFactorInventoriesEqual(producerSelector, wantProducer)
	if err != nil || !equal {
		t.Fatalf("producer selector exact membership = %v/%v, want terminal plus selected Definition", producerSelector, err)
	}
	siblingSelector, err := closure.evaluateFrameSelector(2)
	if err != nil {
		t.Fatal(err)
	}
	wantSibling, err := domain.CloseCoordinateFactorInventory(keys, terminal)
	if err != nil {
		t.Fatal(err)
	}
	equal, err = domain.CoordinateFactorInventoriesEqual(siblingSelector, wantSibling)
	if err != nil || !equal {
		t.Fatalf("sibling selector exact membership = %v/%v, want terminal selector without Definition", siblingSelector, err)
	}
	closure.sealDependencies()
	assertFormalCoordinateDependent(t, closure, closure.cellNodeFirst, closure.frameNodeFirst)
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
