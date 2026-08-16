package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	typerefine "github.com/wippyai/go-lua/analysis/domain/type/refinement"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestFormalTupleLeafEvaluatorDirectBorrowedCorrelatedAndOwned(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	caller := fixture.callerLeaf(t)
	target := fixture.targetLeaf(t)

	direct := formalQualifiedBinding{
		value:       relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[0]},
		path:        relationArenaPathRef{owner: 1, arena: fixture.callerArena, term: fixture.callerPaths[0]},
		pathPresent: true,
	}
	got, err := caller.evaluate(direct)
	if err != nil || !product.Equal(fixture.reg, got.value, fixture.callerActual[0]) || !got.pathPresent || !got.path.Equal(pathdom.NewPath(fixture.callerSymbols[0], "")) {
		t.Fatalf("direct IN root = %#v, %v", got, err)
	}

	borrowed, err := formalApplyInputBinding(fixture.apply, fixture.targetExpression, fixture.targetDerivedPath, 0)
	if err != nil || !borrowed.apply.present() {
		t.Fatalf("borrow target expression = %#v, %v", borrowed, err)
	}
	got, err = caller.evaluate(borrowed)
	wantPath := pathdom.NewPath(fixture.callerSymbols[0], "").Field("member")
	if err != nil || !product.Equal(fixture.reg, got.value, fixture.callerActual[0]) || !got.pathPresent || !got.path.Equal(wantPath) {
		t.Fatalf("borrowed expression = %#v, %v", got, err)
	}

	// Equal root ordinals in another lexical body are not aliases. The target
	// evaluator observes its own Values group; the caller evaluator rejects the
	// same arena-qualified reference as foreign.
	targetDirect := formalQualifiedBinding{value: relationArenaValueRef{owner: 2, arena: fixture.targetArena, term: fixture.targetValues[0]}}
	targetValue, err := target.evaluate(targetDirect)
	if err != nil || !product.Equal(fixture.reg, targetValue.value, fixture.targetActual[0]) {
		t.Fatalf("target direct root = %#v, %v", targetValue, err)
	}
	if _, err := caller.evaluate(targetDirect); err == nil {
		t.Fatal("caller leaf accepted another lexical owner's equal root ordinal")
	}
}

func TestFormalTupleLeafEvaluatorFeedsCanonicalGenericContractActualTuple(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	leaf := fixture.callerLeaf(t)
	bindings := []formalQualifiedBinding{
		{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[0]}},
		{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[1]}},
	}
	actuals := make([]product.Value, len(bindings))
	for index, binding := range bindings {
		actual, err := leaf.evaluate(binding)
		if err != nil {
			t.Fatal(err)
		}
		actuals[index] = actual.value
	}
	contracts, _ := instantiateBoundaryContractsFromValues(fixture.reg, &fixture.program.bodies[1], func(index int) product.Value {
		return actuals[index]
	}, nil)
	if len(contracts) != 2 {
		t.Fatalf("generic contracts = %d", len(contracts))
	}
	for index, contract := range contracts {
		contractType, ok := typevalue.TypeOf(fixture.reg, contract)
		if !ok || contractType == nil || typerefine.ContainsFreeTypeParam(contractType) ||
			!boundaryArgumentSatisfiesContract(fixture.reg, actuals[index], contract) {
			t.Fatalf("generic contract %d = %v/%t, want closed contract accepting actual", index, contractType, ok)
		}
	}
}

func TestFormalTupleLeafEvaluatorFailsClosedForUnsupportedAndForeignBindings(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	leaf := fixture.callerLeaf(t)
	unsupported := formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.unsupported}}
	if _, err := leaf.evaluate(unsupported); err == nil {
		t.Fatal("factor-dependent dynamic read acquired scalar fallback semantics")
	}
	foreign := formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.targetArena, term: fixture.targetValues[0]}}
	if _, err := leaf.evaluate(foreign); err == nil {
		t.Fatal("foreign arena binding was accepted")
	}
	malformed := leaf
	malformed.leaves.dense = malformed.leaves.dense[:len(malformed.leaves.dense)-1]
	if _, err := malformed.evaluate(formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[0]}}); err == nil {
		t.Fatal("incomplete product leaf row was accepted")
	}
}

