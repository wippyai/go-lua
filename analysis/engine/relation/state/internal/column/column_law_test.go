package column

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

type lawFixture struct {
	column *Column
	base   Version
	guards *guard.Manager
	issuer binding.Issuer
	typeID model.TypeID
	owner  model.OwnerID
}

func newLawFixture(t *testing.T) lawFixture {
	t.Helper()
	owner := issueOwner(t, "column")
	relation, ok := model.IssueRelationID(owner, lawContent(t, "relation"))
	if !ok {
		t.Fatal("issue relation")
	}
	columnID, ok := model.IssueColumnID(relation, lawContent(t, "column-id"))
	if !ok {
		t.Fatal("issue column")
	}
	typeID, ok := model.IssueTypeID(owner, lawContent(t, "type"))
	if !ok {
		t.Fatal("issue type")
	}
	schemaID, ok := model.IssueSchemaID(owner, lawContent(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}
	schema := model.DefineColumnSchema(columnID, typeID)
	fence, ok := binding.NewFence(schemaID, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("issue fence")
	}
	guards, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatalf("issue guards: %v", err)
	}
	column, ok := NewColumn(schema, fence, guards)
	if !ok {
		t.Fatal("construct column")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("construct issuer")
	}
	base := column.Initial()
	if !base.Available() {
		t.Fatal("initial version unavailable")
	}
	return lawFixture{column: column, base: base, guards: guards, issuer: issuer, typeID: typeID, owner: owner}
}

func lawContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("column/state-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(lawContent(t, "owner/"+label))
	if !ok {
		t.Fatalf("issue owner %q", label)
	}
	return owner
}

func newMask(t *testing.T, manager *guard.Manager, atom guard.Atom, value bool) support.Mask {
	t.Helper()
	work := support.New(manager)
	if work == nil {
		t.Fatal("open support work")
	}
	mask, ok := work.Literal(atom, value)
	if !ok || !work.Seal() {
		work.Discard()
		t.Fatal("seal support literal")
	}
	return mask
}

func newConjoinedMask(t *testing.T, base support.Mask, atom guard.Atom, value bool) support.Mask {
	t.Helper()
	work := support.New(base.Manager())
	if work == nil {
		t.Fatal("open support work")
	}
	mask, ok := work.Conjoin(base, atom, value)
	if !ok || !work.Seal() {
		work.Discard()
		t.Fatal("seal support conjunction")
	}
	return mask
}

func trueMask(t *testing.T, manager *guard.Manager) support.Mask {
	t.Helper()
	mask, ok := support.True(manager)
	if !ok {
		t.Fatal("construct true support")
	}
	return mask
}

func (fixture lawFixture) cell(t *testing.T, label string, presence model.PresenceKind) (Cell, model.LineageRef) {
	t.Helper()
	presenceValue, ok := model.NewPresence(presence)
	if !ok {
		t.Fatalf("construct presence %s", presence)
	}
	var value binding.ValueToken
	if presence == model.Present || presence == model.AuthenticatedOpaque {
		value, ok = fixture.issuer.IssueValue(fixture.typeID, lawContent(t, "value/"+label))
		if !ok {
			t.Fatal("issue value")
		}
	}
	cell, ok := NewCell(value, presenceValue)
	if !ok {
		t.Fatal("construct cell")
	}
	lineage, ok := model.IssueLineageRef(fixture.owner, lawContent(t, "lineage/"+label))
	if !ok {
		t.Fatal("issue lineage")
	}
	return cell, lineage
}

func newUpdate(t *testing.T, fixture lawFixture, key geometry.Key, mask support.Mask, label string) Update {
	t.Helper()
	cell, lineage := fixture.cell(t, label, model.Present)
	update, ok := NewUpdate(key, mask, cell, lineage)
	if !ok {
		t.Fatal("construct update")
	}
	return update
}

