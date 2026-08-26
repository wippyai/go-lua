package read_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestChangeReaderSemanticRowsAreSuccessorOwnedCanonicalRows(t *testing.T) {
	fixture := testfixture.New(t)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok || !handle.Available() {
		t.Fatal("bind semantic change reader")
	}

	rows := scanChangeRows(t, handle)
	if len(rows) != len(fixture.RowsLeft()) {
		t.Fatalf("semantic row count=%d, want %d", len(rows), len(fixture.RowsLeft()))
	}
	for index, row := range rows {
		if !handle.Reader().Owns(row) {
			t.Fatalf("row %d is not owned by successor reader", index)
		}
		if _, ok := tuple.Input(fixture.Mounted(), handle.Reader(), row); !ok {
			t.Fatalf("row %d is not tuple.Input-compatible", index)
		}
		for prior := range rows[:index] {
			if rows[prior].ID() == row.ID() && rows[prior].Scope().Same(row.Scope()) {
				t.Fatalf("duplicate semantic extent for row %v", row.ID())
			}
		}
	}
}

func TestChangeReaderLineageOnlyChangeEmits(t *testing.T) {
	fixture := testfixture.New(t)
	base := fixture.LeftRoot()
	columnID := fixture.PayloadColumnsLeft()[0]
	rowID := fixture.RowsLeft()[0]
	part := firstPart(t, fixture, base, columnID, rowID)
	lineage := freshLineage(t, "change-reader-lineage-only")
	_, delta := publishCellUpdate(t, fixture, base, columnID, part, part.Value(), part.Presence(), lineage)
	if len(delta.SemanticColumnIDs()) != 0 || len(delta.LineageColumnIDs()) != 1 {
		t.Fatal("expected lineage-only aggregate delta")
	}
	change, ok := delta.Change(columnID)
	if !ok || change.Len() == 0 {
		t.Fatal("lineage-only column change missing")
	}

	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok || !handle.Available() {
		t.Fatal("bind lineage-only change reader")
	}
	rows := scanChangeRows(t, handle)
	if len(rows) == 0 {
		t.Fatal("lineage-only change emitted no row")
	}
	found := false
	for _, row := range rows {
		for _, cell := range row.Cells() {
			if cell.Lineage() == lineage {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("lineage-only successor lineage did not reach canonical row")
	}
}

func TestChangeReaderOverlappingMultiColumnMasksCoalesce(t *testing.T) {
	fixture := testfixture.New(t)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	relevant := 0
	for _, change := range delta.Changes() {
		if change.RelationID() == fixture.RelationLeft() {
			relevant++
		}
	}
	if relevant < len(fixture.KeyColumnsLeft())+len(fixture.PayloadColumnsLeft()) {
		t.Fatalf("fixture did not provide multi-column overlap: %d", relevant)
	}
	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("bind overlapping change reader")
	}
	rows := scanChangeRows(t, handle)
	if len(rows) != len(fixture.RowsLeft()) {
		t.Fatalf("coalesced row count=%d, want %d", len(rows), len(fixture.RowsLeft()))
	}
	seen := make(map[model.RowID]bool, len(rows))
	for _, row := range rows {
		if seen[row.ID()] {
			t.Fatalf("overlapping masks emitted duplicate row %v", row.ID())
		}
		seen[row.ID()] = true
	}
}

func TestChangeReaderForeignRelationIsAvailableAndEmpty(t *testing.T) {
	fixture := testfixture.New(t)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	handle, ok := read.BindChanges(delta, fixture.LayoutRightPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok || !handle.Available() {
		t.Fatal("foreign relation should bind as an empty reader")
	}
	count := 0
	completed, valid := handle.ScanChanges(func(read.RowChange) bool {
		count++
		return true
	})
	if !completed || !valid || count != 0 {
		t.Fatalf("foreign relation scan=(%v,%v), count=%d", completed, valid, count)
	}
}

func TestChangeReaderPreservesSparseUndefinedAndExplicitAbsence(t *testing.T) {
	fixture := testfixture.New(t)
	sparseDelta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	sawSparse := false
	for _, change := range sparseDelta.Changes() {
		if change.RelationID() != fixture.RelationLeft() {
			continue
		}
		for index := 0; index < change.Len(); index++ {
			entry, ok := change.At(index)
			if !ok {
				t.Fatal("read sparse change entry")
			}
			_, afterPresence, afterOK := entry.After()
			if !afterOK || !afterPresence.Is(model.Present) {
				t.Fatal("successor present cell missing from sparse change")
			}
			if _, _, beforeOK := entry.Before(); !beforeOK {
				sawSparse = true
			}
		}
	}
	if !sawSparse {
		t.Fatal("fixture did not expose an undefined sparse predecessor")
	}

	base := fixture.Base()
	columnID := fixture.ApplyFactColumn()
	seed, _ := publishNewCell(t, fixture, base, columnID, fixture.RowApply(), "explicit-absence-seed")
	part := firstPart(t, fixture, seed, columnID, fixture.RowApply())
	absent, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		t.Fatal("explicit absence presence")
	}
	_, delta := publishCellUpdate(t, fixture, seed, columnID, part, binding.ValueToken{}, absent, part.Lineage())
	handle, ok := read.BindChanges(delta, fixture.LayoutApplyFact(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("bind explicit absence reader")
	}
	rows := scanChangeRows(t, handle)
	foundAbsent := false
	for _, row := range rows {
		for _, cell := range row.Cells() {
			if cell.Column() == columnID {
				if !cell.Presence().Is(model.ProvenAbsent) || cell.Value().Available() {
					t.Fatal("explicit absence changed its independent value/presence contract")
				}
				foundAbsent = true
			}
		}
	}
	if !foundAbsent {
		t.Fatal("explicit ProvenAbsent cell was omitted")
	}
}

func TestChangeReaderRefusesStaleAndForeignDeltaLayoutViews(t *testing.T) {
	fixture := testfixture.New(t)
	foreign := testfixture.New(t, 0x72)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	if _, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), foreign.Geometry(), foreign.Scratch()); ok {
		t.Fatal("foreign geometry redeemed the delta")
	}
	if _, ok := read.BindChanges(delta, foreign.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch()); ok {
		t.Fatal("foreign layout redeemed the delta")
	}
	if _, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), foreign.Scratch()); ok {
		t.Fatal("foreign scratch redeemed the delta")
	}
}