func TestFormalTupleLeafEvaluatorDistinguishesAbsentFromSelectedBottom(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	evaluator := fixture.callerLeaf(t)
	slot, ok := fixture.program.formalSlots.Slot(fixture.program.bodies[0].body, Root{Kind: RootParam, Index: 0})
	if !ok {
		t.Fatal("caller parameter has no formal slot")
	}
	position, ok := evaluator.values.valueSlotPosition[slot]
	if !ok {
		t.Fatal("caller parameter has no Values position")
	}
	top := evaluator.values.valueTop
	value := evaluator.values.valueSlots[position].ordinal
	topLeaf, present := evaluator.leaves.leaf(top)
	if !present {
		t.Fatal("dense evaluator has no Values top")
	}
	binding := formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[0]}}

	missing := evaluator
	missingPositions, err := sealFormalOrdinalPositions(evaluator.span.count, []formalFiberOrdinal{top})
	if err != nil {
		t.Fatal(err)
	}
	missing.leaves = formalFiberLeafSelection{
		span: evaluator.span, ordinals: []formalFiberOrdinal{top}, positions: missingPositions, leaves: []decisionLeaf{topLeaf},
	}
	if _, err := missing.evaluate(binding); err == nil {
		t.Fatal("absent sparse Values member was interpreted as Bottom")
	}

	bottom := evaluator
	ordinals := []formalFiberOrdinal{top, value}
	if ordinals[0] > ordinals[1] {
		ordinals[0], ordinals[1] = ordinals[1], ordinals[0]
	}
	leaves := make([]decisionLeaf, len(ordinals))
	for index, ordinal := range ordinals {
		if ordinal == top {
			leaves[index] = 0
		}
	}
	bottomPositions, err := sealFormalOrdinalPositions(evaluator.span.count, ordinals)
	if err != nil {
		t.Fatal(err)
	}
	bottom.leaves = formalFiberLeafSelection{span: evaluator.span, ordinals: ordinals, positions: bottomPositions, leaves: leaves}
	got, err := bottom.evaluate(binding)
	if err != nil || !product.Equal(fixture.reg, got.value, product.Bottom(fixture.reg)) {
		t.Fatalf("selected physical Bottom = %#v, %v", got.value, err)
	}
}

func TestFormalTupleLeafEvaluatorDynamicReadUsesRegisteredFactorCapability(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	leaf := fixture.callerLeaf(t)
	access, err := fixture.program.bodies[0].valueTermFactorAccess(fixture.unsupported)
	if err != nil {
		t.Fatal(err)
	}
	_, factors, err := leaf.productFactors()
	if err != nil {
		t.Fatal(err)
	}
	binding := formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.unsupported}}
	got, err := leaf.evaluateWithFactorAccess(binding, &formalValueFactorAccess{access: access, factors: factors})
	if err != nil {
		t.Fatal(err)
	}
	if !product.BelongsToRegistry(fixture.reg, got.value) {
		t.Fatal("factor-backed dynamic read produced a foreign product value")
	}
}

func TestFormalProductExecutionCapabilityRetainsSymbolicValuesUntilObservationSpecializes(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	leaf := fixture.callerLeaf(t)
	binding := formalQualifiedBinding{value: relationArenaValueRef{owner: leaf.variable, arena: fixture.callerArena, term: fixture.callerValues[0]}}
	symbolicLeaf, err := leaf.authority.internBinding(binding)
	if err != nil {
		t.Fatal(err)
	}

	// Model the exact producer row the entry-free WTO can retain. The
	// capability must accept this formal spelling; asking the concrete adapter
	// to consume it is reserved for the later specialized observation.
	symbolic := leaf
	symbolic.leaves.dense = append([]decisionLeaf(nil), leaf.leaves.dense...)
	ordinal := leaf.values.valueSlots[0].ordinal
	symbolic.leaves.dense[ordinal] = symbolicLeaf
	vector, err := formalFactorExecutionVector(symbolic)
	if err != nil {
		t.Fatal(err)
	}
	producer := formalProductLeafEvaluator{
		algebra: fixture.algebra, authority: leaf.authority, span: leaf.span, layout: leaf.layout, leaves: vector,
	}
	values, factors, err := producer.formalProductFactors()
	if err != nil {
		t.Fatal(err)
	}
	if value := values.Values[leaf.values.valueSlots[0].slot]; !value.isSymbolic || value.symbolicLeaf != symbolicLeaf {
		t.Fatalf("producer Values carrier = %#v", values)
	}
	capability := formalFactorExecutionCapability{values: values}
	if _, err := capability.specializedIdentitySupport(t.Context(), leaf.authority, values, factors); err == nil {
		t.Fatal("symbolic execution capability rebuilt State identity support before specialization")
	}
	if _, _, err := producer.productFactors(); err == nil {
		t.Fatal("symbolic producer row crossed the generic concrete adapter")
	}
}

