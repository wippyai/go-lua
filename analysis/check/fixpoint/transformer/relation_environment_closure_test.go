package transformer

import (
	"errors"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRelationTermClosureTurnsFormalEnvironmentIntoRebaseableArithmetic(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	leftID, rightID := symbol.ID(801), symbol.ID(802)
	left, right := arena.bindEnvironmentSymbol(leftID), arena.bindEnvironmentSymbol(rightID)
	sum, exact := arena.ScalarBinaryValue("+", left, right)
	if !exact {
		t.Fatal("test arithmetic term did not construct")
	}
	shape := Shape{Params: 2}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{leftID, rightID}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, shape, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := arena.EnvironmentPath(leftID, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	closed, err := closure.close(TermRebaseInput{Values: []ValueTerm{sum}, Paths: []PathTerm{sourcePath}, Guards: []Guard{arena.Truthy(sum)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.Values) != 1 || len(closed.Paths) != 1 || len(closed.Guards) != 1 {
		t.Fatalf("closed transaction has wrong widths: %#v", closed)
	}
	closedNode := arena.values[closed.Values[0]]
	if closedNode.op != valueBinaryOperation || len(closedNode.args) != 2 || arena.values[closedNode.args[0]].op != valueRoot || arena.values[closedNode.args[1]].op != valueRoot {
		t.Fatalf("arithmetic did not close over formal roots: %s", arena.canonicalValue(closed.Values[0]))
	}
	pathNode := arena.paths[closed.Paths[0]]
	if pathNode.environment != 0 || pathNode.root != (Root{Kind: RootMiddle, Index: 0}) || len(pathNode.segments) != 1 {
		t.Fatalf("environment path did not close over stable Middle root: %s", arena.canonicalPath(closed.Paths[0]))
	}
	if got := arena.middle.entries; len(got) != 2 || got[0] != (relationMiddleEntry{middle: Root{Kind: RootMiddle, Index: 0}, input: Root{Kind: RootParam, Index: 0}}) || got[1] != (relationMiddleEntry{middle: Root{Kind: RootMiddle, Index: 1}, input: Root{Kind: RootParam, Index: 1}}) {
		t.Fatalf("IN -> MID entry bindings = %#v", got)
	}
	cursor, _ := NewBindingCursor(shape, make([]product.Value, shape.InputCount()), nil)
	inputs := []product.Value{typevalue.LiteralInt(reg, 2), typevalue.LiteralInt(reg, 3)}
	got, evaluated := arena.evalValue(closed.Values[0], cursor, SpecializationContext{MiddleValue: func(root Root) (product.Value, bool) {
		if root.Kind != RootMiddle || int(root.Index) >= len(inputs) {
			return product.Value{}, false
		}
		return inputs[root.Index], true
	}})
	want, wantOK := luasourcevalue.BinaryOperationValue(reg, nil, "+", typevalue.LiteralInt(reg, 2), typevalue.LiteralInt(reg, 3))
	if evaluated != wantOK || evaluated && !product.Equal(reg, got, want) {
		t.Fatalf("closed arithmetic evaluated to %#v/%v, canonical kernel wants %#v/%v", got, evaluated, want, wantOK)
	}
}

func TestRelationTermClosureBindsAmbientEnvironmentToDistinctRoot(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	id := symbol.ID(809)
	value := arena.bindEnvironmentSymbol(id)
	path := arena.EnvironmentPath(id, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	shape := Shape{Ambients: 1}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, shape, plan, []AmbientRoot{{Symbol: id, Mutable: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closure.close(TermRebaseInput{Values: []ValueTerm{value}, Paths: []PathTerm{path}})
	if err != nil {
		t.Fatal(err)
	}
	want := Root{Kind: RootMiddle, Index: 0}
	if got := arena.values[closed.Values[0]].root; got != want {
		t.Fatalf("ambient value closed to root %#v, want %#v", got, want)
	}
	pathNode := arena.paths[closed.Paths[0]]
	if pathNode.environment != 0 || pathNode.root != want || len(pathNode.segments) != 1 {
		t.Fatalf("ambient path did not close over distinct ambient root: %s", arena.canonicalPath(closed.Paths[0]))
	}
	if got := arena.middle.entries; len(got) != 1 || got[0].middle != want || got[0].input != (Root{Kind: RootAmbient, Index: 0}) {
		t.Fatalf("ambient IN -> MID binding = %#v", got)
	}
}

func TestRelationTermClosureKeepsResolvedFormalPathInsideDynamicRead(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	id := symbol.ID(811)
	owner := arena.bindEnvironmentSymbol(id)
	path := arena.EnvironmentPath(id)
	read := arena.DynamicReadValueAt(
		cfg.Point(7), owner, path,
		arena.Constant(typevalue.LiteralString(reg, "member")),
	)
	shape := Shape{Params: 1}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{id}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, shape, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closure.close(TermRebaseInput{Values: []ValueTerm{read}})
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.Values) != 1 {
		t.Fatalf("closed DynamicRead width = %d, want one", len(closed.Values))
	}
	node := arena.values[closed.Values[0]]
	if node.op != valueDynamicRead || node.point != 7 || node.path == 0 {
		t.Fatalf("closed DynamicRead lost exact path: %s", arena.canonicalValue(closed.Values[0]))
	}
	pathNode := arena.paths[node.path]
	if pathNode.environment != 0 || pathNode.root != (Root{Kind: RootMiddle}) || len(pathNode.segments) != 0 {
		t.Fatalf("closed DynamicRead path = %s, want stable Middle root", arena.canonicalPath(node.path))
	}
}

func TestRelationTermClosureRetainsDefinedInvocationLocalPath(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	local := symbol.ID(810)
	environment := arena.bindEnvironmentSymbol(local)
	path := arena.EnvironmentPath(local, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, Shape{}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closure.close(TermRebaseInput{Values: []ValueTerm{environment}, Paths: []PathTerm{path}, Guards: []Guard{arena.Truthy(environment)}})
	if err != nil {
		t.Fatal(err)
	}
	middleValue, middle := arena.middleValue(statekey.SymbolValue(local))
	if len(closed.Values) != 1 || !middle || closed.Values[0] != middleValue || closed.Values[0] == environment || arena.values[closed.Values[0]].root != (Root{Kind: RootMiddle}) {
		t.Fatalf("invocation-local scalar did not close to stable Middle root: %v", closed.Values)
	}
	first := closed
	closed, err = closure.close(TermRebaseInput{Values: closed.Values, Paths: closed.Paths, Guards: closed.Guards})
	if err != nil || !reflect.DeepEqual(closed, first) {
		t.Fatalf("sequential closure changed already-MID scalar/path/guard: first=%#v second=%#v err=%v", first, closed, err)
	}
}

func TestRelationTermClosureComposesScalarCellBeforePublication(t *testing.T) {
	reg := standard.Registry()
	callee := NewArena(reg)
	calleeShape := Shape{Params: 1}
	param := callee.Root(Root{Kind: RootParam})
	one := callee.Constant(typevalue.LiteralInt(reg, 1))
	result, exact := callee.ScalarBinaryValue("+", param, one)
	if !exact {
		t.Fatal("test cell closure did not construct")
	}

	caller := NewArena(reg)
	id := symbol.ID(811)
	environment := caller.bindEnvironmentSymbol(id)
	cell := CellRef{Function: 44, Slot: 2}
	deferred := caller.CellResultValue(cell, environment)
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{id}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(caller, Shape{Params: 1}, plan, nil, map[CellRef]relationScalarClosure{
		cell: {arena: callee, shape: calleeShape, result: result},
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closure.close(TermRebaseInput{Values: []ValueTerm{deferred}})
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.Values) != 1 || caller.values[closed.Values[0]].op == valueCellResult {
		t.Fatalf("cell result survived closure: %s", caller.canonicalValue(closed.Values[0]))
	}
	cursor, _ := NewBindingCursor(Shape{Params: 1}, make([]product.Value, 1), nil)
	got, evaluated := caller.evalValue(closed.Values[0], cursor, SpecializationContext{MiddleValue: func(root Root) (product.Value, bool) {
		if root != (Root{Kind: RootMiddle}) {
			return product.Value{}, false
		}
		return typevalue.LiteralInt(reg, 8), true
	}})
	want, wantOK := luasourcevalue.BinaryOperationValue(reg, nil, "+", typevalue.LiteralInt(reg, 8), typevalue.LiteralInt(reg, 1))
	if evaluated != wantOK || evaluated && !product.Equal(reg, got, want) {
		t.Fatalf("composed cell evaluated to %#v/%v, canonical kernel wants %#v/%v", got, evaluated, want, wantOK)
	}
}

func TestRelationTermClosureClosesEveryDeclaredInternalRegisterToMiddle(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	formal, local := symbol.ID(821), symbol.ID(822)
	arena.bindEnvironmentSymbol(formal)
	unresolved := arena.bindEnvironmentSymbol(local)
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{formal}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, Shape{Params: 1}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := closure.close(TermRebaseInput{Values: []ValueTerm{unresolved}})
	if err != nil || len(got.Values) != 1 || arena.values[got.Values[0]].root.Kind != RootMiddle {
		t.Fatalf("declared internal register did not close to MID: %#v, %v", got, err)
	}

	frame := arena.callFrame(CellRef{Function: 55, Slot: 1}, 1, 0, Shape{Params: 1}, []ValueTerm{arena.Root(Root{Kind: RootParam})}, []PathTerm{0}, 1)
	frameResult := arena.frameResultValue(frame, 0)
	closedFrame, err := closure.close(TermRebaseInput{Values: []ValueTerm{frameResult}})
	if err != nil || len(closedFrame.Values) != 1 || closedFrame.Values[0] != frameResult {
		t.Fatalf("post-Apply frame selector did not remain exact: %#v, %v", closedFrame, err)
	}
}

func TestRelationTermClosureRetainsExpressionJoinRegister(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	expression := arena.bindExpressionValue(factflow.ExprRef(823))
	if expression == 0 {
		t.Fatal("expression join register did not construct")
	}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, Shape{}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	closed, err := closure.close(TermRebaseInput{
		Values: []ValueTerm{expression},
		Guards: []Guard{arena.Truthy(expression)},
	})
	if err != nil {
		t.Fatalf("invocation-owned expression register did not close: %v", err)
	}
	middle, exact := arena.middleValue(statekey.ExpressionValue(823))
	if !exact || len(closed.Values) != 1 || closed.Values[0] != middle {
		t.Fatalf("expression register did not close to MID: %v, want %d", closed.Values, middle)
	}
	if len(closed.Guards) != 1 || closed.Guards[0] == 0 || arena.guards[closed.Guards[0]].value != middle {
		t.Fatalf("guard lost its exact expression register: %v", closed.Guards)
	}
}

func TestRelationTermClosureLeavesNoReachableRawEnvironmentSelectors(t *testing.T) {
	arena := NewArena(standard.Registry())
	symbolRegister := arena.bindEnvironmentSymbol(symbol.ID(825))
	callRegister := arena.bindCallResult(cfg.Point(7), 2)
	expressionRegister := arena.bindExpressionValue(factflow.ExprRef(826))
	path := arena.EnvironmentPath(symbol.ID(825), segment.Segment{Kind: segment.SegmentField, Name: "field"})
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{825}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, Shape{Params: 1}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closed, err := closure.close(TermRebaseInput{
		Values: []ValueTerm{symbolRegister, callRegister, expressionRegister},
		Paths:  []PathTerm{path},
		Guards: []Guard{arena.Truthy(expressionRegister)},
	})
	if err != nil {
		t.Fatal(err)
	}
	seenValues := make(map[ValueTerm]struct{})
	var inspectValue func(ValueTerm)
	inspectValue = func(term ValueTerm) {
		if term == 0 {
			return
		}
		if _, seen := seenValues[term]; seen {
			return
		}
		seenValues[term] = struct{}{}
		node := arena.values[term]
		if node.op == valueEnvironment {
			t.Fatalf("closed DAG retained raw environment selector %d", node.slot)
		}
		for _, child := range node.args {
			inspectValue(child)
		}
	}
	for _, term := range closed.Values {
		inspectValue(term)
		if arena.values[term].op != valueRoot || arena.values[term].root.Kind != RootMiddle {
			t.Fatalf("closed register %d is not a MID root", term)
		}
	}
	for _, guard := range closed.Guards {
		inspectValue(arena.guards[guard].value)
	}
	for _, term := range closed.Paths {
		node := arena.paths[term]
		if node.environment != 0 || node.root.Kind != RootMiddle {
			t.Fatalf("closed path retained raw environment selector: %#v", node)
		}
	}
}

func TestRelationCodeClosureRetainsOrderedExpressionJoinWriteWithoutPathAddress(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	const expressionRef = factflow.ExprRef(824)
	expression := arena.bindExpressionValue(expressionRef)
	value := arena.Constant(typevalue.LiteralBool(reg, true))
	if expression == 0 || value == 0 {
		t.Fatal("expression join fixture did not construct")
	}
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	code := &relationCode{
		terms: arena, effects: NewEffectArena(arena), root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{{
				kind: boundaryStepEnvironmentWrite, slot: statekey.ExpressionValue(uint32(expressionRef)), value: value,
			}}, next: 2},
			{kind: relationNodeChoice, guard: arena.Truthy(expression), whenTrue: 3, whenFalse: 4},
			{kind: relationNodeOutcome, outcome: 1},
			{kind: relationNodeOutcome, outcome: 2},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}, {}},
	}
	closure, err := newRelationTermClosure(arena, Shape{}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeRelationCodeTerms(code, closure, plan, []*relationCode{code}); err != nil {
		t.Fatalf("expression join code closure: %v", err)
	}
	if len(code.nodes[1].steps) != 1 || code.nodes[1].steps[0].kind != boundaryStepEnvironmentWrite {
		t.Fatalf("expression join write was discarded as non-path-addressable: %#v", code.nodes[1].steps)
	}
	middle, exact := arena.middleValue(statekey.ExpressionValue(uint32(expressionRef)))
	if !exact || code.nodes[2].guard == 0 || arena.guards[code.nodes[2].guard].value != middle {
		t.Fatalf("downstream guard lost expression join register: %#v", code.nodes[2])
	}
}

func TestRelationTermClosureRejectsReturnSlotAsInternalEnvironment(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	slot := statekey.ReturnSlot(0)
	// ReturnSlot is publication vocabulary, never an invocation register. The
	// direct construction deliberately bypasses Arena's sanctioned binders to
	// prove closure rejects this malformed spelling rather than preserving it.
	arena.environment[slot] = struct{}{}
	term := arena.internValue(valueNode{op: valueEnvironment, slot: slot})
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	closure, err := newRelationTermClosure(arena, Shape{}, plan, nil, nil)
	if err == nil {
		_, err = closure.close(TermRebaseInput{Values: []ValueTerm{term}})
	}
	var closureErr *relationTermClosureError
	if !errors.As(err, &closureErr) || closureErr.kind != relationTermClosureMalformed {
		t.Fatalf("return slot did not fail as non-invocation vocabulary: %v", err)
	}
}

func TestRelationCodeClosureBindsCanonicalGenericForTargetBeforeLaterTerms(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	target, sink := symbol.ID(831), symbol.ID(832)
	op, ok := operationplan.NewGenericForOperation(0, target, target, nil, nil)
	if !ok {
		t.Fatal("generic-for operation rejected")
	}
	plan := operationplan.New(graph, factflow.FactsInput{}).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil).
		WithExtensions([]operationplan.ExtensionInput{{Point: point, Kind: operationplan.BodyGenericFor, GenericFor: op}})
	arena := NewArena(reg)
	targetRegister := arena.bindEnvironmentSymbol(target)
	if arena.bindEnvironmentSymbol(sink) == 0 || arena.EnvironmentPath(sink) == 0 {
		t.Fatal("sink register did not acquire canonical local path ownership")
	}
	code := &relationCode{
		terms: arena, effects: NewEffectArena(arena), root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepGenericFor, point: point},
				{kind: boundaryStepEnvironmentWrite, slot: statekey.SymbolValue(sink), value: targetRegister},
			}, next: 2},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	closure, err := newRelationTermClosure(arena, Shape{}, plan, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := closeRelationCodeTerms(code, closure, plan, []*relationCode{code}); err != nil {
		t.Fatalf("post-generic-for target did not close through its canonical producer: %v", err)
	}
	middle, exact := arena.middleValue(statekey.SymbolValue(target))
	if got := code.nodes[1].steps[1].value; !exact || got != middle {
		t.Fatalf("post-generic-for target = %d, want Middle register %d", got, middle)
	}
}

func TestRelationCodeClosureCarriesGenericForRegisterOnlyAlongExactLoopExitRoute(t *testing.T) {
	for _, test := range []struct {
		name             string
		zeroIterationUse bool
	}{
		{name: "body exit carries the transaction-owned target"},
		{name: "zero-iteration exit retains the same uninitialized Middle coordinate", zeroIterationUse: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := standard.Registry()
			graph := cfg.New()
			point := graph.AddNode(cfg.NodeAssign)
			condition, target := symbol.ID(841), symbol.ID(842)
			op, ok := operationplan.NewGenericForOperation(0, target, target, nil, nil)
			if !ok {
				t.Fatal("generic-for operation rejected")
			}
			plan := operationplan.New(graph, factflow.FactsInput{}).
				WithBoundaryParams([]symbol.ID{condition}).
				WithBoundaryCaptures(nil).
				WithBoundaryGlobals(nil).
				WithExtensions([]operationplan.ExtensionInput{{Point: point, Kind: operationplan.BodyGenericFor, GenericFor: op}})
			arena := NewArena(reg)
			conditionRegister := arena.bindEnvironmentSymbol(condition)
			targetRegister := arena.bindEnvironmentSymbol(target)
			binder := arena.loopMu(point, 0, []cfg.Point{point}, []loopMuBackedge{{from: point, to: point}})
			if conditionRegister == 0 || targetRegister == 0 || binder == 0 {
				t.Fatal("loop fixture did not acquire canonical registers")
			}
			zeroOutcome := relationRootRef(8)
			outcomes := []boundaryOutcomeTuple{
				{},
				{returnTransaction: testReturnTransactionTerm(t, point, targetRegister)},
				{returnTransaction: testReturnTransactionTerm(t, point, arena.Constant(typevalue.Nil(reg)))},
			}
			if test.zeroIterationUse {
				zeroOutcome = 7
				outcomes = outcomes[:2]
			}
			code := &relationCode{
				terms: arena, effects: NewEffectArena(arena), root: 1,
				nodes: []relationNode{
					{},
					{kind: relationNodeLoopMu, binder: binder, body: 2, exits: []relationRootRef{7, zeroOutcome}},
					{kind: relationNodeChoice, guard: arena.Truthy(conditionRegister), whenTrue: 3, whenFalse: 6},
					{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepGenericFor, point: point}}, next: 4},
					{kind: relationNodeChoice, guard: arena.Truthy(conditionRegister), whenTrue: 5, whenFalse: 9},
					{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: binder, route: 0}}},
					{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: binder, route: 1}}},
					{kind: relationNodeOutcome, outcome: 1},
					{kind: relationNodeOutcome, outcome: 2},
					{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: binder}}},
				},
				outcomes: outcomes,
			}
			closure, err := newRelationTermClosure(arena, Shape{Params: 1}, plan, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = closeRelationCodeTerms(code, closure, plan, []*relationCode{code})
			if err != nil {
				t.Fatalf("body exit lost generic-for target ownership: %v", err)
			}
			middle, exact := arena.middleValue(statekey.SymbolValue(target))
			if got := code.outcomes[code.nodes[7].outcome].returnTransaction.sources[0]; !exact || got != middle {
				t.Fatalf("body exit target = %d, want stable Middle register %d", got, middle)
			}
		})
	}
}

