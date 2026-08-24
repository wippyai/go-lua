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

func TestFoldExactTreatsAuthenticatedEmptySupportAsNoCandidate(t *testing.T) {
	fixture := newExecutionFixture(t)
	work := support.New(fixture.whole.Manager())
	if work == nil {
		t.Fatal("empty support work")
	}
	empty := work.False()
	if !work.Seal() {
		t.Fatal("empty support seal")
	}
	read, readOK := NewExactRead(fixture.binding, fixture.unit, 0)
	write, writeOK := NewExactWrite(fixture.binding, fixture.target, 0)
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, empty, []carrier.State{fixture.state}, 1, 1, 1, 1)
	if !readOK || !writeOK || !issued {
		t.Fatal("empty exact invocation")
	}
	var readScratch, writeScratch Scratch[uint64, uint64]
	outcome, valid := FoldExact(ticket, read, write, &readScratch, &writeScratch)
	if !valid || outcome != structure.NoCandidate {
		t.Fatalf("empty FoldExact = %v/%v", outcome, valid)
	}
	if !ticket.Submit(outcome) {
		t.Fatal("empty submit")
	}
	if _, count, drained := run.Drain(nil); !drained || count != 0 {
		t.Fatalf("empty drain = %d/%v", count, drained)
	}
}

func TestFoldExactEmptySupportStillAuthenticatesPorts(t *testing.T) {
	fixture := newExecutionFixture(t)
	work := support.New(fixture.whole.Manager())
	if work == nil {
		t.Fatal("empty support work")
	}
	empty := work.False()
	if !work.Seal() {
		t.Fatal("empty support seal")
	}
	for _, testCase := range []struct {
		name      string
		readPort  uint16
		writePort uint16
	}{
		{name: "foreign read port", readPort: 1, writePort: 0},
		{name: "foreign write port", readPort: 0, writePort: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			read, readOK := NewExactRead(fixture.binding, fixture.unit, testCase.readPort)
			write, writeOK := NewExactWrite(fixture.binding, fixture.target, testCase.writePort)
			run := NewRun(1, 1)
			ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, empty, []carrier.State{fixture.state}, 1, 1, 1, 1)
			if !readOK || !writeOK || !issued {
				t.Fatal("empty malformed invocation setup")
			}
			var readScratch, writeScratch Scratch[uint64, uint64]
			if outcome, valid := FoldExact(ticket, read, write, &readScratch, &writeScratch); valid || outcome != structure.Refuse {
				t.Fatalf("malformed empty FoldExact = %v/%v", outcome, valid)
			}
			if !ticket.Close() {
				t.Fatal("malformed empty ticket close")
			}
		})
	}
}

func TestFoldExactPublishesEveryGuardedInputRowInOnePatch(t *testing.T) {
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
	algebra, admitted := factbinding.Admit[uint64, uint64](2, 0, lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return 0 },
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
	}, func(uint64, uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !admitted {
		t.Fatal("algebra")
	}
	var input, output carrier.Unit
	var inputTarget, outputTarget carrier.Target
	binding, bound := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		var ok bool
		input, ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		output, ok = binding.DeclareExact(1)
		if !ok {
			return false
		}
		inputTarget, ok = binding.DeclareStrong(input)
		if !ok {
			return false
		}
		outputTarget, ok = binding.DeclareStrong(output)
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

	publishRun := NewRun(0, 1)
	publishTicket, issued := issueExecutionRow(publishRun, work, state, whole, nil, 1, 1, 1, 1)
	publish, publishOK := NewExactWrite(binding, inputTarget, 0)
	var publishScratch Scratch[uint64, uint64]
	if !issued || !publishOK || !publish.Stage(publishTicket, &publishScratch, on, 10) ||
		!publish.Stage(publishTicket, &publishScratch, off, 20) || !publish.Close(publishTicket, &publishScratch) ||
		!publishTicket.Submit(structure.Concrete) {
		t.Fatal("publish guarded input")
	}
	patches := make([]carrier.Patch, 1)
	_, count, drained := publishRun.Drain(patches)
	if !drained || count != 1 {
		t.Fatal("publish drain")
	}
	state, _, stateOK = work.Commit(state, patches[:count])
	if !stateOK {
		t.Fatal("publish commit")
	}

	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, work, state, whole, []carrier.State{state}, 1, 2, 2, 2)
	read, readOK := NewExactRead(binding, input, 0)
	write, writeOK := NewExactWrite(binding, outputTarget, 0)
	var readScratch, writeScratch Scratch[uint64, uint64]
	outcome, valid := FoldExact(ticket, read, write, &readScratch, &writeScratch)
	if !issued || !readOK || !writeOK || !valid || outcome != structure.Concrete || !ticket.Submit(outcome) {
		t.Fatalf("guarded FoldExact = %v/%t", outcome, valid)
	}
	_, count, drained = run.Drain(patches)
	if !drained || count != 1 {
		t.Fatalf("guarded patch count = %d/%t", count, drained)
	}
	next, _, committed := work.Commit(state, patches[:count])
	if !committed {
		t.Fatal("guarded commit")
	}

	observeRun := NewRun(1, 0)
	observeTicket, issued := issueExecutionRow(observeRun, work, next, whole, []carrier.State{next}, 0, 3, 3, 3)
	observe, observeOK := NewExactRead(binding, output, 0)
	var observeScratch Scratch[uint64, uint64]
	seen := map[uint64]bool{}
	for issued && observeOK {
		switch observe.Read(observeTicket, &observeScratch) {
		case ReadAvailable:
			value, ok := observeScratch.Value()
			if !ok || !observeScratch.Present() {
				t.Fatal("guarded output absence")
			}
			seen[value] = true
		case ReadExhausted:
			if !observe.Close(observeTicket, &observeScratch) || !observeTicket.Close() {
				t.Fatal("observe close")
			}
			if !seen[10] || !seen[20] || len(seen) != 2 {
				t.Fatalf("guarded outputs = %#v", seen)
			}
			return
		default:
			t.Fatal("observe refusal")
		}
	}
	t.Fatal("observe setup")
}
