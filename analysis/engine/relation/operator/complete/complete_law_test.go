package complete_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

func completeWitness(t testing.TB, fixture testfixture.Fixture, ref model.DenominatorRef) bindingpkg.DenominatorWitness {
	t.Helper()
	witnessValue, ok := fixture.Mounted().Denominator(ref)
	if !ok || !witnessValue.Available() || !witnessValue.ValidFor(fixture.Mounted().RuntimeFence()) || !witnessValue.Matches(ref) {
		t.Fatal("complete denominator witness")
	}
	return witnessValue
}

func TestExecutePreservesPresentAndClosesMissingMembers(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	inputNode, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	inputRange, ok := inputNode.Range()
	if !ok {
		t.Fatal("left input range")
	}
	leftScope, _ := fixture.OverlapScopes()
	reader, ok := fixture.ReaderLeftPayload(fixture.BothRoot())
	if !ok {
		t.Fatal("left payload reader")
	}
	var first tuple.Tuple
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(fixture.Mounted(), reader, row)
		if !valueOK {
			return false
		}
		first = value
		return false
	})
	if completed || !valid || !first.ValidFor(fixture.Mounted()) || !first.Scope().Same(leftScope) {
		t.Fatalf("capture first tuple completed=%v valid=%v available=%v scope=%v want=%v", completed, valid, first.Available(), first.Scope().Available(), leftScope.Available())
	}
	firstRow, firstRowOK := first.SourceFor(fixture.RelationLeft())
	if !firstRowOK {
		t.Fatal("capture first source row")
	}
	firstLineage := first.Lineage()
	input, ok := tuple.NewRangeBatch(fixture.Mounted(), inputRange, leftScope, []tuple.Tuple{first}, bindingpkg.DenominatorWitness{})
	if !ok {
		t.Fatal("input batch")
	}
	output, ok := complete.Execute(binding, fixture.Mounted(), input, completeWitness(t, fixture, binding.Denominator()))
	if !ok || output.Len() != 2 {
		t.Fatalf("result ok=%v len=%d", ok, output.Len())
	}
	if !output.ValidFor(fixture.Mounted()) || !output.Scope().Same(leftScope) {
		t.Fatal("output batch authority")
	}
	for index := 0; index < output.Len(); index++ {
		value, valueOK := output.At(index)
		if !valueOK {
			t.Fatalf("output row %d", index)
		}
		row, rowOK := value.SourceFor(fixture.RelationLeft())
		if !rowOK {
			t.Fatalf("output row %d missing source", index)
		}
		wantLineage, lineageOK := fixture.Mounted().RowLineage(row)
		if row == firstRow {
			wantLineage = firstLineage
		}
		if !lineageOK || value.Lineage() != wantLineage {
			t.Fatalf("output row %d lineage was not redeemed from mount", index)
		}
		for _, column := range leftColumns(fixture) {
			cell, cellOK := value.CellFor(column)
			if !cellOK || !cell.Type().Available() || cell.Column() != column {
				t.Fatalf("row %d column %v missing typed cell", index, column)
			}
			if row == firstRow {
				if column == fixture.PayloadColumnsLeft()[0] || column == fixture.PayloadColumnsLeft()[1] {
					if !cell.Presence().Is(model.Present) || !cell.Value().Available() {
						t.Fatalf("present payload was not preserved for %v", column)
					}
				}
			} else if !cell.Presence().Is(model.ProvenAbsent) || cell.Value().Available() {
				t.Fatalf("missing row fabricated payload for %v", column)
			}
		}
	}
}