func TestFormalTupleLeafEvaluatorDirectRootAllocations(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	leaf := fixture.callerLeaf(t)
	binding := formalQualifiedBinding{value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.callerValues[0]}}
	if allocations := testing.AllocsPerRun(1000, func() {
		got, err := leaf.evaluate(binding)
		if err != nil || !product.Equal(fixture.reg, got.value, fixture.callerActual[0]) {
			panic("direct-root formal evaluation drifted")
		}
	}); allocations != 0 {
		t.Fatalf("direct-root allocations/run = %.2f, want 0", allocations)
	}
}

func TestFormalTupleLeafEvaluatorSelectUsesExactRegionBeforeAbstractTruthiness(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	atom := fixture.targetArena.values[fixture.targetExpression].guard
	guardNode := fixture.targetArena.guards[atom]
	if guardNode.op != guardTruthy {
		t.Fatalf("Select guard = %#v, want Truthy atom", guardNode)
	}
	vocabulary := &formalGuardVocabulary{
		ranks: map[formalGuardRankKey]uint32{{
			variable: 2, arena: fixture.targetArena, term: guardNode.value,
		}: 0},
		apply: make(map[formalRelationCell]formalGuardBoundary), definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary),
		loops: make(map[formalGuardLoopLifetime]formalGuardRankSet), size: 1, sealed: true,
	}
	if !vocabulary.valid() {
		t.Fatal("formal guard fixture is invalid")
	}
	fixture.program.formalGuards = vocabulary

	span, _, _, ok := fixture.algebra.span(1)
	if !ok {
		t.Fatal("caller span")
	}
	values, ok := span.valuesGroup()
	if !ok {
		t.Fatal("caller Values group")
	}
	body := &fixture.program.bodies[0]
	first, _ := fixture.program.formalSlots.Slot(body.body, Root{Kind: RootParam, Index: 0})
	second, _ := fixture.program.formalSlots.Slot(body.body, Root{Kind: RootParam, Index: 1})
	ambiguous := product.Join(fixture.reg, fixture.callerActual[1], typevalue.Nil(fixture.reg))
	tuple, err := fixture.algebra.writeValuesFactor(formalTupleTestLive(t, fixture.algebra, 1), values,
		state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{first: fixture.callerActual[0], second: ambiguous}})
	if err != nil {
		t.Fatal(err)
	}
	condition, err := fixture.algebra.decisionForGuard(2, 0, fixture.targetArena, atom)
	if err != nil {
		t.Fatal(err)
	}
	complement, err := formalDecisionBooleanNot(fixture.algebra, condition)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := formalApplyInputBinding(fixture.apply, fixture.targetExpression, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		care  decisionRef
		value product.Value
	}{{"true", condition, fixture.callerActual[0]}, {"false", complement, typevalue.Nil(fixture.reg)}} {
		t.Run(test.name, func(t *testing.T) {
			guarded, writeErr := fixture.algebra.writeCare(tuple, test.care)
			if writeErr != nil {
				t.Fatal(writeErr)
			}
			regions, regionsErr := fixture.algebra.tupleLeafRegions(guarded)
			if regionsErr != nil || len(regions) != 1 {
				t.Fatalf("regions = %d, %v", len(regions), regionsErr)
			}
			got, evalErr := regions[0].evaluator.evaluate(binding)
			if evalErr != nil || !product.Equal(fixture.reg, got.value, test.value) {
				t.Fatalf("region Select = %#v, %v; want %v", got, evalErr, test.value)
			}
		})
	}
}

