package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
)

func TestFormalChoicePartitionsAmbiguousTupleAndPreservesEveryProductFiber(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeBottom},
		{kind: relationNodeBottom},
	})
	run, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	root := run.values[program.formalRegion.roots[0]]
	left := run.values[formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}]
	right := run.values[formalRelationCell{Variable: 1, Root: 3, Kind: formalRelationCellNode}]
	if root.bottom() || left.bottom() || right.bottom() {
		t.Fatalf("ambiguous Choice lost a feasible route: root=%#v true=%#v false=%#v", root, left, right)
	}
	leftCare, _ := run.algebra.care(left)
	rightCare, _ := run.algebra.care(right)
	union, err := run.algebra.decisions.apply(context.Background(), uint8(decisionOr), true, leftCare, rightCare, decisionLeafOr)
	if err != nil || union != decisionTrue {
		t.Fatalf("Choice partition union = %d, %v; want true", union, err)
	}
	intersection, err := run.algebra.decisions.apply(context.Background(), uint8(decisionAnd), true, leftCare, rightCare, decisionLeafAnd)
	if err != nil || intersection != decisionFalse {
		t.Fatalf("Choice partition intersection = %d, %v; want false", intersection, err)
	}
	joined := run.algebra.combine(formalComponentJoin, left, right)
	if !run.algebra.same(root, joined) || run.algebra.err() != nil {
		t.Fatalf("complementary Choice routes did not join to source: root=%#v joined=%#v err=%v", root, joined, run.algebra.err())
	}
	formalControlAssertNonCareFibersEqual(t, run.algebra, root, left)
	formalControlAssertNonCareFibersEqual(t, run.algebra, root, right)
}

func TestFormalLoopFeedbackClosesDescendantConeOnceAndExitPreservesIt(t *testing.T) {
	program, outer, inner, sibling, guards := formalNestedLoopControlProgram(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, ok := program.formalTemplate.equation(program.formalRegion.roots[0])
	if !ok {
		t.Fatal("missing formal root equation")
	}
	tuple, err := algebra.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make([]decisionRef, len(guards))
	for index, guard := range guards {
		decisions[index], err = algebra.decisionForGuard(1, guard.scope, guard.arena, guard.guard)
		if err != nil {
			t.Fatal(err)
		}
	}
	care, err := algebra.decisions.reduce(context.Background(), uint8(decisionAnd), decisionTrue, true, decisions, decisionLeafAnd)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err = algebra.writeCare(tuple, care)
	if err != nil {
		t.Fatal(err)
	}

	feedbackEquation, feedbackInput := formalControlInput(t, program, formalRelationCell{Variable: 1, Root: 4, Kind: formalRelationCellNode}, formalRelationInfluenceLoopFeedback)
	closed, handled, err := evaluateFormalControlInput(algebra, feedbackEquation, feedbackInput, tuple)
	if err != nil || !handled {
		t.Fatalf("inner feedback = %#v/%t, %v", closed, handled, err)
	}
	wantInnerClose, err := algebra.decisions.apply(context.Background(), uint8(decisionAnd), true, decisions[0], decisions[2], decisionLeafAnd)
	if err != nil {
		t.Fatal(err)
	}
	closedCare, _ := algebra.care(closed)
	if closedCare != wantInnerClose {
		t.Fatalf("inner feedback Care = %d, want outer+sibling %d", closedCare, wantInnerClose)
	}
	formalControlAssertAllProductFibersValid(t, algebra, closed)
	innerLifetime, ok := program.formalGuards.loopLifetime(1, inner)
	if !ok {
		t.Fatal("missing inner lifetime")
	}
	formalTupleGuardTestNoRank(t, algebra, closed, innerLifetime)

	exitEquation, exitInput := formalControlInput(t, program, formalRelationCell{Variable: 1, Root: 6, Kind: formalRelationCellNode}, formalRelationInfluenceLoopExit)
	exited, handled, err := evaluateFormalControlInput(algebra, exitEquation, exitInput, tuple)
	if err != nil || !handled || !algebra.same(tuple, exited) {
		t.Fatalf("loop exit changed exact final-iteration tuple: exited=%#v handled=%t err=%v", exited, handled, err)
	}

	outerClosed, err := algebra.closeLoopTuple(tuple, 1, outer)
	if err != nil {
		t.Fatal(err)
	}
	outerCare, _ := algebra.care(outerClosed)
	if outerCare != decisionTrue {
		t.Fatalf("outer feedback omitted inner/sibling descendants: Care=%d", outerCare)
	}
	if inner == 0 || sibling == 0 { // pin the fixture's distinct typed binders.
		t.Fatal("nested loop fixture lost binders")
	}
}

func TestFormalRecursiveLoopExecutesFeedbackAndExitDistinctlyAndDeterministically(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	const binder loopMuTerm = 1
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{3}},
		{kind: relationNodeChoice, guard: guard, whenTrue: 4, whenFalse: 5},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: binder, route: 0}}},
	})

	first, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}

	feedbackCell := formalRelationCell{Variable: 1, Root: 4, Step: 1, Kind: formalRelationCellStep}
	exitCell := formalRelationCell{Variable: 1, Root: 5, Step: 1, Kind: formalRelationCellStep}
	bodyCell := formalRelationCell{Variable: 1, Root: 2, Kind: formalRelationCellNode}
	exitTargetCell := formalRelationCell{Variable: 1, Root: 3, Kind: formalRelationCellNode}
	for name, run := range map[string]*formalRelationExecution{"first": first, "second": second} {
		feedback, exited := run.values[feedbackCell], run.values[exitCell]
		if feedback.bottom() || exited.bottom() {
			t.Fatalf("%s recursive loop lost a live control arm: feedback=%#v exit=%#v", name, feedback, exited)
		}
		feedbackCare, err := run.algebra.care(feedback)
		if err != nil {
			t.Fatal(err)
		}
		exitCare, err := run.algebra.care(exited)
		if err != nil {
			t.Fatal(err)
		}
		intersection, err := run.algebra.decisions.apply(context.Background(), uint8(decisionAnd), true, feedbackCare, exitCare, decisionLeafAnd)
		if err != nil || intersection != decisionFalse {
			t.Fatalf("%s feedback/exit partition overlaps: %d, %v", name, intersection, err)
		}
		union, err := run.algebra.decisions.apply(context.Background(), uint8(decisionOr), true, feedbackCare, exitCare, decisionLeafOr)
		if err != nil || union != decisionTrue {
			t.Fatalf("%s feedback/exit partition is incomplete: %d, %v", name, union, err)
		}
		closed, err := run.algebra.closeLoopTuple(feedback, 1, binder)
		if err != nil || !run.algebra.same(closed, run.values[bodyCell]) {
			t.Fatalf("%s feedback did not close to the loop head: closed=%#v head=%#v err=%v", name, closed, run.values[bodyCell], err)
		}
		if !run.algebra.same(exited, run.values[exitTargetCell]) {
			t.Fatalf("%s exit did not preserve its final-iteration tuple: exit=%#v target=%#v", name, exited, run.values[exitTargetCell])
		}
	}

	firstSnapshot := formalControlExecutionSnapshot(t, first, program.formalRegion.cells)
	secondSnapshot := formalControlExecutionSnapshot(t, second, program.formalRegion.cells)
	if len(first.algebra.decisions.nodes) != len(second.algebra.decisions.nodes) || len(firstSnapshot) != len(secondSnapshot) {
		t.Fatalf("recursive solve shape drifted: decisions=%d/%d cells=%d/%d", len(first.algebra.decisions.nodes), len(second.algebra.decisions.nodes), len(firstSnapshot), len(secondSnapshot))
	}
	for cell := range firstSnapshot {
		if len(firstSnapshot[cell]) != len(secondSnapshot[cell]) {
			t.Fatalf("recursive cell %d width drifted: %d/%d", cell, len(firstSnapshot[cell]), len(secondSnapshot[cell]))
		}
		for ordinal := range firstSnapshot[cell] {
			if firstSnapshot[cell][ordinal] != secondSnapshot[cell][ordinal] {
				t.Fatalf("recursive cell %d fiber %d drifted: %d/%d", cell, ordinal, firstSnapshot[cell][ordinal], secondSnapshot[cell][ordinal])
			}
		}
	}
}

