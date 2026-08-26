package merge_test

import (
	"testing"

	physicalmerge "github.com/wippyai/go-lua/analysis/engine/relation/operator/merge"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

func inputSourceAtRoot(t testing.TB, fixture testfixture.Fixture, root database.Version) tuple.Batch {
	t.Helper()
	value, ok := fixture.LeftInputBatch(t, root)
	if !ok {
		t.Fatal("sealed left input source")
	}
	return value
}

func emptyBatch(t testing.TB, fixture testfixture.Fixture, source tuple.Batch) tuple.Batch {
	t.Helper()
	value, ok := tuple.PreserveRange(fixture.Mounted(), source, source.Scope(), []tuple.Tuple{})
	if !ok || !value.ValidFor(fixture.Mounted()) {
		t.Fatal("empty preserved input range")
	}
	return value
}

func oracleBatch(t testing.TB, relation model.RelationID, batch tuple.Batch) relationoracle.Relation {
	t.Helper()
	relationContent := relation.Content()
	scopeID, ok := identity.DeriveContentID("analysis/engine/relation/operator/merge/law/scope/v1", relationContent[:])
	if !ok {
		t.Fatal("oracle scope identity")
	}
	scope, ok := relationoracle.NewScope(scopeID)
	if !ok {
		t.Fatal("oracle scope")
	}
	rows := make([]relationoracle.Row, 0, batch.Len())
	for index := 0; index < batch.Len(); index++ {
		value, valueOK := batch.At(index)
		if !valueOK || value.SourceLen() != 1 {
			t.Fatalf("oracle source tuple %d", index)
		}
		rowID, sourceOK := value.SourceAt(0)
		if !sourceOK || rowID.Relation() != relation {
			t.Fatalf("oracle source row %d", index)
		}
		cells := make([]relationoracle.Cell, 0, value.Len())
		for cellIndex := 0; cellIndex < value.Len(); cellIndex++ {
			cell, cellOK := value.At(cellIndex)
			if !cellOK || cell.Source() != 0 || !cell.Presence().Available() {
				t.Fatalf("oracle source cell %d/%d", index, cellIndex)
			}
			var oracleCell relationoracle.Cell
			var cellOKValue bool
			switch {
			case cell.Presence().Is(model.Present), cell.Presence().Is(model.AuthenticatedOpaque):
				if !cell.Value().Available() {
					t.Fatalf("oracle present value %d/%d", index, cellIndex)
				}
				oracleValue, valueOK := relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !valueOK {
					t.Fatalf("oracle value %d/%d", index, cellIndex)
				}
				if cell.Presence().Is(model.Present) {
					oracleCell, cellOKValue = relationoracle.PresentCell(cell.Column(), cell.Type(), oracleValue)
				} else {
					oracleCell, cellOKValue = relationoracle.OpaqueCell(cell.Column(), cell.Type(), oracleValue)
				}
			case cell.Presence().Is(model.ProvenAbsent):
				oracleCell, cellOKValue = relationoracle.AbsentCell(cell.Column(), cell.Type())
			case cell.Presence().Is(model.UnprovenMissing):
				oracleCell, cellOKValue = relationoracle.MissingCell(cell.Column(), cell.Type())
			default:
				t.Fatalf("oracle unsupported presence %d/%d: %v", index, cellIndex, cell.Presence().Kind())
			}
			if !cellOKValue {
				t.Fatalf("oracle cell %d/%d", index, cellIndex)
			}
			cells = append(cells, oracleCell)
		}
		var row relationoracle.Row
		var rowOK bool
		if lineage := value.Lineage(); lineage.Available() {
			row, rowOK = relationoracle.NewRow(rowID, scope, cells, lineage)
		} else {
			row, rowOK = relationoracle.NewRow(rowID, scope, cells)
		}
		if !rowOK {
			t.Fatalf("oracle row %d", index)
		}
		rows = append(rows, row)
	}
	relationValue, relationOK := relationoracle.NewRelation(relation, rows)
	if !relationOK {
		t.Fatal("oracle relation")
	}
	return relationValue
}

func oracleRegistry(t testing.TB, batch tuple.Batch) relationoracle.AlgebraRegistry {
	t.Helper()
	value, ok := batch.At(0)
	if !ok || value.Len() == 0 {
		t.Fatal("oracle type tuple")
	}
	cell, ok := value.At(0)
	if !ok {
		t.Fatal("oracle type cell")
	}
	entry, ok := relationoracle.NewAlgebraEntry(cell.Type(), relationoracle.IdentityAlgebra{})
	if !ok {
		t.Fatal("oracle algebra entry")
	}
	registry, ok := relationoracle.NewAlgebraRegistry([]relationoracle.AlgebraEntry{entry})
	if !ok {
		t.Fatal("oracle algebra registry")
	}
	return registry
}

func assertTupleMatchesRow(t testing.TB, value tuple.Tuple, row relationoracle.Row, relation model.RelationID) {
	t.Helper()
	if !value.Available() || !row.Available() {
		t.Fatal("unavailable merge result")
	}
	rowID, ok := value.SourceFor(relation)
	if !ok || rowID != row.ID() {
		t.Fatalf("merged source=%v want=%v", rowID, row.ID())
	}
	if value.Lineage() != row.Lineage() {
		t.Fatalf("merged lineage=%v want=%v", value.Lineage(), row.Lineage())
	}
	if value.Len() != len(row.Cells()) {
		t.Fatalf("merged cells=%d want=%d", value.Len(), len(row.Cells()))
	}
	for _, want := range row.Cells() {
		got, ok := value.CellFor(want.Column())
		if !ok || got.Type() != want.Type() || got.Presence().Kind() != want.Presence().Kind() {
			t.Fatalf("merged cell %v diverged", want.Column())
		}
		wantValue, wantOK := want.Value()
		gotValue := got.Value()
		if wantOK != gotValue.Available() || wantOK && wantValue.Content() != gotValue.Opaque() {
			t.Fatalf("merged value %v diverged", want.Column())
		}
	}
}

func assertMerge(t testing.TB, physical []tuple.Batch, expected relationoracle.Relation, mounted witness.Mounted, relation model.RelationID) {
	t.Helper()
	if !expected.Available() {
		t.Fatal("oracle merge refused valid input")
	}
	rows := expected.Rows()
	if len(physical) != len(rows) {
		t.Fatalf("merge batch count=%d oracle rows=%d", len(physical), len(rows))
	}
	seen := make(map[model.RowID]struct{}, len(rows))
	for index, batch := range physical {
		if !batch.ValidFor(mounted) || batch.Len() != 1 {
			t.Fatalf("merge output batch %d invalid/len=%d", index, batch.Len())
		}
		value, ok := batch.At(0)
		if !ok {
			t.Fatalf("merge output tuple %d", index)
		}
		rowID, ok := value.SourceFor(relation)
		if !ok {
			t.Fatalf("merge output source %d", index)
		}
		if _, duplicate := seen[rowID]; duplicate {
			t.Fatalf("merge output duplicated row %v", rowID)
		}
		seen[rowID] = struct{}{}
		row, ok := expected.Row(rowID)
		if !ok {
			t.Fatalf("merge output row %v absent from oracle", rowID)
		}
		assertTupleMatchesRow(t, value, row, relation)
	}
}

func TestExecuteDifferentialPositiveAndEmpty(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("merge node")
	}
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	physical, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{input, input})
	if !ok || len(physical) == 0 {
		t.Fatalf("physical merge=(%v,%d)", ok, len(physical))
	}
	registry := oracleRegistry(t, input)
	expected := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), input),
		oracleBatch(t, fixture.RelationLeft(), input),
	}, registry)
	assertMerge(t, physical, expected, fixture.Mounted(), fixture.RelationLeft())

	empty := emptyBatch(t, fixture, input)
	emptyPhysical, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{empty, empty})
	if !ok || emptyPhysical == nil || len(emptyPhysical) != 0 {
		t.Fatalf("physical empty merge=(%v,%v,%d)", ok, emptyPhysical == nil, len(emptyPhysical))
	}
	emptyExpected := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), empty),
		oracleBatch(t, fixture.RelationLeft(), empty),
	}, registry)
	if !emptyExpected.Available() || len(emptyExpected.Rows()) != 0 {
		t.Fatal("oracle empty merge")
	}
}

