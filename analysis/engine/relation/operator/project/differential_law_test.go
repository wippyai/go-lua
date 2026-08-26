package project

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	relationfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

// projectBatch redeems both sides from the fixture's sealed arrangement. The
// test never constructs a ProjectBinding, range, key, or row identity.
func projectBatch(t testing.TB, fixture relationfixture.Fixture, sourceRoot database.Version, targetRoot database.Version) (tuple.Batch, arrangement.ProjectBinding, read.Reader, read.Reader, bool) {
	t.Helper()
	inputNode, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	input, ok := inputNode.Input()
	if !ok || !input.Available() {
		t.Fatal("left input binding")
	}
	rangeAuthority, ok := input.Range()
	if !ok || !rangeAuthority.Available() {
		t.Fatal("left input range")
	}
	sourceReader, ok := read.Bind(sourceRoot, input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("source reader")
	}
	values := make([]tuple.Tuple, 0, len(fixture.RowsLeft()))
	completed, valid := sourceReader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(fixture.Mounted(), sourceReader, row)
		if !valueOK {
			return false
		}
		values = append(values, value)
		return true
	})
	if !completed || !valid || len(values) == 0 {
		t.Fatalf("source scan=(%v,%v,%d)", completed, valid, len(values))
	}
	source, ok := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, values[0].Scope(), values, binding.DenominatorWitness{})
	if !ok {
		t.Fatal("source range")
	}
	projectNode, ok := fixture.ProjectNode()
	if !ok {
		t.Fatal("project node")
	}
	projectBinding, ok := projectNode.Project()
	if !ok {
		t.Fatal("project binding")
	}
	targetReader, ok := read.Bind(targetRoot, projectBinding.Target(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("target row reader")
	}
	keyReader, ok := fixture.ReaderRightKey(targetRoot)
	if !ok {
		t.Fatal("target key reader")
	}
	return source, projectBinding, targetReader, keyReader, true
}

func TestProjectPreservesCellsAndRedeemsDestinationIdentity(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	source, projectBinding, target, keyTarget, ok := projectBatch(t, fixture, fixture.LeftRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("project inputs")
	}
	if !target.Layout().Equal(projectBinding.Target()) || !keyTarget.Layout().Equal(projectBinding.Key()) || target.Layout().Access().Key().Available() {
		t.Fatal("project did not retain separate complete-target and key layouts")
	}
	if len(target.Layout().Columns()) <= len(projectBinding.Key().KeyColumns()) {
		t.Fatal("project target layout collapsed to its key columns")
	}
	outputs, ok := Execute(projectBinding, fixture.Mounted(), source, target)
	if !ok || len(outputs) != 1 {
		t.Fatalf("project outputs=(%v,%d)", ok, len(outputs))
	}

	oracle := oracleProject(t, fixture, source, target, projectBinding.Key().KeyColumns())
	if len(oracle.Rows()) != outputs[0].Len() {
		t.Fatalf("oracle rows=%d physical=%d", len(oracle.Rows()), outputs[0].Len())
	}
	want := make(map[identity.ContentID]relationoracle.Cell, len(oracle.Rows()))
	for _, row := range oracle.Rows() {
		cell, cellOK := row.Cell(fixture.KeyColumnsRight()[0])
		if !cellOK {
			t.Fatal("oracle projected cell")
		}
		value, valueOK := cell.Value()
		if !valueOK {
			t.Fatal("oracle projected value")
		}
		want[value.Content()] = cell
	}
	for index := 0; index < outputs[0].Len(); index++ {
		value, valueOK := outputs[0].At(index)
		if !valueOK || value.SourceLen() != 2 {
			t.Fatal("projected tuple sources")
		}
		for mappingIndex := 0; mappingIndex < projectBinding.MappingCount(); mappingIndex++ {
			mapping, mappingOK := projectBinding.MappingAt(mappingIndex)
			cell, cellOK := value.At(mappingIndex)
			if !mappingOK || !cellOK || cell.Column() != mapping.Target() {
				t.Fatalf("projected target order at %d", mappingIndex)
			}
		}
		leftRow, leftOK := value.SourceAt(0)
		rightRow, rightOK := value.SourceAt(1)
		if !leftOK || !rightOK || leftRow.Relation() != fixture.RelationLeft() || rightRow.Relation() != fixture.RelationRight() {
			t.Fatal("projected owner-issued source rows")
		}
		cell, cellOK := value.CellFor(fixture.KeyColumnsRight()[0])
		if !cellOK || !cell.Column().Available() || !cell.Type().Available() || !cell.Presence().Available() || !cell.Presence().Is(model.Present) || !cell.Value().Available() {
			t.Fatal("projected target cell")
		}
		oracleCell, oracleOK := want[cell.Value().Opaque()]
		if !oracleOK || oracleCell.Type() != cell.Type() || !oracleCell.Presence().Is(cell.Presence().Kind()) {
			t.Fatal("physical projection diverged from logical oracle")
		}
		assertDestination(t, keyTarget, value, rightRow, fixture.KeyColumnsRight())
	}
}

