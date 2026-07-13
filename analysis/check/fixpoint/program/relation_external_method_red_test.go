package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestStrictRelationPhaseCollapseOwnsSealedExternalStringMethod(t *testing.T) {
	stmts := parseChunk(t, `
local function trim(value: any): string
  if type(value) ~= "string" then return "" end
  return (value:gsub("^%s*(.-)%s*$", "%1"))
end
local a = trim("  first  ")
local b = trim(false)
local c = trim("  second  ")
local diagnostic_parity: number = "intentional"
return a, b, c, diagnostic_parity
`)
	reg := standard.Registry()
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO,
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("strict-external-string-method")),
		Signatures:    signaturelookup.Source{IncludeStdlib: true},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(check)})
	trim := findLocalFunctionByName(t, bindings, "trim")
	prepared, err := body.PrepareBoundFunction(trim, bindings, check)
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	surface, ok := plan.CallSurface()
	if !ok || !surface.Complete() {
		t.Fatalf("trim call surface = %#v/%v, want complete", surface, ok)
	}
	methods := 0
	for _, site := range surface.Sites() {
		call, represented := plan.Facts().CallSiteView(site.Point)
		if !represented || call.MethodName() != "gsub" {
			continue
		}
		methods++
		op, exact := plan.SignatureCallOperation(site.Point)
		if site.Target.Kind() != operationplan.CallSurfaceTargetExternal || !exact || !site.Target.MatchesExternalOperation(op) {
			t.Fatalf("gsub point %d is not one exact sealed external operation", site.Point)
		}
	}
	if methods != 1 {
		t.Fatalf("sealed gsub method calls = %d, want 1", methods)
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
	if strictDiagnosticCount(strict.RootResult()) == 0 {
		t.Fatal("external-method parity fixture produced no diagnostics")
	}
	if strictStats.RelationUnexpectedMisses != 0 || strictStats.RelationActivationFallbacks != 0 {
		t.Fatalf("external-method misses/fallbacks = %d/%d, want 0/0", strictStats.RelationUnexpectedMisses, strictStats.RelationActivationFallbacks)
	}
	if strictStats.RelationPlannerOwnersActivated == 0 || strictStats.RelationSummaryEquationsOmitted == 0 || strictStats.RelationMaterializationsReused == 0 {
		t.Fatalf("external-method transaction = activated:%d omitted:%d reused:%d, want nonzero", strictStats.RelationPlannerOwnersActivated, strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
	}
	if strictStats.Body.BodySolves >= legacyStats.Body.BodySolves || strictStats.Body.Transfer.Solver.TransferCalls >= legacyStats.Body.Transfer.Solver.TransferCalls {
		t.Fatalf("external-method work did not fall: legacy=%d/%d strict=%d/%d", legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls, strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls)
	}
	t.Logf("external-method work legacy=%d/%d strict=%d/%d compiled/activated/contexts/omitted/reused=%d/%d/%d/%d/%d", legacyStats.Body.BodySolves, legacyStats.Body.Transfer.Solver.TransferCalls, strictStats.Body.BodySolves, strictStats.Body.Transfer.Solver.TransferCalls, strictStats.RelationPlannerOwnersCompiled, strictStats.RelationPlannerOwnersActivated, strictStats.RelationContextsSpecialized, strictStats.RelationSummaryEquationsOmitted, strictStats.RelationMaterializationsReused)
}
