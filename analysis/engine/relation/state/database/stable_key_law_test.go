package database_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/internal/column"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestDatabaseStableKeyAbsentToPresentIsAllowed(t *testing.T) {
	fixture := testfixture.New(t)
	columnID := fixture.KeyColumnsLeft()[0]
	coordinate, typeID, lineage := stableLawCoordinate(t, fixture, columnID)
	cell := stableLawPresentCell(t, fixture.Mounted(), typeID, "absent-to-present")

	source := stableLawSource(t, fixture.Base(), columnID, coordinate, cell, lineage)
	next, delta, ok := stableLawPublish(fixture, fixture.Base(), source)
	if !ok || !next.SuccessorOf(fixture.Base()) || !delta.Available() {
		t.Fatal("an unavailable key cell could not become available")
	}
	change, ok := delta.Change(columnID)
	if !ok || !change.Available() {
		t.Fatal("key change was not published")
	}
	entry, ok := change.At(0)
	if !ok {
		t.Fatal("key change lost its canonical extent")
	}
	if _, _, beforeOK := entry.Before(); beforeOK {
		t.Fatal("absent key unexpectedly carried a predecessor value")
	}
	if after, _, afterOK := entry.After(); !afterOK || !after.Available() {
		t.Fatal("present key successor was not retained")
	}
}

func TestDatabaseStableKeyOwnerEqualValueIsPreserved(t *testing.T) {
	fixture := testfixture.New(t)
	columnID := fixture.KeyColumnsLeft()[0]
	coordinate, typeID, lineage := stableLawCoordinate(t, fixture, columnID)
	value := stableLawValue(t, fixture.Mounted(), typeID, "owner-equal")
	cell := stableLawCell(t, value, model.Present)

	seedSource := stableLawSource(t, fixture.Base(), columnID, coordinate, cell, lineage)
	seed, seedDelta, ok := stableLawPublish(fixture, fixture.Base(), seedSource)
	if !ok || !seedDelta.Available() {
		t.Fatal("key seed")
	}

	// A lineage-only successor keeps the exact semantic key value while still
	// forcing the raw store candidate through database.Prepare.
	lineageAuthority, ok := fixture.Mounted().Lineage()
	if !ok || lineageAuthority == nil {
		t.Fatal("lineage authority")
	}
	otherDenominator, ok := model.NewDenominatorRef(fixture.RelationRight(), fixture.KeyRight())
	if !ok {
		t.Fatal("other denominator")
	}
	otherLineage, ok := fixture.Mounted().DenominatorLineage(otherDenominator)
	if !ok || !lineageAuthority.Validate(otherLineage) {
		t.Fatal("other lineage")
	}
	equalSource := stableLawSource(t, seed, columnID, coordinate, cell, otherLineage)
	next, delta, ok := stableLawPublish(fixture, seed, equalSource)
	if !ok || !next.SuccessorOf(seed) || !delta.Available() {
		t.Fatal("owner-equal key value was not preserved")
	}
}

func TestDatabaseStableKeyChangedValueIsRefusedAtomically(t *testing.T) {
	fixture := testfixture.New(t)
	keyID := fixture.KeyColumnsLeft()[0]
	payloadID := fixture.PayloadColumnsLeft()[0]
	keyCoordinate, keyType, keyLineage := stableLawCoordinate(t, fixture, keyID)
	payloadCoordinate, payloadType, payloadLineage := stableLawCoordinate(t, fixture, payloadID)

	seedCell := stableLawPresentCell(t, fixture.Mounted(), keyType, "changed-seed")
	seedSource := stableLawSource(t, fixture.Base(), keyID, keyCoordinate, seedCell, keyLineage)
	seed, seedDelta, ok := stableLawPublish(fixture, fixture.Base(), seedSource)
	if !ok || !seedDelta.Available() {
		t.Fatal("key seed")
	}

	changedKey := stableLawPresentCell(t, fixture.Mounted(), keyType, "changed-value")
	payload := stableLawPresentCell(t, fixture.Mounted(), payloadType, "changed-payload")
	keyDelta := stableLawColumnDelta(t, seed, keyID, keyCoordinate, changedKey, keyLineage)
	payloadDelta := stableLawColumnDelta(t, seed, payloadID, payloadCoordinate, payload, payloadLineage)
	source := stableLawSource(t, seed, keyDelta, payloadDelta)
	prepared, ok := database.Prepare(seed, source, fixture.Scratch(), seed.ContributionDirectory(), seed.ContributionState(), nil)
	if ok || prepared.Available() {
		t.Fatal("changed key crossed the atomic database publication boundary")
	}
}