func TestProjectRefusesMissingSourceCell(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	_, projectBinding, target, _, ok := projectBatch(t, fixture, fixture.LeftRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("project inputs")
	}
	inputNode, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	rangeAuthority, ok := inputNode.Range()
	if !ok {
		t.Fatal("left input range")
	}
	reader, ok := fixture.ReaderLeftKey(fixture.LeftRoot())
	if !ok {
		t.Fatal("left key reader")
	}
	values := make([]tuple.Tuple, 0, len(fixture.RowsLeft()))
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(fixture.Mounted(), reader, row)
		if !valueOK {
			return false
		}
		values = append(values, value)
		return true
	})
	if !completed || !valid || len(values) == 0 {
		t.Fatal("left key scan")
	}
	missing, ok := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, values[0].Scope(), values, binding.DenominatorWitness{})
	if !ok {
		t.Fatal("missing-source range")
	}
	if output, valid := Execute(projectBinding, fixture.Mounted(), missing, target); valid || output != nil {
		t.Fatal("missing source cell was treated as a no-selection")
	}
}

func TestProjectRefusesForeignBindingMount(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	foreign := relationfixture.New(t, 0x72)
	source, binding, _, _, ok := projectBatch(t, fixture, fixture.LeftRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("project inputs")
	}
	foreignSource, _, foreignTarget, _, ok := projectBatch(t, foreign, foreign.LeftRoot(), foreign.BothRoot())
	if !ok {
		t.Fatal("foreign project inputs")
	}
	if output, valid := Execute(binding, foreign.Mounted(), foreignSource, foreignTarget); valid || output != nil {
		t.Fatal("foreign mounted binding was accepted")
	}
	if output, valid := Execute(binding, fixture.Mounted(), source, foreignTarget); valid || output != nil {
		t.Fatal("foreign target reader was accepted")
	}
}

