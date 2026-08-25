package execution

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type executionFixture struct {
	binding *factbinding.Binding[uint64, uint64]
	unit    carrier.Unit
	target  carrier.Target
	state   carrier.State
	whole   support.Mask
	work    *carrier.Work
}

type testInvocationRow struct {
	once    sync.Once
	catalog *executioncatalog.Catalog
	row     executioncatalog.Row
}

var testInvocationRows [3][3]testInvocationRow

func newExecutionBinding(t testing.TB, manager *guard.Manager) (*factbinding.Binding[uint64, uint64], carrier.Unit, carrier.Target) {
	t.Helper()
	algebra, ok := factbinding.Admit[uint64, uint64](1, 0, lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 },
		Top:    func() uint64 { return 0 },
		Equal:  func(left, right uint64) bool { return left == right },
		Same:   func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool {
			return left <= right
		},
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
	}, func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("algebra")
	}
	var unit carrier.Unit
	var target carrier.Target
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		var declared bool
		unit, declared = binding.DeclareExact(0)
		if !declared {
			return false
		}
		target, declared = binding.DeclareStrong(unit)
		return declared
	})
	if !ok {
		t.Fatal("binding")
	}
	return binding, unit, target
}

func newExecutionFixture(t testing.TB) executionFixture {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	binding, unit, target := newExecutionBinding(t, manager)
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	return executionFixture{binding: binding, unit: unit, target: target, state: state, whole: whole, work: work}
}

func issueExecution(t testing.TB, run *Run, fixture executionFixture, inputs ...carrier.State) Ticket {
	t.Helper()
	ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, len(run.outputs), 1, 17, 3)
	if !ok {
		t.Fatal("issue")
	}
	return ticket
}

func issueExecutionRow(run *Run, work *carrier.Work, state carrier.State, within support.Mask, inputs []carrier.State, outputs int, epoch, revision, generation uint64) (Ticket, bool) {
	if outputs < 0 || outputs > int(^uint16(0)) || len(inputs) > int(^uint16(0)) {
		return Ticket{}, false
	}
	if len(inputs) >= len(testInvocationRows) || outputs >= len(testInvocationRows[0]) {
		return Ticket{}, false
	}
	entry := &testInvocationRows[len(inputs)][outputs]
	entry.once.Do(func() {
		entry.catalog, _ = executioncatalog.Seal([]executioncatalog.Draft{{Rule: 1, Member: 2, Candidate: 3, InputCount: uint16(len(inputs)), OutputCount: uint16(outputs)}})
		if entry.catalog != nil {
			entry.row, _ = entry.catalog.At(0)
		}
	})
	if entry.catalog == nil {
		return Ticket{}, false
	}
	return run.Issue(entry.catalog, entry.row, work, state, within, inputs, carrier.SlotCoverage{}, epoch, revision, generation)
}

func TestExecutionReadStatusAndIndependentWriteTransaction(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(1, 1)
	read, ok := NewExactRead(fixture.binding, fixture.unit, 0)
	if !ok || !read.Valid() {
		t.Fatal("read axis")
	}
	write, ok := NewExactWrite(fixture.binding, fixture.target, 0)
	if !ok || !write.Valid() {
		t.Fatal("write axis")
	}
	ticket := issueExecution(t, run, fixture, fixture.state)
	var scratch Scratch[uint64, uint64]
	if status := read.Read(ticket, &scratch); status != ReadAvailable {
		t.Fatalf("initial read status = %d", status)
	}
	if scratch.Present() {
		t.Fatal("default row reported present")
	}
	region, ok := scratch.Region()
	if !ok || !region.Equal(fixture.whole) {
		t.Fatal("read region")
	}
	if status := read.Read(ticket, &scratch); status != ReadExhausted {
		t.Fatalf("read exhaustion status = %d", status)
	}
	if !read.Close(ticket, &scratch) {
		t.Fatal("read close")
	}
	if !write.Stage(ticket, &scratch, fixture.whole, 7) {
		t.Fatal("independent stage")
	}
	if !write.Close(ticket, &scratch) {
		t.Fatal("write close")
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("submit")
	}
	if _, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 1, 17, 3); issued {
		t.Fatal("issue while submitted")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, ok := run.Drain(patches)
	if !ok || disposition != structure.Concrete || count != 1 {
		t.Fatal("drain")
	}
	if _, _, drained := run.Drain(patches); drained {
		t.Fatal("second drain accepted")
	}
	next, _, ok := fixture.work.Commit(fixture.state, patches[:count])
	if !ok {
		t.Fatal("commit")
	}

	if ticket.Valid() {
		t.Fatal("closed ticket remained valid")
	}
	if read.Read(ticket, &scratch) != ReadRefuse || write.Stage(ticket, &scratch, fixture.whole, 8) {
		t.Fatal("stale ticket was accepted")
	}
	if ticket.Close() {
		t.Fatal("double close accepted")
	}

	second := issueExecution(t, run, executionFixture{binding: fixture.binding, unit: fixture.unit, target: fixture.target, state: next, whole: fixture.whole, work: fixture.work}, next)
	if status := read.Read(second, &scratch); status != ReadAvailable || !scratch.Present() {
		t.Fatal("published value was not readable")
	}
	value, ok := scratch.Value()
	if !ok || value != 7 {
		t.Fatalf("published value = %d/%t", value, ok)
	}
	if !read.Close(second, &scratch) || !second.Close() {
		t.Fatal("second lifecycle")
	}
}

