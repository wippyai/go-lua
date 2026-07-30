package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFormalRelationTemplateFreezesAcyclicCellsAndOperators(t *testing.T) {
	code := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalTemplateTestProgram(t, code)
	template := formalTemplateTestFreeze(t, program)
	if len(template.equations) != len(program.formalRegion.cells) || !template.region.plan.Matches(program.formalRegion.cells) {
		t.Fatalf("template equation coverage = %d/%d", len(template.equations), len(program.formalRegion.cells))
	}
	for index, equation := range template.equations {
		operator, operatorPresent := equation.terminalOperator()
		if !equation.Cell.valid() || equation.Cell.index != index || !operatorPresent || operator.kind != equation.Cell.cell.Kind {
			t.Fatalf("equation %d is malformed: %#v", index, equation)
		}
		for _, input := range equation.Inputs {
			if !input.valid(equation.Cell) {
				t.Fatalf("equation %d has invalid input %#v", index, input)
			}
		}
	}
	node, ok := template.equation(formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode})
	if !ok || node.Operator.code != code || node.Operator.root != 1 || node.Operator.step != 0 {
		t.Fatalf("node operator = %#v/%t", node.Operator, ok)
	}
	step, ok := template.equation(formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep})
	if !ok || step.Operator.code != code || step.Operator.root != 1 || step.Operator.step != 1 {
		t.Fatalf("step operator = %#v/%t", step.Operator, ok)
	}
	outcome, ok := template.equation(formalRelationCell{Variable: 1, Outcome: 1, Kind: formalRelationCellOutcome})
	if !ok || outcome.Operator.code != code || outcome.Operator.outcome != 1 {
		t.Fatalf("outcome operator = %#v/%t", outcome.Operator, ok)
	}
	if _, ok := template.equation(formalRelationCell{Variable: 2, Root: 1, Kind: formalRelationCellNode}); ok {
		t.Fatal("foreign cell acquired template equation")
	}
}

func TestFormalRelationTemplateRetainsLoopMuAsFiniteBackreference(t *testing.T) {
	const binder loopMuTerm = 1
	code := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalTemplateTestProgram(t, code)
	template := formalTemplateTestFreeze(t, program)
	if template.region.plan.ComponentCount() == 0 {
		t.Fatal("loop fixture did not produce a WTO component")
	}
	body := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
	feedback := formalRelationCell{Variable: 1, Root: 2, Step: 1, Kind: formalRelationCellStep}
	equation, ok := template.equation(body)
	if !ok || !formalTemplateHasInput(equation, feedback, formalRelationInfluenceLoopFeedback) {
		t.Fatalf("loop body inputs = %#v", equation.Inputs)
	}
	if len(template.equations) != len(template.region.cells) {
		t.Fatal("loop template expanded its recursive backedge")
	}
}

func TestFormalRelationRegionRetainsMutualCallsAsCellReferences(t *testing.T) {
	call := func(target relationVar) *relationCode {
		return formalRegionTestCode([]relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: target, frame: 1}}}, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		}, 1)
	}
	left, right := call(2), call(1)
	program := formalRegionTestProgram(left, right)
	program.recursiveSCCs = [][]relationVar{{1, 2}}
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	if region.plan.ComponentCount() == 0 {
		t.Fatal("mutual-call fixture did not produce a WTO component")
	}
	for _, pair := range []struct {
		caller, target relationVar
	}{{1, 2}, {2, 1}} {
		stepCell := formalRelationCell{Variable: pair.caller, Root: 1, Step: 1, Kind: formalRelationCellStep}
		outcomeCell := formalRelationCell{Variable: pair.target, Outcome: 1, Kind: formalRelationCellOutcome}
		found := false
		for _, influence := range region.incoming[stepCell] {
			found = found || influence.Source == outcomeCell && influence.Target == stepCell &&
				influence.Kind == formalRelationInfluenceCalleeOutcome
		}
		if !found {
			t.Fatalf("caller %d Apply region omits callee outcome cell", pair.caller)
		}
	}
}