func readParts(t *testing.T, version Version, key geometry.Key, within support.Mask) []ReadPart {
	t.Helper()
	scratch := NewReadScratch(version.Guards())
	if scratch == nil {
		t.Fatal("construct read scratch")
	}
	parts := make([]ReadPart, 0, 8)
	completed, valid := version.Read(key, within, scratch, func(part ReadPart) bool {
		parts = append(parts, part)
		return true
	})
	if !completed || !valid {
		t.Fatal("read was not complete")
	}
	return parts
}

func TestColumnDisjointMasksRemainIndependentAndReadAllTerminals(t *testing.T) {
	fixture := newLawFixture(t)
	left := newMask(t, fixture.guards, 1, true)
	right := newMask(t, fixture.guards, 1, false)
	leftUpdate := newUpdate(t, fixture, geometry.Key(0), left, "left")
	rightUpdate := newUpdate(t, fixture, geometry.Key(0), right, "right")
	version, delta, ok := fixture.base.Next(rightUpdate, leftUpdate)
	if !ok || !version.Available() || delta.Empty() || delta.Len() != 2 {
		t.Fatalf("disjoint batch did not publish two semantic regions: ok=%v delta=%d", ok, delta.Len())
	}
	parts := readParts(t, version, geometry.Key(0), trueMask(t, fixture.guards))
	if len(parts) != 2 {
		t.Fatalf("read returned %d partitions, want 2", len(parts))
	}
	seenLeft, seenRight := false, false
	for _, part := range parts {
		switch {
		case part.Region().Equal(left):
			seenLeft = true
		case part.Region().Equal(right):
			seenRight = true
		default:
			t.Fatalf("read returned a scope-qualified or merged region")
		}
	}
	if !seenLeft || !seenRight {
		t.Fatalf("read lost one of the disjoint terminal partitions")
	}
}

func TestColumnRemovalPublishesSparseBeforeAfterAndExactLineageFlags(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	cell, lineage := fixture.cell(t, "removal", model.Present)
	seed, _, ok := fixture.base.Next(mustUpdate(t, geometry.Key(20), scope, cell, lineage))
	if !ok {
		t.Fatal("publish seed")
	}
	next, delta, ok := seed.Next(mustRemoval(t, geometry.Key(20), scope))
	if !ok || !next.SuccessorOf(seed) || !delta.Available() || delta.Len() != 1 {
		t.Fatalf("sparse removal did not publish one exact extent: ok=%v len=%d", ok, delta.Len())
	}
	entry, ok := delta.At(0)
	if !ok || !entry.Region().Equal(scope) {
		t.Fatal("removal lost exact support region")
	}
	if _, beforeOK := entry.Before(); !beforeOK {
		t.Fatal("removal lost predecessor semantic cell")
	}
	if _, afterOK := entry.After(); afterOK {
		t.Fatal("removal fabricated a successor semantic cell")
	}
	if _, beforeOK := entry.BeforeLineage(); !beforeOK {
		t.Fatal("removal lost predecessor lineage")
	}
	if _, afterOK := entry.AfterLineage(); afterOK {
		t.Fatal("removal fabricated successor lineage")
	}
	if !entry.SemanticChanged() || !entry.LineageChanged() {
		t.Fatal("removal did not mark both semantic and lineage changes")
	}
	if parts := readParts(t, next, geometry.Key(20), scope); len(parts) != 0 {
		t.Fatalf("sparse removal left %d readable partitions", len(parts))
	}
}

