package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/lattice"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// guardedSelectedFixture is the selected fixture over a one-atom guard space,
// so a coordinate can be written on one region of the read region and left
// unwritten on the complement - the shape a coordinate carried into a branch
// arm has.
type guardedSelectedFixture struct {
	selectedFixture
	on  support.Mask
	off support.Mask
}

func newGuardedSelectedFixture(t testing.TB) guardedSelectedFixture {
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
	algebra, admitted := factbinding.Admit[uint64, uint64](selectedFixtureWidth, 0, lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) },
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
	units := make([]carrier.Unit, selectedFixtureWidth)
	targets := make([]carrier.Target, selectedFixtureWidth)
	binding, bound := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for index := range units {
			unit, declared := binding.DeclareExact(uint64(index))
			if !declared {
				return false
			}
			units[index] = unit
		}
		for index, unit := range units {
			target, strong := binding.DeclareStrong(unit)
			if !strong {
				return false
			}
			targets[index] = target
		}
		return true
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
	return guardedSelectedFixture{
		selectedFixture: selectedFixture{binding: binding, units: units, targets: targets, state: state, whole: whole, work: work},
		on:              on,
		off:             off,
	}
}

// publish writes one value at the fixture's first coordinate over one region
// and returns the fixture whose state carries it.
func (fixture guardedSelectedFixture) publish(t testing.TB, region support.Mask, value uint64) guardedSelectedFixture {
	t.Helper()
	run := NewRun(0, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 5, 5, 5)
	write, writeOK := NewExactWrite(fixture.binding, fixture.targets[0], 0)
	if !issued || !writeOK {
		t.Fatal("publish issue")
	}
	var scratch Scratch[uint64, uint64]
	if !write.Stage(ticket, &scratch, region, value) || !write.Close(ticket, &scratch) || !ticket.Submit(structure.Concrete) {
		t.Fatal("publish stage")
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
	fixture.state = next
	return fixture
}

// TestSelectedReadObservesTheWrittenBlockAtEitherGuardOrder is the selected
// half of the exact delivery law. A selected member is observed at one exact
// coordinate, and that coordinate's read region is partitioned by guard exactly
// as a directly read one is. The cell a member delivers is therefore what the
// coordinate HOLDS, over the region it holds it on, and not the block the
// guard's canonical order happened to enumerate first - which would make a
// route's predecessor fact depend on the branch structure above the selection.
func TestSelectedReadObservesTheWrittenBlockAtEitherGuardOrder(t *testing.T) {
	for _, written := range []struct {
		name  string
		which func(guardedSelectedFixture) support.Mask
	}{
		{name: "high-guard", which: func(fixture guardedSelectedFixture) support.Mask { return fixture.on }},
		{name: "low-guard", which: func(fixture guardedSelectedFixture) support.Mask { return fixture.off }},
	} {
		t.Run(written.name, func(t *testing.T) {
			fixture := newGuardedSelectedFixture(t)
			region := written.which(fixture)
			fixture = fixture.publish(t, region, 9)
			read, readOK := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseExplicit), ReadCellPolicy[uint64]{})
			if !readOK {
				t.Fatal("selected read")
			}
			run := NewRun(1, 1)
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 6, 6, 6)
			if !issued {
				t.Fatal("issue")
			}
			members := []RouteMember{fixture.member(0, 1)}
			cells := make([]operand.SelectedCell[uint64], selectedFixtureWidth)
			var scratch SelectedScratch[uint64, uint64]
			if status := read.Observe(ticket, &scratch, members, cells); status != ReadAvailable {
				t.Fatalf("observe status = %d", status)
			}
			if !cells[0].Present || cells[0].Value != 9 {
				t.Fatalf("selected member cell = %d/%t, want the written 9", cells[0].Value, cells[0].Present)
			}
			if !cells[0].Region.Equal(region) {
				t.Fatal("selected member region is not the region the value was written over")
			}
		})
	}
}
