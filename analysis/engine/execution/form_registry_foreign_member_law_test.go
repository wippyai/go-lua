package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// publishSelectedFixtureValue commits one fact at one dense coordinate of the
// selectedFixture's Factor, so a member read can observe an authenticated row
// rather than only resolving a Unit.
func publishSelectedFixtureValue(t *testing.T, fixture *selectedFixture, index int, value uint64) {
	t.Helper()
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.binding, fixture.targets[index], 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !writeOK || !issued {
		t.Fatal("member fixture publish write")
	}
	var scratch Scratch[uint64, uint64]
	patches := make([]carrier.Patch, 1)
	if !write.Stage(ticket, &scratch, fixture.whole, value) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("member fixture publish stage")
	}
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("member fixture publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("member fixture publish commit")
	}
	fixture.state = next
}

// TestForeignMemberExactReadResolvesADenseCoordinateAtTheForeignAxisOwnTypes
// states what ForeignMemberExactRead protects: a caller that already resolved
// one member's dense coordinate through its owner's own directory (not
// through a route table's selection) recovers an exact read at that
// coordinate, at the foreign axis's own types, and nothing else. A self-
// provided nested member set publishes no destinations, so the geometry
// behind it carries units and no targets - this is the "no route table"
// shape the primitive is built for.
func TestForeignMemberExactReadResolvesADenseCoordinateAtTheForeignAxisOwnTypes(t *testing.T) {
	fixture := newSelectedFixture(t)
	table, tableOK := NewRouteTable(fixture.units, nil)
	if !tableOK {
		t.Fatal("member geometry")
	}
	if table.Routed() {
		t.Fatal("a self-provided nested member set geometry carries no destinations")
	}
	foreign, foreignOK := NewForeignFactor(fixture.binding, table)
	if !foreignOK {
		t.Fatal("foreign handle")
	}
	read, readOK := ForeignMemberExactRead[uint64, uint64](foreign, 1, 0)
	if !readOK || !read.Valid() {
		t.Fatal("a dense coordinate inside the owner's directory resolved no exact read")
	}
	if !read.unit.Same(fixture.units[1]) {
		t.Fatal("the member read does not name the coordinate its position holds")
	}
	if _, resolved := ForeignMemberExactRead[uint32, uint32](foreign, 1, 0); resolved {
		t.Fatal("a member read was sealed at another Factor's types")
	}
	if _, resolved := ForeignMemberExactRead[uint64, uint64](foreign, selectedFixtureWidth, 0); resolved {
		t.Fatal("a dense coordinate outside the owner's directory resolved a read")
	}

	own := newSelectedFixture(t)
	stranger, strangerOK := NewForeignFactor(own.binding, table)
	if !strangerOK {
		t.Fatal("stranger handle")
	}
	if _, resolved := ForeignMemberExactRead[uint64, uint64](stranger, 1, 0); resolved {
		t.Fatal("a coordinate another binding minted resolved against this one")
	}
}

// TestForeignMemberExactReadReadsTheAuthenticatedValue proves the primitive
// is a real exact read and not just a Unit lookup: it observes the fact the
// owner already committed at the dense coordinate the caller resolved,
// exactly as ForeignExactRead does for a caller-held Unit.
func TestForeignMemberExactReadReadsTheAuthenticatedValue(t *testing.T) {
	fixture := newSelectedFixture(t)
	publishSelectedFixtureValue(t, &fixture, 2, 77)
	table, tableOK := NewRouteTable(fixture.units, nil)
	if !tableOK {
		t.Fatal("member geometry")
	}
	foreign, foreignOK := NewForeignFactor(fixture.binding, table)
	if !foreignOK {
		t.Fatal("foreign handle")
	}
	read, readOK := ForeignMemberExactRead[uint64, uint64](foreign, 2, 0)
	if !readOK {
		t.Fatal("member read")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue member read invocation")
	}
	var scratch Scratch[uint64, uint64]
	if read.Read(ticket, &scratch) != ReadAvailable {
		t.Fatal("member read cursor")
	}
	value, present := scratch.Value()
	if !present || value != 77 {
		t.Fatalf("member value = %d/%t, want the authenticated 77", value, present)
	}
	if !read.Close(ticket, &scratch) {
		t.Fatal("member read close")
	}
	if !run.Submit(&ticket, structure.NoCandidate) {
		t.Fatal("submit member read invocation")
	}
	if _, _, _, drained := run.Consume(); !drained {
		t.Fatal("member read consume")
	}
}

// TestForeignMemberExactReadAllocatesNothing is the zero-allocation warm
// invocation law ForeignExactRead already holds: a sealed member read is an
// immutable descriptor, so repeated warm reads through it allocate nothing.
func TestForeignMemberExactReadAllocatesNothing(t *testing.T) {
	fixture := newSelectedFixture(t)
	publishSelectedFixtureValue(t, &fixture, 0, 5)
	table, tableOK := NewRouteTable(fixture.units, nil)
	if !tableOK {
		t.Fatal("member geometry")
	}
	foreign, foreignOK := NewForeignFactor(fixture.binding, table)
	if !foreignOK {
		t.Fatal("foreign handle")
	}
	read, readOK := ForeignMemberExactRead[uint64, uint64](foreign, 0, 0)
	if !readOK {
		t.Fatal("member read")
	}
	run := NewRun(1, 1)
	var scratch Scratch[uint64, uint64]
	inputs := []carrier.State{fixture.state}
	invoke := func() bool {
		ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, 1, 4, 9, 2)
		if !issued {
			return false
		}
		if read.Read(ticket, &scratch) != ReadAvailable {
			return false
		}
		if !read.Close(ticket, &scratch) {
			return false
		}
		if !run.Submit(&ticket, structure.NoCandidate) {
			return false
		}
		_, _, _, drained := run.Consume()
		return drained
	}
	measureWarmInvocation(t, invoke, 0)
}