func TestColumnExplicitProvenAbsentRemainsPresent(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	cell, lineage := fixture.cell(t, "explicit-absence", model.ProvenAbsent)
	next, delta, ok := fixture.base.Next(mustUpdate(t, geometry.Key(21), scope, cell, lineage))
	if !ok || delta.Empty() {
		t.Fatal("explicit absence did not publish")
	}
	parts := readParts(t, next, geometry.Key(21), scope)
	if len(parts) != 1 || !parts[0].Cell().Presence().Is(model.ProvenAbsent) || parts[0].Cell().Value().Available() {
		t.Fatal("explicit ProvenAbsent was collapsed into sparse undefined")
	}
	entry, ok := delta.At(0)
	if !ok {
		t.Fatal("missing explicit-absence delta")
	}
	after, afterOK := entry.After()
	if !afterOK || !after.Presence().Is(model.ProvenAbsent) {
		t.Fatal("delta did not retain explicit ProvenAbsent successor")
	}
}

func TestColumnRemovalPreservesDisjointSupportSurvivor(t *testing.T) {
	fixture := newLawFixture(t)
	left := newMask(t, fixture.guards, 1, true)
	right := newMask(t, fixture.guards, 1, false)
	leftCell, leftLineage := fixture.cell(t, "remove-left", model.Present)
	rightCell, rightLineage := fixture.cell(t, "keep-right", model.Present)
	leftUpdate, ok := NewUpdate(geometry.Key(22), left, leftCell, leftLineage)
	if !ok {
		t.Fatal("construct left update")
	}
	rightUpdate, ok := NewUpdate(geometry.Key(22), right, rightCell, rightLineage)
	if !ok {
		t.Fatal("construct right update")
	}
	seed, _, ok := fixture.base.Next(leftUpdate, rightUpdate)
	if !ok {
		t.Fatal("publish disjoint seed")
	}
	next, delta, ok := seed.Next(mustRemoval(t, geometry.Key(22), left))
	if !ok || delta.Len() != 1 {
		t.Fatalf("partial removal did not publish one changed partition: ok=%v len=%d", ok, delta.Len())
	}
	parts := readParts(t, next, geometry.Key(22), trueMask(t, fixture.guards))
	if len(parts) != 1 || !parts[0].Region().Equal(right) || !parts[0].Cell().SemanticSame(rightCell) {
		t.Fatal("partial removal discarded the disjoint survivor")
	}
}

func mustRemoval(t *testing.T, key geometry.Key, mask support.Mask) Update {
	t.Helper()
	update, ok := NewRemoval(key, mask)
	if !ok {
		t.Fatal("construct removal")
	}
	return update
}

func TestColumnOverlappingReplacementPartitionsDeltaOnlyChangedRegion(t *testing.T) {
	fixture := newLawFixture(t)
	scope := newMask(t, fixture.guards, 1, true)
	nested := newConjoinedMask(t, scope, 2, true)
	first, _, ok := fixture.base.Next(newUpdate(t, fixture, geometry.Key(3), scope, "before"))
	if !ok {
		t.Fatal("publish predecessor")
	}
	second, delta, ok := first.Next(newUpdate(t, fixture, geometry.Key(3), nested, "after"))
	if !ok || delta.Empty() || delta.Len() != 1 {
		t.Fatalf("replacement produced delta len=%d ok=%v", delta.Len(), ok)
	}
	if !delta.Available() || !delta.Base().Same(first) || !delta.Next().Same(second) || delta.ColumnID() != fixture.column.ID() || delta.RelationID() != fixture.column.Relation() {
		t.Fatal("delta did not bind exact predecessor/successor roots")
	}
	entry, ok := delta.At(0)
	if !ok || !entry.Region().Equal(nested) {
		t.Fatal("delta did not retain exact overlapping replacement region")
	}
	before, beforeOK := entry.Before()
	after, afterOK := entry.After()
	if !beforeOK || !afterOK || !before.Presence().Is(model.Present) || !after.Presence().Is(model.Present) || before.SemanticSame(after) {
		t.Fatal("delta did not distinguish predecessor and successor semantic cells")
	}
	parts := readParts(t, second, geometry.Key(3), scope)
	if len(parts) != 2 {
		t.Fatalf("replacement read returned %d partitions, want 2", len(parts))
	}
	for _, part := range parts {
		if !part.Region().Entails(scope) {
			t.Fatal("replacement escaped its support scope")
		}
		if part.Region().Equal(nested) && !part.Cell().SemanticSame(after) {
			t.Fatal("nested replacement did not win on overlap")
		}
	}
}

