package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestPreparedFunctionSolvesWithIndependentEntryStates(t *testing.T) {
	reg, markKey := testRegistry(t)
	fn := parseFunction(t, "function f(x) local y = x return y end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	local := fn.Stmts[0].(*ast.LocalAssignStmt)
	stats := Stats{}
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg, Stats: &stats})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}

	firstWant := markedValue(reg, markKey, markLow)
	first := solvePreparedForTest(t, prepared, SolveConfig{
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(slot.Symbol), firstWant),
		Stats:      &stats,
	})
	secondWant := markedValue(reg, markKey, markHigh)
	second := solvePreparedForTest(t, prepared, SolveConfig{
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(slot.Symbol), secondWant),
		Stats:      &stats,
	})

	firstExit, ok := first.ExitState()
	if !ok {
		t.Fatal("missing first exit state")
	}
	secondExit, ok := second.ExitState()
	if !ok {
		t.Fatal("missing second exit state")
	}
	firstLocal := mustLocalAt(t, first, local, 0)
	secondLocal := mustLocalAt(t, second, local, 0)
	assertProductEqual(t, reg, firstExit.ReadValue(reg, key.SymbolValue(firstLocal)), firstWant)
	assertProductEqual(t, reg, secondExit.ReadValue(reg, key.SymbolValue(secondLocal)), secondWant)
	if stats.StaticChunkPrepares != 0 || stats.StaticFunctionPrepares != 1 || stats.BodySolves != 2 {
		t.Fatalf("stats = %#v, want one function prepare and two body solves", stats)
	}
}

func TestPreparedStaticOwnsEntrySeedPlan(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, "function f(x: string) return x end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if !prepared.entrySeedsPrepared {
		t.Fatal("prepared body did not record entry seed ownership")
	}
	if len(prepared.entrySeeds) != 1 {
		t.Fatalf("entry seed count = %d, want 1", len(prepared.entrySeeds))
	}

	entry, initial := prepared.solveEntryState(prepared.typeValues, state.State{}, nil)
	if initial != nil {
		t.Fatal("solveEntryState installed an initial wrapper without a caller initial")
	}
	value := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("entry seed type = %v/%v, want string", got, ok)
	}
}

func TestPreparedSolveKeepsPreparedTypeValuesAsCanonicalCache(t *testing.T) {
	reg, _ := testRegistry(t)
	preparedTypeValues := typevalue.NewCache()
	solveTypeValues := typevalue.NewCache()
	stmts := parseChunk(t, `local out = f()`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		TypeValues: preparedTypeValues,
	})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	var factoryTypeValues *typevalue.Cache
	result := solvePreparedForTest(t, prepared, SolveConfig{
		TypeValues: solveTypeValues,
		CallOutcomeFactory: func(ctx CallOutcomeContext) callpayload.CallOutcomeProvider {
			factoryTypeValues = ctx.TypeValues
			return nil
		},
	})

	if result.TypeValues() != preparedTypeValues {
		t.Fatalf("result TypeValues = %p, want prepared cache %p", result.TypeValues(), preparedTypeValues)
	}
	if factoryTypeValues != preparedTypeValues {
		t.Fatalf("factory TypeValues = %p, want prepared cache %p", factoryTypeValues, preparedTypeValues)
	}
	if result.TypeValues() == solveTypeValues || factoryTypeValues == solveTypeValues {
		t.Fatal("SolveConfig TypeValues overrode the prepared cache")
	}
}

