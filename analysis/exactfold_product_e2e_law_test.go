package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/value"
)

// TestExactFoldProductExecutesEveryGuardCombination proves the production
// composition executes Value's sealed exact fold family over the complete
// common refinement at fold width two. The two independently guarded
// operands produce four arithmetic cells. A first-row implementation can publish at most one and a
// generic one-read FormExact executor cannot claim this two-read Program, so a
// four-alternative result is evidence from the real exactfold worker rather
// than an installer or payload unit test.
func TestExactFoldProductExecutesEveryGuardCombination(t *testing.T) {
	const source = `
return function(first, second)
    local left
    if first then left = 1 else left = 2 end
    local right
    if second then right = 10 else right = 20 end
    return left + right
end
`
	linked := planLawMountedLink(t, []linkproject.Module{{Name: "main", Program: planLawProgram(t, source)}})
	workspace := NewWorkspace()
	defer workspace.Close()
	plan, status, diagnostics := workspace.CompileWithDiagnostics(linked)
	if plan != nil {
		defer plan.Close()
	}
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.binding == nil {
		t.Fatalf("exact fold product compile = %v/%v diagnostics=%v", status, plan, diagnostics)
	}
	schema := plan.state.binding.ValueSchema()
	if schema == nil || !schema.Valid() {
		t.Fatal("exact fold product has no sealed Value schema")
	}

	var arithmetic value.BinaryArithmetic
	arithmeticCount := 0
	for ordinal := 0; ordinal < schema.EndpointCount(); ordinal++ {
		candidate, ok := schema.BinaryArithmeticAt(ordinal)
		if !ok {
			continue
		}
		arithmetic = candidate
		arithmeticCount++
	}
	if arithmeticCount != 1 {
		t.Fatalf("arithmetic candidate count = %d, want exactly one", arithmeticCount)
	}
	left, leftOK := arithmetic.Left()
	right, rightOK := arithmetic.Right()
	write, writeOK := arithmetic.Write()
	leftID, leftIDOK := exactFoldCoordinateID(schema, left)
	rightID, rightIDOK := exactFoldCoordinateID(schema, right)
	writeID, writeIDOK := exactFoldCoordinateID(schema, write)
	if !leftOK || !rightOK || !writeOK || !leftIDOK || !rightIDOK || !writeIDOK {
		t.Fatal("arithmetic owner did not publish its three canonical coordinates")
	}

	analysisResult, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("exact fold product solve = %v/%v", solveStatus, analysisResult)
	}
	publication, publicationOK := analysisResult.FamilyByKey(value.SummaryResultFamily)
	if !publicationOK {
		t.Fatal("exact fold product has no Value summary publication")
	}

	var leftRow, rightRow, writeRow plane.Row
	var solvedView plane.View
	found := false
	for queryIndex := 0; queryIndex < publication.QueryCount(); queryIndex++ {
		query, queryOK := publication.QueryAt(queryIndex)
		if !queryOK || query.Status() != result.QueryHit {
			continue
		}
		cell, cellOK := query.Cell()
		view, refusal := plane.Admit(summaryLayout(t, workspace), cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || refusal.Available() {
			continue
		}
		leftRow, leftOK = view.Lookup(leftID)
		rightRow, rightOK = view.Lookup(rightID)
		writeRow, writeOK = view.Lookup(writeID)
		if leftOK && rightOK && writeOK && leftRow.Written() && rightRow.Written() && writeRow.Written() {
			solvedView = view
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no solved Value summary contains the exact fold operands and result")
	}
	if leftRow.Flag(value.SummaryColumnTop) || rightRow.Flag(value.SummaryColumnTop) || writeRow.Flag(value.SummaryColumnTop) {
		t.Fatal("exact fold product widened a finite guarded operand or result to Top")
	}
	stride := 0
	sources := linked.Boundary().Values()
	for index := 0; index < sources.Count(); index++ {
		source, sourceOK := sources.At(index)
		sourceID, sourceIDOK := sources.ID(source)
		fact, factOK := schema.SourceValueID(sourceID)
		row, rowOK := solvedView.Lookup(sourceID)
		if sourceOK && sourceIDOK && factOK && schema.ValueAtomCount(fact) == 1 && rowOK && row.Written() && !row.Flag(value.SummaryColumnTop) && row.Count() > 0 {
			stride = row.Count()
			break
		}
	}
	if stride == 0 {
		t.Fatal("exact fold result has no published one-atom source from which to derive Value stride")
	}
	if leftRow.Count() != 2*stride || rightRow.Count() != 2*stride || writeRow.Count() != 4*stride {
		t.Fatalf("guarded exact product words left=%d right=%d result=%d stride=%d, want 2S/2S/4S", leftRow.Count(), rightRow.Count(), writeRow.Count(), stride)
	}
}

func exactFoldCoordinateID(schema *value.Schema, coordinate value.Coordinate) (identity.ContentID, bool) {
	dense, ok := schema.CoordinateIndex(coordinate)
	if !ok {
		return identity.ContentID{}, false
	}
	for position := 0; position < schema.CoordinateCount(); position++ {
		id, candidateDense, resolved := schema.CanonicalCoordinateAt(position)
		if resolved && candidateDense == dense {
			return id, id.Available()
		}
	}
	return identity.ContentID{}, false
}
