package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

type lawFixture struct {
	owner       model.OwnerID
	relation    model.RelationID
	columns     [2]model.ColumnID
	types       [2]model.TypeID
	schema      model.SchemaID
	fence       binding.Fence
	issuer      binding.Issuer
	guards      *guard.Manager
	initial     [2]column.Version
	arrangement lawArrangement
}

type lawArrangement struct{ digest identity.ContentID }

func (value lawArrangement) Available() bool            { return value.digest.Available() }
func (value lawArrangement) Digest() identity.ContentID { return value.digest }

func lawContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/state/store/law", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

func newLawFixture(t *testing.T) lawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(lawContent(t, "owner"))
	if !ok {
		t.Fatal("issue owner")
	}
	relation, ok := model.IssueRelationID(owner, lawContent(t, "relation"))
	if !ok {
		t.Fatal("issue relation")
	}
	var columns [2]model.ColumnID
	var types [2]model.TypeID
	for index := range columns {
		columns[index], ok = model.IssueColumnID(relation, lawContent(t, "column/"+string(rune('a'+index))))
		if !ok {
			t.Fatal("issue column")
		}
		types[index], ok = model.IssueTypeID(owner, lawContent(t, "type/"+string(rune('a'+index))))
		if !ok {
			t.Fatal("issue type")
		}
	}
	schema, ok := model.IssueSchemaID(owner, lawContent(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("issue fence")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issue issuer")
	}
	guards, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatalf("issue guards: %v", err)
	}
	var initial [2]column.Version
	for index := range initial {
		owned, ok := column.NewColumn(model.DefineColumnSchema(columns[index], types[index]), fence, guards)
		if !ok {
			t.Fatal("construct column")
		}
		initial[index] = owned.Initial()
		if !initial[index].Available() {
			t.Fatal("initial column unavailable")
		}
	}
	// Deliberately provide the census in a different order. NewVersion must
	// publish the canonical logical order rather than retain caller order.
	return lawFixture{
		owner: owner, relation: relation, columns: columns, types: types,
		schema: schema, fence: fence, issuer: issuer, guards: guards,
		initial: initial, arrangement: lawArrangement{digest: lawContent(t, "arrangement")},
	}
}

func lawMask(t *testing.T, fixture lawFixture) support.Mask {
	t.Helper()
	mask, ok := support.True(fixture.guards)
	if !ok {
		t.Fatal("construct true support")
	}
	return mask
}

func lawCell(t *testing.T, fixture lawFixture, index int, label string) (column.Cell, model.LineageRef) {
	t.Helper()
	value, ok := fixture.issuer.IssueValue(fixture.types[index], lawContent(t, "value/"+label))
	if !ok {
		t.Fatal("issue value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("construct presence")
	}
	cell, ok := column.NewCell(value, presence)
	if !ok {
		t.Fatal("construct cell")
	}
	lineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "lineage/"+label))
	if !ok {
		t.Fatal("issue lineage")
	}
	return cell, lineage
}

func lawUpdate(t *testing.T, fixture lawFixture, index int, label string) column.Update {
	t.Helper()
	cell, lineage := lawCell(t, fixture, index, label)
	update, ok := column.NewUpdate(geometry.Key(index), lawMask(t, fixture), cell, lineage)
	if !ok {
		t.Fatal("construct update")
	}
	return update
}

func lawSuccessor(t *testing.T, fixture lawFixture, index int, label string) (column.Version, column.Delta) {
	t.Helper()
	next, delta, ok := fixture.initial[index].Next(lawUpdate(t, fixture, index, label))
	if !ok || !next.Available() || !delta.Available() {
		t.Fatal("publish column successor")
	}
	return next, delta
}

