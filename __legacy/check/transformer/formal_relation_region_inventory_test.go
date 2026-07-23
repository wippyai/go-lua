package transformer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func formalRegionTestCode(nodes []relationNode, outcomeCount int, publications ...relationPointPublication) *relationCode {
	if len(nodes) == 0 || nodes[0].kind != relationNodeInvalid {
		nodes = append([]relationNode{{}}, nodes...)
	}
	return &relationCode{
		root: 1, nodes: nodes, outcomes: make([]boundaryOutcomeTuple, outcomeCount+1),
		publication: relationPublicationPlan{points: append([]relationPointPublication(nil), publications...)}, sealed: true,
	}
}

func formalRegionTestProgram(codes ...*relationCode) *RelationProgram {
	program := &RelationProgram{bodies: make([]relationProgramBody, len(codes))}
	for index, code := range codes {
		program.bodies[index] = relationProgramBody{variable: relationVar(index + 1), relation: Relation{code: code}}
	}
	return program
}

func hasFormalInfluence(incoming []formalRelationInfluence, source formalRelationCell, kind formalRelationInfluenceKind) bool {
	for _, influence := range incoming {
		if influence.Source == source && influence.Kind == kind {
			return true
		}
	}
	return false
}

func TestFormalRelationObservableStepQuotientRemovesOnlyLinearIntermediates(t *testing.T) {
	code := formalRegionTestCode([]relationNode{{}, {
		kind: relationNodeSequence,
		steps: []boundaryStep{
			{kind: boundaryStepContribution}, {kind: boundaryStepContribution},
			{kind: boundaryStepContribution}, {kind: boundaryStepContribution},
		},
		next: 2,
	}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	program := formalRegionTestProgram(code)
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	before := len(region.cells)
	if err := region.freezeObservableStepQuotient(program); err != nil {
		t.Fatal(err)
	}
	terminal := formalRelationCell{Variable: 1, Root: 1, Step: 4, Kind: formalRelationCellStep}
	if got := len(region.cells); got != before-3 {
		t.Fatalf("quotient cells = %d, want %d", got, before-3)
	}
	if segment := region.stepSegments[terminal]; len(segment) != 4 || segment[0].cell.Step != 1 || segment[3].cell.Step != 4 {
		t.Fatalf("quotient lexical stage order = %#v", segment)
	}
	for step := uint32(1); step < 4; step++ {
		cell := formalRelationCell{Variable: 1, Root: 1, Step: step, Kind: formalRelationCellStep}
		if _, retained := region.plan.CanonicalIndex(cell); retained {
			t.Fatalf("linear intermediate Step %d retained a solver cell", step)
		}
	}
}

func TestFormalRelationRegionEveryWTOComponentOwnsTypedWidenHead(t *testing.T) {
	outer, inner := loopMuTerm(1), loopMuTerm(2)
	loopBody := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeLoopMu, binder: outer, body: 2, exits: []relationRootRef{7}},
		{kind: relationNodeLoopMu, binder: inner, body: 3, exits: []relationRootRef{5}},
		{kind: relationNodeChoice, whenTrue: 4, whenFalse: 6},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: inner}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: outer}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: inner, route: 0}}},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	left := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 3, frame: 1}}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	right := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	acyclic := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalRegionTestProgram(loopBody, left, right, acyclic)
	program.recursiveSCCs = [][]relationVar{{2, 3}}
	program.definitions = []relationProgramDefinition{{owner: 4, target: 2, point: 9, frame: 2, externallyReachable: true}}

	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range []formalRelationCell{
		{Variable: 1, Root: 2, Kind: formalRelationCellNode},
		{Variable: 1, Root: 3, Kind: formalRelationCellNode},
	} {
		if !inventory.widen[cell] {
			t.Fatalf("lexical loop body %+v is not a widening head", cell)
		}
	}
	recursiveBackedge := formalRelationCell{Variable: 3, Root: 1, Step: 1, Kind: formalRelationCellStep}
	recursiveTreeEdge := formalRelationCell{Variable: 2, Root: 1, Step: 1, Kind: formalRelationCellStep}
	acyclicApply := formalRelationCell{Variable: 4, Root: 1, Step: 1, Kind: formalRelationCellStep}
	if !inventory.widen[recursiveBackedge] || inventory.widen[recursiveTreeEdge] || inventory.widen[acyclicApply] {
		t.Fatalf("recursive cuts: backedge=%t tree=%t acyclic=%t", inventory.widen[recursiveBackedge], inventory.widen[recursiveTreeEdge], inventory.widen[acyclicApply])
	}
	if len(inventory.resources) != 2 || !inventory.widen[inventory.resources[1].cell] {
		t.Fatalf("resource widening head = %#v", inventory.resources)
	}
	nonreturningComponent := false
	var assertComponents func([]solve.WTOElement[formalRelationCell])
	assertComponents = func(elements []solve.WTOElement[formalRelationCell]) {
		for _, element := range elements {
			if element.IsComponent() {
				if !inventory.componentOwnsTypedWidenHead(element) {
					t.Fatalf("WTO component headed by %+v has no typed widening head", element.Vertex)
				}
				allNonreturning, physicalWiden := true, false
				var inspect func(solve.WTOElement[formalRelationCell])
				inspect = func(item solve.WTOElement[formalRelationCell]) {
					allNonreturning = allNonreturning && item.Vertex.Kind == formalRelationCellNonreturning
					physicalWiden = physicalWiden || inventory.widen[item.Vertex]
					for _, nested := range item.Body {
						inspect(nested)
					}
				}
				inspect(element)
				if allNonreturning {
					nonreturningComponent = true
					if physicalWiden {
						t.Fatal("finite-height nonreturning recurrence was marked WidenAt")
					}
				}
			}
			assertComponents(element.Body)
		}
	}
	assertComponents(inventory.plan.Elements())
	if !nonreturningComponent {
		t.Fatal("mutual recursion did not expose its finite-height nonreturning WTO component")
	}
}