func formalControlExecutionSnapshot(t *testing.T, run *formalRelationExecution, cells []formalRelationCell) [][]formalFiberValue {
	t.Helper()
	out := make([][]formalFiberValue, len(cells))
	for index, cell := range cells {
		tuple := run.values[cell]
		if tuple.bottom() {
			continue
		}
		span, directory, _, ok := run.algebra.span(tuple.variable)
		if !ok || tuple.root.owner != directory {
			t.Fatalf("cell %+v has foreign tuple %#v", cell, tuple)
		}
		out[index] = make([]formalFiberValue, span.count)
		for ordinal := range out[index] {
			value, err := directory.valueAt(tuple.root, formalFiberOrdinal(ordinal))
			if err != nil {
				t.Fatalf("cell %+v fiber %d: %v", cell, ordinal, err)
			}
			out[index][ordinal] = value
		}
	}
	return out
}

func TestFormalControlBottomAndMalformedMetadataFailClosed(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{}, {kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeBottom}, {kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := algebra.restrictTupleCare(formalRelationTuple{}, decisionTrue); err != nil || !got.bottom() {
		t.Fatalf("Bottom restriction = %#v, %v", got, err)
	}
	program.bodies[0].relation.code.nodes[1].guard = 0
	if result, err := executeFormalRelation(context.Background(), program); err == nil || result != nil {
		t.Fatalf("missing Choice guard published %#v, %v", result, err)
	}

	loopProgram, _, inner, _, _ := formalNestedLoopControlProgram(t)
	loopAlgebra, err := newFormalTupleAlgebra(context.Background(), loopProgram)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, _ := loopProgram.formalTemplate.equation(loopProgram.formalRegion.roots[0])
	tuple, err := loopAlgebra.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	delete(loopProgram.formalGuards.loops, formalGuardLoopLifetime{variable: 1, binder: inner})
	feedbackEquation, feedbackInput := formalControlInput(t, loopProgram, formalRelationCell{Variable: 1, Root: 4, Kind: formalRelationCellNode}, formalRelationInfluenceLoopFeedback)
	if got, handled, err := evaluateFormalControlInput(loopAlgebra, feedbackEquation, feedbackInput, tuple); err == nil || !handled || !got.bottom() {
		t.Fatalf("missing loop lifetime = %#v/%t, %v", got, handled, err)
	}
}

func TestFormalScopedGuardDecisionIsRunLocalAndCached(t *testing.T) {
	program, _, _, _, guards := formalNestedLoopControlProgram(t)
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	guard := guards[0]
	first, err := algebra.decisionForGuard(1, guard.scope, guard.arena, guard.guard)
	if err != nil {
		t.Fatal(err)
	}
	nodes := len(algebra.decisions.nodes)
	second, err := algebra.decisionForGuard(1, guard.scope, guard.arena, guard.guard)
	if err != nil || first != second || len(algebra.decisions.nodes) != nodes || len(algebra.guards) != 1 {
		t.Fatalf("guard cache = %d/%d nodes=%d/%d entries=%d err=%v", first, second, nodes, len(algebra.decisions.nodes), len(algebra.guards), err)
	}
}

type formalControlGuard struct {
	scope loopMuTerm
	arena *Arena
	guard Guard
}

func formalNestedLoopControlProgram(t *testing.T) (*RelationProgram, loopMuTerm, loopMuTerm, loopMuTerm, []formalControlGuard) {
	t.Helper()
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	outer, inner, sibling := loopMuTerm(1), loopMuTerm(2), loopMuTerm(3)
	outerGuard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	innerGuard := arena.Truthy(arena.Root(Root{Kind: RootGlobal, Index: 0}))
	siblingGuard := arena.Truthy(arena.Root(Root{Kind: RootCapture, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeLoopMu, binder: outer, body: 2, exits: []relationRootRef{10}},
		{kind: relationNodeChoice, guard: outerGuard, whenTrue: 3, whenFalse: 3},
		{kind: relationNodeLoopMu, binder: inner, body: 4, exits: []relationRootRef{6}},
		{kind: relationNodeChoice, guard: innerGuard, whenTrue: 5, whenFalse: 11},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: inner}}},
		{kind: relationNodeLoopMu, binder: sibling, body: 7, exits: []relationRootRef{9}},
		{kind: relationNodeChoice, guard: siblingGuard, whenTrue: 8, whenFalse: 8},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: sibling}}},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: outer}}},
		{kind: relationNodeBottom},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: inner, route: 0}}},
	})
	return program, outer, inner, sibling, []formalControlGuard{
		{scope: outer, arena: arena, guard: outerGuard},
		{scope: inner, arena: arena, guard: innerGuard},
		{scope: sibling, arena: arena, guard: siblingGuard},
	}
}