func TestRelationEnvironmentClosureStopsDescendantLocalMutationAtDeclaringOwner(t *testing.T) {
	root, owner, leaf := relationEnvironmentTestBodies(t)
	local := symbol.ID(901)
	ownerPlan := relationEnvironmentTestPlan(t, local, false, nil)
	leafPlan := relationEnvironmentTestPlan(t, local, true, []symbol.ID{local})

	closure := relationEnvironmentTestClosure(t, []RelationProgramUnit{
		{Body: root, Plan: relationEnvironmentTestPlan(t, 0, false, nil), Definitions: []RelationProgramDefinition{{Target: owner}}},
		{Body: owner, Plan: ownerPlan, Definitions: []RelationProgramDefinition{{Target: leaf}}},
		{Body: leaf, Plan: leafPlan},
	})

	if len(closure.ambient[0]) != 0 || len(closure.mutable[0]) != 0 {
		t.Fatalf("ancestor inherited descendant-owned local: ambient=%v mutable=%v", closure.ambient[0], closure.mutable[0])
	}
	if len(closure.ambient[1]) != 0 || len(closure.mutable[1]) != 0 {
		t.Fatalf("declaring owner acquired a duplicate ambient carrier: ambient=%v mutable=%v", closure.ambient[1], closure.mutable[1])
	}
	if _, ok := closure.mutable[2][local]; !ok {
		t.Fatal("direct capture write was not classified mutable at the writing body")
	}
}

