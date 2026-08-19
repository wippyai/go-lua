package publications

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts(), ledgerRefs(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestPublicationsPreserveExactDenseRelation proves the authored row survives
// the seal unchanged and is reachable by its canonical term.
func TestPublicationsPreserveExactDenseRelation(t *testing.T) {
	publications := sealTable(t, ledgerInput()).View()
	first, ok := publications.At(0)
	if !ok || first != term(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publications.At(0) = %v/%v, want publication 1", first, ok)
	}
	assign, pair, target, ok := publications.Get(first)
	if !ok || assign != term(keyspace.FamilyAssign, 1) || pair != 0 ||
		target != term(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publications.Get() = (%v, %d, %v, %v), want exact authored row", assign, pair, target, ok)
	}
}

// TestPublicationsAcceptResolvedTargetsAndDistinctPairs proves both resolved
// dispositions publish and that the pair is the full uint32 domain.
func TestPublicationsAcceptResolvedTargetsAndDistinctPairs(t *testing.T) {
	input := ledgerInput()
	input.Type[1] = Publication{
		Assign: term(keyspace.FamilyAssign, 1), Pair: math.MaxUint32,
		Target: term(keyspace.FamilyTypeRef, 2),
	}
	publications := sealTable(t, input).View()
	if publications.Count() != 2 {
		t.Fatalf("Publications.Count() = %d, want 2", publications.Count())
	}
	_, pair, target, ok := publications.Get(term(keyspace.FamilyTypePublication, 2))
	if !ok || pair != math.MaxUint32 || target != term(keyspace.FamilyTypeRef, 2) {
		t.Fatalf("second publication = (%d, %v, %v), want maximum pair and canonical target", pair, target, ok)
	}
}

// TestPublicationsRejectInvalidRows proves every admission this vertical owns.
// Sharing one target between two parents is a combined-forest defect and
// belongs to the enclosing owner's containment seal, not here.
func TestPublicationsRejectInvalidRows(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"foreign assign", func(in *Input) { in.Type[0].Assign = term(keyspace.FamilyAssign, 9) }},
		{"wrong assign family", func(in *Input) { in.Type[0].Assign = term(keyspace.FamilyCell, 1) }},
		{"wrong target family", func(in *Input) { in.Type[0].Target = term(keyspace.FamilyTypePrimitive, 1) }},
		{"foreign target", func(in *Input) { in.Type[0].Target = term(keyspace.FamilyTypeRef, 9) }},
		{"unresolved target", func(in *Input) { in.Type[0].Target = term(keyspace.FamilyTypeRef, 3) }},
		{"duplicate assign pair", func(in *Input) {
			in.Type[1].Assign = in.Type[0].Assign
			in.Type[1].Pair = in.Type[0].Pair
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput()
			test.edit(&input)
			if _, err := Build(input, ledgerCounts(), ledgerRefs(t)); err == nil {
				t.Fatal("Build() accepted an invalid publication relation")
			}
		})
	}
}

// TestPublicationsCopyFenceBoundsAndQueriesDoNotAllocate proves the seal takes
// a copy, every read is total, and the hot queries allocate nothing.
func TestPublicationsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput()
	table := sealTable(t, input)
	input.Type[0] = Publication{}

	publications := table.View()
	first := term(keyspace.FamilyTypePublication, 1)
	assign, pair, target, ok := publications.Get(first)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("publication copy fence = (%v, %d, %v, %v)", assign, pair, target, ok)
	}
	if _, ok := publications.At(2); ok {
		t.Fatal("Publications.At accepted out-of-bounds index")
	}
	if _, _, _, ok := publications.Get(0); ok {
		t.Fatal("Publications.Get accepted zero term")
	}
	if _, _, _, ok := publications.Get(term(keyspace.FamilyTypePublication, 9)); ok {
		t.Fatal("Publications.Get accepted foreign ordinal")
	}
	if _, _, _, ok := publications.Get(term(keyspace.FamilyTypeRef, 1)); ok {
		t.Fatal("Publications.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		publications.Count()
		publications.At(0)
		publications.Get(first)
	}); allocations != 0 {
		t.Fatalf("publication queries allocated %.2f times", allocations)
	}
}

// TestDecoderRetainsAssignPairAndTarget proves the decoded row maps each wire
// field back to the relation it names, which the byte round-trip alone would
// not distinguish from a consistent permutation.
func TestDecoderRetainsAssignPairAndTarget(t *testing.T) {
	input := ledgerInput()
	input.Type[0].Pair = 4
	encoded := sectionBytes(t, input)
	decoded, err := Decode(sectionReader(t, encoded))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Type) != 2 {
		t.Fatalf("decoded publication count = %d, want 2", len(decoded.Type))
	}
	row := decoded.Type[0]
	if row.Assign != term(keyspace.FamilyAssign, 1) || row.Pair != 4 ||
		row.Target != term(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("decoded publication = %+v", row)
	}
}

// TestZeroViewFailsClosed proves an unavailable view answers nothing.
func TestZeroViewFailsClosed(t *testing.T) {
	var view View
	if view.Available() || view.Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, ok := view.At(0); ok {
		t.Fatal("zero View minted a term")
	}
	if _, _, _, ok := view.Get(term(keyspace.FamilyTypePublication, 1)); ok {
		t.Fatal("zero View returned a row")
	}
}