func TestFormalRelationRegionOwnsBodyNonreturningCell(t *testing.T) {
	program := formalRegionTestProgram(formalRegionTestCode([]relationNode{{}, {kind: relationNodeNonreturning}}, 0))
	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	nonreturning := formalRelationCell{Variable: 1, Kind: formalRelationCellNonreturning}
	source := formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode}
	if inventory.nonreturning[0] != nonreturning || !hasFormalInfluence(inventory.incoming[nonreturning], source, formalRelationInfluenceLocalNonreturning) {
		t.Fatalf("body nonreturning equation = %#v / %#v", inventory.nonreturning, inventory.incoming[nonreturning])
	}
}

func TestFormalRelationRegionLinksCalleeNonreturningAtExactApply(t *testing.T) {
	caller := formalRegionTestCode([]relationNode{
		{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	callee := formalRegionTestCode([]relationNode{{}, {kind: relationNodeNonreturning}}, 0)
	inventory, err := freezeFormalRelationRegionInventory(formalRegionTestProgram(caller, callee))
	if err != nil {
		t.Fatal(err)
	}
	target := inventory.nonreturning[0]
	calleeNonreturning := inventory.nonreturning[1]
	predecessor := formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode}
	wantSite := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	matchedCallee, matchedPredecessor := false, false
	for _, influence := range inventory.incoming[target] {
		if influence.Source == calleeNonreturning && influence.Kind == formalRelationInfluenceCalleeNonreturning && influence.Site == wantSite {
			matchedCallee = true
		}
		if influence.Source == predecessor && influence.Kind == formalRelationInfluenceApplyNonreturningPredecessor && influence.Site == wantSite {
			matchedPredecessor = true
		}
	}
	if !matchedCallee || !matchedPredecessor {
		t.Fatalf("caller nonreturning inputs = %#v", inventory.incoming[target])
	}
	if !inventory.plan.CoversInfluence(calleeNonreturning, target) || !inventory.plan.CoversInfluence(predecessor, target) {
		t.Fatal("formal WTO omits an Apply nonreturning operand")
	}
	for _, successor := range inventory.successors[predecessor] {
		if successor == inventory.roots[1] {
			t.Fatal("ordinary Apply invented caller-to-callee root edge")
		}
	}
}

func TestFormalRelationRegionBindsOrdinaryDefinitionFromPointPublication(t *testing.T) {
	owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1, relationPointPublication{point: 7, ref: 1})
	target := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	program := formalRegionTestProgram(owner, target)
	program.definitions = []relationProgramDefinition{{owner: 1, target: 2, point: 7, frame: 1}}
	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	definition := inventory.definitions[1]
	publication := formalRelationCell{Variable: 1, Root: 1, Kind: formalRelationCellNode}
	targetOutcome := inventory.outcomes[1][0]
	if !hasFormalInfluence(inventory.incoming[definition.cell], publication, formalRelationInfluenceDefinitionSeed) ||
		!hasFormalInfluence(inventory.incoming[definition.cell], targetOutcome, formalRelationInfluenceDefinitionOutcome) {
		t.Fatalf("ordinary definition equations = %#v / %#v", inventory.incoming[definition.cell], inventory.incoming[inventory.roots[1]])
	}
	if len(inventory.incoming[inventory.roots[1]]) != 0 {
		t.Fatalf("target root has caller-owned inputs: %#v", inventory.incoming[inventory.roots[1]])
	}
}

func TestFormalRelationRegionBuildsMutualLexicalResourceWorld(t *testing.T) {
	owner := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	left := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	right := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	program := formalRegionTestProgram(owner, left, right)
	program.definitions = []relationProgramDefinition{
		{owner: 1, target: 3, point: 9, frame: 2, externallyReachable: true},
		{owner: 1, target: 2, point: 8, frame: 1, externallyReachable: true},
	}
	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.resources) != 2 || len(inventory.resources[1].members) != 2 || !inventory.widen[inventory.resources[1].cell] {
		t.Fatalf("resource inventory = %#v, widen=%t", inventory.resources, inventory.widen[inventory.resources[1].cell])
	}
	world := inventory.resources[1].cell
	if !hasFormalInfluence(inventory.incoming[world], inventory.outcomes[0][0], formalRelationInfluenceResourceSeed) ||
		!hasFormalInfluence(inventory.incoming[world], inventory.definitions[1].cell, formalRelationInfluenceResourceFeedback) ||
		!hasFormalInfluence(inventory.incoming[world], inventory.definitions[2].cell, formalRelationInfluenceResourceFeedback) {
		t.Fatalf("resource world inputs = %#v", inventory.incoming[world])
	}
	for _, member := range inventory.resources[1].members {
		definition := inventory.definitions[member]
		if !hasFormalInfluence(inventory.incoming[definition.cell], world, formalRelationInfluenceDefinitionSeed) ||
			!hasFormalInfluence(inventory.incoming[definition.cell], inventory.outcomes[definition.target-1][0], formalRelationInfluenceDefinitionOutcome) {
			t.Fatalf("resource member %d is not a complete world+outcome->definition artifact", member)
		}
		if len(inventory.incoming[inventory.roots[definition.target-1]]) != 0 {
			t.Fatalf("resource member %d fed target root: %#v", member, inventory.incoming[inventory.roots[definition.target-1]])
		}
		for _, targetOutcome := range inventory.outcomes[definition.target-1] {
			if hasFormalInfluence(inventory.incoming[world], targetOutcome, formalRelationInfluenceResourceFeedback) {
				t.Fatalf("resource member %d bypassed its definition artifact", member)
			}
		}
	}
}

