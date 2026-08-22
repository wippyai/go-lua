package empty_test

import (
	"context"
	"testing"

	analysis "github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	analysisresult "github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Empty self-create is exercised through the mounted receipt assembly. The
// detached boundary exposes a typed Value publication, while the internal
// ineligible/eligible Heap distinction remains outside this Result contract.
func TestEmptyReceiptSelfCreatePublishesReadableValueFamilyThroughMountedSolver(t *testing.T) {
	linked := emptySuccessorLink(t, `local function make() return {} end; local closure = function() end; return make(), make(), closure`)
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
		t.Fatal("empty self-create selected body unavailable")
	}
	assertEmptyValuePublicationReadable(t, summaryLayout(t, plan), result, body)
}

func assertEmptyValuePublicationReadable(t testing.TB, layout *plane.Sealed, input *analysisresult.Result, selected analysisresult.Body) {
	t.Helper()
	bodyID, bodyIDOK := selected.ID()
	if !bodyIDOK {
		t.Fatal("empty self-create selected body has no identity")
	}
	family, familyOK := input.FamilyByKey(value.SummaryResultFamily)
	if !familyOK {
		t.Fatal("empty self-create value publication family unavailable")
	}
	if family.QueryCount() == 0 {
		t.Fatal("empty self-create value publication has no queries")
	}
	selectedBodyReferences := 0
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("empty self-create value query[%d] unavailable", queryIndex)
		}
		status := query.Status()
		if status != analysisresult.QueryHit && status != analysisresult.QueryProvenAbsent {
			t.Fatalf("empty self-create value query[%d] status=%d", queryIndex, status)
		}
		matched := false
		for bodyIndex := 0; bodyIndex < query.BodyCount(); bodyIndex++ {
			queryBody, queryBodyOK := query.BodyAt(bodyIndex)
			if !queryBodyOK {
				t.Fatalf("empty self-create value query[%d] body[%d] unavailable", queryIndex, bodyIndex)
			}
			queryBodyID, queryBodyIDOK := queryBody.ID()
			if !queryBodyIDOK {
				t.Fatalf("empty self-create value query[%d] body[%d] has no identity", queryIndex, bodyIndex)
			}
			if queryBodyID == bodyID {
				matched = true
			}
		}
		if matched {
			selectedBodyReferences++
		}
		if status != analysisresult.QueryHit {
			continue
		}
		cell, cellOK := query.Cell()
		view, refusal := plane.Admit(layout, cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || refusal.Available() || !view.Owner().Available() || view.RowCount() == 0 {
			t.Fatalf("empty self-create value query[%d] summary unavailable or incomplete: %s", queryIndex, refusal)
		}
		coordinateCount := 0
		for index := 0; index < view.RowCount(); index++ {
			row, rowOK := view.At(index)
			if !rowOK || !row.ID().Available() {
				t.Fatalf("empty self-create value query[%d] coordinate unavailable or unidentified", queryIndex)
			}
			coordinateCount++
		}
		if coordinateCount != view.RowCount() {
			t.Fatalf("empty self-create value query[%d] coordinates=%d summary=%d", queryIndex, coordinateCount, view.RowCount())
		}
	}
	if selectedBodyReferences == 0 {
		t.Fatalf("empty self-create value publication does not reference selected body %s", bodyID.String())
	}
}

func emptySuccessorLink(t testing.TB, text string) *link.Link {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "empty_successor.lua", Text: []byte(text)})
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

// summaryLayout is the sealed layout the plan published its value summaries
// under. A law opens a published cell against the compilation's own
// declaration rather than a copy of it kept beside the reader.
func summaryLayout(t testing.TB, plan *analysis.Plan) *plane.Sealed {
	t.Helper()
	layout, layoutOK := plan.QueryResultLayout(value.SummaryResultFamily)
	if !layoutOK || !layout.Available() {
		t.Fatal("the plan sealed no value-summary layout")
	}
	return layout
}
