package program

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

const frozenGraphPath = "/tmp/frozen-graph.lua"

// TestFrozenGraphStrStrictDifferential is an opt-in real-source value gate for
// the parameter-plus-immutable-global slice. It must reduce the contextual work
// of graph.str without changing any stabilized product.
func TestFrozenGraphStrStrictDifferential(t *testing.T) {
	if os.Getenv("GO_LUA_FROZEN_GRAPH") != "1" {
		t.Skip("set GO_LUA_FROZEN_GRAPH=1 to run the frozen graph contract")
	}
	source, err := os.ReadFile(frozenGraphPath)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := parse.ParseString(string(source), frozenGraphPath)
	if err != nil {
		t.Fatal(err)
	}
	check := frozenThreadsCheck(standard.Registry())
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})

	legacyStats, strictStats := &Stats{}, &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: legacyStats, forceLegacyRelations: true})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: strictStats, enableStrictRelationPhaseCollapse: true})
	if err != nil {
		t.Fatal(err)
	}
	if divergence := firstFrozenProductDivergence(standard.Registry(), legacy, strict); divergence != "" {
		t.Fatalf("graph semantic product diverged: %s", divergence)
	}
	legacyDiagnostics, _ := frozenDiagnosticBytes(legacy.RootResult())
	strictDiagnostics, _ := frozenDiagnosticBytes(strict.RootResult())
	if sha256Hex(legacyDiagnostics) != sha256Hex(strictDiagnostics) {
		t.Fatal("graph diagnostic digest changed")
	}
	legacySolves, legacyTransfers := frozenFunctionWork(t, bindings, legacy, legacyStats.BodySolveAttribution(), 95, 98)
	strictSolves, strictTransfers := frozenFunctionWork(t, bindings, strict, strictStats.BodySolveAttribution(), 95, 98)
	legacyReservedSolves, legacyReservedTransfers := frozenFunctionWork(t, bindings, legacy, legacyStats.BodySolveAttribution(), 104, 106)
	strictReservedSolves, strictReservedTransfers := frozenFunctionWork(t, bindings, strict, strictStats.BodySolveAttribution(), 104, 106)
	t.Logf("FROZEN_GRAPH_STR legacy=%d/%d strict=%d/%d planner=%d/%d/%d/%d contexts/omitted/reused=%d/%d/%d misses/fallbacks=%d/%d", legacySolves, legacyTransfers, strictSolves, strictTransfers,
		strictStats.RelationPlannerOwnersScanned, strictStats.RelationPlannerOwnersPrefiltered, strictStats.RelationPlannerOwnersCompiled, strictStats.RelationPlannerOwnersActivated,
		strictStats.RelationContextsSpecialized, strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused,
		strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	if strictSolves >= legacySolves || strictTransfers >= legacyTransfers {
		t.Fatalf("graph.str work did not fall: legacy=%d/%d strict=%d/%d", legacySolves, legacyTransfers, strictSolves, strictTransfers)
	}
	if legacyReservedSolves != 8 || legacyReservedTransfers != 40 || strictReservedSolves != 4 || strictReservedTransfers != 20 {
		t.Fatalf("is_reserved_id work = legacy %d/%d strict %d/%d, want 8/40 -> 4/20",
			legacyReservedSolves, legacyReservedTransfers, strictReservedSolves, strictReservedTransfers)
	}
	if legacyStats.Body.BodySolves != 361 || legacyStats.Body.Transfer.Solver.TransferCalls != 18763 ||
		strictStats.Body.BodySolves != 315 || strictStats.Body.Transfer.Solver.TransferCalls != 18425 {
		t.Fatalf("graph aggregate = legacy %d/%d strict %d/%d, want 361/18763 -> 315/18425",
			legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls,
			strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
	if strictStats.RelationContextsSpecialized != 20 || strictStats.RelationSummaryEquationsOmitted != 23 || strictStats.RelationMaterializationsReused != 46 {
		t.Fatalf("graph contexts/omitted/reused = %d/%d/%d, want 20/23/46",
			strictStats.RelationContextsSpecialized, strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
	}
}

func frozenFunctionWork(t testing.TB, bindings *bind.Result, result Result, attribution []BodySolveAttribution, line, lastLine int) (int, int) {
	t.Helper()
	byKey := make(map[summary.SummaryKey]*ast.FunctionExpr)
	for _, fn := range bindings.Functions() {
		symbol, ok := bindings.FunctionSymbol(fn)
		if !ok {
			continue
		}
		key, ok := result.FunctionKey(symbol)
		if ok {
			byKey[key] = fn
		}
	}
	var solves, transfers int
	for _, entry := range attribution {
		fn := byKey[summary.DefaultSummaryKey(entry.Function.Ref)]
		if fn == nil || fn.Line() != line || fn.LastLine() != lastLine {
			continue
		}
		solves += entry.BodySolves
		transfers += entry.PointTransfers
	}
	if solves == 0 {
		t.Fatalf("function at lines %d-%d was not attributed", line, lastLine)
	}
	return solves, transfers
}