func TestPreparedChunkSolvesWithIndependentCallOutcomes(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f()`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	local := stmts[0].(*ast.LocalAssignStmt)
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	firstWant := markedValue(reg, markKey, markLow)
	first := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(firstWant),
	})
	secondWant := markedValue(reg, markKey, markHigh)
	second := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(secondWant),
	})

	firstExit, ok := first.ExitState()
	if !ok {
		t.Fatal("missing first exit state")
	}
	secondExit, ok := second.ExitState()
	if !ok {
		t.Fatal("missing second exit state")
	}
	firstLocal := mustLocalAt(t, first, local, 0)
	secondLocal := mustLocalAt(t, second, local, 0)
	assertProductEqual(t, reg, firstExit.ReadValue(reg, key.SymbolValue(firstLocal)), firstWant)
	assertProductEqual(t, reg, secondExit.ReadValue(reg, key.SymbolValue(secondLocal)), secondWant)
}

func TestPreparedSolveBoundaryCacheDoesNotLeak(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f()`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	local := stmts[0].(*ast.LocalAssignStmt)
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	firstWant := markedValue(reg, markKey, markLow)
	first := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(firstWant),
	})
	point := requireLocalAssignmentPoint(t, first, local, 0)
	fact, ok := first.LocalAssignment(point)
	if !ok {
		t.Fatalf("missing first local assignment at %d", point)
	}
	firstGot, ok := first.LocalAssignmentSourceValueAtBoundary(point, fact.Source)
	if !ok {
		t.Fatal("first boundary source read failed")
	}
	assertProductEqual(t, reg, firstGot, firstWant)
	if first.boundary == nil {
		t.Fatal("first solve did not populate its boundary cache")
	}

	secondWant := markedValue(reg, markKey, markHigh)
	second := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(secondWant),
	})
	if second.boundary != nil {
		t.Fatal("second solve started with a boundary cache")
	}
	secondGot, ok := second.LocalAssignmentSourceValueAtBoundary(point, fact.Source)
	if !ok {
		t.Fatal("second boundary source read failed")
	}
	assertProductEqual(t, reg, secondGot, secondWant)
}

func TestRebindBoundaryProvidersClearsLazyCallOutcomeCache(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, `local out = f()`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	local := stmts[0].(*ast.LocalAssignStmt)
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	firstWant := markedValue(reg, markKey, markLow)
	result := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(firstWant),
	})
	_ = requireLocalAssignmentPoint(t, result, local, 0)
	point := requireOnlyCallSitePoint(t, result)
	first, ok := result.CallOutcomeAt(point)
	if !ok || len(first.Results) != 1 {
		t.Fatalf("first CallOutcomeAt = %#v/%v, want one result", first, ok)
	}
	assertProductEqual(t, reg, first.Results[0].Value, firstWant)

	secondWant := markedValue(reg, markKey, markHigh)
	RebindBoundaryProviders(result, prepared, SolveConfig{
		CallOutcome: staticCallOutcome(secondWant),
	})
	second, ok := result.CallOutcomeAt(point)
	if !ok || len(second.Results) != 1 {
		t.Fatalf("second CallOutcomeAt = %#v/%v, want one result", second, ok)
	}
	assertProductEqual(t, reg, second.Results[0].Value, secondWant)
}

func TestOpenTailReturnPresenceRelationsUseCachedCallOutcome(t *testing.T) {
	reg, _ := testRegistry(t)
	stmts := parseChunk(t, `return f()`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}

	callOutcomeCalls := 0
	result := solvePreparedForTest(t, prepared, SolveConfig{
		CallOutcome: func(_ transfer.NodeContext, _ factflow.CallSiteView, _ state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
			callOutcomeCalls++
			return callpayload.CallOutcome{ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{{
				TriggerIndex:    1,
				TriggerPresence: presence.Present(),
				TargetIndex:     0,
				TargetPresence:  presence.Absent(),
			}}}
		},
	})
	callPoint := requireOnlyCallSitePoint(t, result)
	if _, ok := result.CallOutcomeAt(callPoint); !ok {
		t.Fatal("CallOutcomeAt returned !ok")
	}
	callsAfterCachedRead := callOutcomeCalls
	returnPoints := result.ReturnPoints()
	if len(returnPoints) != 1 {
		t.Fatalf("ReturnPoints = %v, want one return", returnPoints)
	}

	first := result.ReturnPresenceRelations(returnPoints[0])
	second := result.ReturnPresenceRelations(returnPoints[0])
	if callOutcomeCalls != callsAfterCachedRead {
		t.Fatalf("ReturnPresenceRelations bypassed CallOutcomeAt cache: provider called %d extra times", callOutcomeCalls-callsAfterCachedRead)
	}
	for name, relations := range map[string][]factflow.ReturnPresenceRelation{"first": first, "second": second} {
		if len(relations) != 1 {
			t.Fatalf("%s ReturnPresenceRelations len = %d, want 1", name, len(relations))
		}
		relation := relations[0]
		if relation.TriggerIndex() != 1 ||
			!presence.Equal(relation.TriggerPresence(), presence.Present()) ||
			relation.TargetIndex() != 0 ||
			!presence.Equal(relation.TargetPresence(), presence.Absent()) {
			t.Fatalf("%s ReturnPresenceRelations[0] = %#v, want slot 1 present => slot 0 absent", name, relation)
		}
	}
}