func TestDatabaseStableJoinCorrespondenceChangedValueIsRefusedAtomically(t *testing.T) {
	fixture := testfixture.New(t)
	columnID := fixture.PayloadColumnsLeft()[0]
	coordinate, typeID, lineage := stableLawCoordinate(t, fixture, columnID)
	seedCell := stableLawPresentCell(t, fixture.Mounted(), typeID, "join-correspondence-seed")
	seedSource := stableLawSource(t, fixture.Base(), columnID, coordinate, seedCell, lineage)
	seed, seedDelta, ok := stableLawPublish(fixture, fixture.Base(), seedSource)
	if !ok || !seedDelta.Available() {
		t.Fatal("join correspondence seed")
	}
	changed := stableLawPresentCell(t, fixture.Mounted(), typeID, "join-correspondence-changed")
	changedSource := stableLawSource(t, seed, columnID, coordinate, changed, lineage)
	prepared, ok := database.Prepare(seed, changedSource, fixture.Scratch(), seed.ContributionDirectory(), seed.ContributionState(), nil)
	if ok || prepared.Available() {
		t.Fatal("changed Join correspondence crossed the atomic database boundary")
	}
}

func TestDatabaseStableKeyAvailableToUnavailableIsRefused(t *testing.T) {
	fixture := testfixture.New(t)
	columnID := fixture.KeyColumnsLeft()[0]
	coordinate, typeID, lineage := stableLawCoordinate(t, fixture, columnID)
	seedCell := stableLawPresentCell(t, fixture.Mounted(), typeID, "available-seed")
	seedSource := stableLawSource(t, fixture.Base(), columnID, coordinate, seedCell, lineage)
	seed, seedDelta, ok := stableLawPublish(fixture, fixture.Base(), seedSource)
	if !ok || !seedDelta.Available() {
		t.Fatal("key seed")
	}

	absent := stableLawCell(t, binding.ValueToken{}, model.ProvenAbsent)
	source := stableLawSource(t, seed, columnID, coordinate, absent, lineage)
	prepared, ok := database.Prepare(seed, source, fixture.Scratch(), seed.ContributionDirectory(), seed.ContributionState(), nil)
	if ok || prepared.Available() {
		t.Fatal("available key became unavailable")
	}
}

func TestDatabaseMergeFactPayloadAscentRemainsAdmitted(t *testing.T) {
	fixture := testfixture.New(t)
	var lookup arrangement.Layout
	for _, candidate := range fixture.Base().Layouts() {
		if !candidate.Available() || candidate.CoordinateClass() != arrangement.CoordinateClassLookupOnly || candidate.Access().Relation() != fixture.RelationApply() {
			continue
		}
		containsFact := false
		for _, column := range candidate.Access().Columns() {
			if column == fixture.ApplyFactColumn() {
				containsFact = true
				break
			}
		}
		if containsFact {
			lookup = candidate
			break
		}
	}
	if !lookup.Available() || lookup.CoordinateClass() != arrangement.CoordinateClassLookupOnly {
		t.Fatal("fixture Merge fact has no exact sealed lookup-only vector")
	}
	columnID := fixture.ApplyFactColumn()
	coordinate, typeID, lineage := stableLawCoordinateFor(t, fixture, fixture.RelationApply(), fixture.KeyApply(), fixture.RowApply(), columnID)
	first := stableLawPresentCell(t, fixture.Mounted(), typeID, "non-key-first")
	firstSource := stableLawSource(t, fixture.Base(), columnID, coordinate, first, lineage)
	seed, seedDelta, ok := stableLawPublish(fixture, fixture.Base(), firstSource)
	if !ok || !seedDelta.Available() {
		t.Fatal("non-key seed")
	}

	second := stableLawPresentCell(t, fixture.Mounted(), typeID, "non-key-second")
	secondSource := stableLawSource(t, seed, columnID, coordinate, second, lineage)
	next, delta, ok := stableLawPublish(fixture, seed, secondSource)
	if !ok || !next.SuccessorOf(seed) || !delta.Available() {
		t.Fatal("non-key ascent was refused")
	}
}

func stableLawCoordinate(t *testing.T, fixture testfixture.Fixture, columnID model.ColumnID) (geometry.Coordinate, model.TypeID, model.LineageRef) {
	return stableLawCoordinateFor(t, fixture, fixture.RelationLeft(), fixture.KeyLeft(), fixture.RowsLeft()[0], columnID)
}

