package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// exactCellFixture is one Factor with one exact coordinate over a one-atom
// guard space: enough to publish a coordinate on part of the space and read it
// back over the whole of it.
type exactCellFixture struct {
	binding *factbinding.Binding[uint64, uint64]
	unit    carrier.Unit
	target  carrier.Target
	work    *carrier.Work
	state   carrier.State
	whole   support.Mask
	on      support.Mask
	off     support.Mask
}

// exactCellBottom is the fixture Factor's own reading of an unwritten
// coordinate, and exactCellWritten is a value above it, so a written block can
// never be confused with the reading of an unwritten one under this fixture's
// join.
const (
	exactCellBottom  uint64 = 99
	exactCellWritten uint64 = 200
)

func newExactCellFixture(t testing.TB) exactCellFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, onOK := regions.Literal(1, true)
	off, offOK := regions.Literal(1, false)
	if !onOK || !offOK || !regions.Seal() {
		t.Fatal("guard partition")
	}
	// Bottom is deliberately not the language's zero: an unwritten coordinate
	// is delivered as the FACTOR read it, and a fold that substituted a zero
	// would be indistinguishable from a correct one under a zero Bottom.
	algebra, admitted := factbinding.Admit[uint64, uint64](1, exactCellBottom, lattice.Lattice[uint64]{
		Bottom: func() uint64 { return exactCellBottom }, Top: func() uint64 { return ^uint64(0) },
		Equal:    func(left, right uint64) bool { return left == right },
		Same:     func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}, func(uint64, uint64) bool { return true }, func(value uint64) uint64 { return value },
		factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !admitted {
		t.Fatal("algebra")
	}
	var unit carrier.Unit
	var target carrier.Target
	binding, bound := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		var ok bool
		unit, ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		target, ok = binding.DeclareStrong(unit)
		return ok
	})
	if !bound {
		t.Fatal("binding")
	}
	prepared, preparedOK := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !preparedOK {
		t.Fatal("prepare")
	}
	composition, attached := prepared.Attach()
	state, stateOK := carrier.NewState(composition, composition.Scope(), whole)
	work, workOK := composition.NewWork()
	if !attached || !stateOK || !workOK {
		t.Fatal("carrier")
	}
	return exactCellFixture{binding: binding, unit: unit, target: target, work: work, state: state, whole: whole, on: on, off: off}
}