func TestRelationEnvironmentClosureCarriesTransitiveCapturedMutationToOwner(t *testing.T) {
	root, middle, leaf := relationEnvironmentTestBodies(t)
	captured := symbol.ID(902)
	leafPlan := relationEnvironmentTestPlan(t, captured, true, []symbol.ID{captured})

	closure := relationEnvironmentTestClosure(t, []RelationProgramUnit{
		{Body: root, Plan: relationEnvironmentTestPlan(t, captured, false, nil), Definitions: []RelationProgramDefinition{{Target: middle}}},
		{Body: middle, Plan: relationEnvironmentTestPlan(t, 0, false, nil), Definitions: []RelationProgramDefinition{{Target: leaf}}},
		{Body: leaf, Plan: leafPlan},
	})

	if got, want := closure.ambient[1], []symbol.ID{captured}; !reflect.DeepEqual(got, want) {
		t.Fatalf("intermediate carrier ambient roots = %v, want %v", got, want)
	}
	if _, ok := closure.mutable[1][captured]; !ok {
		t.Fatal("transitive capture mutation lost its outbound carrier")
	}
	if len(closure.ambient[0]) != 0 || len(closure.mutable[0]) != 0 {
		t.Fatalf("mutation did not terminate at declaring owner: ambient=%v mutable=%v", closure.ambient[0], closure.mutable[0])
	}
}

