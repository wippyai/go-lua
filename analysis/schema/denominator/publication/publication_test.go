package publication

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func relationCountRows(t testing.TB, values ...uint64) denominator.CountRows {
	t.Helper()
	entries := denominator.GeneratedRelationEntries()
	if len(values) > len(entries) {
		t.Fatalf("count fixture has %d values for %d declarations", len(values), len(entries))
	}
	rows := make([]denominator.CountRow, 0, len(values))
	for index, value := range values {
		row, ok := denominator.NewCountRow(entries[index].ID(), value)
		if !ok {
			t.Fatalf("relation %d did not admit count row", index)
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		t.Fatal("count rows did not seal")
	}
	return sealed
}

func completeRelationCountRows(t testing.TB, values ...uint64) denominator.CountRows {
	t.Helper()
	entries := denominator.GeneratedRelationEntries()
	all := make([]uint64, len(entries))
	copy(all, values)
	return relationCountRows(t, all...)
}

func TestBuildContentSumsMountsAndPublishesZeroCoverage(t *testing.T) {
	schemaID := identity.ContentID{1, 2, 3}
	first := completeRelationCountRows(t, 2, 0)
	second := completeRelationCountRows(t, 5)
	content, ok := BuildContent(schemaID, first, second)
	if !ok {
		t.Fatal("relation count content rejected")
	}
	entries := denominator.GeneratedRelationEntries()
	if len(content.Members) != len(entries) || len(content.Rows) != len(entries) {
		t.Fatalf("coverage = members %d, rows %d, declarations %d", len(content.Members), len(content.Rows), len(entries))
	}
	if content.Denominator != mustUniverse(schemaID) {
		t.Fatal("content denominator is not bound to the schema")
	}
	if got := content.Rows[entries[0].ID()]; got != 7 {
		t.Fatalf("summed first relation = %d, want 7", got)
	}
	if got := content.Rows[entries[1].ID()]; got != 0 {
		t.Fatalf("zero relation = %d, want explicit zero", got)
	}
	if got := content.Rows[entries[len(entries)-1].ID()]; got != 0 {
		t.Fatalf("unmentioned relation = %d, want explicit zero", got)
	}
	if _, ok := BuildContent(schemaID, relationCountRows(t, 2)); ok {
		t.Fatal("missing relation count was treated as zero")
	}
}

func TestBuildContentRejectsUnknownAndOverflowRows(t *testing.T) {
	entries := denominator.GeneratedRelationEntries()
	unknownID := schema.NewEntryID(schema.SurfaceKindDenominator, "not-generated")
	unknown, unknownOK := denominator.NewCountRow(unknownID, 1)
	if !unknownOK {
		t.Fatal("unknown fixture row did not construct")
	}
	unknownRows, unknownRowsOK := denominator.NewCountRows([]denominator.CountRow{unknown})
	if !unknownRowsOK {
		t.Fatal("unknown fixture rows did not seal")
	}
	if _, ok := BuildContent(identity.ContentID{4}, unknownRows); ok {
		t.Fatal("unknown relation count was accepted")
	}

	maximum, maximumOK := denominator.NewCountRow(entries[0].ID(), math.MaxUint64)
	one, oneOK := denominator.NewCountRow(entries[0].ID(), 1)
	if !maximumOK || !oneOK {
		t.Fatal("overflow fixture rows did not construct")
	}
	left, leftOK := denominator.NewCountRows([]denominator.CountRow{maximum})
	right, rightOK := denominator.NewCountRows([]denominator.CountRow{one})
	if !leftOK || !rightOK {
		t.Fatal("overflow fixture rows did not seal")
	}
	if _, ok := BuildContent(identity.ContentID{5}, left, right); ok {
		t.Fatal("overflowing relation count was accepted")
	}
}

func TestPublishUsesSnapshotColumn(t *testing.T) {
	schemaID := identity.ContentID{6, 7, 8}
	store := identity.StoreID(1)
	generation := identity.Generation(1)
	builder := snapshot.NewBuilder(schemaID, store, generation)
	axis := snapshot.Axis[schema.EntryID, uint64]{SchemaID: schemaID, Slot: 0}
	if err := Publish(&builder, axis, schemaID, completeRelationCountRows(t, 9)); err != nil {
		t.Fatalf("publish relation counts: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal relation counts: %v", err)
	}
	entry := denominator.GeneratedRelationEntries()[0]
	got, status := snapshot.Read(&sealed, axis, entry.ID())
	if status != snapshot.ReadHit || got != 9 {
		t.Fatalf("snapshot read = (%d, %s), want (9, hit)", got, status)
	}
	missing := denominator.GeneratedRelationEntries()[len(denominator.GeneratedRelationEntries())-1].ID()
	got, status = snapshot.Read(&sealed, axis, missing)
	if status != snapshot.ReadHit || got != 0 {
		t.Fatalf("zero snapshot read = (%d, %s), want (0, hit)", got, status)
	}
}

func TestPublishRejectsForeignSchemaAxis(t *testing.T) {
	declared := identity.ContentID{9, 9, 9}
	foreign := identity.ContentID{8, 8, 8}
	builder := snapshot.NewBuilder(declared, identity.StoreID(2), identity.Generation(1))
	axis := snapshot.Axis[schema.EntryID, uint64]{SchemaID: foreign, Slot: 0}
	if err := Publish(&builder, axis, declared, completeRelationCountRows(t)); err == nil {
		t.Fatal("foreign schema axis was accepted")
	}
}

func mustUniverse(schemaID identity.ContentID) identity.ContentID {
	id, ok := UniverseID(schemaID)
	if !ok {
		panic("unavailable test schema")
	}
	return id
}