func TestFormalRelationTemplateOwnsEveryInitialStateSeed(t *testing.T) {
	seededProgram := func(t *testing.T, publicationRefs ...relationRootRef) (*RelationProgram, cfg.Point) {
		t.Helper()
		publications := make([]relationPointPublication, len(publicationRefs))
		for index, ref := range publicationRefs {
			publications[index] = relationPointPublication{point: 1, ref: ref}
		}
		code := formalRegionTestCode([]relationNode{
			{},
			{kind: relationNodeSequence, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		}, 1, publications...)
		program := formalTemplateTestProgram(t, code)
		formalTemplateTestPrepareRootInputs(t, program)
		return program, program.bodies[0].graph.Exit()
	}

	t.Run("entry-and-published-point", func(t *testing.T) {
		program, point := seededProgram(t, 2)
		body := &program.bodies[0]
		entryState := state.RecomposeValueLane(program.registry, body.domain, state.State{}, state.ValueLaneFactor{Top: true})
		body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
			state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), entryState),
			state.NewInitialStateSeed(state.InitialCoordinate(point), state.State{}),
		)
		template := formalTemplateTestFreeze(t, program)
		if len(template.constants) != 2 {
			t.Fatalf("formal constants = %d, want 2", len(template.constants))
		}
		for _, want := range []struct {
			cell  formalRelationCell
			point cfg.Point
			entry bool
		}{
			{cell: formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode}, point: body.graph.Entry(), entry: true},
			{cell: formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}, point: point},
		} {
			equation, ok := template.equation(want.cell)
			if !ok || len(equation.Seeds) != 1 {
				t.Fatalf("seed equation %+v = %#v/%t", want.cell, equation.Seeds, ok)
			}
			seed := equation.Seeds[0]
			constant, valid := seed.constant(equation.Cell)
			if !valid || seed.point != want.point || seed.entry != want.entry || constant.variable != want.cell.Variable || !constant.care {
				t.Fatalf("seed %+v constant = %#v/%t", seed, constant, valid)
			}
			if want.entry {
				foundValuesTop := false
				for _, group := range constant.groups {
					foundValuesTop = foundValuesTop || group.group.kind == formalFiberGroupValues && group.values.Top
				}
				if !foundValuesTop {
					t.Fatal("entry seed lost its exact Values Top factor")
				}
			}
		}
	})

	t.Run("foreign-plan", func(t *testing.T) {
		program, _ := seededProgram(t, 2)
		body := &program.bodies[0]
		foreign := body.body
		foreign[0] ^= 0xff
		body.initialStatePlan = testInitialStatePlan(t, foreign, body.graph)
		if _, err := freezeFormalRelationTemplate(program); err == nil {
			t.Fatal("foreign initial-state plan was accepted")
		}
	})

	t.Run("unpublished-point", func(t *testing.T) {
		program, point := seededProgram(t)
		body := &program.bodies[0]
		body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
			state.NewInitialStateSeed(state.InitialCoordinate(point), state.State{}),
		)
		if _, err := freezeFormalRelationTemplate(program); err == nil {
			t.Fatal("unpublished initial-state point was accepted")
		}
	})

	for _, test := range []struct {
		name string
		refs []relationRootRef
	}{
		{name: "ambiguous-publication", refs: []relationRootRef{1, 2}},
		{name: "duplicate-publication", refs: []relationRootRef{2, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			program, point := seededProgram(t, test.refs...)
			body := &program.bodies[0]
			body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
				state.NewInitialStateSeed(state.InitialCoordinate(point), state.State{}),
			)
			if _, err := freezeFormalRelationTemplate(program); err == nil {
				t.Fatal("non-exact initial-state publication was accepted")
			}
		})
	}
}