func lawAggregate(t *testing.T, fixture lawFixture) Version {
	t.Helper()
	version, ok := newVersion(fixture.fence, lawContent(t, "mounted"), fixture.arrangement.Digest(), []model.ColumnID{fixture.columns[1], fixture.columns[0]}, fixture.initial[:])
	if !ok || !version.Available() {
		t.Fatal("construct aggregate")
	}
	return version
}

func commit(base Version, changes ...column.Delta) (Version, Delta, bool) {
	prepared, ok := Prepare(base, changes...)
	if !ok {
		return Version{}, Delta{}, false
	}
	return redeem(prepared)
}

// redeem is intentionally test-local. The store package laws can inspect the
// sealed candidate fields directly; production callers must compose and
// publish through database.Commit.
func redeem(prepared Prepared) (Version, Delta, bool) {
	if !prepared.Available() {
		return Version{}, Delta{}, false
	}
	if prepared.noop {
		return prepared.base, Delta{}, true
	}
	return prepared.next, prepared.delta, true
}

func TestNewVersionIsClosedWorldAndExactLookup(t *testing.T) {
	fixture := newLawFixture(t)
	version := lawAggregate(t, fixture)
	ids := version.ColumnIDs()
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatal("aggregate census is incomplete")
	}
	if ids[0] != fixture.columns[0] && ids[0] != fixture.columns[1] {
		t.Fatal("aggregate census contains an unexpected ID")
	}
	ids[0] = model.ColumnID{}
	if version.ColumnIDs()[0] == (model.ColumnID{}) {
		t.Fatal("aggregate exposed mutable ID storage")
	}
	if got, ok := version.Column(fixture.columns[0]); !ok || !got.Same(fixture.initial[0]) {
		t.Fatal("exact column lookup failed")
	}
	if _, ok := version.Column(model.ColumnID{}); ok {
		t.Fatal("zero column lookup succeeded")
	}

	if _, ok := newVersion(fixture.fence, lawContent(t, "mounted"), fixture.arrangement.Digest(), []model.ColumnID{fixture.columns[0]}, fixture.initial[:]); ok {
		t.Fatal("subset catalogue accepted")
	}
	if _, ok := newVersion(fixture.fence, lawContent(t, "mounted"), fixture.arrangement.Digest(), []model.ColumnID{fixture.columns[0], fixture.columns[0]}, fixture.initial[:]); ok {
		t.Fatal("duplicate catalogue ID accepted")
	}
	foreignMount := identity.MountID{2}
	foreignFence, ok := binding.NewFence(fixture.schema, foreignMount, fixture.fence.Generation())
	if !ok {
		t.Fatal("construct foreign fence")
	}
	if _, ok := newVersion(foreignFence, lawContent(t, "mounted"), fixture.arrangement.Digest(), []model.ColumnID{fixture.columns[0], fixture.columns[1]}, fixture.initial[:]); ok {
		t.Fatal("foreign initial fence accepted")
	}
	if _, ok := NewVersion(witness.Mounted{}, fixture.initial[:]); ok {
		t.Fatal("zero mounted catalogue accepted")
	}
	if version.MountedDigest() != lawContent(t, "mounted") || version.ArrangementDigest() != fixture.arrangement.Digest() {
		t.Fatal("root identity did not retain mounted and arrangement digests")
	}
}

func TestPrepareDoesNotPublishUntilCommit(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	_, delta := lawSuccessor(t, fixture, 0, "prepare-only")
	prepared, ok := Prepare(base, delta)
	if !ok || !prepared.Available() || prepared.Empty() || !prepared.Base().Same(base) {
		t.Fatal("prepare did not retain exact private candidate")
	}
	if base.Revision() != 1 || !base.Same(prepared.Base()) {
		t.Fatal("prepare changed or replaced the published base")
	}
	next, aggregate, ok := redeem(prepared)
	if !ok || !next.SuccessorOf(base) || !aggregate.Available() || !aggregate.Next().Same(next) {
		t.Fatal("commit did not publish exact prepared candidate")
	}
}

