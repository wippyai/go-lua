package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestValueMayHoldTable_PathIdentity(t *testing.T) {
	holder := constraint.Path{Root: "holder", Symbol: 1}
	entries := holder.Field("entries")
	clock := holder.Field("clock")
	other := constraint.Path{Root: "other", Symbol: 2}
	sameName := constraint.Path{Root: "holder", Symbol: 3}
	read := func(path constraint.Path) ValueSource { return ValueSource{ValuePath: path} }
	cases := []struct {
		name   string
		source ValueSource
		target constraint.Path
		writes []IndexerAssignment
		want   bool
	}{
		{name: "same table", source: read(entries), target: entries, want: true},
		{name: "parent retains nested table", source: read(holder), target: entries, want: true},
		{name: "sibling scalar", source: read(clock), target: entries},
		{name: "different binding", source: read(sameName.Field("entries")), target: entries},
		{name: "nested value from sibling", source: ValueSource{ValueFields: []ValueFieldSource{{Name: "used", ValueSource: read(clock)}}}, target: entries},
		{name: "indirect sibling", source: read(other), target: entries, writes: []IndexerAssignment{{Symbol: other.Symbol, ValueSource: read(clock)}}},
		{name: "indirect reference", source: read(other), target: entries, writes: []IndexerAssignment{{Symbol: other.Symbol, ValueSource: read(entries)}}, want: true},
		{name: "nested element reference", source: read(other.IndexInt(1)), target: entries, writes: []IndexerAssignment{{Symbol: other.Symbol, ValueSource: read(entries)}}, want: true},
		{name: "nested table cycle", source: read(clock), target: entries, writes: []IndexerAssignment{{Symbol: holder.Symbol, Segments: clock.Segments, ValueSource: read(entries)}}, want: true},
		{name: "unrelated cycle terminates", source: read(other), target: entries, writes: []IndexerAssignment{{Symbol: other.Symbol, ValueSource: read(other)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			solution := &Solution{inputs: &Inputs{IndexerAssignments: tc.writes}}
			if got := solution.valueMayHoldTable(tc.source, tc.target); got != tc.want {
				t.Fatalf("valueMayHoldTable() = %v, want %v", got, tc.want)
			}
		})
	}
}
