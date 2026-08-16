package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestStaticSequenceQueriesUseProductiveRecursiveProofs(t *testing.T) {
	sequence := typ.NewRecursivePlaceholder("Sequence")
	sequence.SetBody(&typ.Union{Members: []typ.Type{sequence, typ.NewTuple(typ.String, typ.Number)}})
	if floor := staticSequenceLengthFloor(sequence); floor != 2 {
		t.Fatalf("recursive sequence floor = %d, want 2", floor)
	}
	if length, ok := staticSequenceExactLength(sequence); !ok || length != 2 {
		t.Fatalf("recursive exact length = %d/%v, want 2/true", length, ok)
	}

	mismatch := typ.NewRecursivePlaceholder("Mismatch")
	mismatch.SetBody(&typ.Union{Members: []typ.Type{mismatch, typ.NewTuple(typ.String), typ.NewTuple(typ.String, typ.Number)}})
	if length, ok := staticSequenceExactLength(mismatch); ok {
		t.Fatalf("productive length mismatch = %d/%v, want inexact", length, ok)
	}

	loop := typ.NewRecursive("Loop", func(self typ.Type) typ.Type { return self })
	if staticSequenceLengthFloor(loop) != 0 {
		t.Fatal("cycle-only type manufactured a length floor")
	}
	if _, ok := staticSequenceExactLength(loop); ok {
		t.Fatal("cycle-only type manufactured an exact length")
	}
}

func TestStaticSequenceQueriesTraverseDeepAcyclicGraph(t *testing.T) {
	var value typ.Type = typ.NewTuple(typ.String, typ.Number, typ.Boolean)
	for range 257 {
		value = &typ.Alias{Name: "Deep", Target: value}
	}
	if floor := staticSequenceLengthFloor(value); floor != 3 {
		t.Fatalf("deep sequence floor = %d, want 3", floor)
	}
	if length, ok := staticSequenceExactLength(value); !ok || length != 3 {
		t.Fatalf("deep exact length = %d/%v, want 3/true", length, ok)
	}
}

func TestKeySegmentProofUsesExactRecursivePolarity(t *testing.T) {
	seg := segment.Segment{Kind: segment.SegmentIndexString, Name: "name"}
	exactType := typ.NewRecursivePlaceholder("Exact")
	exactType.SetBody(&typ.Union{Members: []typ.Type{exactType, typ.LiteralString("name")}})
	if exact, not := keyTypeDefinitelyMatchesSegment(exactType, seg); !exact || not {
		t.Fatalf("recursive exact key = %v/%v, want true/false", exact, not)
	}

	mismatch := typ.NewRecursivePlaceholder("Mismatch")
	mismatch.SetBody(&typ.Union{Members: []typ.Type{mismatch, typ.LiteralString("other")}})
	if exact, not := keyTypeDefinitelyMatchesSegment(mismatch, seg); exact || !not {
		t.Fatalf("recursive mismatched key = %v/%v, want false/true", exact, not)
	}

	mixed := typ.NewRecursivePlaceholder("Mixed")
	mixed.SetBody(&typ.Union{Members: []typ.Type{mixed, typ.LiteralString("name"), typ.LiteralString("other")}})
	if exact, not := keyTypeDefinitelyMatchesSegment(mixed, seg); exact || not {
		t.Fatalf("recursive mixed key = %v/%v, want unknown", exact, not)
	}
}
