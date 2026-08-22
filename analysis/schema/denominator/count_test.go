package denominator

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestSumCountRowsAddsDuplicateIdentities(t *testing.T) {
	entries := GeneratedRelationEntries()
	if len(entries) < 2 {
		t.Fatal("generated relation catalog is empty")
	}
	left, leftOK := NewCountRow(entries[0].ID(), 4)
	right, rightOK := NewCountRow(entries[0].ID(), 9)
	other, otherOK := NewCountRow(entries[1].ID(), 3)
	if !leftOK || !rightOK || !otherOK {
		t.Fatal("generated relation count row was not admitted")
	}
	first, firstOK := NewCountRows([]CountRow{left})
	second, secondOK := NewCountRows([]CountRow{right, other})
	if !firstOK || !secondOK {
		t.Fatal("owner count rows did not seal")
	}
	summed, summedOK := SumCountRows(first, second)
	if !summedOK {
		t.Fatal("duplicate owner counts were rejected")
	}
	if got, ok := summed.Value(entries[0].ID()); !ok || got != 13 {
		t.Fatalf("summed duplicate = (%d, %t), want (13, true)", got, ok)
	}
	if got, ok := summed.Value(entries[1].ID()); !ok || got != 3 {
		t.Fatalf("summed distinct = (%d, %t), want (3, true)", got, ok)
	}
}

func TestSumCountRowsRejectsOverflow(t *testing.T) {
	entries := GeneratedRelationEntries()
	maximum, maximumOK := NewCountRow(entries[0].ID(), math.MaxUint64)
	one, oneOK := NewCountRow(entries[0].ID(), 1)
	if !maximumOK || !oneOK {
		t.Fatal("overflow fixture row was not admitted")
	}
	left, leftOK := NewCountRows([]CountRow{maximum})
	right, rightOK := NewCountRows([]CountRow{one})
	if !leftOK || !rightOK {
		t.Fatal("overflow fixture rows did not seal")
	}
	if _, ok := SumCountRows(left, right); ok {
		t.Fatal("overflowing owner counts were accepted")
	}
}

func TestGeneratedCountRowsRequireEveryRelationIncludingZero(t *testing.T) {
	entries := GeneratedRelationEntries()
	rows := make([]CountRow, 0, len(entries))
	for _, entry := range entries {
		row, ok := NewCountRow(entry.ID(), 0)
		if !ok {
			t.Fatal("zero relation count row was not admitted")
		}
		rows = append(rows, row)
	}
	complete, completeOK := NewCountRows(rows)
	if !completeOK || !GeneratedCountRowsComplete(complete) {
		t.Fatal("zero-filled generated catalog was not complete")
	}
	missing, missingOK := NewCountRows(rows[:len(rows)-1])
	if !missingOK || GeneratedCountRowsComplete(missing) {
		t.Fatal("incomplete generated catalog was accepted")
	}
}

// TestLinkCountRowsCoverTheNineSealedLinkOwners states the Link column law in
// both directions: a set carrying exactly the nine Link-sealed owners is
// complete, a set that drops one of their rows is not, and a set that adds a
// ProgramModule row is not, because that family is derived at the Program
// artifact boundary and no Link authority publishes it.
func TestLinkCountRowsCoverTheNineSealedLinkOwners(t *testing.T) {
	linkOwners := []RelationOwner{
		RelationOwnerProgramSource, RelationOwnerProgramFlow, RelationOwnerProgramStatic,
		RelationOwnerTarget, RelationOwnerLinkProject, RelationOwnerLinkBoundary,
		RelationOwnerLinkModule, RelationOwnerLinkStatic, RelationOwnerLinkHost,
	}
	rows := make([]CountRow, 0, len(GeneratedRelationEntries()))
	for _, owner := range linkOwners {
		for _, id := range generatedOwnerIDs(owner) {
			row, ok := NewCountRow(id, 0)
			if !ok {
				t.Fatalf("owner %v count row was not admitted", owner)
			}
			rows = append(rows, row)
		}
	}
	complete, completeOK := NewCountRows(rows)
	if !completeOK || !LinkCountRowsComplete(complete) {
		t.Fatal("the nine sealed Link owners were not accepted as a complete Link column")
	}
	if GeneratedCountRowsComplete(complete) {
		t.Fatal("the Link column was accepted as the complete generated catalog")
	}

	missing, missingOK := NewCountRows(rows[:len(rows)-1])
	if !missingOK || LinkCountRowsComplete(missing) {
		t.Fatal("a Link column missing an owner row was accepted")
	}

	moduleIDs := generatedOwnerIDs(RelationOwnerProgramModule)
	if len(moduleIDs) == 0 {
		t.Fatal("the ProgramModule owner issued no generated identities")
	}
	stray, strayOK := NewCountRow(moduleIDs[0], 0)
	if !strayOK {
		t.Fatal("ProgramModule count row was not admitted")
	}
	extended, extendedOK := NewCountRows(append(append([]CountRow(nil), rows...), stray))
	if !extendedOK || LinkCountRowsComplete(extended) {
		t.Fatal("a Link column carrying a ProgramModule row was accepted")
	}
}

func TestGeneratedOwnerIDsAreTheCatalogTotality(t *testing.T) {
	owners := []RelationOwner{
		RelationOwnerProgramSource, RelationOwnerProgramFlow, RelationOwnerProgramStatic, RelationOwnerProgramModule,
		RelationOwnerTarget, RelationOwnerLinkProject, RelationOwnerLinkBoundary, RelationOwnerLinkModule,
		RelationOwnerLinkStatic, RelationOwnerLinkHost,
	}
	seen := make(map[schema.EntryID]RelationOwner)
	for _, owner := range owners {
		ids := generatedOwnerIDs(owner)
		if len(ids) == 0 {
			t.Fatalf("owner %v issued no generated identities", owner)
		}
		for _, id := range ids {
			if prior, duplicate := seen[id]; duplicate {
				t.Fatalf("identity %v belongs to both %v and %v", id, prior, owner)
			}
			seen[id] = owner
		}
	}
	entries := GeneratedRelationEntries()
	if len(seen) != len(entries) {
		t.Fatalf("generated owner views cover %d identities, catalog has %d", len(seen), len(entries))
	}
	for _, entry := range entries {
		if entry == nil {
			t.Fatal("generated catalog contained a nil entry")
		}
		owner, ok := seen[entry.ID()]
		if !ok || owner != entry.Owner() {
			t.Fatalf("catalog entry %v owner %v missing from generated owner view", entry.ID(), entry.Owner())
		}
	}
}