func formalControlInput(t *testing.T, program *RelationProgram, target formalRelationCell, kind formalRelationInfluenceKind) (formalRelationEquation, formalRelationTemplateInput) {
	t.Helper()
	equation, ok := program.formalTemplate.equation(target)
	if !ok {
		t.Fatalf("missing equation %+v", target)
	}
	for _, input := range equation.Inputs {
		if input.Influence == kind {
			return equation, input
		}
	}
	t.Fatalf("equation %+v has no influence %d", target, kind)
	return formalRelationEquation{}, formalRelationTemplateInput{}
}

func formalControlAssertNonCareFibersEqual(t *testing.T, algebra *formalTupleAlgebra, left, right formalRelationTuple) {
	t.Helper()
	span, directory, _, ok := algebra.span(left.variable)
	if !ok || right.variable != left.variable || right.root.owner != directory {
		t.Fatalf("foreign tuple comparison: left=%#v right=%#v", left, right)
	}
	for ordinal := 1; ordinal < span.count; ordinal++ {
		leftValue, leftErr := directory.valueAt(left.root, formalFiberOrdinal(ordinal))
		rightValue, rightErr := directory.valueAt(right.root, formalFiberOrdinal(ordinal))
		if leftErr != nil || rightErr != nil || leftValue != rightValue {
			t.Fatalf("product fiber %d changed across control operator: %d/%d, %v/%v", ordinal, leftValue, rightValue, leftErr, rightErr)
		}
	}
}

func formalControlAssertAllProductFibersValid(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple) {
	t.Helper()
	if err := algebra.validateTuple(tuple); err != nil {
		t.Fatal(err)
	}
	span, directory, authority, ok := algebra.span(tuple.variable)
	if !ok || span.count != len(span.descriptors()) {
		t.Fatalf("tuple lost descriptor span: %#v", span)
	}
	for ordinal, descriptor := range span.descriptors() {
		root, err := directory.valueAt(tuple.root, formalFiberOrdinal(ordinal))
		if err != nil {
			t.Fatalf("product fiber %d is absent: %v", ordinal, err)
		}
		if err := algebra.validateDescriptorRoot(authority, descriptor, decisionRef(root)); err != nil {
			t.Fatalf("product fiber %d/%#v is malformed: %v", ordinal, descriptor, err)
		}
	}
}
