package transformer

import (
	"context"
	"strings"
	"testing"
)

func TestFormalLocalNonreturningPreservesCompleteProductAndSeparateNormalRoute(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeNonreturning},
		{kind: relationNodeOutcome, outcome: 1},
	})
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	root := result.values[program.formalRegion.roots[0]]
	nonreturning := result.values[program.formalRegion.nonreturning[0]]
	if root.bottom() || !result.algebra.same(root, nonreturning) {
		t.Fatalf("local nonreturning = %#v, root=%#v, want exact complete tuple identity", nonreturning, root)
	}
	if normal := result.values[program.formalRegion.outcomes[0][0]]; !normal.bottom() {
		t.Fatalf("disjoint normal route = %#v, want Bottom", normal)
	}
	span, _, _, ok := result.algebra.span(nonreturning.variable)
	if !ok {
		t.Fatal("local nonreturning descriptor span")
	}
	groups := span.groupDescriptors()
	want := program.bodies[0].productDomain.LaneInventory()
	if len(groups) != len(want) {
		t.Fatalf("local nonreturning groups = %d, want every %d registered lanes", len(groups), len(want))
	}
	for index := range groups {
		if groups[index].lane != want[index] {
			t.Fatalf("local nonreturning group %d = %q, want %q", index, groups[index].lane.ID(), want[index].ID())
		}
	}
}

func TestFormalLocalNonreturningBottomStaysBottom(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeBottom},
		{kind: relationNodeNonreturning},
	})
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.values[program.formalRegion.nonreturning[0]]; !got.bottom() {
		t.Fatalf("unreachable local nonreturning = %#v, want Bottom", got)
	}
}

func TestFormalLocalNonreturningRejectsMalformedAndForeignOwnership(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{{}, {kind: relationNodeNonreturning}})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	live := formalTupleTestLive(t, algebra, 1)
	for name, operator := range map[string]formalRelationOperatorRef{
		"wrong-kind": {kind: formalRelationCellOutcome, code: program.bodies[0].relation.code},
		"no-owner":   {kind: formalRelationCellNonreturning},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := algebra.projectLocalNonreturning(operator, live); err == nil {
				t.Fatal("malformed local nonreturning operator was accepted")
			}
		})
	}
	foreignProgram := formalRelationExecutorTestProgram(t, []relationNode{{}, {kind: relationNodeNonreturning}})
	foreign, err := newFormalTupleAlgebra(context.Background(), foreignProgram)
	if err != nil {
		t.Fatal(err)
	}
	operator := formalRelationOperatorRef{kind: formalRelationCellNonreturning, code: program.bodies[0].relation.code}
	if _, err := algebra.projectLocalNonreturning(operator, formalTupleTestLive(t, foreign, 1)); err == nil {
		t.Fatal("foreign local nonreturning tuple was accepted")
	}
}

func TestFormalApplyNonreturningSitesRemainDistinctAndRejectUnsealedTransaction(t *testing.T) {
	callee := formalRegionTestCode([]relationNode{{}, {kind: relationNodeNonreturning}}, 0)
	caller := formalRegionTestCode([]relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{
			{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}},
			{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: 1}},
		}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	}, 1)
	program := formalTemplateTestProgram(t, caller, callee)
	if len(program.formalRegion.nonreturning) == 0 {
		t.Fatal("caller nonreturning equation")
	}
	cell := program.formalRegion.nonreturning[0]
	type pair struct{ predecessor, callee int }
	pairs := make(map[formalRelationCell]pair)
	for _, input := range program.formalRegion.incoming[cell] {
		counts := pairs[input.Site]
		switch input.Kind {
		case formalRelationInfluenceApplyNonreturningPredecessor:
			counts.predecessor++
		case formalRelationInfluenceCalleeNonreturning:
			counts.callee++
		default:
			continue
		}
		pairs[input.Site] = counts
	}
	if len(pairs) != 2 {
		t.Fatalf("Apply nonreturning sites = %#v, want two distinct exact Sites", pairs)
	}
	for site, counts := range pairs {
		if site.Kind != formalRelationCellStep || counts != (pair{predecessor: 1, callee: 1}) {
			t.Fatalf("Apply nonreturning Site %+v pair = %#v", site, counts)
		}
	}
	formalTemplateTestPrepareRootInputs(t, program)
	program.formalSlots = program.formalFibers.slots
	components, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents = components
	cellRef := func(cell formalRelationCell) formalRelationCellRef {
		index, ok := program.formalRegion.plan.CanonicalIndex(cell)
		if !ok {
			t.Fatalf("formal cell %+v is outside the region", cell)
		}
		return formalRelationCellRef{region: program.formalRegion, cell: cell, index: index}
	}
	equation := formalRelationEquation{
		Cell:     cellRef(cell),
		Operator: formalRelationOperatorRef{kind: formalRelationCellNonreturning, code: caller, region: program.formalRegion},
	}
	for _, influence := range program.formalRegion.incoming[cell] {
		equation.Inputs = append(equation.Inputs, formalRelationTemplateInput{
			Source: cellRef(influence.Source), Influence: influence.Kind, Site: cellRef(influence.Site),
		})
	}

	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	values := map[relationVar]formalRelationTuple{
		1: formalTupleTestLive(t, algebra, 1),
		2: formalTupleTestLive(t, algebra, 2),
	}
	got := evaluateFormalRelationEquation(algebra, equation, func(cell formalRelationCell) formalRelationTuple {
		return values[cell.Variable]
	})
	if !got.bottom() || algebra.err() == nil || !strings.Contains(algebra.err().Error(), "transaction is incomplete") {
		t.Fatalf("live Apply nonreturning dependency published %#v, err=%v", got, algebra.err())
	}
}
