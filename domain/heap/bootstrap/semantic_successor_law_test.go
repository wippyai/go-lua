package bootstrap_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	valuepublication "github.com/wippyai/go-lua/domain/value/publication"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Link bootstrap is validated only after the complete value (header,
// mutability, and raw absence/presence) has crossed receipt assembly and the
// solver has published a detached result.
func TestBootstrapReceiptStagesCompleteValueThroughMountedSolver(t *testing.T) {
	linked := bootstrapSuccessorLink(t, `local missing = nil; local number = 1; return number`)
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
		t.Fatal("bootstrap selected body unavailable")
	}
	present, _, coordinateCount := bootstrapPublishedValuePresence(t, result, body)
	if coordinateCount == 0 {
		t.Fatal("bootstrap published no value coordinates")
	}
	if !present {
		t.Fatal("bootstrap did not publish any present detached value")
	}
}

func bootstrapPublishedValuePresence(t testing.TB, input *analysisresult.Result, selected analysisresult.Body) (somePresent, allPresent bool, coordinateCount int) {
	t.Helper()
	bodyID, bodyIDOK := selected.ID()
	if !bodyIDOK {
		t.Fatal("bootstrap selected body has no identity")
	}
	family, familyOK := valuepublication.Open(input)
	if !familyOK {
		t.Fatal("bootstrap value publication family unavailable")
	}
	presence := make(map[identity.ContentID]bool)
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("bootstrap value query[%d] unavailable", queryIndex)
		}
		if query.Status() != analysisresult.QueryHit {
			continue
		}
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			queryBody, queryBodyOK := query.BodyAt(bodyIndex)
			if !queryBodyOK {
				t.Fatalf("bootstrap value query[%d] body[%d] unavailable", queryIndex, bodyIndex)
			}
			queryBodyID, queryBodyIDOK := queryBody.ID()
			if !queryBodyIDOK {
				t.Fatalf("bootstrap value query[%d] body[%d] has no identity", queryIndex, bodyIndex)
			}
			if queryBodyID != bodyID {
				continue
			}
			summary, summaryOK := query.Summary()
			if !summaryOK {
				t.Fatalf("bootstrap value query[%d] summary unavailable", queryIndex)
			}
			iterator := summary.Coordinates()
			for {
				coordinate, coordinateOK := iterator.Next()
				if !coordinateOK {
					break
				}
				if !coordinate.Available() {
					t.Fatalf("bootstrap value query[%d] coordinate unavailable", queryIndex)
				}
				coordinateID := coordinate.ID()
				if !coordinateID.Available() {
					t.Fatalf("bootstrap value query[%d] coordinate has no identity", queryIndex)
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

func bootstrapSuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "bootstrap_successor.lua", Text: []byte(text)})
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