type formalTupleLeafEvaluatorFixture struct {
	reg                        *axis.Registry
	program                    *RelationProgram
	algebra                    *formalTupleAlgebra
	callerTuple, targetTuple   formalRelationTuple
	callerArena, targetArena   *Arena
	callerValues, targetValues []ValueTerm
	callerPaths                []PathTerm
	callerActual, targetActual []product.Value
	callerSymbols              []symbol.ID
	apply                      relationLazyApplyBinding
	targetExpression           ValueTerm
	targetDerivedPath          PathTerm
	unsupported                ValueTerm
}

func newFormalTupleLeafEvaluatorFixture(t *testing.T) formalTupleLeafEvaluatorFixture {
	t.Helper()
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callerID, targetID := lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1)
	callerSymbols := []symbol.ID{101, 102}
	targetSymbols := []symbol.ID{201, 202}
	shape := Shape{Params: 2}

	callerArena, targetArena := NewArena(reg), NewArena(reg)
	if !callerArena.bindLexicalOwner(callerID) || !targetArena.bindLexicalOwner(targetID) {
		t.Fatal("bind lexical owners")
	}
	callerValues := []ValueTerm{
		callerArena.Root(Root{Kind: RootParam, Index: 0}),
		callerArena.Root(Root{Kind: RootParam, Index: 1}),
	}
	callerPaths := []PathTerm{
		callerArena.Path(Root{Kind: RootParam, Index: 0}),
		callerArena.Path(Root{Kind: RootParam, Index: 1}),
	}
	targetValues := []ValueTerm{
		targetArena.Root(Root{Kind: RootParam, Index: 0}),
		targetArena.Root(Root{Kind: RootParam, Index: 1}),
	}
	targetExpression := targetArena.SelectValue(
		targetArena.Truthy(targetValues[1]),
		targetValues[0],
		targetArena.Constant(typevalue.Nil(reg)),
	)
	targetDerivedPath := targetArena.Path(Root{Kind: RootParam, Index: 0}, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	unsupported := callerArena.DynamicReadTableValue(callerValues[0], callerPaths[0], callerValues[1])
	frame := callerArena.relationFrame(2, cfg.Point(7), 1, shape, callerValues, callerPaths, 0)
	if targetExpression == 0 || targetDerivedPath == 0 || unsupported == 0 || frame == 0 {
		t.Fatal("build formal leaf term vocabulary")
	}
	if err := callerArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}

	callerEffects, targetEffects := NewEffectArena(callerArena), NewEffectArena(targetArena)
	callerCode := &relationCode{
		terms: callerArena, effects: callerEffects, descriptors: DefaultDescriptorRegistry(), shape: shape,
		nodes: []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, outcomes: []boundaryOutcomeTuple{{}, {}},
		contributions: []semanticContribution{{}}, root: 1,
	}
	targetCode := &relationCode{
		terms: targetArena, effects: targetEffects, descriptors: DefaultDescriptorRegistry(), shape: shape,
		nodes: []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}}, outcomes: []boundaryOutcomeTuple{{}, {}},
		contributions: []semanticContribution{{}}, root: 1,
	}
	apply, err := freezeRelationLazyApplyBinding(1, callerCode, targetCode, relationApplyRef{variable: 2, frame: frame})
	if err != nil {
		t.Fatal(err)
	}
	callerCode.applicationGuards = make([]relationApplicationGuardPlan, len(callerArena.callFrames))
	callerCode.applicationGuards[frame] = relationApplicationGuardPlan{
		frame: frame, target: 2, callerScope: 0, scopeFrozen: true,
		targetScopes: map[loopMuTerm]struct{}{0: {}}, binding: apply,
		guards: []relationApplicationGuardPair{}, boundAtoms: []ValueTerm{}, frozen: true,
	}
	callerArena.Seal()
	targetArena.Seal()
	callerEffects.Seal()
	targetEffects.Seal()
	callerCode.sealed, targetCode.sealed = true, true

	callerKeys, targetKeys := keyspace.New(), keyspace.New()
	callerPlan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(callerSymbols).
		WithBoundaryParamContracts([]product.Value{product.Top(), product.Top()}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	typeParam := typ.NewTypeParam("T", nil)
	genericContract := typevalue.NewCache().FromTypeWithWitness(reg, typeParam)
	targetPlan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(targetSymbols).
		WithBoundaryParamContracts([]product.Value{genericContract, genericContract}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	callerRoots, err := sealRelationRootCarrier(callerPlan, callerKeys, shape)
	if err != nil {
		t.Fatal(err)
	}
	targetRoots, err := sealRelationRootCarrier(targetPlan, targetKeys, shape)
	if err != nil {
		t.Fatal(err)
	}
	productDomain := state.RegisteredProductDomain(reg)
	program := &RelationProgram{
		registry: reg,
		bodies: []relationProgramBody{
			{body: callerID, variable: 1, keys: callerKeys, roots: callerRoots, plan: callerPlan, relation: Relation{shape: shape, arena: callerArena, effects: callerEffects, descriptors: callerCode.descriptors, code: callerCode, root: 1}, productDomain: productDomain},
			{body: targetID, variable: 2, keys: targetKeys, roots: targetRoots, plan: targetPlan, relation: Relation{shape: shape, arena: targetArena, effects: targetEffects, descriptors: targetCode.descriptors, code: targetCode, root: 1}, productDomain: productDomain},
		},
		byBody: map[lexicalidentity.StableLexicalBodyID]relationVar{callerID: 1, targetID: 2},
	}
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
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	callerActual := []product.Value{typevalue.LiteralString(reg, "alpha"), typevalue.LiteralString(reg, "beta")}
	targetActual := []product.Value{typevalue.LiteralInt(reg, 17), typevalue.LiteralInt(reg, 29)}
	writeValues := func(variable relationVar, actual []product.Value) formalRelationTuple {
		span, _, _, ok := algebra.span(variable)
		if !ok {
			t.Fatal("missing formal span")
		}
		group, ok := span.valuesGroup()
		if !ok {
			t.Fatal("missing Values group")
		}
		factor := state.ValueFactor[FormalSlot]{Values: make(map[FormalSlot]product.Value, len(actual))}
		body := &program.bodies[variable-1]
		for index, value := range actual {
			slot, found := slots.Slot(body.body, Root{Kind: RootParam, Index: uint32(index)})
			if !found {
				t.Fatalf("param %d has no FormalSlot", index)
			}
			factor.Values[slot] = value
		}
		tuple, writeErr := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, variable), group, factor)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		return tuple
	}
	return formalTupleLeafEvaluatorFixture{
		reg: reg, program: program, algebra: algebra,
		callerTuple: writeValues(1, callerActual), targetTuple: writeValues(2, targetActual),
		callerArena: callerArena, targetArena: targetArena,
		callerValues: callerValues, targetValues: targetValues, callerPaths: callerPaths,
		callerActual: callerActual, targetActual: targetActual, callerSymbols: callerSymbols,
		apply: apply, targetExpression: targetExpression, targetDerivedPath: targetDerivedPath, unsupported: unsupported,
	}
}

func (f formalTupleLeafEvaluatorFixture) callerLeaf(t *testing.T) formalTupleLeafEvaluator {
	t.Helper()
	return f.onlyLeaf(t, f.callerTuple)
}

func (f formalTupleLeafEvaluatorFixture) targetLeaf(t *testing.T) formalTupleLeafEvaluator {
	t.Helper()
	return f.onlyLeaf(t, f.targetTuple)
}

func (f formalTupleLeafEvaluatorFixture) onlyLeaf(t *testing.T, tuple formalRelationTuple) formalTupleLeafEvaluator {
	t.Helper()
	regions, err := f.algebra.tupleLeafRegions(tuple)
	if err != nil || len(regions) != 1 || regions[0].guard != decisionTrue {
		t.Fatalf("tuple leaf regions = %d, %v", len(regions), err)
	}
	return regions[0].evaluator
}
