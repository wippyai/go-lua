package body

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func resolveReferenceTransformerFixture(t testing.TB) *Static {
	t.Helper()
	src, err := os.ReadFile("../../../testdata/fixtures/regression/deadlock-compiler-lua/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	stmts := parseChunk(t, string(src))
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"uuid"}})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func.Line() != 289 {
			continue
		}
		prepared, err := PrepareBoundFunction(origin.Func, bindings, Config{
			Registry: standard.Registry(), Globals: []string{"uuid"},
			Signatures: signaturelookup.Source{IncludeStdlib: true}, Schedule: transfer.ScheduleWTO,
		})
		if err != nil {
			t.Fatalf("PrepareBoundFunction(resolve_reference): %v", err)
		}
		return prepared
	}
	t.Fatal("FlowGraph.resolve_reference at line 289 is missing")
	return nil
}

func TestResolveReferenceFalsyBranchesAreWholeFunctionExact(t *testing.T) {
	prepared := resolveReferenceTransformerFixture(t)
	shape := transformer.Shape{
		Params:   uint32(len(prepared.operationPlan.BoundaryParams())),
		Captures: uint32(len(prepared.operationPlan.BoundaryCaptures())),
	}
	dynamicExact, dynamicTotal := 0, 0
	rootExact, rootTotal := 0, 0
	for _, entry := range transformer.NewPlanCompiler().EligibilityCensus(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, shape) {
		switch entry.Family {
		case "DynamicIndexExpressions":
			dynamicTotal++
			if entry.Exact {
				dynamicExact++
			}
		case "RootAssignments":
			rootTotal++
			if entry.Exact {
				rootExact++
			}
		}
	}
	if dynamicExact != 1 || dynamicTotal != 1 {
		t.Fatalf("resolve_reference dynamic expression exact = %d/%d, want 1/1", dynamicExact, dynamicTotal)
	}
	if rootExact == 0 || rootTotal == 0 {
		t.Fatalf("resolve_reference root assignment exact = %d/%d, want dynamic-read local admitted", rootExact, rootTotal)
	}
	relation := transformer.NewPlanCompiler().Compile(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("resolve_reference relation is contextual: %s", reason)
	}
	t.Logf("resolve_reference concat, dynamic read, both normalized falsy branches, and correlated returns are exact; root assignments=%d/%d", rootExact, rootTotal)
}