func TestExecutionRejectsForeignRunAndAllowsZeroInputSource(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, ok := NewExactWrite(fixture.binding, fixture.target, 0)
	if !ok {
		t.Fatal("source write axis")
	}
	ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !ok {
		t.Fatal("zero-input issue")
	}
	var scratch Scratch[uint64, uint64]
	if !write.Stage(ticket, &scratch, fixture.whole, 11) {
		t.Fatal("source stage")
	}
	if !write.Close(ticket, &scratch) {
		t.Fatal("source close")
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("source submit")
	}
	if disposition, count, ok := run.Drain(make([]carrier.Patch, 1)); !ok || disposition != structure.Concrete || count != 1 {
		t.Fatal("source drain")
	}

	foreignRun := NewRun(0, 1)
	foreign, ok := issueExecutionRow(foreignRun, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !ok {
		t.Fatal("foreign issue")
	}
	if !write.Stage(foreign, &scratch, fixture.whole, 12) {
		t.Fatal("foreign ticket refused")
	}
	if !scratch.Discard(foreign) {
		t.Fatal("foreign staged patch discard")
	}
	if !foreign.Close() {
		t.Fatal("foreign close")
	}
}

func TestExecutionSubmitRefuseAndOpaqueFailClosed(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 1)
	write, ok := NewExactWrite(fixture.binding, fixture.target, 0)
	if !ok {
		t.Fatal("write axis")
	}
	for _, outcome := range []structure.ReductionOutcome{structure.Refuse, structure.AuthenticatedOpaque} {
		ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 1, 31, 1)
		if !issued {
			t.Fatal("issue")
		}
		var scratch Scratch[uint64, uint64]
		if !write.Stage(ticket, &scratch, fixture.whole, 4) || !write.Close(ticket, &scratch) {
			t.Fatal("stage output")
		}
		if !run.Submit(&ticket, outcome) {
			t.Fatalf("outcome %d was not transported", outcome)
		}
		if ticket.Valid() {
			t.Fatal("fail-closed ticket remained live")
		}
		if disposition, count, drained := run.Drain(nil); !drained || disposition != outcome || count != 0 {
			t.Fatal("fail-closed drain")
		}
	}
}

func TestExecutionMultiInputAndHeterogeneousOutputSlots(t *testing.T) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	readBinding, readUnit, _ := newExecutionBinding(t, manager)
	writeBinding, _, writeTarget := newExecutionBinding(t, manager)
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{readBinding, writeBinding})
	if !ok {
		t.Fatal("prepare pair")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach pair")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("pair state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("pair work")
	}
	run := NewRun(2, 1)
	read, ok := NewExactRead(readBinding, readUnit, 1)
	if !ok {
		t.Fatal("port read")
	}
	write, ok := NewExactWrite(writeBinding, writeTarget, 0)
	if !ok {
		t.Fatal("heterogeneous write")
	}
	ticket, ok := issueExecutionRow(run, work, state, whole, []carrier.State{state, state}, 1, 2, 41, 7)
	if !ok {
		t.Fatal("pair issue")
	}
	var readScratch Scratch[uint64, uint64]
	if read.Read(ticket, &readScratch) != ReadAvailable || !read.Close(ticket, &readScratch) {
		t.Fatal("port read lifecycle")
	}
	var writeScratch Scratch[uint64, uint64]
	if !write.Stage(ticket, &writeScratch, whole, 19) || !write.Close(ticket, &writeScratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("heterogeneous output lifecycle")
	}
	patches := make([]carrier.Patch, 1)
	if disposition, count, drained := run.Drain(patches); !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("pair drain")
	}
	if !work.Discard(patches[0]) {
		t.Fatal("pair discard")
	}
}

