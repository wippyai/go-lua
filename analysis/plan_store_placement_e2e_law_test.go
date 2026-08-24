package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
)

func TestCompiledPlanStorageEscapePublishesOwnedHeapAndFrameControl(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   placementdomain.Fact
	}{
		{
			// Retention must be proved by the storage owner. Merely belonging to
			// the entry body does not make a local module-owned: an uncaptured
			// chunk local dies with that activation. The closure relation is the
			// canonical proof that this cell outlives its introducing frame.
			name: "closure-cell",
			source: `
local retained = { value = 1 }
local function retain()
  return retained
end
return retain
`,
			want: placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven},
		},
		{
			name: "frame-cell",
			source: `
local function localOnly()
  local value = { value = 1 }
  return true
end
localOnly()
return true
`,
			want: placementdomain.Fact{Class: placementdomain.Stack, RetainEscape: placementdomain.EvidenceRefuted},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			contract := fixtureContract(t)
			linked := fixtureSourceLink(t, contract, test.name+"-storage.lua", []byte(test.source))
			plan, status, diagnostics := CompileWithDiagnostics(linked)
			if status != CompileComplete || plan == nil {
				t.Fatalf("compile storage %s = %v plan=%t diagnostics=%+v", test.name, status, plan != nil, diagnostics)
			}
			t.Cleanup(func() {
				if !plan.Close() {
					t.Error("close storage Plan")
				}
			})
			result, solveStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
			if solveStatus != AnalyzeComplete || result == nil {
				refusal, stage, rule, runtimeOK := plan.state.instantiateRuntimeTopology()
				row, rowOK := refusal.ConstructionRow()
				t.Fatalf("solve storage %s = %v result=%t diagnostics=%+v runtime=%t stage=%v rule=%v refusal=%v row=%v/%t", test.name, solveStatus, result != nil, solveDiagnostics, runtimeOK, stage, rule, refusal, row, rowOK)
			}
			schema, schemaOK := plan.PlacementSchema()
			if !schemaOK || !schema.Valid() {
				t.Fatal("storage Placement schema unavailable")
			}
			tableID, tableIDOK := storageTableRootID(schema)
			if !tableIDOK {
				t.Fatal("storage table root unavailable")
			}
			publication, publicationOK := placementpublication.Open(result)
			if !publicationOK || publication.QueryCount() == 0 {
				t.Fatal("storage typed Placement publication unavailable")
			}
			found := false
			for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
				query, queryOK := publication.QueryAt(queryIndex)
				if !queryOK || query.Status() != analysisresult.QueryHit {
					continue
				}
				summary, summaryOK := query.Placement(schema)
				if !summaryOK {
					t.Fatalf("storage query %d has no typed Placement summary", queryIndex)
				}
				rows := summary.Allocations()
				for {
					row, rowOK := rows.Next()
					if !rowOK {
						break
					}
					kind, kindOK := row.Kind()
					fact, factOK := row.Fact()
					if kindOK && factOK && kind == placementdomain.AllocationKindTable && row.AllocationID() == tableID && fact == test.want {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("storage %s never published authored table as %s", test.name, test.want)
			}
		})
	}
}

func storageTableRootID(schema placementdomain.Schema) (id identity.ContentID, ok bool) {
	if !schema.Valid() {
		return id, false
	}
	found := 0
	for index := 0; index < schema.DenseKeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK || key.Kind() != heap.RootAllocation {
			continue
		}
		_, _, _, kind, _, originOK := schema.Heap().AllocationOriginForKey(key)
		if !originOK || kind != heap.AllocationTable {
			continue
		}
		candidate, candidateOK := schema.Heap().KeyID(key)
		if !candidateOK || !candidate.Available() {
			return id, false
		}
		id = candidate
		found++
	}
	return id, found == 1
}