func TestColumnLineageOnlyReplacementIsOneCanonicalExtent(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	cell, firstLineage := fixture.cell(t, "semantic", model.Present)
	firstUpdate, ok := NewUpdate(geometry.Key(4), scope, cell, firstLineage)
	if !ok {
		t.Fatal("construct first update")
	}
	first, _, ok := fixture.base.Next(firstUpdate)
	if !ok {
		t.Fatal("publish first version")
	}
	_, secondLineage := fixture.cell(t, "lineage-only", model.Present)
	secondUpdate, ok := NewUpdate(geometry.Key(4), scope, cell, secondLineage)
	if !ok {
		t.Fatal("construct lineage-only update")
	}
	second, delta, ok := first.Next(secondUpdate)
	if !ok || !second.SuccessorOf(first) || !delta.Available() || !delta.Base().Same(first) || !delta.Next().Same(second) || delta.Empty() || delta.Len() != 1 {
		t.Fatalf("lineage-only update did not publish one canonical extent: ok=%v empty=%v len=%d", ok, delta.Empty(), delta.Len())
	}
	if second.Revision() != first.Revision() || second.LineageRevision() != first.LineageRevision()+1 {
		t.Fatal("lineage-only update changed the wrong revision")
	}
	if second.Same(first) {
		t.Fatal("lineage-only update reused the whole publication identity")
	}
}

func TestColumnChangeStreamPreservesSparseLineageAndOverlap(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	cell, lineage := fixture.cell(t, "change-seed", model.Present)
	seed, _, ok := fixture.base.Next(mustUpdate(t, geometry.Key(13), scope, cell, lineage))
	if !ok {
		t.Fatal("publish change seed")
	}
	// A lineage-only replacement remains one complete atomic change extent;
	// semantic consumers filter its entry with SemanticChanged.
	_, lineageOnly := fixture.cell(t, "change-lineage-only", model.Present)
	lineageUpdate := mustUpdate(t, geometry.Key(13), scope, cell, lineageOnly)
	lineageNext, lineageDelta, ok := seed.Next(lineageUpdate)
	if !ok || !lineageNext.Available() || lineageDelta.Empty() || lineageDelta.Len() != 1 {
		t.Fatalf("lineage change empty=%v changes=%d", lineageDelta.Empty(), lineageDelta.Len())
	}
	lineageEntry, ok := lineageDelta.At(0)
	if !ok || lineageEntry.SemanticChanged() || !lineageEntry.LineageChanged() {
		t.Fatal("lineage-only extent classification was lost")
	}
	beforeCell, beforeCellOK := lineageEntry.Before()
	afterCell, afterCellOK := lineageEntry.After()
	beforeLineage, beforeLineageOK := lineageEntry.BeforeLineage()
	afterLineage, afterLineageOK := lineageEntry.AfterLineage()
	if !beforeCellOK || !afterCellOK || !beforeCell.SemanticSame(afterCell) || !beforeLineageOK || !afterLineageOK || beforeLineage == afterLineage {
		t.Fatal("lineage-only extent did not preserve unchanged semantic and both lineage sides")
	}

	// Semantic and lineage replacement over the same support is one extent,
	// not two parallel notifications.
	overlapCell, _ := fixture.cell(t, "change-overlap", model.Present)
	_, overlapLineage := fixture.cell(t, "change-overlap-lineage", model.Present)
	overlapNext, overlapDelta, ok := lineageNext.Next(mustUpdate(t, geometry.Key(13), scope, overlapCell, overlapLineage))
	if !ok || !overlapNext.Available() || overlapDelta.Len() != 1 {
		t.Fatalf("overlap changes=%d", overlapDelta.Len())
	}
	overlapEntry, ok := overlapDelta.At(0)
	if !ok || !overlapEntry.SemanticChanged() || !overlapEntry.LineageChanged() {
		t.Fatal("semantic+lineage overlap was not one complete extent")
	}

	// A first write retains sparse predecessor semantics and lineage rather
	// than manufacturing either side as a default.
	sparseNext, sparseDelta, ok := fixture.base.Next(mustUpdate(t, geometry.Key(14), scope, cell, lineage))
	if !ok || sparseDelta.Len() != 1 {
		t.Fatal("sparse change")
	}
	sparseEntry, ok := sparseDelta.At(0)
	if !ok {
		t.Fatal("sparse extent missing")
	}
	if _, present := sparseEntry.Before(); present {
		t.Fatal("sparse semantic predecessor was synthesized")
	}
	if _, present := sparseEntry.BeforeLineage(); present {
		t.Fatal("sparse lineage predecessor was synthesized")
	}
	if _, present := sparseEntry.After(); !present {
		t.Fatal("sparse semantic successor missing")
	}
	if _, present := sparseEntry.AfterLineage(); !present {
		t.Fatal("sparse lineage successor missing")
	}
	if !sparseNext.Available() {
		t.Fatal("sparse successor unavailable")
	}
}

