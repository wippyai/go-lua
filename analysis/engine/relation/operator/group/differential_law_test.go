package group_test

import (
	"testing"

	physicalgroup "github.com/wippyai/go-lua/analysis/engine/relation/operator/group"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
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

// oracleBatch is a read-only projection of the already authenticated tuple
// facts. It does not issue identities or duplicate the physical operator.
func oracleBatch(t testing.TB, relation model.RelationID, batch tuple.Batch) relationoracle.Relation {
	t.Helper()
	relationContent := relation.Content()
	scopeID, ok := identity.DeriveContentID("analysis/engine/relation/operator/group/law/scope/v1", relationContent[:])
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

func rowSet(rows []relationoracle.Row) map[model.RowID]struct{} {
	result := make(map[model.RowID]struct{}, len(rows))
	for _, row := range rows {
		result[row.ID()] = struct{}{}
	}
	return result
}

func physicalGroupSet(t testing.TB, batch tuple.Batch, relation model.RelationID) map[model.RowID]struct{} {
	t.Helper()
	result := make(map[model.RowID]struct{}, batch.Len())
	for index := 0; index < batch.Len(); index++ {
		value, ok := batch.At(index)
		if !ok {
			t.Fatalf("physical group tuple %d", index)
		}
		row, ok := value.SourceFor(relation)
		if !ok {
			t.Fatalf("physical group source %d", index)
		}
		result[row] = struct{}{}
	}
	return result
}

func assertGroups(t testing.TB, mounted witness.Mounted, physical []tuple.Batch, expected []relationoracle.Grouped, relation model.RelationID) {
	t.Helper()
	if len(physical) != len(expected) {
		t.Fatalf("group count physical=%d oracle=%d", len(physical), len(expected))
	}
	remaining := make([]map[model.RowID]struct{}, len(expected))
	for index, group := range expected {
		if !group.Available() {
			t.Fatal("oracle group unavailable")
		}
		remaining[index] = rowSet(group.Rows())
	}
	used := make([]bool, len(expected))
	for index, batch := range physical {
		if !batch.ValidFor(mounted) {
			t.Fatal("physical group batch invalid")
		}
		got := physicalGroupSet(t, batch, relation)
		found := -1
		for candidate, want := range remaining {
			if !used[candidate] && sameRowSet(got, want) {
				found = candidate
				break
			}
		}
		if found < 0 {
			t.Fatalf("physical group %d has no oracle partition: %v", index, got)
		}
		used[found] = true
	}
}

func sameRowSet(left, right map[model.RowID]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for row := range left {
		if _, ok := right[row]; !ok {
			return false
		}
	}
	return true
}

func assertGroupKeys(t testing.TB, mounted witness.Mounted, batches []tuple.Batch, columns []model.ColumnID) {
	t.Helper()
	for index, batch := range batches {
		if batch.Len() == 0 {
			t.Fatalf("group %d is empty", index)
		}
		value, ok := batch.At(0)
		if !ok {
			t.Fatalf("group %d first tuple", index)
		}
		keys := batch.RangeKeys()
		if len(keys) != len(columns) {
			t.Fatalf("group %d key width=%d want=%d", index, len(keys), len(columns))
		}
		for keyIndex, column := range columns {
			cell, cellOK := value.CellFor(column)
			if !cellOK || !cell.Value().Available() || !keys[keyIndex].Same(cell.Value()) || !keys[keyIndex].ValidFor(mounted.RuntimeFence()) {
				t.Fatalf("group %d key %d does not match first tuple", index, keyIndex)
			}
		}
	}
}

func TestExecuteDifferentialPositiveAndEmpty(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.GroupNode()
	if !ok {
		t.Fatal("group node")
	}
	binding, ok := node.Group()
	if !ok || !binding.Available() {
		t.Fatal("group binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	physical, ok := physicalgroup.Execute(binding, fixture.Mounted(), input)
	if !ok || len(physical) == 0 {
		t.Fatalf("physical group=(%v,%d)", ok, len(physical))
	}
	oracleInput := oracleBatch(t, fixture.RelationLeft(), input)
	registry := oracleRegistry(t, input)
	expected := relationoracle.GroupByKey(oracleInput, fixture.KeyLeft(), binding.Key().KeyColumns(), binding.Cardinality(), registry)
	if expected == nil {
		t.Fatal("oracle group refused valid input")
	}
	assertGroups(t, fixture.Mounted(), physical, expected, fixture.RelationLeft())
	assertGroupKeys(t, fixture.Mounted(), physical, binding.Key().KeyColumns())

	empty := emptyBatch(t, fixture, input)
	emptyPhysical, ok := physicalgroup.Execute(binding, fixture.Mounted(), empty)
	if !ok || emptyPhysical == nil || len(emptyPhysical) != 0 {
		t.Fatalf("physical empty group=(%v,%v,%d)", ok, emptyPhysical == nil, len(emptyPhysical))
	}
	emptyOracle := relationoracle.GroupByKey(oracleBatch(t, fixture.RelationLeft(), empty), fixture.KeyLeft(), binding.Key().KeyColumns(), binding.Cardinality(), registry)
	if emptyOracle == nil || len(emptyOracle) != 0 {
		t.Fatalf("oracle empty group=%v", emptyOracle)
	}
}

func TestExecuteDifferentialPermutationAndRootReplay(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.GroupNode()
	if !ok {
		t.Fatal("group node")
	}
	binding, ok := node.Group()
	if !ok || !binding.Available() {
		t.Fatal("group binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	values := input.Tuples()
	for first, last := 0, len(values)-1; first < last; first, last = first+1, last-1 {
		values[first], values[last] = values[last], values[first]
	}
	permuted, ok := tuple.PreserveRange(fixture.Mounted(), input, input.Scope(), values)
	if !ok {
		t.Fatal("permuted input")
	}
	forward, ok := physicalgroup.Execute(binding, fixture.Mounted(), input)
	if !ok {
		t.Fatal("forward group")
	}
	backward, ok := physicalgroup.Execute(binding, fixture.Mounted(), permuted)
	if !ok {
		t.Fatal("permuted group")
	}
	registry := oracleRegistry(t, input)
	wantForward := relationoracle.GroupByKey(oracleBatch(t, fixture.RelationLeft(), input), fixture.KeyLeft(), binding.Key().KeyColumns(), binding.Cardinality(), registry)
	wantBackward := relationoracle.GroupByKey(oracleBatch(t, fixture.RelationLeft(), permuted), fixture.KeyLeft(), binding.Key().KeyColumns(), binding.Cardinality(), registry)
	assertGroups(t, fixture.Mounted(), forward, wantForward, fixture.RelationLeft())
	assertGroups(t, fixture.Mounted(), backward, wantBackward, fixture.RelationLeft())
	assertGroupKeys(t, fixture.Mounted(), forward, binding.Key().KeyColumns())
	assertGroupKeys(t, fixture.Mounted(), backward, binding.Key().KeyColumns())
	if len(forward) != len(backward) {
		t.Fatal("permutation changed group count")
	}

	leftDelta, leftOK := fixture.BaseToLeftDelta()
	if !leftOK || !leftDelta.Next().Same(fixture.LeftRoot()) {
		t.Fatal("invalid fixture delta")
	}
	replayed := inputSourceAtRoot(t, fixture, leftDelta.Next())
	replayedGroups, ok := physicalgroup.Execute(binding, fixture.Mounted(), replayed)
	if !ok {
		t.Fatal("replayed group")
	}
	assertGroups(t, fixture.Mounted(), replayedGroups, wantForward, fixture.RelationLeft())
	assertGroupKeys(t, fixture.Mounted(), replayedGroups, binding.Key().KeyColumns())
}

func TestExecuteRefusesCardinalityOverflow(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.GroupNode()
	if !ok {
		t.Fatal("group node")
	}
	binding, ok := node.Group()
	if !ok || !binding.Available() {
		t.Fatal("group binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	first, ok := input.At(0)
	if !ok {
		t.Fatal("first input tuple")
	}
	duplicate, ok := tuple.PreserveRange(fixture.Mounted(), input, input.Scope(), []tuple.Tuple{first, first, first})
	if !ok {
		t.Fatal("duplicate input range")
	}
	if output, ok := physicalgroup.Execute(binding, fixture.Mounted(), duplicate); ok || output != nil {
		t.Fatal("group cardinality overflow accepted")
	}
}

func TestExecuteRefusesForeignOrMalformedInput(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	foreign := testfixture.New(t, 0x72)
	node, ok := fixture.GroupNode()
	if !ok {
		t.Fatal("group node")
	}
	binding, ok := node.Group()
	if !ok || !binding.Available() {
		t.Fatal("group binding")
	}
	foreignInput := inputSourceAtRoot(t, foreign, foreign.LeftRoot())
	if output, ok := physicalgroup.Execute(binding, foreign.Mounted(), foreignInput); ok || output != nil {
		t.Fatal("foreign mounted group accepted")
	}
	if output, ok := physicalgroup.Execute(binding, fixture.Mounted(), foreignInput); ok || output != nil {
		t.Fatal("foreign input batch accepted")
	}
	var unavailable tuple.Batch
	if output, ok := physicalgroup.Execute(binding, fixture.Mounted(), unavailable); ok || output != nil {
		t.Fatal("unavailable input batch accepted")
	}
}

func TestExecuteRetainsLineagePerGroupedTuple(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, ok := fixture.GroupNode()
	if !ok {
		t.Fatal("group node")
	}
	binding, ok := node.Group()
	if !ok || !binding.Available() {
		t.Fatal("group binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	physical, ok := physicalgroup.Execute(binding, fixture.Mounted(), input)
	if !ok {
		t.Fatal("group")
	}
	want := oracleBatch(t, fixture.RelationLeft(), input)
	for _, group := range physical {
		for index := 0; index < group.Len(); index++ {
			value, valueOK := group.At(index)
			if !valueOK {
				t.Fatal("group tuple")
			}
			rowID, rowOK := value.SourceFor(fixture.RelationLeft())
			row, oracleOK := want.Row(rowID)
			if !rowOK || !oracleOK || row.Lineage() != value.Lineage() {
				t.Fatalf("group lineage row=%v", rowID)
			}
		}
	}
}
