package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func closeAndFreezeRelationGuardTestForest(t *testing.T, codes []*relationCode, definitions ...relationProgramDefinition) {
	t.Helper()
	if err := closeRelationGuardBoundarySyntax(codes); err != nil {
		t.Fatal(err)
	}
	for _, code := range codes {
		code.terms.Seal()
		if code.effects != nil {
			code.effects.Seal()
		}
	}
	if err := freezeRelationApplicationGuardPlans(codes); err != nil {
		t.Fatal(err)
	}
	if err := freezeRelationDefinitionGuardPlans(codes, definitions); err != nil {
		t.Fatal(err)
	}
}

func TestFreezeRelationApplicationGuardPlansSubstitutesCompleteStableVocabulary(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 2}
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("application-guard-plan"))
	callerTerms, targetTerms := NewArena(reg), NewArena(reg)
	if !callerTerms.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) ||
		!targetTerms.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 2)) {
		t.Fatal("could not bind application guard test owners")
	}

	callerValue := callerTerms.Root(Root{Kind: RootParam})
	frame := callerTerms.relationFrame(2, 7, 1, shape,
		[]ValueTerm{callerValue, callerValue}, []PathTerm{0, 0}, 0)
	if frame == 0 {
		t.Fatal("could not freeze aliased call frame")
	}
	caller := &relationCode{
		terms: callerTerms, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: frame}}}, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}

	left := targetTerms.Root(Root{Kind: RootParam})
	right := targetTerms.Root(Root{Kind: RootParam, Index: 1})
	local := targetTerms.loopContinuationValue(cfg.Point(19))
	choice := targetTerms.Truthy(left)
	leftStep := targetTerms.Truthy(right)
	rightStep := targetTerms.Falsy(local)
	unreachable := targetTerms.Falsy(targetTerms.JoinValue(left, right))
	target := &relationCode{
		terms: targetTerms, shape: shape, root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeChoice, guard: choice, whenTrue: 2, whenFalse: 3},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepExternalCall, point: 1, guard: leftStep}}, next: 4},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepExternalCall, point: 2, guard: rightStep}}, next: 4},
			{kind: relationNodeOutcome, outcome: 1},
			{kind: relationNodeChoice, guard: unreachable, whenTrue: 4},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}

	forest := []*relationCode{caller, target}
	closeAndFreezeRelationGuardTestForest(t, forest)
	plan := caller.applicationGuards[frame]
	if !plan.validFor(frame, 2) {
		t.Fatalf("application guard plan did not freeze: %#v", plan)
	}
	if len(plan.guards) != 3 {
		t.Fatalf("application guard plan retained %d guards, want only 3 reachable guards", len(plan.guards))
	}
	if got, want := []Guard{plan.guards[0].source, plan.guards[1].source, plan.guards[2].source}, []Guard{choice, leftStep, rightStep}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source guard order = %v, want stable topology order %v", got, want)
	}
	if len(plan.guards[0].atoms) != 1 || plan.guards[0].atoms[0].substituted != callerValue || plan.guards[0].atoms[0].targetLocal {
		t.Fatalf("first guard atom binding = %#v, want caller value %d", plan.guards[0].atoms, callerValue)
	}
	if len(plan.guards[1].atoms) != 1 || plan.guards[1].atoms[0].substituted != callerValue || plan.guards[1].atoms[0].targetLocal {
		t.Fatalf("f(x,x) did not coalesce guard atoms: %#v", plan.guards[1].atoms)
	}
	if len(plan.boundAtoms) != 1 || callerTerms.values[plan.boundAtoms[0]].op != valueLoopContinuation ||
		callerTerms.values[plan.boundAtoms[0]].owner != targetTerms.owner {
		t.Fatalf("target-local guard support = %v, want exact application-owned loop atom", plan.boundAtoms)
	}

	beforeValues, beforeGuards := len(callerTerms.values), len(callerTerms.guards)
	first := plan
	if err := freezeRelationApplicationGuardPlans(forest); err != nil {
		t.Fatal(err)
	}
	if len(callerTerms.values) != beforeValues || len(callerTerms.guards) != beforeGuards || !reflect.DeepEqual(first, caller.applicationGuards[frame]) {
		t.Fatal("repeat freeze changed syntax or immutable plan ordering")
	}
}

