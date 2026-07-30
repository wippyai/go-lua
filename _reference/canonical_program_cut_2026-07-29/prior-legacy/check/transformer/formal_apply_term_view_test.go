package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFormalApplyStructuredTermViewSpecializesSwappedFramesWithoutArenaGrowth(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-apply-structured-view"))
	callerID, targetID := lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1)
	callerTerms, targetTerms := NewArena(reg), NewArena(reg)
	if !callerTerms.bindLexicalOwner(callerID) || !targetTerms.bindLexicalOwner(targetID) {
		t.Fatal("bind lexical owners")
	}
	shape := Shape{Params: 2, Results: 1}
	callerP0 := callerTerms.Root(Root{Kind: RootParam, Index: 0})
	callerP1 := callerTerms.Root(Root{Kind: RootParam, Index: 1})
	callerPath0 := callerTerms.Path(Root{Kind: RootParam, Index: 0})
	callerPath1 := callerTerms.Path(Root{Kind: RootParam, Index: 1})
	point := cfg.Point(23)
	if callerTerms.bindCallResult(point, 0) == 0 {
		t.Fatal("bind caller result")
	}
	leftFrame := callerTerms.relationFrame(2, point, 1, shape, []ValueTerm{callerP0, callerP1}, []PathTerm{callerPath0, callerPath1}, 1)
	rightFrame := callerTerms.relationFrame(2, point, 2, shape, []ValueTerm{callerP1, callerP0}, []PathTerm{callerPath1, callerPath0}, 1)
	if leftFrame == 0 || rightFrame == 0 || leftFrame == rightFrame {
		t.Fatal("freeze distinct swapped frames")
	}

	targetP0 := targetTerms.Root(Root{Kind: RootParam, Index: 0})
	targetP1 := targetTerms.Root(Root{Kind: RootParam, Index: 1})
	// Correlated syntax exercises both target arguments rather than merely
	// retaining a root handle.  Select traversal and product execution remain
	// owned by the canonical value-node algebra.
	marker := targetTerms.Constant(typevalue.LiteralString(reg, "!"))
	resultTerm := targetTerms.SelectValue(targetTerms.Truthy(targetP1), targetP0, marker)
	resultPath := targetTerms.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	if resultTerm == 0 || resultPath == 0 {
		t.Fatal("freeze structured target result")
	}
	bindFormalApplyTestEnvironment(t, callerTerms, shape, 100)
	bindFormalApplyTestEnvironment(t, targetTerms, shape, 110)
	if err := callerTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetTerms.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	bindFormalApplyTestInputs(t, callerTerms, shape, 100)
	bindFormalApplyTestInputs(t, targetTerms, shape, 110)

	callerEffects, targetEffects := NewEffectArena(callerTerms), NewEffectArena(targetTerms)
	callerCode := &relationCode{
		terms: callerTerms, effects: callerEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: leftFrame}},
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: rightFrame}},
			}, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}}, contributions: []semanticContribution{{}},
	}
	targetCode := &relationCode{
		terms: targetTerms, effects: targetEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes:         []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes:      []boundaryOutcomeTuple{{}, {returnTransaction: testReturnTransactionTerm(t, 1, resultTerm)}},
		contributions: []semanticContribution{{}},
	}
	closeAndFreezeRelationGuardTestForest(t, []*relationCode{callerCode, targetCode})
	callerTerms.Seal()
	targetTerms.Seal()
	callerEffects.Seal()
	targetEffects.Seal()
	callerCode.sealed, targetCode.sealed = true, true

	productDomain := state.RegisteredProductDomain(reg)
	program := &RelationProgram{
		registry: reg,
		bodies: []relationProgramBody{
			{body: callerID, variable: 1, keys: keyspace.New(), relation: Relation{shape: shape, arena: callerTerms, effects: callerEffects, descriptors: callerCode.descriptors, code: callerCode, root: 1}, productDomain: productDomain},
			{body: targetID, variable: 2, keys: keyspace.New(), relation: Relation{shape: shape, arena: targetTerms, effects: targetEffects, descriptors: targetCode.descriptors, code: targetCode, root: 1}, productDomain: productDomain},
		},
		byBody: map[lexicalidentity.StableLexicalBodyID]relationVar{callerID: 1, targetID: 2},
	}
	prepareFormalApplyTestProgram(t, program, leftFrame, rightFrame)
	slots, err := freezeSlotSpace(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalSlots = slots
	program.formalFibers, err = freezeFormalFiberInventoryWithSlots(program, slots)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents, err = freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	rootInputs, err := freezeFormalRootInputTemplates(program)
	if err != nil {
		t.Fatal(err)
	}
	// This fixture tests Apply's structured term view without constructing N5
	// outcome syntax. It nevertheless uses the production root-input template
	// and root operator, so parameter constraints have one canonical authority.
	program.formalTemplate = &formalRelationTemplate{program: program, region: region, rootInputs: rootInputs}
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}

	rootEquation := func(variable relationVar) formalRelationEquation {
		cell := region.roots[variable-1]
		index, ok := region.plan.CanonicalIndex(cell)
		if !ok {
			t.Fatal("root input equation index")
		}
		operator, freezeErr := freezeFormalRelationOperator(program, region, rootInputs, cell)
		if freezeErr != nil {
			t.Fatal(freezeErr)
		}
		return formalRelationEquation{Cell: formalRelationCellRef{region: region, cell: cell, index: index}, Operator: operator}
	}
	callerEquation, targetEquation := rootEquation(1), rootEquation(2)
	callerTuple, err := algebra.instantiateRootEquation(callerEquation)
	if err != nil {
		t.Fatal(err)
	}
	targetTuple, err := algebra.instantiateRootEquation(targetEquation)
	if err != nil {
		t.Fatal(err)
	}
	targetTuple = formalApplyTestExposeOutcome(t, algebra, targetTuple, 1)
	type arenaSize struct{ values, paths, guards, frames, environments int }
	size := func(arena *Arena) arenaSize {
		return arenaSize{len(arena.values), len(arena.paths), len(arena.guards), len(arena.callFrames), len(arena.environment)}
	}
	beforeCaller, beforeTarget := size(callerTerms), size(targetTerms)
	beforeTerminals := len(algebra.components.terminals)

	left, _, err := algebra.applyOutcome(operatorForFormalApplyTest(t, program, callerCode, 1), callerTuple, []formalRelationTuple{targetTuple})
	if err != nil {
		t.Fatal(err)
	}
	afterLeftTerminals := len(algebra.components.terminals)
	leftAgain, _, err := algebra.applyOutcome(operatorForFormalApplyTest(t, program, callerCode, 1), callerTuple, []formalRelationTuple{targetTuple})
	if err != nil {
		t.Fatal(err)
	}
	if len(algebra.components.terminals) != afterLeftTerminals {
		t.Fatalf("repeated view construction grew terminal hash-cons: before=%d left=%d repeated=%d", beforeTerminals, afterLeftTerminals, len(algebra.components.terminals))
	}
	right, _, err := algebra.applyOutcome(operatorForFormalApplyTest(t, program, callerCode, 2), callerTuple, []formalRelationTuple{targetTuple})
	if err != nil {
		t.Fatal(err)
	}
	leftBinding := formalApplyTestResultBinding(t, algebra, left, callerID, point)
	leftAgainBinding := formalApplyTestResultBinding(t, algebra, leftAgain, callerID, point)
	rightBinding := formalApplyTestResultBinding(t, algebra, right, callerID, point)
	if !leftBinding.apply.present() || !rightBinding.apply.present() || leftBinding.value.term != resultTerm || rightBinding.value.term != resultTerm ||
		leftBinding.value.arena != targetTerms || rightBinding.value.arena != targetTerms || leftBinding == rightBinding || leftBinding != leftAgainBinding {
		t.Fatalf("structured bindings are not stable distinct frame views: left=%#v repeated=%#v right=%#v", leftBinding, leftAgainBinding, rightBinding)
	}

	callerValues := []product.Value{typevalue.LiteralString(reg, "a"), typevalue.LiteralBool(reg, false)}
	callerPaths := []pathdom.Path{pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)}
	cursor, err := NewBindingCursor(shape, callerValues, callerPaths)
	if err != nil {
		t.Fatal(err)
	}
	leftValue, leftExact := leftBinding.apply.evalValue(leftBinding.value.term, cursor, formalApplyFrameEnvironment{})
	rightValue, rightExact := rightBinding.apply.evalValue(rightBinding.value.term, cursor, formalApplyFrameEnvironment{})
	if !leftExact || !rightExact || !product.Equal(reg, leftValue, typevalue.LiteralString(reg, "!")) ||
		!product.Equal(reg, rightValue, typevalue.LiteralBool(reg, false)) {
		t.Fatalf("structured specialization = left %v/%t right %v/%t", leftValue, leftExact, rightValue, rightExact)
	}

	leftPathBinding, err := formalApplyInputBinding(callerCode.applicationGuards[leftFrame].binding, resultTerm, resultPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	rightPathBinding, err := formalApplyInputBinding(callerCode.applicationGuards[rightFrame].binding, resultTerm, resultPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	leftPath, leftPathExact := leftPathBinding.apply.evalPath(leftPathBinding.path.term, cursor, formalApplyFrameEnvironment{})
	rightPath, rightPathExact := rightPathBinding.apply.evalPath(rightPathBinding.path.term, cursor, formalApplyFrameEnvironment{})
	wantLeft, wantRight := callerPaths[0].Clone(), callerPaths[1].Clone()
	wantLeft.Segments = append(wantLeft.Segments, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	wantRight.Segments = append(wantRight.Segments, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	if !leftPathExact || !rightPathExact || !leftPath.Equal(wantLeft) || !rightPath.Equal(wantRight) {
		t.Fatalf("derived path specialization = left %#v/%t right %#v/%t", leftPath, leftPathExact, rightPath, rightPathExact)
	}
	if size(callerTerms) != beforeCaller || size(targetTerms) != beforeTarget {
		t.Fatal("structured/repeated Apply grew a sealed term arena")
	}
}
