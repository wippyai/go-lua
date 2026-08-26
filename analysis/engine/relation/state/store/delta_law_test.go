package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestAggregateChangesProjectSemanticContentsWithoutSparseDefaults(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	nextColumn, columnDelta := lawSuccessor(t, fixture, 0, "semantic")
	next, aggregateDelta, ok := commit(base, columnDelta)
	if !ok || !next.Available() || !aggregateDelta.Available() || !nextColumn.Available() {
		t.Fatal("publish semantic aggregate")
	}
	if !aggregateDelta.Base().Same(base) || !aggregateDelta.Next().Same(next) || !aggregateDelta.Base().Fence().Same(fixture.fence) {
		t.Fatal("aggregate delta lost exact roots or fence")
	}
	changes := aggregateDelta.Changes()
	if len(changes) != 1 || changes[0].ColumnID() != fixture.columns[0] || changes[0].Empty() {
		t.Fatal("canonical column projection is incomplete")
	}
	entry, ok := changes[0].At(0)
	if !ok || !entry.SemanticChanged() || entry.Key() != geometry.Key(0) || !entry.Region().Valid() {
		t.Fatal("semantic change entry missing geometry")
	}
	if _, _, present := entry.Before(); present {
		t.Fatal("sparse predecessor was replaced with a fabricated default")
	}
	afterValue, afterPresence, present := entry.After()
	if !present || !afterValue.Available() || !afterPresence.Is(model.Present) {
		t.Fatal("successor semantic cell was not projected")
	}
	if got, ok := aggregateDelta.Change(fixture.columns[0]); !ok || !got.Available() || got.ColumnID() != fixture.columns[0] {
		t.Fatal("exact column lookup failed")
	}
	if _, ok := aggregateDelta.Change(fixture.columns[1]); ok {
		t.Fatal("unchanged column fabricated a change")
	}
}

func TestAggregateChangesAreCanonicalAndLineageIsFilteredByPredicate(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	nextA, deltaA := lawSuccessor(t, fixture, 0, "a")
	nextB, deltaB := lawSuccessor(t, fixture, 1, "b")
	forward, aggregateDelta, ok := commit(base, deltaB, deltaA)
	if !ok || !forward.Available() || !aggregateDelta.Available() {
		t.Fatal("publish batch aggregate")
	}
	changes := aggregateDelta.Changes()
	if len(changes) != 2 || changes[0].ColumnID() == changes[1].ColumnID() || changes[0].ColumnID() != aggregateDelta.ChangedColumnIDs()[0] || changes[1].ColumnID() != aggregateDelta.ChangedColumnIDs()[1] {
		t.Fatal("change projection order diverged from canonical IDs")
	}
	if got, ok := aggregateDelta.Change(fixture.columns[0]); !ok || got.ColumnID() != nextA.ID() {
		t.Fatal("column A projection missing")
	}
	if got, ok := aggregateDelta.Change(fixture.columns[1]); !ok || got.ColumnID() != nextB.ID() {
		t.Fatal("column B projection missing")
	}

	current, ok := forward.Column(fixture.columns[1])
	if !ok {
		t.Fatal("read seeded column")
	}
	cell, _ := lawCell(t, fixture, 1, "b")
	newLineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "lineage/only-projection"))
	if !ok {
		t.Fatal("issue lineage")
	}
	mask, ok := support.True(fixture.guards)
	if !ok {
		t.Fatal("construct support")
	}
	update, ok := column.NewUpdate(geometry.Key(1), mask, cell, newLineage)
	if !ok {
		t.Fatal("construct lineage update")
	}
	lineageColumn, lineageDelta, ok := current.Next(update)
	if !ok || !lineageDelta.Available() || lineageDelta.Empty() || lineageColumn.Revision() != current.Revision() {
		t.Fatal("construct lineage-only successor")
	}
	lineageAggregate, aggregateLineageDelta, ok := commit(forward, lineageDelta)
	if !ok || !lineageAggregate.Available() || !aggregateLineageDelta.Available() || len(aggregateLineageDelta.Changes()) != 1 || len(aggregateLineageDelta.SemanticColumnIDs()) != 0 || len(aggregateLineageDelta.LineageColumnIDs()) != 1 {
		t.Fatal("lineage-only change lost canonical projection")
	}
	lineageChange, ok := aggregateLineageDelta.Change(fixture.columns[1])
	if !ok || lineageChange.Len() != 1 {
		t.Fatal("lineage-only change missing canonical contents")
	}
	lineageEntry, ok := lineageChange.At(0)
	if !ok || lineageEntry.SemanticChanged() || !lineageEntry.LineageChanged() {
		t.Fatal("lineage-only change was not filtered by semantic predicate")
	}
}