func TestFormalRelationRegionDefinitionPermutationIsDeterministic(t *testing.T) {
	build := func(reverse bool) *formalRelationRegionInventory {
		program := formalRegionTestProgram(
			formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1, relationPointPublication{point: 7, ref: 1}),
			formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1),
			formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1),
		)
		program.definitions = []relationProgramDefinition{{owner: 1, target: 2, point: 7, frame: 2}, {owner: 1, target: 3, point: 7, frame: 1}}
		if reverse {
			program.definitions[0], program.definitions[1] = program.definitions[1], program.definitions[0]
		}
		inventory, err := freezeFormalRelationRegionInventory(program)
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}
	left, right := build(false), build(true)
	if !reflect.DeepEqual(left.cells, right.cells) || !reflect.DeepEqual(left.incoming, right.incoming) || !reflect.DeepEqual(left.successors, right.successors) {
		t.Fatal("definition permutation changed formal equation inventory")
	}
}

func TestFormalRelationRegionVocabularyHasNoRouteContextOrDemandIdentity(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(formalRelationCell{}), reflect.TypeOf(formalRelationInfluence{}), reflect.TypeOf(formalRelationDefinition{}), reflect.TypeOf(formalRelationResource{})} {
		for field := 0; field < typ.NumField(); field++ {
			name := strings.ToLower(typ.Field(field).Name + " " + typ.Field(field).Type.String())
			for _, forbidden := range []string{"route", "context", "invocation", "demand"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s field %s contains forbidden identity vocabulary %q", typ, typ.Field(field).Name, forbidden)
				}
			}
		}
	}
}

func TestFormalRelationRegionLinksExactClosureDefinitionToApply(t *testing.T) {
	terms := &Arena{callFrames: []callFrameNode{{}, {variable: 2}, {variable: 3, closureProducer: 1}}}
	caller := formalRegionTestCode([]relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{
		{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}},
		{kind: boundaryStepApply, apply: relationApplyRef{variable: 3, frame: 2}},
	}}}, 0)
	caller.terms = terms
	producer := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	target := formalRegionTestCode([]relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, 1)
	program := formalRegionTestProgram(caller, producer, target)
	program.definitions = []relationProgramDefinition{{owner: 2, target: 3, point: cfg.Point(4), frame: 1, externallyReachable: true}}
	inventory, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	step := formalRelationCell{Variable: 1, Root: 1, Step: 2, Kind: formalRelationCellStep}
	definition := inventory.definitions[inventory.resources[1].members[0]].cell
	if !hasFormalInfluence(inventory.incoming[step], definition, formalRelationInfluenceClosureDefinition) {
		t.Fatalf("closure Apply inputs = %#v", inventory.incoming[step])
	}
	if hasFormalInfluence(inventory.incoming[step], inventory.resources[1].cell, formalRelationInfluenceClosureDefinition) {
		t.Fatal("closure Apply bypassed its exact definition artifact")
	}
}