func TestCommitAuthenticatesExactRootsAndAncestry(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	nextColumn, nextDelta := lawSuccessor(t, fixture, 0, "first")
	forkColumn, forkDelta := lawSuccessor(t, fixture, 0, "fork")
	baseColumn, ok := base.Column(fixture.columns[0])
	if !ok || !nextDelta.Base().Same(baseColumn) || !nextDelta.Next().Same(nextColumn) || !nextColumn.SuccessorOf(baseColumn) {
		t.Fatal("column delta did not retain exact roots")
	}
	if !forkColumn.SuccessorOf(baseColumn) || forkColumn.Same(nextColumn) {
		t.Fatal("same-revision fork was not independent")
	}

	first, aggregateDelta, ok := commit(base, nextDelta)
	if !ok || !first.SuccessorOf(base) || first.Revision() != base.Revision()+1 || !aggregateDelta.Available() || !aggregateDelta.Base().Same(base) || !aggregateDelta.Next().Same(first) {
		t.Fatal("aggregate replacement lost exact ancestry")
	}
	forkAggregate, _, ok := commit(base, forkDelta)
	if !ok || forkAggregate.Revision() != first.Revision() || forkAggregate.Same(first) || !forkAggregate.SuccessorOf(base) || forkAggregate.SuccessorOf(first) || first.SuccessorOf(forkAggregate) {
		t.Fatal("aggregate same-revision fork was accepted as ancestry")
	}
	if _, _, ok := commit(first, forkDelta); ok {
		t.Fatal("stale fork delta replaced a different current root")
	}
	if _, _, ok := commit(first, nextDelta); ok {
		t.Fatal("stale exact delta was reused")
	}
}

func TestCommitBatchIsAtomicAndSharesUnchangedColumns(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	nextA, deltaA := lawSuccessor(t, fixture, 0, "batch-a")
	nextB, deltaB := lawSuccessor(t, fixture, 1, "batch-b")
	baseA, _ := base.Column(fixture.columns[0])
	baseB, _ := base.Column(fixture.columns[1])

	forward, aggregateDelta, ok := commit(base, deltaB, deltaA)
	if !ok || !forward.SuccessorOf(base) || forward.Revision() != base.Revision()+1 || !aggregateDelta.Available() {
		t.Fatal("multi-column replacement failed")
	}
	if got, ok := forward.Column(fixture.columns[0]); !ok || !got.Same(nextA) {
		t.Fatal("A successor not retained")
	}
	if got, ok := forward.Column(fixture.columns[1]); !ok || !got.Same(nextB) {
		t.Fatal("B successor not retained")
	}
	changed := aggregateDelta.ChangedColumnIDs()
	if len(changed) != 2 || changed[0] == changed[1] {
		t.Fatal("changed IDs were not deterministic")
	}
	permuted, permutedDelta, ok := commit(base, deltaA, deltaB)
	if !ok || !permutedDelta.Available() || len(permutedDelta.ChangedColumnIDs()) != len(changed) || permutedDelta.ChangedColumnIDs()[0] != changed[0] || permutedDelta.ChangedColumnIDs()[1] != changed[1] {
		t.Fatal("candidate permutation changed changed-ID order")
	}
	if !permuted.SuccessorOf(base) || permuted.Same(forward) {
		t.Fatal("permuted publication reused aggregate identity")
	}

	partial, partialDelta, ok := commit(base, deltaA)
	if !ok || !partialDelta.Available() {
		t.Fatal("single replacement failed")
	}
	if unchanged, ok := partial.Column(fixture.columns[1]); !ok || !unchanged.Same(baseB) {
		t.Fatal("unchanged column was not structurally shared")
	}
	if changedColumn, ok := partial.Column(fixture.columns[0]); !ok || changedColumn.Same(baseA) {
		t.Fatal("changed column remained the base handle")
	}
	if _, _, ok := commit(base, deltaA, deltaA); ok {
		t.Fatal("duplicate candidate accepted")
	}
	if _, _, ok := commit(base, column.Delta{}); ok {
		t.Fatal("missing candidate accepted")
	}
	if _, _, ok := commit(partial, deltaA, deltaB); ok {
		t.Fatal("mixed stale batch committed")
	}
	if !partial.Same(partial) || partial.Revision() != base.Revision()+1 {
		t.Fatal("failed batch changed its predecessor")
	}
}

