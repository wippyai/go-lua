package closed_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// These cases enter the production mounted-artifact receipt assembly and the
// real solver.  The table keeps the semantic witnesses compact while making
// each successor branch observable through the detached result contract.
func TestClosedReceiptSuccessorsRunThroughMountedSolver(t *testing.T) {
	cases := []struct {
		name        string
		source      string
		checkValues bool
	}{
		{
			name:        "source-order-nil-deletion",
			source:      `local child = {}; return { item = child, item = nil }`,
			checkValues: true,
		},
		{
			name:        "diagonal",
			source:      `local a = {}; local x = a; return { [x] = x }`,
			checkValues: true,
		},
		{
			name:        "independent-product",
			source:      `local a = {}; local b = {}; return { [a] = b }`,
			checkValues: true,
		},
		{
			name:   "invalid-key-no-candidate",
			source: `local x = nil; return { [x] = x }`,
		},
		{
			name:        "opaque-containment",
			source:      `local x = {}; return { [x] = x }`,
			checkValues: true,
		},
		{
			name:        "carry-recurrence",
			source:      `local function make() local child = {}; return { item = child } end; return make(), make()`,
			checkValues: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			linked := closedSuccessorLink(t, testCase.source)
			plan, compileStatus := analysis.Compile(linked)
			if compileStatus != analysis.CompileComplete || plan == nil {
				t.Fatalf("receipt compile=%v plan=%t", compileStatus, plan != nil)
			}
			defer plan.Close()
			result, solveStatus := plan.Solve(context.Background())
			if solveStatus != analysis.AnalyzeComplete || result == nil {
				t.Fatalf("mounted solver status=%v result=%t", solveStatus, result != nil)
			}
			body, bodyOK := result.BodyAt(0)
			if !bodyOK {
				t.Fatal("closed selected body unavailable")
			}
			present, _, coordinateCount := closedPublishedValuePresence(t, result, body)
			if testCase.checkValues && coordinateCount == 0 {
				t.Fatal("closed successor published no value coordinates")
			}
			if testCase.checkValues && !present {
				t.Fatal("closed successor published no present value")
			}
		})
	}
}

func closedPublishedValuePresence(t testing.TB, input *analysisresult.Result, selected analysisresult.Body) (somePresent, allPresent bool, coordinateCount int) {
	t.Helper()
	bodyID, bodyIDOK := selected.ID()
	if !bodyIDOK {
		t.Fatal("closed selected body has no identity")
	}
	family, familyOK := input.FamilyByKey(value.SummaryResultFamily)
	if !familyOK {
		t.Fatal("closed value publication family unavailable")
	}
	presence := make(map[identity.ContentID]bool)
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("closed value query[%d] unavailable", queryIndex)
		}
		if query.Status() != analysisresult.QueryHit {
			continue
		}
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			queryBody, queryBodyOK := query.BodyAt(bodyIndex)
			if !queryBodyOK {
				t.Fatalf("closed value query[%d] body[%d] unavailable", queryIndex, bodyIndex)
			}
			queryBodyID, queryBodyIDOK := queryBody.ID()
			if !queryBodyIDOK {
				t.Fatalf("closed value query[%d] body[%d] has no identity", queryIndex, bodyIndex)
			}
			if queryBodyID != bodyID {
				continue
			}
			cell, cellOK := query.Cell()
			summary, summaryOK := value.DecodeSummaryResult(cell.Present(), cell.RowCount(), cell.Payload())
			if !cellOK || !summaryOK {
				t.Fatalf("closed value query[%d] summary unavailable", queryIndex)
			}
			iterator := summary.Coordinates()
			for {
				coordinate, coordinateOK := iterator.Next()
				if !coordinateOK {
					break
				}
				if !coordinate.Available() {
					t.Fatalf("closed value query[%d] coordinate unavailable", queryIndex)
				}
				coordinateID := coordinate.ID()
				if !coordinateID.Available() {
					t.Fatalf("closed value query[%d] coordinate has no identity", queryIndex)
				}
				if coordinate.Present() {
					presence[coordinateID] = true
				} else if _, seen := presence[coordinateID]; !seen {
					presence[coordinateID] = false
				}
			}
		}
	}
	coordinateCount = len(presence)
	allPresent = coordinateCount != 0
	for _, present := range presence {
		somePresent = somePresent || present
		allPresent = allPresent && present
	}
	return somePresent, allPresent, coordinateCount
}

func closedSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "closed_successor.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return linked
}
