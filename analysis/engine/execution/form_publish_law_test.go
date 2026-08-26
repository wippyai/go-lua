package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestPublishExactSealsOneReducedFactAtItsRegion states the publication half
// of the exact form on its own.
//
// A family that performs its own reads - which every installed family already
// does for its prerequisites - reaches its publication through this and not
// through a fold that would also insist on reading for it. That is the whole
// separation: the read geometry and the publication mode compose rather than
// multiply, so a product of two exact reads and a whole vector reduced to one
// cell publish through the same one statement.
func TestPublishExactSealsOneReducedFactAtItsRegion(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	if !writeOK {
		t.Fatal("exact write axis")
	}
	ticket, ticketOK := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !ticketOK {
		t.Fatal("issue")
	}
	var scratch Scratch[uint64, uint64]
	if outcome := PublishExact(ticket, write, &scratch, fixture.whole, 11); outcome != structure.Concrete {
		t.Fatalf("publication settled %v, want Concrete", outcome)
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("submit")
	}
	patches := make([]carrier.Patch, 1)
	if disposition, count, ok := run.Drain(patches); !ok || disposition != structure.Concrete || count != 1 {
		t.Fatalf("drain = %v/%d/%t", disposition, count, ok)
	}
}

// TestPublishExactRefusesAnUnauthenticatedRegion holds the publication to the
// support its reads reported. A region the invocation does not entail is a
// claim over coordinates nothing observed, and the write refuses it rather
// than widening what the row publishes.
func TestPublishExactRefusesAnUnauthenticatedRegion(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	if !writeOK {
		t.Fatal("exact write axis")
	}
	ticket, ticketOK := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !ticketOK {
		t.Fatal("issue")
	}
	var scratch Scratch[uint64, uint64]
	if outcome := PublishExact(ticket, write, &scratch, support.Mask{}, 11); outcome != structure.Refuse {
		t.Fatalf("an unauthenticated region settled %v, want Refuse", outcome)
	}
}

// TestPublishExactRefusesAnUnsealedWrite states that a publication needs a
// live sealed write axis and lane. Neither is something a caller can supply
// after the fact, so both are refused where they are missing.
func TestPublishExactRefusesAnUnsealedWrite(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	if !writeOK {
		t.Fatal("exact write axis")
	}
	ticket, ticketOK := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !ticketOK {
		t.Fatal("issue")
	}
	if outcome := PublishExact(ticket, write, nil, fixture.whole, 11); outcome != structure.Refuse {
		t.Fatalf("publication with no lane settled %v, want Refuse", outcome)
	}
	var scratch Scratch[uint64, uint64]
	if outcome := PublishExact(ticket, ExactWrite[uint64, uint64]{}, &scratch, fixture.whole, 11); outcome != structure.Refuse {
		t.Fatalf("publication through an unsealed write settled %v, want Refuse", outcome)
	}
}

// TestAMemberVectorIsTheSameViewItsReaderIsDeclaredToReceive states why a
// nested member set is a Summary read and not a second kind of read.
//
// The owner's MemberCount and MemberAt are a closed denominator, so a read
// spanning that set is a whole-vector read. Where its cells come from is an
// execution detail - one exact coordinate at a time rather than one Factor
// cursor - and the reader must not be able to tell, because a many-valued
// input is ONE vector argument under the reducer call shape. A second vector
// type would split every fold that consumes one into two spellings of the same
// parameter.
func TestAMemberVectorIsTheSameViewItsReaderIsDeclaredToReceive(t *testing.T) {
	cells := []MemberCell[uint64]{{Value: 4, Present: true}, {Present: false}, {Value: 9, Present: true}}
	vector, ok := NewMemberVector(cells)
	if !ok || !vector.Valid() {
		t.Fatalf("member vector = %t/%t", ok, vector.Valid())
	}
	if vector.Count() != len(cells) {
		t.Fatalf("member vector width = %d, want %d", vector.Count(), len(cells))
	}
	for index, want := range cells {
		value, present, addressed := vector.At(index)
		if !addressed || present != want.Present || value != want.Value {
			t.Fatalf("cell %d = %d/%t/%t, want %d/%t", index, value, present, addressed, want.Value, want.Present)
		}
	}
	// Absence is a cell, not a gap: the position of a cell is the ordinal its
	// owner declared it at, so an absent member keeps its place.
	if _, present, addressed := vector.At(1); !addressed || present {
		t.Fatal("an absent member was compacted away instead of kept as an absent cell")
	}
	if _, _, addressed := vector.At(len(cells)); addressed {
		t.Fatal("an index past the declared width names a cell")
	}
	if _, _, addressed := vector.At(-1); addressed {
		t.Fatal("a negative index names a cell")
	}
}

// TestAnUnopenedMemberVectorIsNotAVector keeps the view fail-closed. A vector
// nothing opened carries no cells, and a caller cannot mint one from a nil
// slice and have it read as an empty declared denominator.
func TestAnUnopenedMemberVectorIsNotAVector(t *testing.T) {
	if vector, ok := NewMemberVector[uint64](nil); ok || vector.Valid() || vector.Count() != 0 {
		t.Fatal("a nil member set opened a vector")
	}
	var unopened SummaryVector[uint64]
	if unopened.Valid() || unopened.Count() != 0 {
		t.Fatal("the zero vector reports itself live")
	}
	if _, _, addressed := unopened.At(0); addressed {
		t.Fatal("the zero vector addressed a cell")
	}
}

// TestAMemberVectorOfNoMembersIsAnEmptyDenominator states the one case that
// must stay distinguishable from an unopened vector: an owner whose member set
// is genuinely empty published a denominator with no cells, and its reader
// receives an open vector of width zero.
func TestAMemberVectorOfNoMembersIsAnEmptyDenominator(t *testing.T) {
	vector, ok := NewMemberVector([]MemberCell[uint64]{})
	if !ok || !vector.Valid() || vector.Count() != 0 {
		t.Fatalf("empty member set = %t/%t/%d", ok, vector.Valid(), vector.Count())
	}
	if _, _, addressed := vector.At(0); addressed {
		t.Fatal("an empty denominator addressed a cell")
	}
}
