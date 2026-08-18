package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSourceRejectsBadFrontierAndBodyContainment(t *testing.T) {
	input, index := sourceFixture(2)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	for at := range index.Positions {
		if index.Positions[at].Term == returned {
			index.Positions[at].FrontierCursor = 2
		}
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted unbounded ordinary frontier")
	}

	input, index = repeatFixture()
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	// Body 2 is a direct source child of Body 1, but the sealed forest claims
	// that it belongs below Body 3. The projection must preserve that witness.
	index.Bodies[1].Parent = body3
	index.Bodies[2].Parent = body1
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted direct Body with mismatched parent")
	}
}

func TestSourceRejectsEntryDirectSourceOccurrence(t *testing.T) {
	input, index := sourceFixture(1)
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	// Make the Entry itself a well-shaped direct source/root occurrence and
	// provide the corresponding position. The parentless Entry is the sole
	// forest root and must never also have a direct Source witness.
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, entry)
	index.Bodies[0].Roots = append(index.Bodies[0].Roots, entry)
	appendCanonicalFixturePosition(&index, Position{
		Term: entry, Root: entry, Body: entry, Offset: 1, Cursor: 1,
		FrontierBody: entry, FrontierCursor: 1,
	})

	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source occurrence for the Entry Body")
	}
}

func TestSourceAllowsMissingNonDirectPositionFamily(t *testing.T) {
	input, index := sourceFixture(2)
	missing := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	index.Positions = removeSourcePosition(index.Positions, missing)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, _, _, ok := component.View().Index().Position(missing); ok {
		t.Fatal("non-direct Cell unexpectedly acquired a source position")
	}
}

func TestSourceRejectsDirectPositionSubstitution(t *testing.T) {
	input, index := sourceFixture(2)
	missingReturn := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	index.Positions = removeSourcePosition(index.Positions, missingReturn)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build missing direct Return position: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct Return source Term omission")
	}

	input, index = sourceFixture(2)
	missing := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	index.Positions = removeSourcePosition(index.Positions, missing)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source Term omission")
	}

	input, index = sourceFixture(2)
	if len(index.Positions) == 0 {
		t.Fatal("fixture unexpectedly empty")
	}
	index.Positions[0] = index.Positions[1]
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build duplicate: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a duplicate Position.Term")
	}

	input, index = sourceFixture(2)
	index.Positions[0].Term = keyspace.MakeTerm(keyspace.FamilyCell, 99)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build invalid: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted an invalid Position.Term")
	}

	input, index = sourceFixture(2)
	index.Positions[0], index.Positions[1] = index.Positions[1], index.Positions[0]
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build noncanonical order: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted noncanonical Position order")
	}

	input, index = sourceFixture(2)
	term := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	row, ok := sourcePositionFor(index.Positions, term)
	if !ok {
		t.Fatal("Bind position missing from fixture")
	}
	other, ok := sourcePositionFor(index.Positions, keyspace.MakeTerm(keyspace.FamilyReturn, 1))
	if !ok {
		t.Fatal("Return position missing from fixture")
	}
	row.Root, row.Body, row.Offset, row.Cursor = other.Root, other.Body, other.Offset, other.Cursor
	row.FrontierBody, row.FrontierCursor = other.FrontierBody, other.FrontierCursor
	for at := range index.Positions {
		if index.Positions[at].Term == term {
			index.Positions[at] = row
		}
	}
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build root mismatch: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source Term under another root")
	}
}

func TestSourceRejectsNoncanonicalPositionOrder(t *testing.T) {
	input, index := sourceFixture(2)
	if len(index.Positions) < 2 {
		t.Fatal("fixture unexpectedly empty")
	}
	// The encoded Term value is not the ordering key: the batch is ordered by
	// explicit (TermFamily, TermOrdinal), so swapping these first rows must be
	// rejected even though every row remains individually well formed.
	index.Positions[0], index.Positions[1] = index.Positions[1], index.Positions[0]
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted noncanonical Position order")
	}
}

func TestSourceDirectLocationScratchScalesWithDirectRows(t *testing.T) {
	var retained []int
	for _, unusedLoops := range []int{0, 100000} {
		input, index := sparsePositionFixture(unusedLoops)
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(%d): %v", unusedLoops, err)
		}
		a := draft.state.authority
		var next indexStore
		next.rootRanges = make([]termRange, a.count(keyspace.FamilyBody))
		next.parents = make([]keyspace.Term, a.count(keyspace.FamilyBody))
		if err := installBodyRoots(a, &next, index.Bodies); err != nil {
			t.Fatalf("installBodyRoots(%d): %v", unusedLoops, err)
		}
		locations, err := buildDirectLocations(a, &next)
		if err != nil {
			t.Fatalf("buildDirectLocations(%d): %v", unusedLoops, err)
		}
		rows := 0
		for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
			rows += len(locations[family].rows)
		}
		if rows != len(a.order.sourceTerms) {
			t.Fatalf("direct rows(%d) = %d, want authored direct rows %d", unusedLoops, rows, len(a.order.sourceTerms))
		}
		retained = append(retained, rows)
	}
	if retained[0] != retained[1] || retained[0] != 1 {
		t.Fatalf("direct-location scratch grew with non-direct family cardinality: %v", retained)
	}
}