func TestExecutionOutputSlotsAreAtomic(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 2)
	first, ok := NewExactWrite(fixture.binding, fixture.target, 0)
	if !ok {
		t.Fatal("first axis")
	}
	second, ok := NewExactWrite(fixture.binding, fixture.target, 1)
	if !ok {
		t.Fatal("second axis")
	}
	ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 2, 3, 51, 8)
	if !ok {
		t.Fatal("issue")
	}
	var one Scratch[uint64, uint64]
	if !first.Stage(ticket, &one, fixture.whole, 2) || !first.Close(ticket, &one) {
		t.Fatal("first output")
	}
	if run.Submit(&ticket, structure.Concrete) {
		t.Fatal("partial output accepted")
	}
	if ticket.Valid() {
		t.Fatal("partial output ticket remained live")
	}
	if disposition, count, drained := run.Drain(nil); !drained || disposition != structure.Refuse || count != 0 {
		t.Fatal("partial output drain")
	}

	ticket, ok = issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 2, 3, 51, 9)
	if !ok {
		t.Fatal("second issue")
	}
	var left, right Scratch[uint64, uint64]
	if !first.Stage(ticket, &left, fixture.whole, 3) || !first.Close(ticket, &left) || !second.Stage(ticket, &right, fixture.whole, 4) || !second.Close(ticket, &right) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("two output submit")
	}
	patches := make([]carrier.Patch, 2)
	if disposition, count, drained := run.Drain(patches); !drained || disposition != structure.Concrete || count != 2 {
		t.Fatal("two output drain")
	}
	for _, patch := range patches {
		if !fixture.work.Discard(patch) {
			t.Fatal("two output discard")
		}
	}
}

func TestExecutionPreservesEveryOutcomeAcrossSubmitDrain(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	for _, want := range []structure.ReductionOutcome{structure.Refuse, structure.NoSelection, structure.NoCandidate, structure.AuthenticatedOpaque} {
		ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 0, 5, 61, uint64(want)+1)
		if !ok || !run.Submit(&ticket, want) {
			t.Fatalf("submit outcome %d", want)
		}
		got, count, drained := run.Drain(nil)
		if !drained || got != want || count != 0 {
			t.Fatalf("drained outcome = %d/%d/%t, want %d/0/true", got, count, drained, want)
		}
	}

	run = NewRun(0, 1)
	write, ok := NewExactWrite(fixture.binding, fixture.target, 0)
	if !ok {
		t.Fatal("concrete write axis")
	}
	ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 5, 61, 10)
	if !ok {
		t.Fatal("concrete issue")
	}
	var scratch Scratch[uint64, uint64]
	if !write.Stage(ticket, &scratch, fixture.whole, 22) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("concrete submit")
	}
	destination := make([]carrier.Patch, 1)
	got, count, drained := run.Drain(destination)
	if !drained || got != structure.Concrete || count != 1 {
		t.Fatal("concrete drain")
	}
	if !fixture.work.Discard(destination[0]) {
		t.Fatal("concrete discard")
	}
}