func TestExecuteDifferentialNoMatchAlternativeDoesNotFabricateRows(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("merge node")
	}
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	disjoint, _ := fixture.DisjointScopes()
	noMatch, ok := tuple.NewRangeBatch(fixture.Mounted(), input.Range(), disjoint, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
	if !ok || !noMatch.ValidFor(fixture.Mounted()) {
		t.Fatal("no-match empty range")
	}
	physical, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{input, noMatch})
	if !ok {
		t.Fatal("merge with no-match alternative")
	}
	registry := oracleRegistry(t, input)
	expected := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), input),
		oracleBatch(t, fixture.RelationLeft(), noMatch),
	}, registry)
	assertMerge(t, physical, expected, fixture.Mounted(), fixture.RelationLeft())
}

func TestExecuteDifferentialPermutationAndRootReplay(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("merge node")
	}
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	forward, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{input, input})
	if !ok {
		t.Fatal("forward merge")
	}
	permutedValues := input.Tuples()
	for first, last := 0, len(permutedValues)-1; first < last; first, last = first+1, last-1 {
		permutedValues[first], permutedValues[last] = permutedValues[last], permutedValues[first]
	}
	permuted, ok := tuple.PreserveRange(fixture.Mounted(), input, input.Scope(), permutedValues)
	if !ok {
		t.Fatal("permuted input")
	}
	backward, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{permuted, input})
	if !ok {
		t.Fatal("permuted merge")
	}
	registry := oracleRegistry(t, input)
	expected := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), input),
		oracleBatch(t, fixture.RelationLeft(), input),
	}, registry)
	permutedExpected := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), permuted),
		oracleBatch(t, fixture.RelationLeft(), input),
	}, registry)
	assertMerge(t, forward, expected, fixture.Mounted(), fixture.RelationLeft())
	assertMerge(t, backward, permutedExpected, fixture.Mounted(), fixture.RelationLeft())
	if len(forward) != len(backward) {
		t.Fatal("permutation changed merge cardinality")
	}

	delta, deltaOK := fixture.BaseToLeftDelta()
	if !deltaOK || !delta.Next().Same(fixture.LeftRoot()) {
		t.Fatal("invalid fixture delta")
	}
	replayed := inputSourceAtRoot(t, fixture, delta.Next())
	replayedPhysical, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{replayed, replayed})
	if !ok {
		t.Fatal("replayed merge")
	}
	assertMerge(t, replayedPhysical, expected, fixture.Mounted(), fixture.RelationLeft())
}