// publishRegions writes one value per named region into the fixture's single
// coordinate and returns the committed state.
func (fixture exactCellFixture) publishRegions(t testing.TB, epoch uint64, regions []support.Mask, values []uint64) carrier.State {
	t.Helper()
	run := NewRun(0, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, epoch, epoch, epoch)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	if !issued || !writeOK {
		t.Fatal("publish issue")
	}
	var scratch Scratch[uint64, uint64]
	for index, region := range regions {
		if !write.Stage(ticket, &scratch, region, values[index]) {
			t.Fatal("publish stage")
		}
	}
	if !write.Close(ticket, &scratch) || !ticket.Submit(structure.Concrete) {
		t.Fatal("publish close")
	}
	patches := make([]carrier.Patch, 1)
	_, count, drained := run.Drain(patches)
	if !drained || count != 1 {
		t.Fatal("publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("publish commit")
	}
	return next
}

// deliver reads the fixture's coordinate over the whole guard space through
// the one delivery statement under test.
func (fixture exactCellFixture) deliver(t testing.TB, epoch uint64, state carrier.State) (ExactCell[uint64], ReadStatus) {
	t.Helper()
	run := NewRun(1, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, state, fixture.whole, []carrier.State{state}, 0, epoch, epoch, epoch)
	read, readOK := NewExactRead(fixture.binding, fixture.unit, 0)
	if !issued || !readOK {
		t.Fatal("deliver issue")
	}
	var scratch Scratch[uint64, uint64]
	cell, status := DeliverExactCell(read, ReadCellPolicy[uint64]{}, ticket, &scratch)
	if !ticket.Close() {
		t.Fatal("deliver close")
	}
	return cell, status
}

// TestExactCellAnswersTheWrittenBlockAtEitherGuardOrder is the delivery law of
// the exact read boundary. A coordinate written on one region of a partitioned
// read region is delivered as that written value over exactly that region, and
// the answer is the same whichever half of the partition the guard's canonical
// enumeration reaches first. A cell taken from the first emitted block instead
// would answer by branch structure above the read rather than by what the
// coordinate holds.
func TestExactCellAnswersTheWrittenBlockAtEitherGuardOrder(t *testing.T) {
	fixture := newExactCellFixture(t)
	for _, written := range []struct {
		name   string
		region support.Mask
	}{
		{name: "high-guard", region: fixture.on},
		{name: "low-guard", region: fixture.off},
	} {
		t.Run(written.name, func(t *testing.T) {
			state := fixture.publishRegions(t, 11, []support.Mask{written.region}, []uint64{exactCellWritten})
			cell, status := fixture.deliver(t, 12, state)
			if status != ReadAvailable {
				t.Fatalf("delivery status = %d", status)
			}
			if !cell.Present || cell.Value != exactCellWritten {
				t.Fatalf("delivered cell = %d/%t, want the written value", cell.Value, cell.Present)
			}
			if !cell.Region.Equal(written.region) {
				t.Fatal("delivered region is not the region the value was written over")
			}
		})
	}
}

// TestExactCellDeliversAbsenceOverTheWholeReadRegion states what a coordinate
// no block writes answers: absence, over the whole region the read spans, so a
// caller's support conjunction is left where it was rather than narrowed to
// one arbitrary block of an absence that holds everywhere.
func TestExactCellDeliversAbsenceOverTheWholeReadRegion(t *testing.T) {
	fixture := newExactCellFixture(t)
	cell, status := fixture.deliver(t, 21, fixture.state)
	if status != ReadAvailable {
		t.Fatalf("delivery status = %d", status)
	}
	if cell.Present {
		t.Fatal("unwritten coordinate delivered a value")
	}
	if cell.Value != exactCellBottom {
		t.Fatalf("absent cell value = %d, want the Factor's own reading %d", cell.Value, exactCellBottom)
	}
	if !cell.Region.Equal(fixture.whole) {
		t.Fatal("absence was not delivered over the whole read region")
	}
}

// TestExactCellCarriesTheFactorReadingOfAnUnwrittenBlockPastAWrittenOne is the
// same law where it is easiest to get wrong: a partitioned coordinate whose
// blocks are all unwritten still delivers the Factor's reading, and a
// delivery that dropped the unwritten blocks' value on the way to answering
// would hand its caller a value the Factor never read. The caller's own
// contract - here the bare policy that substitutes nothing - then sees exactly
// what the coordinate holds.
func TestExactCellCarriesTheFactorReadingOfAnUnwrittenBlockPastAWrittenOne(t *testing.T) {
	fixture := newExactCellFixture(t)
	// A published value on one region leaves the complement unwritten, so the
	// delivery spans a written block and an unwritten one.
	state := fixture.publishRegions(t, 41, []support.Mask{fixture.on}, []uint64{exactCellWritten})
	cell, status := fixture.deliver(t, 42, state)
	if status != ReadAvailable {
		t.Fatalf("delivery status = %d", status)
	}
	if !cell.Present || cell.Value != exactCellWritten {
		t.Fatalf("delivered cell = %d/%t, want the written value", cell.Value, cell.Present)
	}
	if !cell.Region.Equal(fixture.on) {
		t.Fatal("delivered region is not the region the value was written over")
	}
}

// TestExactCellJoinsTwoDisagreeingWrittenBlocks states the one case a single
// cell cannot answer exactly. Two written blocks disagree by construction -
// equal ones are coalesced - so no one block is the coordinate's answer over a
// read region spanning both, and what it holds there is their join, proved
// over the whole of that region. Settling on whichever block the enumeration
// reached first would drop the other branch's value outright.
func TestExactCellJoinsTwoDisagreeingWrittenBlocks(t *testing.T) {
	fixture := newExactCellFixture(t)
	state := fixture.publishRegions(t, 31, []support.Mask{fixture.on, fixture.off}, []uint64{210, 220})
	cell, status := fixture.deliver(t, 32, state)
	if status != ReadAvailable {
		t.Fatalf("disagreeing blocks delivered status %d", status)
	}
	// The fixture's join is the greater of the two.
	if !cell.Present || cell.Value != 220 {
		t.Fatalf("disagreeing blocks delivered %d/%t, want the join 220", cell.Value, cell.Present)
	}
	if !cell.Region.Equal(fixture.whole) {
		t.Fatal("a joined cell is not proved over the whole read region")
	}
}