func TestExecutionRejectsGuardRegionOutsideInvocationSupport(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	_, ok := support.True(manager)
	if !ok {
		t.Fatal("whole")
	}
	guardWork := support.New(manager)
	off, ok := guardWork.Literal(1, false)
	if !ok || !guardWork.Seal() {
		t.Fatal("off support")
	}
	onWork := support.New(manager)
	on, ok := onWork.Literal(1, true)
	if !ok || !onWork.Seal() {
		t.Fatal("on support")
	}
	if off.Entails(on) || on.Entails(off) {
		t.Fatal("guard regions unexpectedly entail one another")
	}
	binding, unit, target := newExecutionBinding(t, manager)
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), on)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	run := NewRun(1, 1)
	read, ok := NewExactRead(binding, unit, 0)
	if !ok {
		t.Fatal("read")
	}
	write, ok := NewExactWrite(binding, target, 0)
	if !ok {
		t.Fatal("write")
	}
	ticket, ok := issueExecutionRow(run, work, state, on, []carrier.State{state}, 1, 1, 71, 1)
	if !ok {
		t.Fatal("issue")
	}
	if !ticket.issuer.within.SameHandle(on) {
		t.Fatal("ticket lost invocation support")
	}
	var scratch Scratch[uint64, uint64]
	if read.Read(ticket, &scratch) != ReadAvailable || !read.Close(ticket, &scratch) {
		t.Fatal("read lifecycle")
	}
	if write.Stage(ticket, &scratch, off, 2) {
		t.Fatal("outside guard region accepted")
	}
	if !write.Stage(ticket, &scratch, on, 2) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("in-support stage")
	}
	destination := make([]carrier.Patch, 1)
	if _, count, drained := run.Drain(destination); !drained || count != 1 || !work.Discard(destination[0]) {
		t.Fatal("guard drain")
	}
}