func mustUpdate(t *testing.T, key geometry.Key, mask support.Mask, cell Cell, lineage model.LineageRef) Update {
	t.Helper()
	update, ok := NewUpdate(key, mask, cell, lineage)
	if !ok {
		t.Fatal("construct change update")
	}
	return update
}

func TestColumnNoOpSharesExactVersionRoots(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	update := newUpdate(t, fixture, geometry.Key(5), scope, "stable")
	first, _, ok := fixture.base.Next(update)
	if !ok {
		t.Fatal("publish first version")
	}
	second, delta, ok := first.Next(update)
	if !ok || !second.Same(first) || !delta.Empty() || second.SuccessorOf(first) {
		t.Fatal("no-op did not preserve exact predecessor roots")
	}
}

func TestColumnRejectsForeignMasksFencesAndInvalidBatchesAtomically(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	valid := newUpdate(t, fixture, geometry.Key(6), scope, "valid")
	foreignGuards, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal("construct foreign guards")
	}
	foreignScope := trueMask(t, foreignGuards)
	foreignMaskUpdate := newUpdate(t, fixture, geometry.Key(7), foreignScope, "foreign-mask")
	if _, _, ok := fixture.base.Next(valid, foreignMaskUpdate); ok {
		t.Fatal("foreign mask admitted into column")
	}
	parts := readParts(t, fixture.base, geometry.Key(6), scope)
	if len(parts) != 0 {
		t.Fatal("failed batch mutated predecessor")
	}
	foreignFence, ok := binding.NewFence(fixture.column.Fence().Schema(), identity.MountID{2}, identity.Generation(1))
	if !ok {
		t.Fatal("construct foreign fence")
	}
	foreignIssuer, ok := binding.NewIssuer(foreignFence)
	if !ok {
		t.Fatal("construct foreign issuer")
	}
	foreignValue, ok := foreignIssuer.IssueValue(fixture.typeID, lawContent(t, "foreign-value"))
	if !ok {
		t.Fatal("issue foreign value")
	}
	presence, _ := model.NewPresence(model.Present)
	foreignCell, ok := NewCell(foreignValue, presence)
	if !ok {
		t.Fatal("construct foreign cell")
	}
	foreignFenceUpdate, ok := NewUpdate(geometry.Key(8), scope, foreignCell, valid.Lineage())
	if !ok {
		t.Fatal("construct foreign-fence update")
	}
	if _, _, ok := fixture.base.Next(foreignFenceUpdate); ok {
		t.Fatal("foreign value fence admitted")
	}
	if _, _, ok := fixture.base.Next(valid, Update{}); ok {
		t.Fatal("invalid mixed batch admitted")
	}
}