func TestExecuteRefusesForeignOrMalformedInput(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	foreign := testfixture.New(t, 0x72)
	node, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("merge node")
	}
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		t.Fatal("merge binding")
	}
	foreignInput := inputSourceAtRoot(t, foreign, foreign.LeftRoot())
	if output, ok := physicalmerge.Execute(binding, foreign.Mounted(), []tuple.Batch{foreignInput}); ok || output != nil {
		t.Fatal("foreign mounted merge accepted")
	}
	if output, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{foreignInput}); ok || output != nil {
		t.Fatal("foreign input batch accepted")
	}
	var unavailable tuple.Batch
	if output, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{unavailable}); ok || output != nil {
		t.Fatal("unavailable input batch accepted")
	}
}

func TestExecuteRetainsLineageAndOwnerRows(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.MergeNode()
	if !ok {
		t.Fatal("merge node")
	}
	binding, ok := node.Merge()
	if !ok || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	physical, ok := physicalmerge.Execute(binding, fixture.Mounted(), []tuple.Batch{input, input})
	if !ok {
		t.Fatal("merge")
	}
	want := relationoracle.Merge([]relationoracle.Relation{
		oracleBatch(t, fixture.RelationLeft(), input),
		oracleBatch(t, fixture.RelationLeft(), input),
	}, oracleRegistry(t, input))
	assertMerge(t, physical, want, fixture.Mounted(), fixture.RelationLeft())
}