func TestExecutionWarmReadReusesRunAndScratch(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(1, 0)
	read, ok := NewExactRead(fixture.binding, fixture.unit, 0)
	if !ok {
		t.Fatal("read axis")
	}
	var scratch Scratch[uint64, uint64]
	readOnce := func() bool {
		ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 0, 6, 21, 5)
		if !issued || read.Read(ticket, &scratch) != ReadAvailable || !read.Close(ticket, &scratch) {
			return false
		}
		return ticket.Close()
	}
	if !readOnce() || !readOnce() {
		t.Fatal("warmup")
	}
	allocations := testing.AllocsPerRun(20, func() {
		if !readOnce() {
			t.Fatal("warm read")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm execution allocated %v times", allocations)
	}
}

func TestExecutionExecutorUsesOpaqueFrameAndRunDrain(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(1, 1)
	row, rowOK := NewExactRow(fixture.binding, fixture.unit, 0, fixture.target, 0)
	family, ok := NewExactFamily([]ExactRow[uint64, uint64]{row})
	var executor Executor
	if rowOK && ok {
		executor = family.NewExecutor(run)
	}
	if !rowOK || !ok || executor == nil {
		t.Fatal("sealed exact executor")
	}
	ticket := issueExecution(t, run, fixture, fixture.state)
	frame, ok := NewFrame(ticket)
	if !ok || !frame.Valid(ticket) {
		t.Fatal("opaque frame")
	}
	result, executed := executor.Execute(frame, ticket)
	if !executed || !result.Valid() || result.Outcome() != structure.NoCandidate || result.Count() != 0 {
		t.Fatalf("opaque execution result = %+v/%t", result, executed)
	}
	if _, _, _, drained := run.Consume(); !drained {
		t.Fatal("run drain")
	}
	if _, executed = executor.Execute(frame, ticket); executed {
		t.Fatal("stale frame was reused")
	}
	foreign := NewRun(1, 1)
	foreignTicket := issueExecution(t, foreign, fixture, fixture.state)
	foreignFrame, ok := NewFrame(foreignTicket)
	if !ok {
		t.Fatal("foreign frame")
	}
	if _, executed = executor.Execute(foreignFrame, foreignTicket); executed {
		t.Fatal("family worker accepted foreign frame")
	}
	_ = foreign.Abort()
}

func TestExecutionSummaryReadAxisPreservesRowsAndSparsePresence(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	whole := regions.True()
	onOne, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("on one")
	}
	onTwo, ok := regions.Literal(2, true)
	if !ok || !regions.Seal() {
		t.Fatal("on two")
	}
	algebra, ok := factbinding.Admit[uint64, uint64](4, 0, lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 },
		Top:    func() uint64 { return 0 },
		Equal:  func(left, right uint64) bool { return left == right },
		Same:   func(left, right uint64) bool { return left == right },
		LessOrEq: func(left, right uint64) bool {
			return left <= right
		},
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
	}, func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("algebra")
	}
	var correlated, distributive carrier.Unit
	var targets [4]carrier.Target
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		var units [4]carrier.Unit
		for key := uint64(0); key < 4; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			units[key] = unit
		}
		var declared bool
		correlated, declared = binding.DeclareSummary([]uint64{0, 1})
		if !declared {
			return false
		}
		distributive, declared = binding.DeclareDistributiveSummary([]uint64{2, 3})
		if !declared {
			return false
		}
		for key, unit := range units {
			target, accepted := binding.DeclareStrong(unit)
			if !accepted {
				return false
			}
			targets[key] = target
		}
		return true
	})
	if !ok {
		t.Fatal("binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(targets[0], onOne, 1) || !patch.Write(targets[1], onTwo, 2) || !patch.Write(targets[2], onOne, 1) || !patch.Write(targets[3], onTwo, 2) {
		t.Fatal("patch")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	next, _, ok := work.Commit(state, []carrier.Patch{candidate})
	if !ok {
		t.Fatal("commit")
	}
	run := NewRun(1, 0)
	axis, ok := NewSummaryRead(binding, correlated, 0)
	if !ok || !axis.Valid() {
		t.Fatal("summary axis")
	}
	ticket, ok := issueExecutionRow(run, work, next, whole, []carrier.State{next}, 0, 1, 17, 3)
	if !ok {
		t.Fatal("issue")
	}
	var scratch Scratch[uint64, uint64]
	rows := 0
	for {
		status := axis.Read(ticket, &scratch)
		if status == ReadExhausted {
			break
		}
		if status != ReadAvailable {
			t.Fatalf("summary read status = %d", status)
		}
		view, available := scratch.View()
		if !available || view.Count() != 2 {
			t.Fatal("summary view")
		}
		for index := 0; index < view.Count(); index++ {
			_, present := view.At(index)
			if index == 0 && !present {
				t.Fatal("first summary entry missing")
			}
		}
		region, available := scratch.Region()
		if !available || !region.Valid() {
			t.Fatal("summary region")
		}
		rows++
	}
	if rows != 4 || !axis.Close(ticket, &scratch) || !ticket.Close() {
		t.Fatalf("summary rows/close = %d", rows)
	}

	distributiveAxis, ok := NewSummaryRead(binding, distributive, 0)
	if !ok {
		t.Fatal("distributive axis")
	}
	ticket, ok = issueExecutionRow(run, work, next, whole, []carrier.State{next}, 0, 1, 17, 4)
	if !ok {
		t.Fatal("distributive issue")
	}
	if status := distributiveAxis.Read(ticket, &scratch); status != ReadAvailable {
		t.Fatalf("distributive read status = %d", status)
	}
	view, ok := scratch.Observation()
	if !ok || view.Count() != 2 {
		t.Fatal("distributive view")
	}
	if status := distributiveAxis.Read(ticket, &scratch); status != ReadExhausted || !distributiveAxis.Close(ticket, &scratch) || !ticket.Close() {
		t.Fatal("distributive lifecycle")
	}
	distributiveWarm := func() bool {
		serial := uint64(100)
		serial++
		ticket, issued := issueExecutionRow(run, work, next, whole, []carrier.State{next}, 0, 1, 17, serial)
		if !issued {
			return false
		}
		if distributiveAxis.Read(ticket, &scratch) != ReadAvailable || distributiveAxis.Read(ticket, &scratch) != ReadExhausted {
			return false
		}
		return distributiveAxis.Close(ticket, &scratch) && ticket.Close()
	}
	if !distributiveWarm() || !distributiveWarm() {
		t.Fatal("distributive warmup")
	}
	distributiveAlloc := testing.AllocsPerRun(20, func() {
		if !distributiveWarm() {
			t.Fatal("distributive warm read")
		}
	})
	if distributiveAlloc != 0 {
		t.Fatalf("warm distributive summary execution allocated %v times", distributiveAlloc)
	}
	serial := uint64(10)
	readSummary := func() bool {
		serial++
		ticket, issued := issueExecutionRow(run, work, next, whole, []carrier.State{next}, 0, 1, 17, serial)
		if !issued {
			return false
		}
		for {
			status := axis.Read(ticket, &scratch)
			if status == ReadExhausted {
				break
			}
			if status != ReadAvailable {
				return false
			}
		}
		return axis.Close(ticket, &scratch) && ticket.Close()
	}
	if !readSummary() || !readSummary() {
		t.Fatal("summary warmup")
	}
	if !readSummary() {
		t.Fatal("summary warm read")
	}
}