func TestProjectRetainsEmptyRangeAndRefusesForeignTargetLayout(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	source, projectBinding, target, _, ok := projectBatch(t, fixture, fixture.LeftRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("project inputs")
	}
	empty, ok := tuple.PreserveRange(fixture.Mounted(), source, source.Scope(), []tuple.Tuple{})
	if !ok {
		t.Fatal("empty source range")
	}
	outputs, ok := Execute(projectBinding, fixture.Mounted(), empty, target)
	if !ok || len(outputs) != 1 || outputs[0].Len() != 0 || !outputs[0].Range().Available() || !outputs[0].Scope().Same(source.Scope()) {
		t.Fatal("empty source result did not retain range")
	}

	noTarget, ok := read.Bind(fixture.LeftRoot(), projectBinding.Target(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("no-target row reader")
	}
	outputs, ok = Execute(projectBinding, fixture.Mounted(), source, noTarget)
	if !ok || len(outputs) != 1 || outputs[0].Len() != 0 || outputs[0].Range().Producer() != source.Range().Producer() {
		t.Fatal("no-match result did not retain source range")
	}

	foreignTarget, ok := fixture.ReaderLeftInput(fixture.LeftRoot())
	if !ok {
		t.Fatal("foreign target layout reader")
	}
	if outputs, valid := Execute(projectBinding, fixture.Mounted(), source, foreignTarget); valid || outputs != nil {
		t.Fatal("foreign target layout was accepted")
	}
}

func TestProjectReplayIsStableAcrossEquivalentCommittedRoots(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	first, projectBinding, target, _, ok := projectBatch(t, fixture, fixture.LeftRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("first project inputs")
	}
	firstResult, ok := Execute(projectBinding, fixture.Mounted(), first, target)
	if !ok || len(firstResult) != 1 {
		t.Fatal("first project")
	}
	second, projectBinding, target, _, ok := projectBatch(t, fixture, fixture.BothRoot(), fixture.BothRoot())
	if !ok {
		t.Fatal("second project inputs")
	}
	secondResult, ok := Execute(projectBinding, fixture.Mounted(), second, target)
	if !ok || len(secondResult) != 1 || !firstResult[0].Same(secondResult[0]) {
		t.Fatal("equivalent committed roots changed projection")
	}
}

func assertDestination(t testing.TB, target read.Reader, value tuple.Tuple, want model.RowID, columns [2]model.ColumnID) {
	t.Helper()
	keys := make([]binding.ValueToken, len(columns))
	for index, column := range columns {
		cell, ok := value.CellFor(column)
		if !ok || !cell.Value().Available() {
			t.Fatal("target key cell")
		}
		keys[index] = cell.Value()
	}
	key, ok := target.TupleFrom(keys)
	if !ok {
		t.Fatal("target key")
	}
	found := false
	completed, valid := target.Lookup(key, func(row read.Row) bool {
		if row.ID() != want {
			t.Fatalf("destination row=%v want=%v", row.ID(), want)
		}
		found = true
		return true
	})
	if !completed || !valid || !found {
		t.Fatal("destination lookup")
	}
}

func oracleProject(t testing.TB, fixture relationfixture.Fixture, source tuple.Batch, target read.Reader, keyColumns []model.ColumnID) relationoracle.Relation {
	t.Helper()
	scopeID, ok := identity.DeriveContentID("analysis/engine/relation/operator/project/law/scope/v1", []byte("left"))
	if !ok {
		t.Fatal("oracle scope identity")
	}
	scope, ok := relationoracle.NewScope(scopeID)
	if !ok {
		t.Fatal("oracle scope")
	}
	rows := make([]relationoracle.Row, 0, source.Len())
	for index := 0; index < source.Len(); index++ {
		value, ok := source.At(index)
		if !ok {
			t.Fatal("oracle source tuple")
		}
		rowID, ok := value.SourceAt(0)
		if !ok {
			t.Fatal("oracle source row")
		}
		cells := make([]relationoracle.Cell, 0, value.Len())
		for cellIndex := 0; cellIndex < value.Len(); cellIndex++ {
			cell, ok := value.At(cellIndex)
			if !ok || !cell.Value().Available() {
				t.Fatal("oracle source cell")
			}
			oracleValue, ok := relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
			if !ok {
				t.Fatal("oracle value")
			}
			oracleCell, ok := relationoracle.PresentCell(cell.Column(), cell.Type(), oracleValue)
			if !ok {
				t.Fatal("oracle cell")
			}
			cells = append(cells, oracleCell)
		}
		var row relationoracle.Row
		if lineage := value.Lineage(); lineage.Available() {
			row, ok = relationoracle.NewRow(rowID, scope, cells, lineage)
		} else {
			row, ok = relationoracle.NewRow(rowID, scope, cells)
		}
		if !ok {
			t.Fatal("oracle row")
		}
		rows = append(rows, row)
	}
	relation, ok := relationoracle.NewRelation(fixture.RelationLeft(), rows)
	if !ok {
		t.Fatal("oracle relation")
	}
	targetRelation := oracleTargetRelation(t, fixture.RelationRight(), target)
	leftColumns := fixture.KeyColumnsLeft()
	rightColumns := fixture.KeyColumnsRight()
	leftType, ok := source.At(0)
	if !ok {
		t.Fatal("oracle type source")
	}
	keyCell, ok := leftType.CellFor(leftColumns[0])
	if !ok {
		t.Fatal("oracle key type")
	}
	entry, ok := relationoracle.NewAlgebraEntry(keyCell.Type(), relationoracle.IdentityAlgebra{})
	if !ok {
		t.Fatal("oracle algebra entry")
	}
	registry, ok := relationoracle.NewAlgebraRegistry([]relationoracle.AlgebraEntry{entry})
	if !ok {
		t.Fatal("oracle algebra registry")
	}
	lineageAuthority, ok := fixture.Mounted().Lineage()
	if !ok || lineageAuthority == nil {
		t.Fatal("oracle lineage authority")
	}
	projected := relationoracle.Project(relation, targetRelation, relationoracle.NewProjectSpec(fixture.RelationRight(), keyColumns, []relationoracle.ColumnMapping{
		relationoracle.NewColumnMapping(leftColumns[0], rightColumns[0], keyCell.Type()),
		relationoracle.NewColumnMapping(leftColumns[1], rightColumns[1], keyCell.Type()),
		relationoracle.NewColumnMapping(fixture.PayloadColumnsLeft()[0], fixture.PayloadColumnsRight()[0], keyCell.Type()),
		relationoracle.NewColumnMapping(fixture.PayloadColumnsLeft()[1], fixture.PayloadColumnsRight()[1], keyCell.Type()),
	}, relationoracle.ExactScope{}, lineageAuthority), registry)
	if !projected.Available() {
		t.Fatal("oracle projection")
	}
	return projected
}

func oracleTargetRelation(t testing.TB, relation model.RelationID, target read.Reader) relationoracle.Relation {
	t.Helper()
	scopeID, ok := identity.DeriveContentID("analysis/engine/relation/operator/project/law/scope/v1", []byte("right"))
	if !ok {
		t.Fatal("target scope identity")
	}
	scope, ok := relationoracle.NewScope(scopeID)
	if !ok {
		t.Fatal("target scope")
	}
	rows := make([]relationoracle.Row, 0)
	completed, valid := target.Scan(func(source read.Row) bool {
		if source == nil || !source.Available() || source.ID().Relation() != relation {
			return false
		}
		cells := make([]relationoracle.Cell, 0, len(source.Cells()))
		for _, cell := range source.Cells() {
			if !cell.Available() {
				return false
			}
			var oracleCell relationoracle.Cell
			var cellOK bool
			switch {
			case cell.Presence().Is(model.Present):
				value, valueOK := relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !valueOK {
					return false
				}
				oracleCell, cellOK = relationoracle.PresentCell(cell.Column(), cell.Type(), value)
			case cell.Presence().Is(model.AuthenticatedOpaque):
				value, valueOK := relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !valueOK {
					return false
				}
				oracleCell, cellOK = relationoracle.OpaqueCell(cell.Column(), cell.Type(), value)
			case cell.Presence().Is(model.ProvenAbsent):
				oracleCell, cellOK = relationoracle.AbsentCell(cell.Column(), cell.Type())
			case cell.Presence().Is(model.UnprovenMissing):
				oracleCell, cellOK = relationoracle.MissingCell(cell.Column(), cell.Type())
			default:
				return false
			}
			if !cellOK {
				return false
			}
			cells = append(cells, oracleCell)
		}
		var row relationoracle.Row
		if lineage := source.Lineage(); lineage.Available() {
			row, ok = relationoracle.NewRow(source.ID(), scope, cells, lineage)
		} else {
			row, ok = relationoracle.NewRow(source.ID(), scope, cells)
		}
		if !ok {
			return false
		}
		rows = append(rows, row)
		return true
	})
	if !completed || !valid {
		t.Fatal("target scan")
	}
	relationValue, ok := relationoracle.NewRelation(relation, rows)
	if !ok {
		t.Fatal("target relation")
	}
	return relationValue
}
