package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/value"
)

// A multi-value call bound to several locals reserves one Value coordinate per
// mounted finite call-result tail slot. Those coordinates belong to the sealed
// coordinate space the value-summary family folds over, so the family's owner
// encoder must name and publish them: an answer it cannot encode zeroes every
// verdict in the program through the fail-closed result projection.
func TestCompiledPlanMultiValueCallPublishesTypedSummary(t *testing.T) {
	const source = "local first, second = string.find(\"abc\", \"b\")\n" +
		"return first\n"

	linked := planLawMountedLink(t, []linkproject.Module{{Name: "main", Program: planLawProgram(t, source)}})
	workspace := NewWorkspace()
	analysisResult, status := workspace.Analyze(context.Background(), linked)
	if status != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("multi-value call solve = %v/%v", status, analysisResult)
	}

	values := linked.Boundary().Values()
	canonical := make([]identity.ContentID, values.Count())
	for index := range canonical {
		row, rowOK := values.At(index)
		id, idOK := values.ID(row)
		if !rowOK || !idOK || !id.Available() {
			t.Fatalf("canonical ingress Value %d", index)
		}
		canonical[index] = id
	}

	publication, publicationOK := analysisResult.FamilyByKey(value.SummaryResultFamily)
	if !publicationOK {
		t.Fatal("multi-value call program has no typed value publication")
	}
	seen := make(map[identity.ContentID]bool, len(canonical))
	coordinateCount := 0
	hits := 0
	for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
		query, queryOK := publication.QueryAt(queryIndex)
		if !queryOK || query.Status() != result.QueryHit {
			continue
		}
		cell, cellOK := query.Cell()
		view, refusal := plane.Admit(summaryLayout(t, workspace), cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || refusal.Available() {
			t.Fatalf("value query %d hit has no typed summary: %s", queryIndex, refusal)
		}
		hits++
		coordinateCount = view.RowCount()
		for index := 0; index < view.RowCount(); index++ {
			row, rowOK := view.At(index)
			if !rowOK || !row.ID().Available() {
				t.Fatalf("value query %d summary coordinate has no identity", queryIndex)
			}
			seen[row.ID()] = true
		}
	}
	if hits == 0 {
		t.Fatal("multi-value call program publishes no hit value summary")
	}
	if coordinateCount <= len(canonical) {
		t.Fatalf("published summary covers %d coordinates, boundary declares %d; the fixture reserves no call-result tail coordinate and does not exercise the law",
			coordinateCount, len(canonical))
	}
	for _, id := range canonical {
		if !seen[id] {
			t.Fatalf("canonical ingress Value %s is absent from the typed Result summary", id)
		}
	}
}

// summaryLayout is the sealed layout this workspace published its value
// summaries under. A law opens a published cell against the compilation's own
// declaration rather than a copy of it kept beside the reader.
func summaryLayout(t testing.TB, workspace *Workspace) *plane.Sealed {
	t.Helper()
	layout, layoutOK := composite.QueryResultLayout(workspace.Compilation(), value.SummaryResultFamily)
	if !layoutOK || !layout.Available() {
		t.Fatal("the compilation sealed no value-summary layout")
	}
	return layout
}
