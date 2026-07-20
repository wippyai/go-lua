package transformer

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestFormalRelationStepDependencyShapeIsClosed(t *testing.T) {
	tests := []struct {
		kind                 boundaryStepKind
		nodeEntry, pointRead bool
	}{
		{boundaryStepEffect, false, false},
		{boundaryStepApply, false, false},
		{boundaryStepExternalCall, false, true},
		{boundaryStepRootAssignment, true, false},
		{boundaryStepEnvironmentWrite, false, false},
		{boundaryStepGenericFor, true, true},
		{boundaryStepContribution, false, false},
		{boundaryStepLoopFeedback, false, false},
		{boundaryStepLoopExit, false, false},
		{boundaryStepBranchRelations, false, false},
		{boundaryStepCallResults, false, false},
		{boundaryStepPresenceImplications, true, false},
		{boundaryStepChannelSelect, false, false},
		{boundaryStepCovariantExposure, true, false},
	}
	for _, test := range tests {
		entry, reads, valid := formalRelationStepDependencyShape(test.kind)
		if !valid || entry != test.nodeEntry || reads != test.pointRead {
			t.Fatalf("kind %d dependency shape = (%t,%t,%t), want (%t,%t,true)", test.kind, entry, reads, valid, test.nodeEntry, test.pointRead)
		}
	}
	if _, _, valid := formalRelationStepDependencyShape(boundaryStepInvalid); valid {
		t.Fatal("invalid boundary Step acquired a dependency contract")
	}
	if _, _, valid := formalRelationStepDependencyShape(boundaryStepKind(boundaryStepCovariantExposure + 1)); valid {
		t.Fatal("foreign boundary Step acquired a dependency contract")
	}
}

func TestFormalRelationStepFrozenInputsMatchEveryBoundaryKind(t *testing.T) {
	for kind := boundaryStepEffect; kind <= boundaryStepCovariantExposure; kind++ {
		kind := kind
		t.Run(fmt.Sprintf("kind-%d", kind), func(t *testing.T) {
			program, target, nodeEntry, published := formalStepDependencyTestProgram(kind)
			inventory, err := freezeFormalRelationRegionInventory(program)
			if err != nil {
				t.Fatal(err)
			}
			contract := inventory.stepInputs[target]
			wantCount := 1
			needsEntry, needsReads, valid := formalRelationStepDependencyShape(kind)
			if !valid {
				t.Fatalf("boundary kind %d is not closed", kind)
			}
			if needsEntry {
				wantCount++
				if !inventory.stepDependencyDeclared(target, nodeEntry, formalRelationInfluenceStepNodeEntry, 0) {
					t.Fatal("missing exact node-entry input")
				}
			}
			if needsReads {
				wantCount++
				if !inventory.stepDependencyDeclared(target, published, formalRelationInfluenceStepPublishedRead, 7) {
					t.Fatal("missing exact published-read input")
				}
			}
			if kind == boundaryStepApply {
				wantCount++
			}
			if len(contract.inputs) != wantCount || len(inventory.incoming[target]) != wantCount {
				t.Fatalf("kind %d inputs = %#v", kind, inventory.incoming[target])
			}
			if err := inventory.validateStepDependencyContracts(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func formalStepDependencyTestProgram(kind boundaryStepKind) (*RelationProgram, formalRelationCell, formalRelationCell, formalRelationCell) {
	step := boundaryStep{kind: kind}
	if kind == boundaryStepApply {
		step.apply = relationApplyRef{variable: 2, frame: 1}
	}
	if kind == boundaryStepExternalCall || kind == boundaryStepGenericFor {
		step.point = 5
	}
	if kind == boundaryStepLoopFeedback || kind == boundaryStepLoopExit {
		step.binder = 1
		code := formalRegionTestCode([]relationNode{
			{},
			{kind: relationNodeLoopMu, binder: 1, body: 2, exits: []relationRootRef{3}},
			{kind: relationNodeSequence, steps: []boundaryStep{step}},
			{kind: relationNodeOutcome, outcome: 1},
		}, 1)
		program := formalRegionTestProgram(code)
		return program,
			formalRelationCell{Variable: 1, Root: 2, Step: 1, Kind: formalRelationCellStep},
			formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode},
			formalRelationCell{}
	}
	code := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{step}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepEffect}}},
	}, 0, relationPointPublication{point: 7, ref: 2})
	program := formalRegionTestProgram(code)
	reads := make([][]cfg.Point, 8)
	reads[5] = []cfg.Point{7}
	program.bodies[0].nodeReads = reads
	if kind == boundaryStepApply {
		callee := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		program = formalRegionTestProgram(code, callee)
		program.bodies[0].nodeReads = reads
	}
	return program,
		formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep},
		formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode},
		formalRelationCell{Variable: 1, Root: 2, Step: 1, Kind: formalRelationCellStep}
}

