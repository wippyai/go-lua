package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestRelationLazyApplyBindingPreservesCallerMiddleAndTargetGuardOwnership(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("lazy-apply-middle-guard"))
	callerArena, targetArena := NewArena(reg), NewArena(reg)
	if !callerArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) ||
		!targetArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 2)) {
		t.Fatal("bind lexical owners")
	}
	callerSymbol, targetSymbol := symbol.ID(7101), symbol.ID(7201)
	callerArena.bindEnvironmentSymbol(callerSymbol)
	targetArena.bindEnvironmentSymbol(targetSymbol)
	if err := callerArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	callerValue, ok := callerArena.middleValue(statekey.SymbolValue(callerSymbol))
	if !ok {
		t.Fatal("caller Middle has no value")
	}
	callerPath := callerArena.middleSymbolPath(callerSymbol)
	targetValue, ok := targetArena.middleValue(statekey.SymbolValue(targetSymbol))
	if !ok {
		t.Fatal("target Middle has no value")
	}
	targetGuard := targetArena.Truthy(targetValue)
	shape := Shape{Params: 1}
	frame := callerArena.relationFrame(2, 11, 1, shape, []ValueTerm{callerValue}, []PathTerm{callerPath}, 0)
	caller := &relationCode{terms: callerArena, shape: shape}
	target := &relationCode{terms: targetArena, shape: shape}
	beforeCallerValues, beforeCallerPaths, beforeCallerGuards := len(callerArena.values), len(callerArena.paths), len(callerArena.guards)
	beforeTargetValues, beforeTargetPaths, beforeTargetGuards := len(targetArena.values), len(targetArena.paths), len(targetArena.guards)

	binding, err := freezeRelationLazyApplyBinding(1, caller, target, relationApplyRef{variable: 2, frame: frame})
	if err != nil {
		t.Fatal(err)
	}
	valueRef, exact := binding.inputValue(Root{Kind: RootParam})
	if !exact || valueRef.owner != 1 || valueRef.arena != callerArena || valueRef.term != callerValue ||
		callerArena.values[valueRef.term].root.Kind != RootMiddle {
		t.Fatalf("lazy IN value did not preserve caller MID: %#v", valueRef)
	}
	pathRef, present, exact := binding.inputPath(Root{Kind: RootParam})
	if !exact || !present || pathRef.owner != 1 || pathRef.arena != callerArena || pathRef.term != callerPath ||
		callerArena.paths[pathRef.term].root.Kind != RootMiddle {
		t.Fatalf("lazy IN path did not preserve caller MID: %#v present=%v exact=%v", pathRef, present, exact)
	}
	if _, exported := binding.inputValue(Root{Kind: RootMiddle}); exported {
		t.Fatal("target MID was treated as a caller-supplied boundary root")
	}
	guardRef, exact := binding.targetGuard(targetGuard)
	if !exact || guardRef.owner != 2 || guardRef.arena != targetArena || guardRef.guard != targetGuard {
		t.Fatalf("guard did not remain target-owned: %#v", guardRef)
	}
	if len(callerArena.values) != beforeCallerValues || len(callerArena.paths) != beforeCallerPaths || len(callerArena.guards) != beforeCallerGuards ||
		len(targetArena.values) != beforeTargetValues || len(targetArena.paths) != beforeTargetPaths || len(targetArena.guards) != beforeTargetGuards {
		t.Fatal("lazy binding manufactured term, path, or guard syntax")
	}
}

func TestFreezeRelationApplicationGuardPlanKeepsMiddleAtomInTargetArena(t *testing.T) {
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("frozen-lazy-middle-guard"))
	callerArena, targetArena := NewArena(reg), NewArena(reg)
	callerArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1))
	targetArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 2))
	callerArena.bindEnvironmentSymbol(7301)
	targetArena.bindEnvironmentSymbol(7401)
	if err := callerArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	callerValue, _ := callerArena.middleValue(statekey.SymbolValue(7301))
	targetValue, _ := targetArena.middleValue(statekey.SymbolValue(7401))
	shape := Shape{Params: 1}
	frame := callerArena.relationFrame(2, 13, 1, shape, []ValueTerm{callerValue}, []PathTerm{0}, 0)
	guard := targetArena.Truthy(targetValue)
	caller := &relationCode{
		terms: callerArena, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: frame}}}, next: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	target := &relationCode{
		terms: targetArena, shape: shape, root: 1,
		nodes:    []relationNode{{}, {kind: relationNodeChoice, guard: guard, whenTrue: 2}, {kind: relationNodeOutcome, outcome: 1}},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	beforeValues, beforeGuards := len(callerArena.values), len(callerArena.guards)
	closeAndFreezeRelationGuardTestForest(t, []*relationCode{caller, target})
	plan := caller.applicationGuards[frame]
	if !plan.validFor(frame, 2) || len(plan.guards) != 1 || len(plan.guards[0].atoms) != 1 {
		t.Fatalf("lazy guard plan is incomplete: %#v", plan)
	}
	atom := plan.guards[0].atoms[0]
	if !atom.targetLocal || atom.source != targetValue || atom.substituted != targetValue || len(plan.boundAtoms) != 0 {
		t.Fatalf("target MID was copied or made caller-owned: %#v bound=%v", atom, plan.boundAtoms)
	}
	if len(callerArena.values) != beforeValues || len(callerArena.guards) != beforeGuards {
		t.Fatal("freezing target-owned MID guard grew caller syntax")
	}
}