func TestFormalRelationOperatorsOwnDefinitionAndResourceReferences(t *testing.T) {
	owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	left := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	right := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	program := formalRegionTestProgram(owner, left, right)
	program.definitions = []relationProgramDefinition{
		{owner: 1, target: 3, point: cfg.Point(9), frame: 2, externallyReachable: true},
		{owner: 1, target: 2, point: cfg.Point(8), frame: 1, externallyReachable: true},
	}
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	if len(region.resources) != 2 {
		t.Fatalf("resource inventory = %#v", region.resources)
	}
	resourceCell := region.resources[1].cell
	for ref := formalRelationDefinitionRef(1); int(ref) < len(region.definitions); ref++ {
		definition := region.definitions[ref]
		incoming := region.incoming[definition.cell]
		if definition.cell.Definition != ref || definition.cell.Kind != formalRelationCellDefinition ||
			!hasFormalInfluence(incoming, resourceCell, formalRelationInfluenceDefinitionSeed) ||
			!hasFormalInfluence(incoming, region.outcomes[definition.target-1][0], formalRelationInfluenceDefinitionOutcome) {
			t.Fatalf("definition %d identity/incoming = %#v/%#v", ref, definition, incoming)
		}
		if !hasFormalInfluence(region.incoming[resourceCell], definition.cell, formalRelationInfluenceResourceFeedback) {
			t.Fatalf("resource feedback omits definition %d: %#v", ref, region.incoming[resourceCell])
		}
	}

}