func TestExecuteEmptyBatchMaterializesOnlyTypedAbsence(t *testing.T) {
	// Reuse the immutable cold world used by the other laws. The fixture's
	// first construction is the only cold cost; this law must not multiply it
	// with a fresh mount merely to exercise an empty batch.
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	inputNode, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	inputRange, ok := inputNode.Range()
	if !ok {
		t.Fatal("left input range")
	}
	fiber, _ := fixture.OverlapScopes()
	empty, ok := tuple.NewRangeBatch(fixture.Mounted(), inputRange, fiber, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
	if !ok || empty.Len() != 0 {
		t.Fatal("empty input batch")
	}
	output, ok := complete.Execute(binding, fixture.Mounted(), empty, completeWitness(t, fixture, binding.Denominator()))
	if !ok || output.Len() != len(fixture.RowsLeft()) {
		t.Fatalf("empty input ok=%v len=%d", ok, output.Len())
	}
	for index := 0; index < output.Len(); index++ {
		value, valueOK := output.At(index)
		row, rowOK := value.SourceFor(fixture.RelationLeft())
		wantLineage, lineageOK := fixture.Mounted().RowLineage(row)
		if !valueOK || !rowOK || !lineageOK || value.Lineage() != wantLineage {
			t.Fatalf("absence row %d did not redeem mounted lineage", index)
		}
		for _, column := range leftColumns(fixture) {
			cell, cellOK := value.CellFor(column)
			if !cellOK || !cell.Presence().Is(model.ProvenAbsent) || cell.Value().Available() {
				t.Fatalf("absence row %d column %v is not ProvenAbsent", index, column)
			}
		}
	}
}

func TestExecuteRefusesForeignDenominatorAndRows(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	inputNode, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	inputRange, ok := inputNode.Range()
	if !ok {
		t.Fatal("left input range")
	}
	foreignReader, ok := fixture.ReaderRightKey(fixture.BothRoot())
	if !ok {
		t.Fatal("foreign reader")
	}
	var foreignTuple tuple.Tuple
	completed, valid := foreignReader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(fixture.Mounted(), foreignReader, row)
		if !valueOK {
			t.Fatal("foreign tuple")
		}
		foreignTuple = value
		return false
	})
	if completed || !valid {
		t.Fatal("foreign scan")
	}
	_, foreignScope := fixture.OverlapScopes()
	input, ok := tuple.NewRangeBatch(fixture.Mounted(), inputRange, foreignScope, []tuple.Tuple{foreignTuple}, bindingpkg.DenominatorWitness{})
	if !ok {
		t.Fatal("foreign batch")
	}
	if output, ok := complete.Execute(binding, fixture.Mounted(), input, completeWitness(t, fixture, binding.Denominator())); ok || output.Available() {
		t.Fatalf("foreign input ok=%v batch=%v", ok, output.Available())
	}
}

func leftColumns(fixture testfixture.Fixture) []model.ColumnID {
	keys := fixture.KeyColumnsLeft()
	payload := fixture.PayloadColumnsLeft()
	return []model.ColumnID{keys[0], keys[1], payload[0], payload[1]}
}

// completeBatch redeems the fixture's exact Input range and payload vector.
// The helper deliberately does not construct a denominator, range, or row
// identity: all three are obtained from the sealed arrangement/mounted world.
func completeBatch(t testing.TB, fixture testfixture.Fixture, reverse, empty bool) tuple.Batch {
	t.Helper()
	node, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	rangeAuthority, ok := node.Range()
	if !ok || !rangeAuthority.Available() {
		t.Fatal("left input range")
	}
	scope, _ := fixture.OverlapScopes()
	values := make([]tuple.Tuple, 0, len(fixture.RowsLeft()))
	if !empty {
		reader, readerOK := fixture.ReaderLeftPayload(fixture.BothRoot())
		if !readerOK || !reader.Available() {
			t.Fatal("left payload reader")
		}
		completed, valid := reader.Scan(func(row read.Row) bool {
			value, valueOK := tuple.Input(fixture.Mounted(), reader, row)
			if !valueOK {
				t.Fatal("left payload tuple")
			}
			values = append(values, value)
			return true
		})
		if !completed || !valid || len(values) == 0 {
			t.Fatal("left payload scan")
		}
		scope = values[0].Scope()
		if reverse {
			for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
				values[left], values[right] = values[right], values[left]
			}
		}
	}
	batch, ok := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, scope, values, bindingpkg.DenominatorWitness{})
	if !ok || !batch.ValidFor(fixture.Mounted()) {
		t.Fatal("complete input batch")
	}
	return batch
}

func oracleCompleteScope(t testing.TB) relationoracle.Scope {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/engine/relation/operator/complete/law/scope/v1", []byte("left"))
	if !ok {
		t.Fatal("oracle scope identity")
	}
	scope, ok := relationoracle.NewScope(id)
	if !ok {
		t.Fatal("oracle scope")
	}
	return scope
}