func TestFormalRelationPublishedReadsRetainPointBucketsAndExactOutputs(t *testing.T) {
	code := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepGenericFor, point: 5}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepEffect}, {kind: boundaryStepContribution}}},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeBottom},
	}, 1,
		relationPointPublication{point: 0, ref: 3},
		relationPointPublication{point: 7, ref: 2},
		relationPointPublication{point: 7, ref: 3},
		relationPointPublication{point: 8, ref: 2},
		relationPointPublication{point: 9, ref: 4},
	)
	program := formalRegionTestProgram(code)
	program.bodies[0].nodeReads = make([][]cfg.Point, 10)
	program.bodies[0].nodeReads[5] = []cfg.Point{0, 7, 8, 9}

	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	target := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	lastExecutable := formalRelationCell{Variable: 1, Root: 2, Step: 1, Kind: formalRelationCellStep}
	outcome := formalRelationCell{Variable: 1, Outcome: 1, Kind: formalRelationCellOutcome}
	for _, want := range []struct {
		source formalRelationCell
		point  cfg.Point
	}{{outcome, 0}, {lastExecutable, 7}, {outcome, 7}, {lastExecutable, 8}} {
		if !inventory.stepDependencyDeclared(target, want.source, formalRelationInfluenceStepPublishedRead, want.point) {
			t.Fatalf("missing Read(%d) source %+v in %#v", want.point, want.source, inventory.incoming[target])
		}
	}
	if inventory.stepDependencyCount(target, formalRelationInfluenceStepPublishedRead) != 4 {
		t.Fatalf("published read inputs = %#v", inventory.incoming[target])
	}
	bottomNode := formalRelationCell{Variable: 1, Root: 4, Kind: formalRelationCellNode}
	if inventory.stepDependencyDeclared(target, bottomNode, formalRelationInfluenceStepPublishedRead, 9) {
		t.Fatal("Bottom publication incorrectly reads the seedable lexical node")
	}
}

func TestFormalRelationStepDependencyContractRejectsMissingForeignAndDuplicate(t *testing.T) {
	for _, mutation := range []struct {
		name string
		edit func(*formalRelationRegionInventory, formalRelationCell)
	}{
		{"missing", func(i *formalRelationRegionInventory, target formalRelationCell) {
			for index, input := range i.incoming[target] {
				if input.Kind == formalRelationInfluenceStepPublishedRead {
					i.incoming[target] = append(i.incoming[target][:index], i.incoming[target][index+1:]...)
					return
				}
			}
		}},
		{"foreign", func(i *formalRelationRegionInventory, target formalRelationCell) {
			for index := range i.incoming[target] {
				if i.incoming[target][index].Kind == formalRelationInfluenceStepPublishedRead {
					i.incoming[target][index].Source = formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
					return
				}
			}
		}},
		{"duplicate", func(i *formalRelationRegionInventory, target formalRelationCell) {
			for _, input := range i.incoming[target] {
				if input.Kind == formalRelationInfluenceStepPublishedRead {
					i.incoming[target] = append(i.incoming[target], input)
					return
				}
			}
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			program, target, _, _ := formalStepDependencyTestProgram(boundaryStepGenericFor)
			inventory, err := freezeFormalRelationRegionInventory(program)
			if err != nil {
				t.Fatal(err)
			}
			mutation.edit(inventory, target)
			if err := inventory.validateStepDependencyContracts(); err == nil {
				t.Fatal("malformed Step dependency contract was accepted")
			}
		})
	}
}

