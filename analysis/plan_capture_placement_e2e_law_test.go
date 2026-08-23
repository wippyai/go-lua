package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/result"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// TestCompiledPlanClosureCapturePublishesOwnedRoots exercises the canonical
// single-module Plan path. Returning a closure that captures a table must
// retain both allocation roots in the typed Placement publication; neither
// root may be widened to Unknown or left at its frame-local Stack class.
func TestCompiledPlanClosureCapturePublishesOwnedRoots(t *testing.T) {
	contract := fixtureContract(t)
	linked := fixtureSourceLink(t, contract, "closure-capture-e2e.lua", []byte(`
local function make()
    local captured = { value = 1 }
    local function read()
        return captured
    end
    return read
end
return make()
`))
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile closure-capture = %v plan=%t diagnostics=%+v", status, plan != nil, diagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close closure-capture Plan")
		}
	})
	captureReceipts := 0
	for _, mount := range plan.state.artifacts.mounts {
		program := mount.Program.Program
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("closure-capture artifact omitted the RuleOccurrence family")
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			if !rowOK || row.Key() != "placement-closure-capture" {
				continue
			}
			input, inputOK := row.InputPoint()
			native, nativeOK := row.Native()
			if row.Stage() != programissuance.StageSuccessor || row.InputSpec() != programissuance.InputLocalStage ||
				!inputOK || !input.Available() || !nativeOK || native {
				t.Fatalf("closure-capture receipt lost its issued successor geometry: stage=%s input-spec=%s input=%s/%t native=%t/%t", row.Stage(), row.InputSpec(), input, inputOK, native, nativeOK)
			}
			captureReceipts++
		}
	}
	if captureReceipts != 1 {
		t.Fatalf("closure-capture artifact receipts = %d, want one", captureReceipts)
	}
	analysisResult, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("solve closure-capture = %v result=%t diagnostics=%+v", solveStatus, analysisResult != nil, solveDiagnostics)
	}
	schema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !schema.Valid() {
		t.Fatal("closure-capture Placement schema unavailable")
	}
	publication, publicationOK := placementpublication.Open(analysisResult)
	if !publicationOK || publication.QueryCount() == 0 {
		t.Fatal("closure-capture typed Placement publication unavailable")
	}
	wantFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	found := map[string]bool{}
	closureOwned, tableOwned := false, false
	for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
		query, queryOK := publication.QueryAt(queryIndex)
		if !queryOK || query.Status() != result.QueryHit {
			continue
		}
		summary, summaryOK := query.Placement(schema)
		if !summaryOK {
			t.Fatalf("closure-capture query %d has no typed Placement summary", queryIndex)
		}
		rows := summary.Allocations()
		for {
			row, rowOK := rows.Next()
			if !rowOK {
				break
			}
			kind, kindOK := row.Kind()
			fact, factOK := row.Fact()
			if !kindOK || !factOK {
				t.Fatalf("closure-capture query %d has incomplete row %s", queryIndex, row.AllocationID())
			}
			_, relevant := map[string]bool{"lua.table": true, "lua.closure": true}[kind.String()]
			if !relevant {
				continue
			}
			found[kind.String()] = true
			if kind.String() == "lua.closure" && fact == wantFact {
				closureOwned = true
			}
			if kind.String() == "lua.table" && fact == wantFact {
				tableOwned = true
			}
		}
	}
	for _, kind := range []string{"lua.table", "lua.closure"} {
		if !found[kind] {
			t.Fatalf("closure-capture publication omitted %s root", kind)
		}
	}
	if !tableOwned || !closureOwned {
		t.Fatalf("closure-capture classes table-owned-heap=%t closure-owned-heap=%t diagnostics=%+v", tableOwned, closureOwned, solveDiagnostics)
	}
}

// TestCompiledPlanNoCaptureControlDoesNotInventClosureCaptureRoots keeps the
// same single-module route free of closure-capture evidence.
func TestCompiledPlanNoCaptureControlDoesNotInventClosureCaptureRoots(t *testing.T) {
	contract := fixtureContract(t)
	linked := fixtureSourceLink(t, contract, "no-capture-e2e.lua", []byte(`
local value = { value = 1 }
return value
`))
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile no-capture = %v plan=%t diagnostics=%+v", status, plan != nil, diagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close no-capture Plan")
		}
	})
	analysisResult, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("solve no-capture = %v result=%t diagnostics=%+v", solveStatus, analysisResult != nil, solveDiagnostics)
	}
	schema, schemaOK := plan.PlacementSchema()
	if !schemaOK {
		t.Fatal("no-capture Placement schema unavailable")
	}
	publication, publicationOK := placementpublication.Open(analysisResult)
	if !publicationOK || publication.QueryCount() == 0 {
		t.Fatal("no-capture typed Placement publication unavailable")
	}
	for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
		query, queryOK := publication.QueryAt(queryIndex)
		if !queryOK || query.Status() != result.QueryHit {
			continue
		}
		summary, summaryOK := query.Placement(schema)
		if !summaryOK {
			t.Fatalf("no-capture query %d has no typed Placement summary", queryIndex)
		}
		rows := summary.Allocations()
		for {
			row, rowOK := rows.Next()
			if !rowOK {
				break
			}
			kind, kindOK := row.Kind()
			if kindOK && kind.String() == "lua.closure" {
				t.Fatalf("no-capture publication invented closure root %s", row.AllocationID())
			}
		}
	}
}
