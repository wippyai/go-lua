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

func TestStrictRelationPhaseCollapseOwnsImmutablePrimitiveCaptureDirectCaller(t *testing.T) {
	stmts := parseChunk(t, `
local suffix = "!"
local function leaf(value: boolean)
  if value then
    return "yes"
  end
  return "no"
end
local function append_suffix(value: boolean): string
  local result = leaf(value)
  return result .. suffix
end
local first = append_suffix(true)
local second = append_suffix(false)
local diagnostic_parity: number = "intentional"
return first, second, diagnostic_parity
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	check := body.Config{
		Registry:      reg,
		TypeValues:    typevalue.NewCache(),
		Schedule:      transfer.ScheduleWTO,
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("strict-capture-direct")),
	}
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
	if strictStats.RelationUnexpectedMisses != 0 || strictStats.RelationActivationFallbacks != 0 {
		t.Fatalf("capture-direct misses/fallbacks = %d/%d, want 0/0", strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	}
	if strictDiagnosticCount(strict.RootResult()) == 0 {
		t.Fatal("capture-direct parity fixture produced no diagnostics")
	}
	if strictStats.RelationPlannerOwnersScanned != 3 || strictStats.RelationPlannerOwnersPrefiltered != 2 ||
		strictStats.RelationPlannerOwnersCompiled != 2 || strictStats.RelationPlannerOwnersActivated != 2 ||
		strictStats.RelationContextsSpecialized != 2 || strictStats.RelationSummaryEquationsOmitted != 4 ||
		strictStats.RelationMaterializationsReused != 4 {
		t.Fatalf("capture-direct transaction = scanned:%d prefiltered:%d compiled:%d activated:%d contexts:%d omitted:%d reused:%d, want 3/2/2/2/2/4/4", strictStats.RelationPlannerOwnersScanned, strictStats.RelationPlannerOwnersPrefiltered, strictStats.RelationPlannerOwnersCompiled, strictStats.RelationPlannerOwnersActivated, strictStats.RelationContextsSpecialized, strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
	}
	if strictStats.Body.BodySolves >= legacyStats.Body.BodySolves ||
		strictStats.Body.Transfer.Solver.TransferCalls >= legacyStats.Body.Transfer.Solver.TransferCalls {
		t.Fatalf("capture-direct work did not fall: legacy=%d/%d strict=%d/%d", legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls, strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
	if legacyStats.Body.BodySolves != 13 || legacyStats.Body.Transfer.Solver.TransferCalls != 101 ||
		strictStats.Body.BodySolves != 9 || strictStats.Body.Transfer.Solver.TransferCalls != 73 {
		t.Fatalf("capture-direct measured shape = legacy:%d/%d strict:%d/%d, want 13/101 -> 9/73", legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls, strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
	t.Logf("capture-direct work legacy=%d/%d strict=%d/%d activated/contexts/omitted/reused=%d/%d/%d/%d",
		legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls,
		strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls,
		strictStats.RelationPlannerOwnersActivated, strictStats.RelationContextsSpecialized,
		strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
}

func TestStrictRelationPhaseCollapseRejectsUnsafeCaptureDirectCallers(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{
			name: "mutable capture",
			source: `
local suffix = "!"
local function leaf(value: boolean)
  if value then return "yes" end
  return "no"
end
local function append_suffix(value: boolean): string
  local result = leaf(value)
  return result .. suffix
end
suffix = "?"
local first = append_suffix(true)
local second = append_suffix(false)
local diagnostic_parity: number = "intentional"
return first, second, diagnostic_parity
`,
		},
		{
			name: "nonprimitive capture",
			source: `
local suffix = { value = "!" }
local function leaf(value: boolean)
  if value then return "yes" end
  return "no"
end
local function append_suffix(value: boolean): string
  local result = leaf(value)
  return result .. suffix.value
end
local first = append_suffix(true)
local second = append_suffix(false)
local diagnostic_parity: number = "intentional"
return first, second, diagnostic_parity
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stmts := parseChunk(t, test.source)
			bindings := bind.BindChunk(stmts, bind.Options{})
			reg := standard.Registry()
			check := body.Config{
				Registry:      reg,
				TypeValues:    typevalue.NewCache(),
				Schedule:      transfer.ScheduleWTO,
				UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("strict-capture-direct-negative-" + test.name)),
			}
			legacy, err := RunBoundChunk(stmts, bindings, Config{Check: check, forceLegacyRelations: true})
			if err != nil {
				t.Fatal(err)
			}
			stats := &Stats{}
			strict, err := RunBoundChunk(stmts, bindings, Config{Check: check, Stats: stats, enableStrictRelationPhaseCollapse: true})
			if err != nil {
				t.Fatal(err)
			}
			compareStrictPhaseCollapseParity(t, reg, legacy, strict)
			if stats.RelationUnexpectedMisses != 0 || stats.RelationActivationFallbacks != 0 {
				t.Fatalf("unsafe capture misses/fallbacks = %d/%d, want 0/0", stats.RelationUnexpectedMisses, stats.RelationActivationFallbacks)
			}
			// The independently safe params-only leaf remains eligible. The
			// capture-bearing caller must not join the strict transaction.
			if stats.RelationPlannerOwnersActivated != 1 || stats.RelationContextsSpecialized != 1 ||
				stats.RelationSummaryEquationsOmitted != 2 || stats.RelationMaterializationsReused != 2 {
				t.Fatalf("unsafe capture transaction = activated:%d contexts:%d omitted:%d reused:%d, want leaf-only 1/1/2/2", stats.RelationPlannerOwnersActivated, stats.RelationContextsSpecialized, stats.RelationSummaryEquationsOmitted, stats.RelationMaterializationsReused)
			}
		})
	}
}
