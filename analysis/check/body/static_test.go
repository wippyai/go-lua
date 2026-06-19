package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
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

func solvePreparedForTest(t *testing.T, prepared *Static, config SolveConfig) *Result {
	t.Helper()
	result, err := SolvePrepared(prepared, config)
	if err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	return result
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