func TestFreezeRelationApplicationGuardPlansSupportsBranchFreeCallee(t *testing.T) {
	reg := standard.Registry()
	callerTerms, targetTerms := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	value := callerTerms.Root(Root{Kind: RootParam})
	frame := callerTerms.relationFrame(2, 3, 1, shape, []ValueTerm{value}, []PathTerm{0}, 0)
	caller := &relationCode{
		terms: callerTerms, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: frame}}}, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	target := &relationCode{
		terms: targetTerms, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	closeAndFreezeRelationGuardTestForest(t, []*relationCode{caller, target})
	plan := caller.applicationGuards[frame]
	if !plan.validFor(frame, 2) || len(plan.guards) != 0 || len(plan.boundAtoms) != 0 {
		t.Fatalf("branch-free application has no exact empty authority: %#v", plan)
	}
}

func TestFreezeRelationApplicationGuardPlansPartitionsCallerAndTargetVocabulary(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("application-guard-typed-binding"))
	callerTerms, targetTerms := NewArena(reg), NewArena(reg)
	if !callerTerms.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) ||
		!targetTerms.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 2)) {
		t.Fatal("could not bind typed guard owners")
	}
	shape := Shape{Params: 1}
	callerValue := callerTerms.Constant(typevalue.LiteralString(reg, "caller"))
	frame := callerTerms.relationFrame(2, 7, 1, shape, []ValueTerm{callerValue}, []PathTerm{0}, 0)
	caller := &relationCode{
		terms: callerTerms, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: frame}}}, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}

	paramRoot := Root{Kind: RootParam}
	param := targetTerms.Root(paramRoot)
	localSymbol := symbol.ID(901)
	local := targetTerms.bindEnvironmentSymbol(localSymbol)
	nestedFrame := targetTerms.relationFrame(1, 11, 1, shape, []ValueTerm{param}, []PathTerm{targetTerms.Path(paramRoot)}, 1)
	frameResult := targetTerms.frameResultValue(nestedFrame, 0)
	ownerPathRead := targetTerms.DynamicReadValueAtPaths(
		13, param, targetTerms.Path(paramRoot, segment.Segment{Kind: segment.SegmentField, Name: "items"}),
		targetTerms.Constant(typevalue.LiteralString(reg, "key")), 0,
	)
	mixed := targetTerms.JoinValue(param, local)
	guard := targetTerms.And(
		targetTerms.Truthy(param),
		targetTerms.Truthy(local),
		targetTerms.Truthy(frameResult),
		targetTerms.Truthy(ownerPathRead),
		targetTerms.Truthy(mixed),
	)
	target := &relationCode{
		terms: targetTerms, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}

	closeAndFreezeRelationGuardTestForest(t, []*relationCode{caller, target})
	plan := caller.applicationGuards[frame]
	if !plan.validFor(frame, 2) || len(plan.guards) != 1 || len(plan.guards[0].atoms) != 5 {
		t.Fatalf("typed application guard plan = %#v", plan)
	}
	bindings := make(map[ValueTerm]relationGuardAtomSubstitution, len(plan.guards[0].atoms))
	for _, atom := range plan.guards[0].atoms {
		bindings[atom.source] = atom
	}
	if got := bindings[param]; got.substituted != callerValue || got.targetLocal {
		t.Fatalf("parameter binding = %#v, want exact caller term %d", got, callerValue)
	}
	for _, term := range []ValueTerm{local, frameResult, ownerPathRead, mixed} {
		got := bindings[term]
		if !got.targetLocal || got.substituted != term {
			t.Fatalf("target-local binding for term %d = %#v", term, got)
		}
	}
	first := plan
	if err := freezeRelationApplicationGuardPlans([]*relationCode{caller, target}); err != nil || !reflect.DeepEqual(first, caller.applicationGuards[frame]) {
		t.Fatalf("repeat typed guard freeze changed the canonical plan: %v", err)
	}
}