func oracleCompleteInput(t testing.TB, fixture testfixture.Fixture, input tuple.Batch) relationoracle.Relation {
	t.Helper()
	scope := oracleCompleteScope(t)
	rows := make([]relationoracle.Row, 0, input.Len())
	for index := 0; index < input.Len(); index++ {
		value, ok := input.At(index)
		if !ok {
			t.Fatal("oracle input tuple")
		}
		rowID, ok := value.SourceFor(fixture.RelationLeft())
		if !ok {
			t.Fatal("oracle input source row")
		}
		cells := make([]relationoracle.Cell, 0, value.Len())
		for cellIndex := 0; cellIndex < value.Len(); cellIndex++ {
			cell, cellOK := value.At(cellIndex)
			if !cellOK {
				t.Fatal("oracle input cell")
			}
			var oracleValue relationoracle.ValueToken
			if cell.Value().Available() {
				oracleValue, ok = relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !ok {
					t.Fatal("oracle input value")
				}
			}
			oracleCell, ok := relationoracle.NewCell(cell.Column(), cell.Type(), oracleValue, cell.Presence())
			if !ok {
				t.Fatal("oracle input cell authority")
			}
			cells = append(cells, oracleCell)
		}
		row, ok := relationoracle.NewRow(rowID, scope, cells, value.Lineage())
		if !ok {
			t.Fatal("oracle input row")
		}
		rows = append(rows, row)
	}
	relation, ok := relationoracle.NewRelation(fixture.RelationLeft(), rows)
	if !ok {
		t.Fatal("oracle input relation")
	}
	return relation
}

func oracleCompleteExpected(t testing.TB, fixture testfixture.Fixture, input tuple.Batch) relationoracle.Relation {
	t.Helper()
	entries := make([]relationoracle.DenominatorEntry, 0, len(fixture.RowsLeft()))
	scope := oracleCompleteScope(t)
	for _, rowID := range fixture.RowsLeft() {
		entry, ok := relationoracle.NewDenominatorEntry(rowID, scope)
		if !ok {
			t.Fatal("oracle denominator entry")
		}
		entries = append(entries, entry)
	}
	denominator, ok := relationoracle.NewDenominator(fixture.RelationLeft(), entries)
	if !ok {
		t.Fatal("oracle denominator")
	}
	columns := make([]relationoracle.ColumnType, 0, len(leftColumns(fixture)))
	types := fixture.Mounted().CodecTypes()
	if len(types) != 1 || !types[0].Available() {
		t.Fatal("oracle declared column type")
	}
	for _, column := range leftColumns(fixture) {
		columns = append(columns, relationoracle.NewColumnType(column, types[0]))
	}
	for _, column := range columns {
		if !column.Available() {
			t.Fatal("oracle denominator column")
		}
	}
	return relationoracle.Complete(oracleCompleteInput(t, fixture, input), denominator, columns)
}

func assertCompleteOracle(t testing.TB, fixture testfixture.Fixture, input, output tuple.Batch) {
	t.Helper()
	want := oracleCompleteExpected(t, fixture, input)
	if !want.Available() || output.Len() != len(want.Rows()) {
		t.Fatalf("complete oracle/physical cardinality = %v/%d, want %d", want.Available(), output.Len(), len(want.Rows()))
	}
	got := make(map[model.RowID]tuple.Tuple, output.Len())
	for index := 0; index < output.Len(); index++ {
		value, ok := output.At(index)
		if !ok {
			t.Fatal("complete output tuple")
		}
		rowID, ok := value.SourceFor(fixture.RelationLeft())
		if !ok {
			t.Fatal("complete output source row")
		}
		got[rowID] = value
	}
	for _, expected := range want.Rows() {
		value, ok := got[expected.ID()]
		if !ok {
			t.Fatalf("complete omitted row %v", expected.ID())
		}
		for _, column := range leftColumns(fixture) {
			wantCell, wantOK := expected.Cell(column)
			gotCell, gotOK := value.CellFor(column)
			if !wantOK || !gotOK || gotCell.Column() != wantCell.Column() || gotCell.Type() != wantCell.Type() || gotCell.Presence().Kind() != wantCell.Presence().Kind() {
				t.Fatalf("complete cell row=%v column=%v diverged", expected.ID(), column)
			}
			wantValue, wantValueOK := wantCell.Value()
			gotValue := gotCell.Value()
			if wantValueOK != gotValue.Available() || wantValueOK && wantValue.Content() != gotValue.Opaque() {
				t.Fatalf("complete value row=%v column=%v diverged", expected.ID(), column)
			}
		}
	}
}