func TestColumnRejectsConflictingOverlapsAndCanonicalizesPermutation(t *testing.T) {
	fixture := newLawFixture(t)
	left := newMask(t, fixture.guards, 1, true)
	right := newMask(t, fixture.guards, 1, false)
	first := newUpdate(t, fixture, geometry.Key(9), left, "first")
	second := newUpdate(t, fixture, geometry.Key(9), right, "second")
	forward, _, ok := fixture.base.Next(first, second)
	if !ok {
		t.Fatal("forward disjoint batch refused")
	}
	reverse, _, ok := fixture.base.Next(second, first)
	if !ok {
		t.Fatal("reverse disjoint batch refused")
	}
	forwardParts := readParts(t, forward, geometry.Key(9), trueMask(t, fixture.guards))
	reverseParts := readParts(t, reverse, geometry.Key(9), trueMask(t, fixture.guards))
	if len(forwardParts) != len(reverseParts) || len(forwardParts) != 2 {
		t.Fatal("permuted batch changed semantic partition count")
	}
	for _, expected := range forwardParts {
		found := false
		for _, actual := range reverseParts {
			if expected.Region().Equal(actual.Region()) && expected.Cell().SemanticSame(actual.Cell()) && expected.Lineage() == actual.Lineage() {
				found = true
			}
		}
		if !found {
			t.Fatal("permuted batch changed a semantic partition")
		}
	}
	overlap := newConjoinedMask(t, left, 2, true)
	conflict := newUpdate(t, fixture, geometry.Key(9), overlap, "conflict")
	if _, _, ok := fixture.base.Next(first, conflict); ok {
		t.Fatal("conflicting same-key overlap admitted")
	}
	duplicate, delta, ok := fixture.base.Next(first, first)
	if !ok || !duplicate.Available() || delta.Empty() {
		t.Fatal("exact duplicate mutation did not collapse")
	}
}

func TestColumnPresenceTerminalIsNotSparseDefault(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	cell, lineage := fixture.cell(t, "proven-absent", model.ProvenAbsent)
	update, ok := NewUpdate(geometry.Key(10), scope, cell, lineage)
	if !ok {
		t.Fatal("construct explicit absence update")
	}
	version, _, ok := fixture.base.Next(update)
	if !ok {
		t.Fatal("publish explicit absence")
	}
	parts := readParts(t, version, geometry.Key(10), scope)
	if len(parts) != 1 || !parts[0].Cell().Presence().Is(model.ProvenAbsent) || parts[0].Cell().Value().Available() {
		t.Fatal("explicit absence was synthesized or lost")
	}
	if got := readParts(t, version, geometry.Key(11), scope); len(got) != 0 {
		t.Fatal("sparse undefined lookup synthesized a default")
	}
}

func TestColumnBorrowedReadWarmPathAllocatesNothing(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	version, _, ok := fixture.base.Next(newUpdate(t, fixture, geometry.Key(12), scope, "borrowed"))
	if !ok {
		t.Fatal("publish borrowed read value")
	}
	scratch := NewReadScratch(version.Guards())
	scratch.semantic = make([]semanticPartition, 0, 2)
	scratch.lineage = make([]lineagePartition, 0, 2)
	visit := func(ReadPart) bool { return true }
	borrowed, ok := version.Borrow()
	if !ok || !borrowed.Available() {
		t.Fatal("open borrowed read")
	}
	if completed, valid := borrowed.Read(geometry.Key(12), scope, scratch, visit); !completed || !valid {
		t.Fatal("warm read setup failed")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if completed, valid := borrowed.Read(geometry.Key(12), scope, scratch, visit); !completed || !valid {
			t.Fatal("borrowed read failed")
		}
	})
	if allocs != 0 {
		t.Fatalf("borrowed read allocated %v times", allocs)
	}
}