func TestAggregateDeltaRetainsSemanticAndLineageOverlap(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	seedColumn, seedDelta := lawSuccessor(t, fixture, 0, "overlap-seed")
	seed, _, ok := commit(base, seedDelta)
	if !ok || !seed.Available() || !seedColumn.Available() {
		t.Fatal("publish overlap seed")
	}
	current, ok := seed.Column(fixture.columns[0])
	if !ok {
		t.Fatal("read overlap seed")
	}
	cell, _ := lawCell(t, fixture, 0, "overlap-semantic")
	newLineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "overlap-lineage"))
	if !ok {
		t.Fatal("issue overlap lineage")
	}
	mask, ok := support.True(fixture.guards)
	if !ok {
		t.Fatal("construct overlap support")
	}
	update, ok := column.NewUpdate(geometry.Key(0), mask, cell, newLineage)
	if !ok {
		t.Fatal("construct semantic+lineage update")
	}
	nextColumn, columnDelta, ok := current.Next(update)
	if !ok || !nextColumn.Available() || !columnDelta.Available() || columnDelta.Empty() || nextColumn.Revision() == current.Revision() || nextColumn.LineageRevision() == current.LineageRevision() {
		t.Fatal("construct semantic+lineage successor")
	}
	next, aggregateDelta, ok := commit(seed, columnDelta)
	if !ok || !next.Available() || !aggregateDelta.Available() {
		t.Fatal("publish semantic+lineage successor")
	}
	semantic := aggregateDelta.SemanticColumnIDs()
	lineage := aggregateDelta.LineageColumnIDs()
	if len(semantic) != 1 || semantic[0] != fixture.columns[0] || len(lineage) != 1 || lineage[0] != fixture.columns[0] {
		t.Fatal("semantic and lineage classifications lost overlap")
	}
	projected, ok := aggregateDelta.Change(fixture.columns[0])
	if !ok || !projected.Available() || projected.Empty() {
		t.Fatal("canonical projection missing for overlapping change")
	}
	entry, ok := projected.At(0)
	if !ok || !entry.SemanticChanged() || !entry.LineageChanged() {
		t.Fatal("overlapping change predicates were lost")
	}
}

func TestAggregateChangesAreCanonicalAndCarryLineageOnlyExtents(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	seedColumn, seedDelta := lawSuccessor(t, fixture, 0, "change-seed")
	seed, _, ok := commit(base, seedDelta)
	if !ok || !seedColumn.Available() {
		t.Fatal("publish change seed")
	}
	current, ok := seed.Column(fixture.columns[0])
	if !ok {
		t.Fatal("read change seed")
	}
	cell, _ := lawCell(t, fixture, 0, "change-seed")
	lineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "change-lineage-only"))
	if !ok {
		t.Fatal("issue change lineage")
	}
	update, ok := column.NewUpdate(geometry.Key(0), lawMask(t, fixture), cell, lineage)
	if !ok {
		t.Fatal("construct change update")
	}
	lineageColumn, lineageDelta, ok := current.Next(update)
	if !ok || !lineageColumn.Available() || lineageDelta.Empty() || lineageDelta.Len() != 1 {
		t.Fatal("construct lineage-only change")
	}
	next, aggregateDelta, ok := commit(seed, lineageDelta)
	if !ok || !next.Available() || !aggregateDelta.Available() {
		t.Fatal("publish lineage-only change")
	}
	changes := aggregateDelta.Changes()
	if len(changes) != 1 || changes[0].ColumnID() != fixture.columns[0] || changes[0].Len() != 1 {
		t.Fatal("lineage-only change was not projected")
	}
	change := changes[0]
	if !change.Base().Same(aggregateDelta.Base()) || !change.Next().Same(aggregateDelta.Next()) || !change.Fence().Same(aggregateDelta.Base().Fence()) {
		t.Fatal("change lost exact aggregate roots or fence")
	}
	entry, ok := change.At(0)
	if !ok || entry.SemanticChanged() || !entry.LineageChanged() {
		t.Fatal("lineage-only extent classification")
	}
	beforeValue, beforePresence, beforeOK := entry.Before()
	afterValue, afterPresence, afterOK := entry.After()
	if !beforeOK || !afterOK || !beforeValue.Same(afterValue) || beforePresence != afterPresence {
		t.Fatal("lineage-only extent did not preserve semantic sides")
	}
	beforeLineage, beforeLineageOK := entry.BeforeLineage()
	afterLineage, afterLineageOK := entry.AfterLineage()
	if !beforeLineageOK || !afterLineageOK || beforeLineage == afterLineage {
		t.Fatal("lineage-only extent did not preserve lineage sides")
	}
	if _, ok := aggregateDelta.Change(fixture.columns[1]); ok {
		t.Fatal("unchanged column fabricated a complete change")
	}
}