func relationEnvironmentTestBodies(t *testing.T) (lexicalidentity.StableLexicalBodyID, lexicalidentity.StableLexicalBodyID, lexicalidentity.StableLexicalBodyID) {
	t.Helper()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	return lexicalidentity.RootBody(namespace), lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.FunctionBody(namespace, 2)
}

func relationEnvironmentTestPlan(t *testing.T, local symbol.ID, write bool, captures []symbol.ID) *operationplan.Plan {
	t.Helper()
	graph := cfg.New()
	facts := factflow.FactsInput{}
	if local != 0 {
		point := graph.AddNode(cfg.NodeAssign)
		kind := factflow.RootAssignmentLocalDeclaration
		if write {
			kind = factflow.RootAssignmentOrdinaryRootWrite
		}
		facts.RootAssignments = map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(kind, local, pathdom.NewPath(local, ""), factflow.ValueSource{}),
		}
	}
	return operationplan.New(graph, facts).
		WithBoundaryParams(nil).
		WithBoundaryCaptures(captures).
		WithBoundaryGlobals(nil)
}

func relationEnvironmentTestClosure(t *testing.T, units []RelationProgramUnit) relationEnvironmentClosure {
	t.Helper()
	byBody := make(map[lexicalidentity.StableLexicalBodyID]relationVar, len(units))
	for index, unit := range units {
		byBody[unit.Body] = relationVar(index + 1)
	}
	closure, err := closeRelationEnvironments(units, make([]programCallSurface, len(units)), byBody)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}
