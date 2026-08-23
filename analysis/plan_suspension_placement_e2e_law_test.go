package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

// TestCompiledPlanCoroutineYieldCaptureDisplacesTheExactTableRoot exercises
// the canonical coroutine activation path. The callback is wrapped and
// invoked, and actually reaches coroutine.yield before using its table again.
// The assertion follows that exact table identity through the typed Placement
// publication; it does not inspect transfer facts or process.send results.
func TestCompiledPlanCoroutineYieldCaptureDisplacesTheExactTableRoot(t *testing.T) {
	contract := fixtureContract(t)
	linked := fixtureSourceLink(t, contract, "coroutine-yield-capture-e2e.lua", []byte(`
local function run()
    local captured = { value = 1 }
    coroutine.yield()
    return captured.value
end
local wrapped = coroutine.wrap(run)
wrapped()
return wrapped
`))
	plan, compileStatus, compileDiagnostics := CompileWithDiagnostics(linked)
	if compileStatus != CompileComplete || plan == nil {
		t.Fatalf("compile coroutine-yield capture = %v plan=%t diagnostics=%+v", compileStatus, plan != nil, compileDiagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close coroutine-yield capture Plan")
		}
	})

	analysisResult, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("solve coroutine-yield capture = %v result=%t diagnostics=%+v", solveStatus, analysisResult != nil, solveDiagnostics)
	}

	schema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !schema.Valid() {
		t.Fatal("coroutine-yield capture Placement schema unavailable")
	}
	tableID := exactTableRootID(t, schema)
	family, familyOK := placementpublication.Open(analysisResult)
	if !familyOK || family.QueryCount() == 0 {
		t.Fatalf("coroutine-yield capture typed Placement publication = %t/%d, want query rows", familyOK, family.QueryCount())
	}

	hits := 0
	displaced := false
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("coroutine-yield capture query %d is not addressable", queryIndex)
		}
		switch query.Status() {
		case analysisresult.QueryProvenAbsent:
			continue
		case analysisresult.QueryHit:
		default:
			t.Fatalf("coroutine-yield capture query %d has invalid status %v", queryIndex, query.Status())
		}
		summary, summaryOK := query.Placement(schema)
		if !summaryOK || !summary.Available() {
			t.Fatalf("coroutine-yield capture query %d has no typed Placement summary", queryIndex)
		}
		allocation, allocationOK := summary.Allocation(tableID)
		if !allocationOK || !allocation.Available() || allocation.AllocationID() != tableID {
			t.Fatalf("coroutine-yield capture query %d omitted exact table allocation %s", queryIndex, tableID)
		}
		fact, factOK := allocation.Fact()
		wantFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceRefuted}
		if !factOK || fact != wantFact {
			t.Fatalf("coroutine-yield capture query %d exact table allocation %s fact = %v/%t, want %v", queryIndex, tableID, fact, factOK, wantFact)
		}
		hits++
		if fact.Class == placementdomain.OwnedHeap {
			displaced = true
		}
	}
	if hits == 0 {
		t.Fatal("coroutine-yield capture published no Placement query hit for exact table allocation")
	}
	if !displaced {
		t.Fatalf("coroutine-yield capture exact table allocation %s was never displaced to OwnedHeap", tableID)
	}
}

// TestCompiledPlanCoroutineNoSuspensionLeavesTheExactTableRootOnStack keeps
// the same callback wrapping/invocation shape but removes the yield. It is the
// adversarial control: no suspension boundary is activated, so the exact
// table root remains at its Stack baseline in every published hit.
func TestCompiledPlanCoroutineNoSuspensionLeavesTheExactTableRootOnStack(t *testing.T) {
	contract := fixtureContract(t)
	linked := fixtureSourceLink(t, contract, "coroutine-no-suspension-e2e.lua", []byte(`
local function run()
    local captured = { value = 1 }
    return captured.value
end
local wrapped = coroutine.wrap(run)
wrapped()
return wrapped
`))
	plan, compileStatus, compileDiagnostics := CompileWithDiagnostics(linked)
	if compileStatus != CompileComplete || plan == nil {
		t.Fatalf("compile coroutine no-suspension = %v plan=%t diagnostics=%+v", compileStatus, plan != nil, compileDiagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close coroutine no-suspension Plan")
		}
	})

	analysisResult, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("solve coroutine no-suspension = %v result=%t diagnostics=%+v", solveStatus, analysisResult != nil, solveDiagnostics)
	}

	schema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !schema.Valid() {
		t.Fatal("coroutine no-suspension Placement schema unavailable")
	}
	tableID := exactTableRootID(t, schema)
	family, familyOK := placementpublication.Open(analysisResult)
	if !familyOK || family.QueryCount() == 0 {
		t.Fatalf("coroutine no-suspension typed Placement publication = %t/%d, want query rows", familyOK, family.QueryCount())
	}

	hits := 0
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("coroutine no-suspension query %d is not addressable", queryIndex)
		}
		switch query.Status() {
		case analysisresult.QueryProvenAbsent:
			continue
		case analysisresult.QueryHit:
		default:
			t.Fatalf("coroutine no-suspension query %d has invalid status %v", queryIndex, query.Status())
		}
		summary, summaryOK := query.Placement(schema)
		if !summaryOK || !summary.Available() {
			t.Fatalf("coroutine no-suspension query %d has no typed Placement summary", queryIndex)
		}
		allocation, allocationOK := summary.Allocation(tableID)
		if !allocationOK || !allocation.Available() || allocation.AllocationID() != tableID {
			t.Fatalf("coroutine no-suspension query %d omitted exact table allocation %s", queryIndex, tableID)
		}
		fact, factOK := allocation.Fact()
		wantFact := placementdomain.Fact{Class: placementdomain.Stack, RetainEscape: placementdomain.EvidenceRefuted}
		if !factOK || fact != wantFact {
			t.Fatalf("coroutine no-suspension query %d exact table allocation %s fact = %v/%t, want %v", queryIndex, tableID, fact, factOK, wantFact)
		}
		hits++
	}
	if hits == 0 {
		t.Fatal("coroutine no-suspension published no Placement query hit for exact table allocation")
	}
}

func exactTableRootID(t testing.TB, schema placementdomain.Schema) identity.ContentID {
	t.Helper()
	if !schema.Valid() || !schema.Heap().Valid() {
		t.Fatal("Placement schema has no valid Heap authority")
	}

	var tableID identity.ContentID
	tableRoots := 0
	for index := 0; index < schema.DenseKeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			t.Fatalf("Placement schema key %d is not addressable", index)
		}
		_, _, allocationID, kind, _, originOK := schema.Heap().AllocationOriginForKey(key)
		keyID, keyIDOK := key.ContentID()
		if !originOK || !allocationID.Available() || !keyIDOK || !keyID.Available() || kind != heapdomain.AllocationTable {
			continue
		}
		tableRoots++
		tableID = keyID
	}
	if tableRoots != 1 || !tableID.Available() {
		t.Fatalf("Placement schema table roots = %d/%s, want exactly one authenticated table root", tableRoots, tableID)
	}
	return tableID
}