func TestFrozenRelationProgramBindsDefinitionAndResourceOperatorFootprints(t *testing.T) {
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	owner := lexicalidentity.RootBody(namespace)
	target := lexicalidentity.FunctionBody(namespace, 1)
	ownerUnit := formalTemplateFreezeUnit(t, owner)
	ownerUnit.Definitions = []RelationProgramDefinition{{
		Target: target, Point: ownerUnit.Graph.Exit(), ExternallyReachable: true,
	}}
	program, err := FreezeRelationProgram(
		[]RelationProgramUnit{ownerUnit, formalTemplateFreezeUnit(t, target)},
		testAcyclicCallTopology(t, owner, target),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(program.formalRegion.resources) != 2 || len(program.formalRegion.definitions) != 2 {
		t.Fatalf("definition/resource topology = %#v/%#v", program.formalRegion.definitions, program.formalRegion.resources)
	}
	definitionCell := program.formalRegion.definitions[1].cell
	resourceCell := program.formalRegion.resources[1].cell
	for _, cell := range []formalRelationCell{definitionCell, resourceCell} {
		equation, ok := program.formalTemplate.equation(cell)
		if !ok || equation.Operator.footprint.cell != cell || equation.Operator.footprint.owner != program.formalFibers.operatorFootprints {
			t.Fatalf("cell %+v has no exact full-pipeline operator footprint: %#v/%t", cell, equation, ok)
		}
	}
	definitionEquation, _ := program.formalTemplate.equation(definitionCell)
	definition := program.formalRegion.definitions[definitionCell.Definition]
	targetSpan, present := program.formalFibers.span(definition.target)
	if !present || definitionEquation.Operator.definitionTransaction == nil ||
		!definitionEquation.Operator.definitionTransaction.validFor(program, definitionEquation.Operator) ||
		definitionEquation.Operator.footprint.source.KeySpace() != targetSpan.keys ||
		!definitionEquation.Operator.footprint.source.ValidFor(program.bodies[definition.target-1].productDomain, targetSpan.keys) {
		t.Fatalf("definition cell %+v has no exact target-owned transaction/source selector: %#v", definitionCell, definitionEquation.Operator)
	}
	resourceEquation, _ := program.formalTemplate.equation(resourceCell)
	resourceTransaction := resourceEquation.Operator.resourceTransaction
	if resourceTransaction == nil || !resourceTransaction.validFor(program, resourceEquation.Operator) ||
		resourceTransaction.ref != resourceCell.Resource || resourceTransaction.owner != resourceCell.Variable {
		t.Fatalf("resource cell %+v has no exact cell-owned transaction: %#v", resourceCell, resourceTransaction)
	}
}

func TestFormalRelationTemplateRepeatedFreezeHasNoStructuralGrowth(t *testing.T) {
	const binder loopMuTerm = 1
	code := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalTemplateTestProgram(t, code)
	first := formalTemplateTestFreeze(t, program)
	wantEquations, wantInputs := formalTemplateShape(first)
	wantCells := append([]formalRelationCell(nil), program.formalRegion.cells...)
	wantIncoming := cloneFormalTemplateIncoming(program.formalRegion.incoming)
	for iteration := 0; iteration < 32; iteration++ {
		next := formalTemplateTestFreeze(t, program)
		equations, inputs := formalTemplateShape(next)
		if equations != wantEquations || inputs != wantInputs || !reflect.DeepEqual(first.equations, next.equations) {
			t.Fatalf("freeze %d grew/changed syntax: equations %d/%d inputs %d/%d", iteration, equations, wantEquations, inputs, wantInputs)
		}
	}
	if !reflect.DeepEqual(program.formalRegion.cells, wantCells) || !reflect.DeepEqual(program.formalRegion.incoming, wantIncoming) {
		t.Fatal("template freeze mutated its formal region authority")
	}
}

func TestFormalRelationTemplateRejectsMissingAndDuplicateInputs(t *testing.T) {
	build := func(t *testing.T) (*RelationProgram, formalRelationCell) {
		code := formalRegionTestCode([]relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution}}, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		}, 1)
		program := formalTemplateTestProgram(t, code)
		return program, formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	}
	t.Run("missing", func(t *testing.T) {
		program, target := build(t)
		program.formalRegion.incoming[target] = nil
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("duplicate", func(t *testing.T) {
		program, target := build(t)
		incoming := program.formalRegion.incoming[target]
		if len(incoming) != 1 {
			t.Fatalf("step inputs = %#v", incoming)
		}
		program.formalRegion.incoming[target] = append(incoming, incoming[0])
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("missing-callee-outcome", func(t *testing.T) {
		caller := formalRegionTestCode([]relationNode{{}, {
			kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}}},
		}}, 0)
		callee := formalRegionTestCode([]relationNode{
			{}, {kind: relationNodeChoice, whenTrue: 2, whenFalse: 3},
			{kind: relationNodeOutcome, outcome: 1}, {kind: relationNodeOutcome, outcome: 2},
		}, 2)
		program := formalTemplateTestProgram(t, caller, callee)
		target := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
		formalTemplateTestDeleteInfluence(t, program, target, formalRelationInfluenceCalleeOutcome)
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("missing-nonreturning-pair", func(t *testing.T) {
		caller := formalRegionTestCode([]relationNode{{}, {
			kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}}},
		}}, 0)
		callee := formalRegionTestCode([]relationNode{{}, {kind: relationNodeNonreturning}}, 0)
		program := formalTemplateTestProgram(t, caller, callee)
		formalTemplateTestDeleteInfluence(t, program, program.formalRegion.nonreturning[0], formalRelationInfluenceCalleeNonreturning)
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("missing-resource-feedback", func(t *testing.T) {
		owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		targetCode := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		program := formalRegionTestProgram(owner, targetCode)
		program.definitions = []relationProgramDefinition{{owner: 1, target: 2, point: 7, frame: 1, externallyReachable: true}}
		region, err := freezeFormalRelationRegionInventory(program)
		if err != nil {
			t.Fatal(err)
		}
		program.formalRegion = region
		formalTemplateTestDeleteInfluence(t, program, region.resources[1].cell, formalRelationInfluenceResourceFeedback)
		formalTemplateTestRejectFreeze(t, program)
	})
}

func TestFormalRelationTemplateRejectsOperatorMismatchedInfluences(t *testing.T) {
	t.Run("ordinary-flow-as-choice", func(t *testing.T) {
		program := formalTemplateTestProgram(t, formalRegionTestCode([]relationNode{{}, {
			kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepContribution}},
		}}, 0))
		target := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
		formalTemplateTestRewriteInfluence(t, program, target, formalRelationInfluenceFlow, func(influence *formalRelationInfluence) {
			influence.Kind = formalRelationInfluenceChoiceTrue
		})
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("choice-arm-swapped", func(t *testing.T) {
		program := formalTemplateTestProgram(t, formalRegionTestCode([]relationNode{
			{}, {kind: relationNodeChoice, whenTrue: 2, whenFalse: 3},
			{kind: relationNodeBottom}, {kind: relationNodeBottom},
		}, 0))
		target := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
		formalTemplateTestRewriteInfluence(t, program, target, formalRelationInfluenceChoiceTrue, func(influence *formalRelationInfluence) {
			influence.Kind = formalRelationInfluenceChoiceFalse
		})
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("loop-route-swapped", func(t *testing.T) {
		const binder loopMuTerm = 1
		program := formalTemplateTestProgram(t, formalRegionTestCode([]relationNode{
			{}, {kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
			{kind: relationNodeBottom},
		}, 0))
		target := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
		formalTemplateTestRewriteInfluence(t, program, target, formalRelationInfluenceLoopFeedback, func(influence *formalRelationInfluence) {
			influence.Kind = formalRelationInfluenceLoopExit
		})
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("nonreturning-pair-wrong-site", func(t *testing.T) {
		caller := formalRegionTestCode([]relationNode{{}, {
			kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepContribution},
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}},
			},
		}}, 0)
		callee := formalRegionTestCode([]relationNode{{}, {kind: relationNodeNonreturning}}, 0)
		program := formalTemplateTestProgram(t, caller, callee)
		target := program.formalRegion.nonreturning[0]
		formalTemplateTestRewriteInfluence(t, program, target, formalRelationInfluenceApplyNonreturningPredecessor, func(influence *formalRelationInfluence) {
			influence.Site = formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
		})
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("definition-outcome-as-seed", func(t *testing.T) {
		owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		targetCode := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		program := formalRegionTestProgram(owner, targetCode)
		program.definitions = []relationProgramDefinition{{owner: 1, target: 2, point: 7, frame: 1, externallyReachable: true}}
		region, err := freezeFormalRelationRegionInventory(program)
		if err != nil {
			t.Fatal(err)
		}
		program.formalRegion = region
		definition := region.definitions[1].cell
		formalTemplateTestRewriteInfluence(t, program, definition, formalRelationInfluenceDefinitionOutcome, func(influence *formalRelationInfluence) {
			influence.Kind = formalRelationInfluenceDefinitionSeed
		})
		formalTemplateTestRejectFreeze(t, program)
	})
	t.Run("resource-seed-as-flow", func(t *testing.T) {
		owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		targetCode := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
		program := formalRegionTestProgram(owner, targetCode)
		program.definitions = []relationProgramDefinition{{owner: 1, target: 2, point: 7, frame: 1, externallyReachable: true}}
		region, err := freezeFormalRelationRegionInventory(program)
		if err != nil {
			t.Fatal(err)
		}
		program.formalRegion = region
		resource := region.resources[1].cell
		formalTemplateTestRewriteInfluence(t, program, resource, formalRelationInfluenceResourceSeed, func(influence *formalRelationInfluence) {
			influence.Kind = formalRelationInfluenceFlow
		})
		formalTemplateTestRejectFreeze(t, program)
	})
}

func formalTemplateTestProgram(t *testing.T, codes ...*relationCode) *RelationProgram {
	t.Helper()
	program := formalRegionTestProgram(codes...)
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	return program
}

func formalTemplateTestFreeze(t *testing.T, program *RelationProgram) *formalRelationTemplate {
	t.Helper()
	formalTemplateTestPrepareRootInputs(t, program)
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	return template
}

func formalTemplateTestRejectFreeze(t *testing.T, program *RelationProgram) {
	t.Helper()
	formalTemplateTestPrepareRootInputs(t, program)
	if _, err := freezeFormalRelationTemplate(program); err == nil {
		t.Fatal("malformed formal influence was accepted")
	}
}

func formalTemplateTestRewriteInfluence(t *testing.T, program *RelationProgram, target formalRelationCell, kind formalRelationInfluenceKind, rewrite func(*formalRelationInfluence)) {
	t.Helper()
	incoming := program.formalRegion.incoming[target]
	for index := range incoming {
		if incoming[index].Kind == kind {
			rewrite(&incoming[index])
			program.formalRegion.incoming[target] = incoming
			return
		}
	}
	t.Fatalf("target %+v has no influence kind %d: %#v", target, kind, incoming)
}

func formalTemplateTestDeleteInfluence(t *testing.T, program *RelationProgram, target formalRelationCell, kind formalRelationInfluenceKind) {
	t.Helper()
	incoming := program.formalRegion.incoming[target]
	for index := range incoming {
		if incoming[index].Kind == kind {
			program.formalRegion.incoming[target] = append(incoming[:index:index], incoming[index+1:]...)
			return
		}
	}
	t.Fatalf("target %+v has no influence kind %d: %#v", target, kind, incoming)
}

func formalTemplateTestPrepareRootInputs(t *testing.T, program *RelationProgram) {
	t.Helper()
	if program.formalFibers != nil {
		return
	}
	reg := standard.Registry()
	program.registry = reg
	program.byBody = make(map[lexicalidentity.StableLexicalBodyID]relationVar, len(program.bodies))
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	for index := range program.bodies {
		body := &program.bodies[index]
		needsContribution := false
		callSites := make(map[cfg.Point]factflow.CallSite)
		for nodeIndex := range body.relation.code.nodes {
			for stepIndex := range body.relation.code.nodes[nodeIndex].steps {
				step := &body.relation.code.nodes[nodeIndex].steps[stepIndex]
				if step.kind == boundaryStepExternalCall && step.point > 0 {
					callSites[step.point] = factflow.NewCallSite(factflow.CallSiteConfig{
						Context: factflow.CallSiteContextStatement, Point: step.point, HasPoint: true,
					})
				}
				if step.kind == boundaryStepContribution && step.contribution == 0 {
					step.contribution = 1
					needsContribution = true
				}
			}
		}
		if needsContribution {
			body.relation.code.contributions = []semanticContribution{{}, {suspensionKnown: true}}
		}
		bodyID := lexicalidentity.FunctionBody(namespace, uint64(index+1))
		arena := NewArena(reg)
		if !arena.bindLexicalOwner(bodyID) {
			t.Fatal("bind template lexical owner")
		}
		if err := arena.sealMiddleRegisterSchema(); err != nil {
			t.Fatal(err)
		}
		if err := arena.middle.bindInputs(Shape{}, nil); err != nil {
			t.Fatal(err)
		}
		effects := NewEffectArena(arena)
		arena.Seal()
		effects.Seal()
		body.body = bodyID
		body.variable = relationVar(index + 1)
		graph := cfg.New()
		graph.AddEdge(graph.Entry(), graph.Exit(), false)
		body.graph = graph
		resolver := visibility.NewResolver(visibility.NewTable(nil))
		paths := factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
		body.keys = resolver.KeySpace()
		body.relation.shape = Shape{}
		body.relation.arena = arena
		body.relation.effects = effects
		body.relation.code.terms = arena
		body.relation.code.effects = effects
		for outcome := boundaryOutcomeRef(1); int(outcome) < len(body.relation.code.outcomes); outcome++ {
			transaction, exact := factapply.PlanReturnTransactionSources(factflow.Facts{}, cfg.Point(outcome), nil)
			if !exact {
				t.Fatalf("freeze template Outcome %d N5", outcome)
			}
			body.relation.code.outcomes[outcome].returnTransaction = returnTransactionTerm{transaction: transaction}
		}
		body.productDomain = state.RegisteredProductDomain(reg)
		body.domain = state.Domain(reg)
		body.plan = operationplan.New(graph, factflow.FactsInput{CallSites: callSites}).
			WithBoundaryParams(nil).
			WithBoundaryParamContracts(nil).
			WithBoundaryCaptures(nil).
			WithBoundaryGlobals(nil)
		body.entrySeedPlan = state.NewEntrySeedPlan(nil)
		body.initialStatePlan = testInitialStatePlan(t, bodyID, graph)
		body.pathSemantics = paths
		body.returns = factapply.NewReturnAuthority(paths, factflow.Facts{})
		program.byBody[bodyID] = body.variable
	}
	formalFibers, err := freezeFormalFiberInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalFibers = formalFibers
}

func formalTemplateHasInput(equation formalRelationEquation, source formalRelationCell, kind formalRelationInfluenceKind) bool {
	for _, input := range equation.Inputs {
		if input.Source.cell == source && input.Influence == kind {
			return true
		}
	}
	return false
}

func formalTemplateShape(template *formalRelationTemplate) (equations, inputs int) {
	if template == nil {
		return 0, 0
	}
	for _, equation := range template.equations {
		inputs += len(equation.Inputs)
	}
	return len(template.equations), inputs
}

func cloneFormalTemplateIncoming(in map[formalRelationCell][]formalRelationInfluence) map[formalRelationCell][]formalRelationInfluence {
	out := make(map[formalRelationCell][]formalRelationInfluence, len(in))
	for cell, influences := range in {
		out[cell] = append([]formalRelationInfluence(nil), influences...)
	}
	return out
}
