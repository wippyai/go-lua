package relationconstructor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/relationconstructor"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
)

// TestASolvedMountPublishesTheRowsItsDeclarationStates carries one committed
// mount through the answering half of the span: the runtime solves the seeded
// root and the settled root is published as a snapshot.
//
// The law reads the published projection rather than the state the solve wrote,
// and requires the output column to carry exactly the values the fixture's own
// declaration states for exactly its rows. That is what makes this an answer
// and not a cardinality: a projection that published the right number of empty
// rows fails here.
func TestASolvedMountPublishesTheRowsItsDeclarationStates(t *testing.T) {
	fixture := arithmetic.New(t)
	solved, ok := relationconstructor.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !solved.Available() {
		t.Fatal("the seeded mount did not solve and publish")
	}
	if solved.Result().Publications() == 0 {
		t.Fatal("a seeded root settled without publishing")
	}
	projection := solved.Projection()
	if len(projection.Columns()) == 0 {
		t.Fatal("the published snapshot declares no column")
	}

	want := fixture.Expected()
	if len(want) == 0 {
		t.Fatal("the fixture states no expected row")
	}
	output := fixture.IDs().OutputWrite
	column, ok := projection.Column(output)
	if !ok || !column.Available() {
		t.Fatal("the published snapshot has no output column")
	}
	keys := projection.Keys(output)
	if len(keys) != len(want) {
		t.Fatalf("the output column published %d rows, the declaration states %d", len(keys), len(want))
	}
	seen := make(map[model.RowID]identity.ContentID, len(keys))
	for _, key := range keys {
		if !key.Available() {
			t.Fatal("the published snapshot carries an unavailable row key")
		}
		cell, status := projection.Read(output, key)
		if status != canonical.ReadHit {
			t.Fatalf("row %v published status %v", key.Row, status)
		}
		if !cell.Available() || !cell.Value.Available() {
			t.Fatalf("row %v published no value", key.Row)
		}
		if _, duplicate := seen[key.Row]; duplicate {
			t.Fatalf("row %v was published twice", key.Row)
		}
		seen[key.Row] = cell.Value.Opaque()
	}
	for row, expected := range want {
		got, published := seen[row]
		if !published {
			t.Fatalf("the declaration states row %v and the snapshot omits it", row)
		}
		if got != expected {
			t.Fatalf("row %v published %v, the declaration states %v", row, got, expected)
		}
	}
}

// TestAnAnswerIsRefusedRatherThanPartiallySealed states that the answer is
// whole or absent. A mount and a view that were not sealed for each other
// cannot produce a projection addressed in a geometry the solve ran in, so the
// construction refuses instead of publishing rows nobody can locate.
func TestAnAnswerIsRefusedRatherThanPartiallySealed(t *testing.T) {
	fixture := arithmetic.New(t)
	foreign := arithmetic.New(t, 0xB7)
	if solved, ok := relationconstructor.Solve(foreign.Mounted(), fixture.Base(), fixture.View()); ok || solved.Available() {
		t.Fatal("a mount solved over another mount's root and view")
	}
	if solved, ok := relationconstructor.Solve(fixture.Mounted(), fixture.Base(), foreign.View()); ok || solved.Available() {
		t.Fatal("a mount solved through another mount's geometry")
	}
	var absent relationconstructor.Solved
	if absent.Available() || absent.Mounted().Available() || absent.Projection().Available() {
		t.Fatal("the zero answer carries an authority")
	}
}