func stableLawCoordinateFor(t *testing.T, fixture testfixture.Fixture, relation model.RelationID, key model.KeyID, row model.RowID, columnID model.ColumnID) (geometry.Coordinate, model.TypeID, model.LineageRef) {
	t.Helper()
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	witnessValue, ok := fixture.Mounted().Denominator(denominator)
	if !ok {
		t.Fatal("denominator witness")
	}
	scope, _ := fixture.OverlapScopes()
	cell, ok := fixture.Mounted().IssueCell(witnessValue, scope, columnID, row)
	if !ok {
		t.Fatal("cell token")
	}
	coordinate, ok := fixture.Geometry().Resolve(cell)
	if !ok || !coordinate.Available() {
		t.Fatal("coordinate")
	}
	version, ok := fixture.Base().Store().Column(columnID)
	if !ok || !version.Available() {
		t.Fatal("column")
	}
	lineage, ok := fixture.Mounted().DenominatorLineage(denominator)
	if !ok {
		t.Fatal("lineage")
	}
	return coordinate, version.Type(), lineage
}

func stableLawValue(t *testing.T, mounted interface {
	IssueValue(model.TypeID, identity.ContentID) (binding.ValueToken, bool)
}, typeID model.TypeID, label string) binding.ValueToken {
	t.Helper()
	content, ok := identity.DeriveContentID("analysis/engine/relation/state/database/stable-key-law/v1", []byte(label))
	if !ok {
		t.Fatal("value identity")
	}
	value, ok := mounted.IssueValue(typeID, content)
	if !ok {
		t.Fatal("value")
	}
	return value
}

func stableLawPresentCell(t *testing.T, mounted interface {
	IssueValue(model.TypeID, identity.ContentID) (binding.ValueToken, bool)
}, typeID model.TypeID, label string) column.Cell {
	t.Helper()
	return stableLawCell(t, stableLawValue(t, mounted, typeID, label), model.Present)
}

func stableLawCell(t *testing.T, value binding.ValueToken, kind model.PresenceKind) column.Cell {
	t.Helper()
	presence, ok := model.NewPresence(kind)
	if !ok {
		t.Fatal("presence")
	}
	cell, ok := column.NewCell(value, presence)
	if !ok {
		t.Fatal("cell")
	}
	return cell
}

func stableLawColumnDelta(t *testing.T, base database.Version, columnID model.ColumnID, coordinate geometry.Coordinate, cell column.Cell, lineage model.LineageRef) column.Delta {
	t.Helper()
	version, ok := base.Store().Column(columnID)
	if !ok {
		t.Fatal("column version")
	}
	update, ok := column.NewUpdate(coordinate.Dense(), coordinate.Mask(), cell, lineage)
	if !ok {
		t.Fatal("column update")
	}
	_, delta, ok := version.Next(update)
	if !ok || !delta.Available() {
		t.Fatal("column delta")
	}
	return delta
}

func stableLawSource(t *testing.T, base database.Version, args ...interface{}) store.Prepared {
	t.Helper()
	changes := make([]column.Delta, 0, len(args))
	if len(args) == 4 {
		columnID, ok := args[0].(model.ColumnID)
		if !ok {
			t.Fatal("column id")
		}
		coordinate, ok := args[1].(geometry.Coordinate)
		if !ok {
			t.Fatal("coordinate")
		}
		cell, ok := args[2].(column.Cell)
		if !ok {
			t.Fatal("cell")
		}
		lineage, ok := args[3].(model.LineageRef)
		if !ok {
			t.Fatal("lineage")
		}
		changes = append(changes, stableLawColumnDelta(t, base, columnID, coordinate, cell, lineage))
	} else {
		for _, arg := range args {
			change, ok := arg.(column.Delta)
			if !ok {
				t.Fatal("column delta")
			}
			changes = append(changes, change)
		}
	}
	prepared, ok := store.Prepare(base.Store(), changes...)
	if !ok || !prepared.Available() {
		t.Fatal("store candidate")
	}
	return prepared
}

func stableLawPublish(fixture testfixture.Fixture, base database.Version, source store.Prepared) (database.Version, database.Delta, bool) {
	prepared, ok := database.Prepare(base, source, fixture.Scratch(), base.ContributionDirectory(), base.ContributionState(), nil)
	if !ok || !prepared.Available() {
		return database.Version{}, database.Delta{}, false
	}
	return database.Commit(prepared)
}
