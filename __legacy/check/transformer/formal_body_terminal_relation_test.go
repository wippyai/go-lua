package transformer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
)

func TestFormalBodyTerminalRelationJoinsOrderedOutcomesOnceWithoutEquationReplay(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeOutcome, outcome: 2},
	})
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	wantEquations := cloneFormalRelationEquations(program.formalTemplate.equations)
	wantValues := make(map[formalRelationCell]formalRelationTuple, len(execution.values))
	for cell, tuple := range execution.values {
		wantValues[cell] = tuple
	}

	relation, err := execution.bodyTerminalRelation(context.Background(), program.bodies[0].body)
	if err != nil {
		t.Fatal(err)
	}
	if len(relation.normal) != 2 || relation.normal[0] != wantValues[program.formalRegion.outcomes[0][0]] ||
		relation.normal[1] != wantValues[program.formalRegion.outcomes[0][1]] {
		t.Fatalf("ordered normal terminals = %#v", relation.normal)
	}
	if relation.joined.bottom() {
		t.Fatal("joined body terminal relation is Bottom")
	}
	formalOutcomeTestHasOccurrence(t, execution.algebra, relation.joined, 1)
	formalOutcomeTestHasOccurrence(t, execution.algebra, relation.joined, 2)
	if !reflect.DeepEqual(program.formalTemplate.equations, wantEquations) || !reflect.DeepEqual(execution.values, wantValues) {
		t.Fatal("terminal projection replayed or mutated the solved equation system")
	}
}

func TestFormalBodyTerminalRelationUnreachableAndCancellationPublishNothing(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeBottom},
		{kind: relationNodeOutcome, outcome: 1},
	})
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	relation, err := execution.bodyTerminalRelation(context.Background(), program.bodies[0].body)
	if err != nil || !relation.joined.bottom() || len(relation.normal) != 1 || !relation.normal[0].bottom() || !relation.nonreturning.bottom() {
		t.Fatalf("unreachable body terminal relation = %#v, %v", relation, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled, err := execution.bodyTerminalRelation(ctx, program.bodies[0].body)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(canceled, formalBodyTerminalRelation{}) {
		t.Fatalf("canceled body terminal relation = %#v, %v", canceled, err)
	}
}
