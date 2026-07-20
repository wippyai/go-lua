package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalRelationExecutorCarriesRootThroughEmptySequence(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeSequence, next: 2},
		{kind: relationNodeBottom},
	})
	first, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	root := program.formalRegion.roots[0]
	target := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
	for name, run := range map[string]*formalRelationExecution{"first": first, "second": second} {
		rootValue, rootOK := run.values[root]
		targetValue, targetOK := run.values[target]
		if !rootOK || !targetOK || rootValue.bottom() ||
			!run.algebra.same(rootValue, targetValue) || run.algebra.err() != nil {
			t.Fatalf("%s execution did not preserve exact node identity: root=%#v target=%#v err=%v", name, rootValue, targetValue, run.algebra.err())
		}
	}
	if first.values[root].root.owner == second.values[root].root.owner {
		t.Fatal("independent formal executions shared a tuple directory")
	}
	if err := first.algebra.validateTuple(second.values[root]); err == nil {
		t.Fatal("first execution accepted the second execution's tuple")
	}
	if err := second.algebra.validateTuple(first.values[root]); err == nil {
		t.Fatal("second execution accepted the first execution's tuple")
	}
}

func TestFormalRelationExecutorReadsOnlyFrozenInputs(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeSequence, next: 2},
		{kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootCell := program.formalRegion.roots[0]
	rootEquation, _ := program.formalTemplate.equation(rootCell)
	rootValue := evaluateFormalRelationEquation(algebra, rootEquation, func(formalRelationCell) formalRelationTuple {
		t.Fatal("root equation performed a hidden cell read")
		return formalRelationTuple{}
	})
	if rootValue.bottom() || algebra.err() != nil {
		t.Fatalf("root equation = %#v, %v", rootValue, algebra.err())
	}
	targetCell := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
	targetEquation, _ := program.formalTemplate.equation(targetCell)
	reads := make(map[formalRelationCell]int)
	targetValue := evaluateFormalRelationEquation(algebra, targetEquation, func(cell formalRelationCell) formalRelationTuple {
		reads[cell]++
		if cell != rootCell {
			t.Fatalf("executor read undeclared cell %+v", cell)
		}
		return rootValue
	})
	if !algebra.same(rootValue, targetValue) || len(reads) != 1 || reads[rootCell] != 1 || algebra.err() != nil {
		t.Fatalf("explicit input reads = %#v, target=%#v, err=%v", reads, targetValue, algebra.err())
	}
}

func TestFormalRelationExecutorRequiresSealedOwnedTemplate(t *testing.T) {
	t.Run("unsealed", func(t *testing.T) {
		program := formalRelationExecutorTestProgram(t, []relationNode{
			{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
		})
		unsealed := *program.formalTemplate
		unsealed.sealed = false
		program.formalTemplate = &unsealed
		if result, err := executeFormalRelation(context.Background(), program); err == nil || result != nil {
			t.Fatalf("unsealed template published result %#v, %v", result, err)
		}
	})

	t.Run("foreign", func(t *testing.T) {
		program := formalRelationExecutorTestProgram(t, []relationNode{
			{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
		})
		foreign := formalRelationExecutorTestProgram(t, []relationNode{
			{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
		})
		program.formalTemplate = foreign.formalTemplate
		if result, err := executeFormalRelation(context.Background(), program); err == nil || result != nil {
			t.Fatalf("foreign template published result %#v, %v", result, err)
		}
	})
}

func TestFormalRelationExecutorCancellationPublishesNothing(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{}, {kind: relationNodeSequence, next: 2}, {kind: relationNodeBottom},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeFormalRelation(ctx, program)
	if result != nil || err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled execution = %#v, %v", result, err)
	}
}

func TestFormalRelationExecutorProjectsLiveOutcome(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeSequence, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil || result == nil {
		t.Fatalf("live Outcome projection = %#v, %v", result, err)
	}
	predecessor := result.values[formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}]
	outcome := result.values[program.formalRegion.outcomes[0][0]]
	formalOutcomeTestExactProjection(t, result.algebra, predecessor, outcome, 1)
	if result.algebra.err() != nil {
		t.Fatal(result.algebra.err())
	}
}

func TestFormalRelationExecutorExecutesLiveChoice(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	guard := base.bodies[0].relation.code.terms.Truthy(base.bodies[0].relation.code.terms.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeBottom},
	})
	result, err := executeFormalRelation(context.Background(), program)
	if result == nil || err != nil {
		t.Fatalf("live Choice = %#v, %v", result, err)
	}
}

func formalRelationExecutorTestProgram(t *testing.T, nodes []relationNode) *RelationProgram {
	t.Helper()
	return formalRelationExecutorTestProgramFromBase(t, formalRootInputTestProgram(t, standard.Registry()), nodes)
}

func formalRelationExecutorTestProgramFromBase(t *testing.T, program *RelationProgram, nodes []relationNode) *RelationProgram {
	t.Helper()
	code := program.bodies[0].relation.code
	code.nodes = append([]relationNode(nil), nodes...)
	code.root = 1
	seedOutcome := boundaryOutcomeTuple{}
	if len(code.outcomes) > 1 {
		seedOutcome = code.outcomes[1]
	}
	maxOutcome := boundaryOutcomeRef(0)
	seenOutcomes := make(map[boundaryOutcomeRef]struct{})
	for _, node := range code.nodes {
		if node.kind != relationNodeOutcome {
			continue
		}
		if node.outcome == 0 {
			t.Fatal("formal executor fixture has zero Outcome ref")
		}
		if _, duplicate := seenOutcomes[node.outcome]; duplicate {
			t.Fatalf("formal executor fixture shares Outcome ref %d", node.outcome)
		}
		seenOutcomes[node.outcome] = struct{}{}
		if node.outcome > maxOutcome {
			maxOutcome = node.outcome
		}
	}
	code.outcomes = make([]boundaryOutcomeTuple, int(maxOutcome)+1)
	for outcome := boundaryOutcomeRef(1); outcome <= maxOutcome; outcome++ {
		code.outcomes[outcome] = seedOutcome
	}
	refreezeFormalTestStaticTopology(t, program)
	components, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents = components
	guards, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalGuards = guards
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalTemplate = template
	return program
}