func completeInputLineages(t testing.TB, fixture testfixture.Fixture, input tuple.Batch) map[model.RowID]model.LineageRef {
	t.Helper()
	lineages := make(map[model.RowID]model.LineageRef, input.Len())
	for index := 0; index < input.Len(); index++ {
		value, ok := input.At(index)
		if !ok {
			t.Fatal("complete input lineage tuple")
		}
		row, ok := value.SourceFor(fixture.RelationLeft())
		if !ok || !value.Lineage().Available() {
			t.Fatal("complete input lineage")
		}
		lineages[row] = value.Lineage()
	}
	return lineages
}

func TestExecuteMatchesCompleteOracleForPresentAndProvenAbsentRows(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	input := completeBatch(t, fixture, false, false)
	output, ok := complete.Execute(binding, fixture.Mounted(), input, completeWitness(t, fixture, binding.Denominator()))
	if !ok || !output.ValidFor(fixture.Mounted()) {
		t.Fatal("complete execution")
	}
	assertCompleteOracle(t, fixture, input, output)
	inputLineages := completeInputLineages(t, fixture, input)
	for index := 0; index < output.Len(); index++ {
		value, valueOK := output.At(index)
		if !valueOK {
			t.Fatal("complete output")
		}
		row, rowOK := value.SourceFor(fixture.RelationLeft())
		if !rowOK {
			t.Fatal("complete output row")
		}
		wantLineage, lineageOK := inputLineages[row]
		if !lineageOK {
			wantLineage, lineageOK = fixture.Mounted().RowLineage(row)
		}
		if !lineageOK || value.Lineage() != wantLineage {
			t.Fatalf("complete output row %v lost mounted lineage", row)
		}
	}
}

func TestExecuteCompleteOracleIsPermutationInvariant(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	firstInput := completeBatch(t, fixture, false, false)
	secondInput := completeBatch(t, fixture, true, false)
	first, firstOK := complete.Execute(binding, fixture.Mounted(), firstInput, completeWitness(t, fixture, binding.Denominator()))
	second, secondOK := complete.Execute(binding, fixture.Mounted(), secondInput, completeWitness(t, fixture, binding.Denominator()))
	if !firstOK || !secondOK || !first.Same(second) {
		t.Fatal("complete changed under input permutation")
	}
	assertCompleteOracle(t, fixture, secondInput, second)
}

func TestExecuteCompleteOracleClosesEmptyInput(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	input := completeBatch(t, fixture, false, true)
	output, ok := complete.Execute(binding, fixture.Mounted(), input, completeWitness(t, fixture, binding.Denominator()))
	if !ok || output.Len() != len(fixture.RowsLeft()) {
		t.Fatalf("empty complete=(%v,%d)", ok, output.Len())
	}
	assertCompleteOracle(t, fixture, input, output)
	for index := 0; index < output.Len(); index++ {
		value, valueOK := output.At(index)
		if !valueOK {
			t.Fatal("empty complete output")
		}
		for _, column := range leftColumns(fixture) {
			cell, cellOK := value.CellFor(column)
			if !cellOK || !cell.Presence().Is(model.ProvenAbsent) || cell.Value().Available() {
				t.Fatalf("empty complete fabricated value for %v", column)
			}
		}
	}
}

func TestExecuteCompleteRejectsMalformedInputWithoutOracleResult(t *testing.T) {
	fixture := testfixture.New(t, 0x81)
	binding, ok := fixture.CompleteBinding()
	if !ok {
		t.Fatal("complete binding")
	}
	if output, accepted := complete.Execute(binding, fixture.Mounted(), tuple.Batch{}, completeWitness(t, fixture, binding.Denominator())); accepted || output.Available() {
		t.Fatal("malformed input was accepted")
	}
}
