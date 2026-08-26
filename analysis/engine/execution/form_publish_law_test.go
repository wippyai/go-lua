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
