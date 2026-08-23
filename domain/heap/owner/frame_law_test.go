package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// TestHeapAxisPublishesItsFacts states the publication half of this axis's
// declaration. The heap factor holds one fact per Coordinate -- the world
// disjunction that Coordinate is proven to inhabit -- and that population is
// what a consumer of the analyzer reads, so the axis declares one column for it
// and names itself as the principal admitted to write it: the lane whose rules
// write the factor is the lane the engine admits to fill the column.
func TestHeapAxisPublishesItsFacts(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatal("heap axis declaration rejected")
	}
	if count := entry.OutputCount(); count != 1 {
		t.Fatalf("heap axis publishes %d columns, want the one its facts are read out of", count)
	}
	output, outputOK := entry.OutputAt(0)
	if !outputOK {
		t.Fatal("heap axis declares an output it cannot state")
	}
	if output.Key != "heap/facts" {
		t.Fatalf("heap axis publishes its facts under %q", output.Key)
	}
	if output.Writer != "heap" {
		t.Fatalf("heap facts are written by %q, not by the axis whose rules produce them", output.Writer)
	}
}

// TestHeapPublishedColumnProvesAbsence states what a reader concludes from a
// Coordinate the published column holds no row for. The heap key space is
// dense, so the column is published with the key universe it is total over and
// an in-universe miss is a fact rather than ignorance.
func TestHeapPublishedColumnProvesAbsence(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatal("heap axis declaration rejected")
	}
	if coverage := entry.Coverage(); coverage != axis.CoverageTotal {
		t.Fatalf("heap publishes coverage %d, not the total coverage its dense key space declares", coverage)
	}
}
