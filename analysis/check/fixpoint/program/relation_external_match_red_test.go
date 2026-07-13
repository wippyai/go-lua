package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestStrictRelationPhaseCollapseOwnsExactBoundaryStringMatch(t *testing.T) {
	const source = `
local function is_reserved_id(id: string): boolean
  return id:match("^__") ~= nil
end
local first = is_reserved_id("__first")
local second = is_reserved_id("ordinary")
local third = is_reserved_id("__third")
local diagnostic_parity: number = "intentional"
return first, second, third, diagnostic_parity
`
	stmts := parseChunk(t, source)
	reg := standard.Registry()
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO,
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte(source)),
		Signatures:    signaturelookup.Source{IncludeStdlib: true},
	}
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
	compareStrictPhaseCollapseParity(t, reg, legacy, strict)
	if strictDiagnosticCount(strict.RootResult()) == 0 {
		t.Fatal("string.match parity fixture produced no diagnostics")
	}
	if strictStats.RelationUnexpectedMisses != 0 || strictStats.RelationActivationFallbacks != 0 {
		t.Fatalf("string.match misses/fallbacks = %d/%d, want 0/0", strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	}
	if strictStats.RelationPlannerOwnersScanned != 2 || strictStats.RelationPlannerOwnersPrefiltered != 1 ||
		strictStats.RelationPlannerOwnersCompiled != 1 || strictStats.RelationPlannerOwnersActivated != 1 ||
		strictStats.RelationContextsSpecialized != 1 || strictStats.RelationSummaryEquationsOmitted != 2 ||
		strictStats.RelationMaterializationsReused != 2 {
		t.Fatalf("string.match transaction = planner:%d/%d/%d/%d contexts/omitted/reused:%d/%d/%d, want 2/1/1/1 and 1/2/2",
			strictStats.RelationPlannerOwnersScanned, strictStats.RelationPlannerOwnersPrefiltered,
			strictStats.RelationPlannerOwnersCompiled, strictStats.RelationPlannerOwnersActivated,
			strictStats.RelationContextsSpecialized, strictStats.RelationSummaryEquationsOmitted,
			strictStats.RelationMaterializationsReused)
	}
	if legacyStats.Body.BodySolves != 9 || legacyStats.Body.Transfer.Solver.TransferCalls != 63 ||
		strictStats.Body.BodySolves != 7 || strictStats.Body.Transfer.Solver.TransferCalls != 53 {
		t.Fatalf("string.match work = legacy=%d/%d strict=%d/%d, want 9/63 -> 7/53",
			legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls,
			strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
}
