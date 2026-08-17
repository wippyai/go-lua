package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// TestCallAxisPublishesItsFacts states the publication half of this axis's
// declaration. The call factor holds one fact per coordinate -- the dispatch
// relation that call site is proven to reach -- and that population is what a
// consumer of the analyzer reads, so the axis declares one column for it and
// names itself as the principal admitted to write it: the lane whose rules write
// the factor is the lane the engine admits to fill the column.
func TestCallAxisPublishesItsFacts(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatal("call axis declaration rejected")
	}
	if count := entry.OutputCount(); count != 1 {
		t.Fatalf("call axis publishes %d columns, want the one its facts are read out of", count)
	}
	output, outputOK := entry.OutputAt(0)
	if !outputOK {
		t.Fatal("call axis declares an output it cannot state")
	}
	if output.Key != "call/facts" {
		t.Fatalf("call axis publishes its facts under %q", output.Key)
	}
	if output.Writer != "call" {
		t.Fatalf("call facts are written by %q, not by the axis whose rules produce them", output.Writer)
	}
}

// TestCallPublishedColumnProvesAbsence states what a reader concludes from a
// coordinate the published column holds no row for. The call key space is
// dense, so the column is published with the key universe it is total over and
// an in-universe miss is a fact rather than ignorance.
func TestCallPublishedColumnProvesAbsence(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatal("call axis declaration rejected")
	}
	if coverage := entry.Coverage(); coverage != axis.CoverageTotal {
		t.Fatalf("call publishes coverage %d, not the total coverage its dense key space declares", coverage)
	}
}