func TestFormalRelationRegionRejectsMalformedPointReadInputs(t *testing.T) {
	newProgram := func(t *testing.T) (*RelationProgram, formalRelationCell) {
		t.Helper()
		code := formalRegionTestCode([]relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepGenericFor, point: 1}}},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepEffect}}},
		}, 0, relationPointPublication{point: 1, ref: 2})
		program := formalRegionTestProgram(code)
		program.bodies[0].nodeReads = make([][]cfg.Point, 2)
		program.bodies[0].nodeReads[1] = []cfg.Point{1}
		region, err := freezeFormalRelationRegionInventory(program)
		if err != nil {
			t.Fatal(err)
		}
		program.formalRegion = region
		return program, formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	}
	for _, mutation := range []struct {
		name string
		edit func([]formalRelationInfluence) []formalRelationInfluence
	}{
		{"missing-read", func(row []formalRelationInfluence) []formalRelationInfluence {
			for index, input := range row {
				if input.Kind == formalRelationInfluenceStepPublishedRead {
					return append(row[:index:index], row[index+1:]...)
				}
			}
			return row
		}},
		{"foreign-source", func(row []formalRelationInfluence) []formalRelationInfluence {
			for index := range row {
				if row[index].Kind == formalRelationInfluenceStepPublishedRead {
					row[index].Source = formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
				}
			}
			return row
		}},
		{"foreign-point", func(row []formalRelationInfluence) []formalRelationInfluence {
			for index := range row {
				if row[index].Kind == formalRelationInfluenceStepPublishedRead {
					row[index].ReadPoint = 2
				}
			}
			return row
		}},
		{"duplicate-read", func(row []formalRelationInfluence) []formalRelationInfluence {
			for _, input := range row {
				if input.Kind == formalRelationInfluenceStepPublishedRead {
					return append(row, input)
				}
			}
			return row
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			program, target := newProgram(t)
			program.formalRegion.incoming[target] = mutation.edit(program.formalRegion.incoming[target])
			if err := program.formalRegion.validateStepDependencyContracts(); err == nil {
				t.Fatal("malformed point-read input was accepted by the frozen region")
			}
		})
	}
}

func TestFormalRelationBottomPublicationIgnoresSeededLexicalNode(t *testing.T) {
	code := formalRegionTestCode([]relationNode{
		{},
		// ExternalCall and GenericFor share the exact published-read dependency
		// law. Use the syntax-free ExternalCall shape here because this test owns
		// no value vocabulary; executable GenericFor admission is covered by the
		// dedicated factor differential.
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepExternalCall, point: 1}}},
		{kind: relationNodeBottom},
	}, 0, relationPointPublication{point: 1, ref: 2})
	program := formalRegionTestProgram(code)
	body := &program.bodies[0]
	body.nodeReads = make([][]cfg.Point, 2)
	body.nodeReads[1] = []cfg.Point{1}
	formalTemplateTestPrepareRootInputs(t, program)
	seed := state.RecomposeValueLane(program.registry, body.domain, state.State{}, state.ValueLaneFactor{Top: true})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(1), seed),
	)
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	bottom := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
	bottomEquation, ok := template.equation(bottom)
	if !ok || len(bottomEquation.Seeds) != 1 {
		t.Fatalf("seeded lexical Bottom equation = %#v/%t", bottomEquation.Seeds, ok)
	}
	step := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	stepEquation, ok := template.equation(step)
	if !ok {
		t.Fatal("published-read equation is absent")
	}
	for _, input := range stepEquation.Inputs {
		if input.Influence == formalRelationInfluenceStepPublishedRead {
			t.Fatalf("shared Bottom read captured seeded lexical node: %#v", input)
		}
	}
}
