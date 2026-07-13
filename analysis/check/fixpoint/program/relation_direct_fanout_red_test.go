package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestStrictRelationPhaseCollapseOwnsClosedDirectFanout is the red contract
// for dependency-closed relation activation. The leaf is one stable lexical
// body, but receives four concrete calls through two distinct direct callers.
// Strict mode may collapse the transaction only when it owns all three lexical
// bases and all three coalesced context equations together; collapsing only
// the call-free leaf would leave the callers paying the legacy re-solve cost.
func TestStrictRelationPhaseCollapseOwnsClosedDirectFanout(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: boolean)
  if value then
    return "yes"
  end
  return "no"
end
local function left(value: boolean)
  local result = leaf(value)
  return result
end
local function right(value: boolean)
  local result = leaf(value)
  return result
end
local a = left(true)
local b = left(false)
local c = right(true)
local d = right(false)
local diagnostic_parity: number = "intentional"
return a, b, c, d, diagnostic_parity
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO,
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("strict-direct-fanout")),
	}
	leaf := findLocalFunctionByName(t, bindings, "leaf")
	leafPrepared, err := body.PrepareBoundFunction(leaf, bindings, check)
	if err != nil {
		t.Fatal(err)
	}
	leafSurface, ok := leafPrepared.OperationPlan().CallSurface()
	if !ok || !leafSurface.Complete() || len(leafSurface.Sites()) != 0 {
		t.Fatalf("leaf call surface = %#v/%v, want complete and empty", leafSurface, ok)
	}
	for _, name := range []string{"left", "right"} {
		caller := findLocalFunctionByName(t, bindings, name)
		prepared, prepareErr := body.PrepareBoundFunction(caller, bindings, check)
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}
		surface, available := prepared.OperationPlan().CallSurface()
		if !available || !surface.Complete() || len(surface.Sites()) != 1 {
			t.Fatalf("%s call surface = %#v/%v, want one complete call", name, surface, available)
		}
		target, lexical := surface.Sites()[0].Target.LexicalBody()
		if !lexical || target != leafPrepared.StableLexicalBodyID() {
			t.Fatalf("%s target = %x/%v, want shared leaf %x", name, target, lexical, leafPrepared.StableLexicalBodyID())
		}
	}
	legacyStats, strictStats := &Stats{}, &Stats{}
	legacy, err := RunBoundChunk(stmts, bindings, Config{
		Check: check, Stats: legacyStats, forceLegacyRelations: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	strict, err := RunBoundChunk(stmts, bindings, Config{
		Check: check, Stats: strictStats, enableStrictRelationPhaseCollapse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// This is the full stabilized oracle: summary keys and products, every
	// prepared body state, ResultVersion lineage, diagnostic bytes, manifests,
	// and the recursively materialized result tree must remain exact.
	compareStrictPhaseCollapseParity(t, reg, legacy, strict)
	if strictDiagnosticCount(strict.RootResult()) == 0 {
		t.Fatal("direct-fanout parity fixture produced no diagnostics")
	}
	if strictStats.RelationUnexpectedMisses != 0 || strictStats.RelationActivationFallbacks != 0 {
		t.Fatalf("direct-fanout misses/fallbacks = %d/%d, want 0/0",
			strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	}

	// leaf + left + right form one closed lexical-call transaction. Requiring
	// all three owners prevents a call-free-leaf-only implementation from
	// satisfying this regression accidentally.
	if strictStats.RelationPlannerOwnersCompiled != 3 || strictStats.RelationPlannerOwnersActivated != 3 ||
		strictStats.RelationSummaryEquationsOmitted != 6 || strictStats.RelationMaterializationsReused != 6 {
		t.Fatalf("direct-fanout transaction = compiled:%d activated:%d omitted:%d reused:%d, want 3/3/6/6",
			strictStats.RelationPlannerOwnersCompiled, strictStats.RelationPlannerOwnersActivated,
			strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
	}
	if strictStats.RelationContextsSpecialized < 3 {
		t.Fatalf("direct-fanout specialized contexts = %d, want at least one exact context per transaction owner",
			strictStats.RelationContextsSpecialized)
	}
	if strictStats.Body.BodySolves >= legacyStats.Body.BodySolves ||
		strictStats.Body.Transfer.Solver.TransferCalls >= legacyStats.Body.Transfer.Solver.TransferCalls {
		t.Fatalf("direct-fanout work did not fall: legacy=%d solves/%d transfers strict=%d/%d",
			legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls,
			strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
}
