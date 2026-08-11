package source

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSourceDirectLocationPatriciaPermutation(t *testing.T) {
	for _, ordinals := range [][]uint32{
		{1, 2, 4},
		{4, 2, 1},
		{1, 4, 2, 3, 8, 7, 6, 5},
	} {
		var index directLocationIndex
		for _, ordinal := range ordinals {
			term := keyspace.MakeTerm(keyspace.FamilyNil, ordinal)
			if err := index.add(ordinal, directLocation{term: term}); err != nil {
				t.Fatalf("add ordinal %d in %v: %v", ordinal, ordinals, err)
			}
		}
		if got, want := len(index.branches), len(index.rows)-1; got > want {
			t.Fatalf("branch count for %v = %d, want <= %d", ordinals, got, want)
		}
		for _, ordinal := range ordinals {
			location, ok, steps := index.lookupOrdinal(ordinal)
			if !ok || location.term != keyspace.MakeTerm(keyspace.FamilyNil, ordinal) {
				t.Fatalf("lookup ordinal %d in %v = %#v/%v", ordinal, ordinals, location, ok)
			}
			if steps > directLocationOrdinalBits {
				t.Fatalf("lookup ordinal %d in %v took %d branch steps", ordinal, ordinals, steps)
			}
		}
	}
}

func TestSourceDirectLocationPatriciaScalesByBitWidth(t *testing.T) {
	const width = 4096
	var index directLocationIndex
	for ordinal := uint32(1); ordinal <= width; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyNil, ordinal)
		if err := index.add(ordinal, directLocation{term: term}); err != nil {
			t.Fatalf("add ordinal %d: %v", ordinal, err)
		}
	}
	if got, want := len(index.branches), len(index.rows)-1; got > want {
		t.Fatalf("branch count = %d, want <= %d", got, want)
	}
	totalSteps := 0
	for ordinal := uint32(1); ordinal <= width; ordinal++ {
		_, ok, steps := index.lookupOrdinal(ordinal)
		if !ok {
			t.Fatalf("lookup ordinal %d failed", ordinal)
		}
		if steps > directLocationOrdinalBits {
			t.Fatalf("lookup ordinal %d took %d branch steps", ordinal, steps)
		}
		totalSteps += steps
	}
	if totalSteps > width*directLocationOrdinalBits {
		t.Fatalf("lookup branch steps = %d, want <= %d", totalSteps, width*directLocationOrdinalBits)
	}
}

func TestSourceDirectLocationPatriciaHighOrdinalIsSparse(t *testing.T) {
	var index directLocationIndex
	term := keyspace.MakeTerm(keyspace.FamilyNil, keyspace.MaxTermOrdinal)
	if err := index.add(keyspace.MaxTermOrdinal, directLocation{term: term}); err != nil {
		t.Fatalf("add high ordinal: %v", err)
	}
	if len(index.rows) != 1 || len(index.branches) != 0 {
		t.Fatalf("high ordinal scratch rows=%d branches=%d, want 1/0", len(index.rows), len(index.branches))
	}
	if location, ok, steps := index.lookupOrdinal(keyspace.MaxTermOrdinal); !ok || location.term != term || steps != 0 {
		t.Fatalf("high ordinal lookup = %#v/%v/%d", location, ok, steps)
	}
}