func TestChangeReaderVisitorStopSemantics(t *testing.T) {
	fixture := testfixture.New(t)
	delta, ok := fixture.BaseToLeftDelta()
	if !ok {
		t.Fatal("base-to-left delta")
	}
	handle, ok := read.BindChanges(delta, fixture.LayoutLeftPayload(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("bind change reader")
	}
	visits := 0
	completed, valid := handle.ScanChanges(func(read.RowChange) bool {
		visits++
		return false
	})
	if completed || !valid || visits != 1 {
		t.Fatalf("visitor stop=(%v,%v), visits=%d", completed, valid, visits)
	}
}

func scanChangeRows(t *testing.T, handle read.ChangeReader) []read.Row {
	t.Helper()
	rows := make([]read.Row, 0)
	completed, valid := handle.ScanChanges(func(change read.RowChange) bool {
		row, present := change.After()
		if !present || row == nil || !row.Available() {
			t.Fatal("unavailable changed row")
		}
		rows = append(rows, row)
		return true
	})
	if !completed || !valid {
		t.Fatalf("change scan=(%v,%v)", completed, valid)
	}
	return rows
}

func firstPart(t *testing.T, fixture testfixture.Fixture, root database.Version, columnID model.ColumnID, rowID model.RowID) store.ReadPart {
	t.Helper()
	index, ok := fixture.Mounted().RowIndex(rowID.Relation(), rowID)
	if !ok {
		t.Fatal("row index")
	}
	within, ok := fixture.Geometry().Universe()
	if !ok {
		t.Fatal("geometry universe")
	}
	var result store.ReadPart
	found := false
	completed, valid := root.Store().Read(columnID, geometry.Key(index), within, fixture.Scratch(), func(part store.ReadPart) bool {
		if !found {
			result, found = part, true
		}
		return true
	})
	if !completed || !valid || !found || !result.Region().Valid() {
		t.Fatal("read source part")
	}
	return result
}

func publishCellUpdate(t *testing.T, fixture testfixture.Fixture, base database.Version, columnID model.ColumnID, part store.ReadPart, value binding.ValueToken, presence model.Presence, lineage model.LineageRef) (database.Version, database.Delta) {
	t.Helper()
	cell, ok := column.NewCell(value, presence)
	if !ok {
		t.Fatal("construct update cell")
	}
	owned, ok := base.Store().Column(columnID)
	if !ok {
		t.Fatal("source column")
	}
	update, ok := column.NewUpdate(part.Key(), part.Region(), cell, lineage)
	if !ok {
		t.Fatal("construct column update")
	}
	_, delta, ok := owned.Next(update)
	if !ok || !delta.Available() {
		t.Fatal("construct column delta")
	}
	preparedStore, ok := store.Prepare(base.Store(), delta)
	if !ok || !preparedStore.Available() {
		t.Fatal("prepare store successor")
	}
	prepared, ok := database.Prepare(base, preparedStore, fixture.Scratch(), base.ContributionDirectory(), base.ContributionState(), nil)
	if !ok || !prepared.Available() {
		t.Fatal("prepare database successor")
	}
	next, aggregate, ok := database.Commit(prepared)
	if !ok || !next.Available() || !aggregate.Available() {
		t.Fatal("commit database successor")
	}
	return next, aggregate
}

func publishNewCell(t *testing.T, fixture testfixture.Fixture, base database.Version, columnID model.ColumnID, row model.RowID, label string) (database.Version, database.Delta) {
	t.Helper()
	denominator, ok := model.NewDenominatorRef(fixture.RelationApply(), fixture.KeyApply())
	if !ok {
		t.Fatal("apply denominator")
	}
	witnessValue, ok := fixture.Mounted().Denominator(denominator)
	if !ok {
		t.Fatal("apply denominator witness")
	}
	scope, _ := fixture.OverlapScopes()
	cellToken, ok := fixture.Mounted().IssueCell(witnessValue, scope, columnID, row)
	if !ok {
		t.Fatal("apply cell token")
	}
	coordinate, ok := fixture.Geometry().Resolve(cellToken)
	if !ok || !coordinate.Available() {
		t.Fatal("apply coordinate")
	}
	columnVersion, ok := base.Store().Column(columnID)
	if !ok {
		t.Fatal("apply column")
	}
	content, ok := identity.DeriveContentID("analysis/engine/relation/state/read/new-cell/v1", []byte(label))
	if !ok {
		t.Fatal("apply value identity")
	}
	value, ok := fixture.Mounted().IssueValue(columnVersion.Type(), content)
	if !ok {
		t.Fatal("apply value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("present")
	}
	cell, ok := column.NewCell(value, presence)
	if !ok {
		t.Fatal("apply cell")
	}
	lineage := freshLineage(t, "new-cell/"+label)
	update, ok := column.NewUpdate(coordinate.Dense(), coordinate.Mask(), cell, lineage)
	if !ok {
		t.Fatal("apply update")
	}
	_, delta, ok := columnVersion.Next(update)
	if !ok || !delta.Available() {
		t.Fatal("apply column delta")
	}
	changes := []column.Delta{delta}
	if columnID == fixture.ApplyFactColumn() {
		valueColumnID := fixture.ApplyValueColumn()
		valueVersion, valueOK := base.Store().Column(valueColumnID)
		if !valueOK {
			t.Fatal("apply value column")
		}
		valueContent, valueOK := identity.DeriveContentID("analysis/engine/relation/state/read/new-cell/value/v1", []byte(label))
		if !valueOK {
			t.Fatal("apply value identity")
		}
		valueToken, valueOK := fixture.Mounted().IssueValue(valueVersion.Type(), valueContent)
		if !valueOK {
			t.Fatal("apply value token")
		}
		valueCell, valueOK := column.NewCell(valueToken, presence)
		if !valueOK {
			t.Fatal("apply value cell")
		}
		valueUpdate, valueOK := column.NewUpdate(coordinate.Dense(), coordinate.Mask(), valueCell, freshLineage(t, "new-cell/value/"+label))
		if !valueOK {
			t.Fatal("apply value update")
		}
		_, valueDelta, valueOK := valueVersion.Next(valueUpdate)
		if !valueOK || !valueDelta.Available() {
			t.Fatal("apply value delta")
		}
		changes = append(changes, valueDelta)
	}
	preparedStore, ok := store.Prepare(base.Store(), changes...)
	if !ok || !preparedStore.Available() {
		t.Fatal("prepare apply store")
	}
	prepared, ok := database.Prepare(base, preparedStore, fixture.Scratch(), base.ContributionDirectory(), base.ContributionState(), nil)
	if !ok || !prepared.Available() {
		t.Fatal("prepare apply database")
	}
	next, aggregate, ok := database.Commit(prepared)
	if !ok || !next.Available() || !aggregate.Available() {
		t.Fatal("commit apply cell")
	}
	return next, aggregate
}

func freshLineage(t *testing.T, label string) model.LineageRef {
	t.Helper()
	ownerContent, ok := identity.DeriveContentID("analysis/engine/relation/state/read/law-owner/v1", []byte(label))
	if !ok {
		t.Fatal("lineage owner content")
	}
	owner, ok := model.IssueOwnerID(ownerContent)
	if !ok {
		t.Fatal("lineage owner")
	}
	content, ok := identity.DeriveContentID("analysis/engine/relation/state/read/law-lineage/v1", []byte(label))
	if !ok {
		t.Fatal("lineage content")
	}
	lineage, ok := model.IssueLineageRef(owner, content)
	if !ok {
		t.Fatal("lineage")
	}
	return lineage
}