func TestLineageOnlyChangeIsVisibleInCanonicalStream(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	semanticColumn, semanticDelta := lawSuccessor(t, fixture, 1, "seed")
	seed, _, ok := commit(base, semanticDelta)
	if !ok {
		t.Fatal("seed semantic column")
	}
	current, ok := seed.Column(fixture.columns[1])
	if !ok || !current.Same(semanticColumn) {
		t.Fatal("seed column missing")
	}
	cell, _ := lawCell(t, fixture, 1, "seed")
	newLineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "lineage/only"))
	if !ok {
		t.Fatal("issue lineage-only ref")
	}
	update, ok := column.NewUpdate(geometry.Key(1), lawMask(t, fixture), cell, newLineage)
	if !ok {
		t.Fatal("construct lineage-only update")
	}
	lineageColumn, lineageDelta, ok := current.Next(update)
	if !ok || !lineageDelta.Available() || lineageDelta.Empty() || lineageDelta.Len() != 1 || lineageColumn.Revision() != current.Revision() || lineageColumn.LineageRevision() == current.LineageRevision() {
		t.Fatal("column lineage-only successor did not expose one canonical extent")
	}
	published, aggregateDelta, ok := commit(seed, lineageDelta)
	if !ok || !aggregateDelta.Available() || len(aggregateDelta.SemanticColumnIDs()) != 0 || len(aggregateDelta.LineageColumnIDs()) != 1 || aggregateDelta.LineageColumnIDs()[0] != fixture.columns[1] {
		t.Fatal("aggregate hid lineage-only change")
	}
	if got, ok := published.Column(fixture.columns[1]); !ok || !got.Same(lineageColumn) {
		t.Fatal("lineage successor was not published")
	}
}

func TestCommitRejectsForeignFenceAtomically(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	foreignFence, ok := binding.NewFence(fixture.schema, identity.MountID{9}, fixture.fence.Generation())
	if !ok {
		t.Fatal("construct foreign fence")
	}
	foreignColumn, ok := column.NewColumn(model.DefineColumnSchema(fixture.columns[0], fixture.types[0]), foreignFence, fixture.guards)
	if !ok {
		t.Fatal("construct foreign column")
	}
	foreignIssuer, ok := binding.NewIssuer(foreignFence)
	if !ok {
		t.Fatal("construct foreign issuer")
	}
	foreignValue, ok := foreignIssuer.IssueValue(fixture.types[0], lawContent(t, "foreign-value"))
	if !ok {
		t.Fatal("issue foreign value")
	}
	presence, _ := model.NewPresence(model.Present)
	cell, ok := column.NewCell(foreignValue, presence)
	if !ok {
		t.Fatal("construct foreign cell")
	}
	lineage, _ := model.IssueLineageRef(fixture.owner, lawContent(t, "foreign-lineage"))
	update, ok := column.NewUpdate(geometry.Key(0), lawMask(t, fixture), cell, lineage)
	if !ok {
		t.Fatal("construct foreign update")
	}
	_, foreignDelta, ok := foreignColumn.Initial().Next(update)
	if !ok || !foreignDelta.Available() {
		t.Fatal("construct foreign delta")
	}
	before := base
	if _, _, ok := commit(base, foreignDelta); ok {
		t.Fatal("foreign fence delta accepted")
	}
	if !base.Same(before) || base.Revision() != 1 {
		t.Fatal("foreign rejection mutated aggregate")
	}
}