func TestResultReturnPointsCachesReadOnlySlice(t *testing.T) {
	reg, _ := testRegistry(t)
	stmts := parseChunk(t, `return "ok"`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})

	first := result.ReturnPoints()
	second := result.ReturnPoints()
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("return points first=%v second=%v, want one point", first, second)
	}
	if &first[0] != &second[0] {
		t.Fatal("ReturnPoints did not reuse the cached read-only slice")
	}
}

func TestResultParameterReadModelsCacheReadOnlyData(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, `function f(x) x = "changed" return x end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})

	firstSlots := result.ParameterValueSlots()
	secondSlots := result.ParameterValueSlots()
	if len(firstSlots) != 1 || len(secondSlots) != 1 {
		t.Fatalf("parameter slots first=%v second=%v, want one slot", firstSlots, secondSlots)
	}
	if &firstSlots[0] != &secondSlots[0] {
		t.Fatal("ParameterValueSlots did not reuse the cached read-only slice")
	}

	firstReassigned := result.ReassignedParameterValueSlots()
	secondReassigned := result.ReassignedParameterValueSlots()
	if len(firstReassigned) != 1 || len(secondReassigned) != 1 {
		t.Fatalf("reassigned parameters first=%v second=%v, want one reassigned slot", firstReassigned, secondReassigned)
	}
	if firstReassigned == nil || secondReassigned == nil {
		t.Fatal("reassigned parameter maps were nil")
	}
	for slot := range firstReassigned {
		if _, ok := secondReassigned[slot]; !ok {
			t.Fatalf("cached reassigned map lost slot %v", slot)
		}
	}
}

func TestResultReturnTypeValuesCachesReadOnlySlice(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, `function f(): string return "ok" end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})

	first := result.ReturnTypeValues()
	second := result.ReturnTypeValues()
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("return type values first=%d second=%d, want one", len(first), len(second))
	}
	if &first[0] != &second[0] {
		t.Fatal("ReturnTypeValues did not reuse the cached read-only slice")
	}
}

func solvePreparedForTest(t *testing.T, prepared *Static, config SolveConfig) *Result {
	t.Helper()
	result, err := SolvePrepared(prepared, config)
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	return result
}

func requireOnlyCallSitePoint(t *testing.T, result *Result) cfg.Point {
	t.Helper()
	var out cfg.Point
	for _, candidate := range result.Graph().RPO() {
		if _, ok := result.CallSite(candidate); !ok {
			continue
		}
		if out != 0 {
			t.Fatalf("multiple call sites: %d and %d", out, candidate)
		}
		out = candidate
	}
	if out == 0 {
		t.Fatal("call site not found")
	}
	return out
}

func markedValue(reg *axis.Registry, markKey axis.Key[markValue], mark markValue) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, mark)
}

func staticCallOutcome(value product.Value) callpayload.CallOutcomeProvider {
	return func(_ transfer.NodeContext, _ factflow.CallSiteView, _ state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{
			Index: 0,
			Value: value,
		}}}
	}
}
