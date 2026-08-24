// installer_external_law_test.go is deliberately an external test package. The
// installed-family seam exists so a rule package outside this one can author
// the execution of its own rule, and only a compiler that refuses unexported
// access can prove that surface is complete.
package execution_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// externalKey is the axis's own dense key type. A rule package names it to
// seal rows on its plane, which is what makes the installer implementable
// here at all.
type externalKey uint32

// externalReducer is one row's domain judgment. Its value differs per row -
// the successor it publishes is indexed by the row's candidate - while its
// type is the family's, so the fold's call stays a static direct call.
type externalReducer struct{ candidate uint32 }

func (reducer externalReducer) Reduce(read uint64, present bool) (uint64, structure.ReductionOutcome) {
	if !present {
		return 0, structure.NoCandidate
	}
	return read + uint64(reducer.candidate)*1000, structure.Concrete
}

type externalRow struct {
	reducer externalReducer
	read    execution.ExactRead[externalKey, uint64]
	write   execution.CarryWrite[externalKey, uint64]
}

type externalFamily struct{ rows []externalRow }

func (family *externalFamily) NewExecutor(run *execution.Run) execution.Executor {
	if family == nil || run == nil {
		return nil
	}
	return &externalWorker{family: family, run: run}
}
func (*externalFamily) InputCapacity() int  { return 1 }
func (*externalFamily) OutputCapacity() int { return 1 }

type externalWorker struct {
	family *externalFamily
	run    *execution.Run
	reads  execution.Scratch[externalKey, uint64]
	writes execution.Scratch[externalKey, uint64]
}

func (worker *externalWorker) Execute(frame execution.Frame, ticket execution.Ticket) (execution.Result, bool) {
	if worker == nil || worker.family == nil || !frame.Valid(ticket) || !worker.run.Owns(ticket) {
		return execution.Result{}, false
	}
	local, localOK := ticket.LocalOrdinal()
	if !localOK || uint64(local) >= uint64(len(worker.family.rows)) {
		return execution.Result{}, false
	}
	row := worker.family.rows[local]
	outcome := execution.FoldCarry(ticket, row.reducer, row.read, &worker.reads, row.write, &worker.writes)
	if !ticket.Submit(outcome) {
		return execution.Result{}, false
	}
	count := 0
	if outcome == structure.Concrete {
		count = 1
	}
	return execution.NewResult(outcome, count)
}

// externalInstaller is the rule package's own family author.
type externalInstaller struct{ rule uint32 }

func (installer externalInstaller) InstallRuleFamily(plane execution.FormPlane[externalKey, uint64], rule uint32, rows []execution.FormRow) (execution.Family, []execution.FormAddress, bool) {
	if rule != installer.rule || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	family := &externalFamily{}
	addresses := make([]execution.FormAddress, 0, len(rows))
	for _, row := range rows {
		read, readOK := plane.ExactRead(row.Unit, row.Input)
		// The transition is candidate-indexed, so the map is sealed per row
		// against the candidate that row carries. It still has to fix the
		// Factor default, which the plane checks where the write is sealed.
		candidate := row.Candidate
		write, writeOK := plane.RowCarry(row, func(prior uint64) (uint64, bool) {
			if prior == 0 {
				return 0, true
			}
			return prior + uint64(candidate) + 100, true
		})
		if !readOK || !writeOK {
			return nil, nil, false
		}
		addresses = append(addresses, execution.FormAddress{Member: row.Member, Local: uint32(len(family.rows))})
		family.rows = append(family.rows, externalRow{reducer: externalReducer{candidate: candidate}, read: read, write: write})
	}
	return family, addresses, true
}

func externalLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return 0 },
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
	}
}

type externalFixture struct {
	binding *factbinding.Binding[externalKey, uint64]
	units   [2]carrier.Unit
	targets [2]carrier.Target
	state   carrier.State
	whole   support.Mask
	work    *carrier.Work
}

func newExternalFixture(t testing.TB) externalFixture {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, wholeOK := support.True(manager)
	algebra, algebraOK := factbinding.Admit[externalKey, uint64](2, 0, externalLattice(),
		func(_ externalKey, _ uint64) bool { return true }, func(value uint64) uint64 { return value },
		factbinding.Measure[externalKey, uint64]{}, factbinding.Measure[externalKey, uint64]{})
	if !wholeOK || !algebraOK {
		t.Fatal("algebra")
	}
	fixture := externalFixture{whole: whole}
	binding, bindingOK := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[externalKey, uint64]) bool {
		for key := externalKey(0); key < 2; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			fixture.units[key] = unit
		}
		for key := externalKey(0); key < 2; key++ {
			target, strong := binding.DeclareStrong(fixture.units[key])
			if !strong {
				return false
			}
			fixture.targets[key] = target
		}
		return true
	})
	if !bindingOK {
		t.Fatal("binding")
	}
	prepared, preparedOK := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !preparedOK {
		t.Fatal("prepare")
	}
	composition, attached := prepared.Attach()
	if !attached {
		t.Fatal("attach")
	}
	state, stateOK := carrier.NewState(composition, composition.Scope(), whole)
	work, workOK := composition.NewWork()
	if !stateOK || !workOK {
		t.Fatal("state")
	}
	fixture.binding, fixture.state, fixture.work = binding, state, work
	return fixture
}

func externalRowTicket(t testing.TB, run *execution.Run, fixture *externalFixture, candidate uint32, inputs []carrier.State) execution.Ticket {
	t.Helper()
	catalog, sealed := executioncatalog.Seal([]executioncatalog.Draft{{
		Rule: 5, Member: 0, Candidate: candidate, InputCount: uint16(len(inputs)), OutputCount: 1,
	}})
	if !sealed || catalog == nil {
		t.Fatal("catalog")
	}
	row, rowOK := catalog.At(0)
	if !rowOK {
		t.Fatal("catalog row")
	}
	ticket, issued := run.Issue(catalog, row, fixture.work, fixture.state, fixture.whole, inputs, carrier.SlotCoverage{}, 4, 9, 2)
	if !issued {
		t.Fatal("issue")
	}
	return ticket
}

func externalPublish(t testing.TB, fixture *externalFixture, target carrier.Target, value uint64) {
	t.Helper()
	run := execution.NewRun(0, 1)
	write, writeOK := execution.NewExactWrite(fixture.binding, target, 0)
	if !writeOK {
		t.Fatal("publish write")
	}
	ticket := externalRowTicket(t, run, fixture, 0, nil)
	var scratch execution.Scratch[externalKey, uint64]
	if !write.Stage(ticket, &scratch, fixture.whole, value) || !write.Close(ticket, &scratch) || !ticket.Submit(structure.Concrete) {
		t.Fatal("publish stage")
	}
	disposition, drained, drainedOK := run.Consume()
	if !drainedOK || disposition != structure.Concrete || len(drained) != 1 {
		t.Fatal("publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, drained)
	if !committed {
		t.Fatal("publish commit")
	}
	fixture.state = next
}

// TestARulePackageOutsideTheEngineAuthorsItsOwnFamily is the installed-family
// seam stated from where it is used. A rule whose fold the engine cannot type
// authors its own execution, and the surface it needs - the plane's sealed
// read and transformed-carry write, the row's candidate, the invocation's
// local ordinal, and the run that owns it - has to be complete without any
// unexported access at all. This file compiles outside the package, so a
// missing accessor is a build failure rather than a reviewer's judgment.
func TestARulePackageOutsideTheEngineAuthorsItsOwnFamily(t *testing.T) {
	fixture := newExternalFixture(t)
	externalPublish(t, &fixture, fixture.targets[0], 7)
	externalPublish(t, &fixture, fixture.targets[1], 5)

	rule := uint32(5)
	// The claim table is opened over the sealed rule table's own width: a
	// claim is a position in that table, so an external package opens one the
	// same way the engine does rather than growing a map as it claims.
	families, opened := execution.NewRuleFamilies[externalKey, uint64](8)
	if !opened {
		t.Fatal("family table")
	}
	if !families.Install(rule, externalInstaller{rule: rule}) {
		t.Fatal("family claim")
	}
	read, readOK := execution.NewForeignFactor(fixture.binding, execution.RouteTable{})
	if !readOK {
		t.Fatal("foreign read side")
	}
	plane, planeOK := execution.NewFormPlane(fixture.binding, nil, nil, execution.RouteTable{}, []execution.ForeignFactor{read}, families)
	if !planeOK {
		t.Fatal("form plane")
	}
	installer := externalInstaller{rule: rule}
	rows := []execution.FormRow{{Member: 0, Form: execution.FormCarry, Input: 0, Candidate: 3, Unit: fixture.units[0], Target: fixture.targets[0]}}
	family, addresses, built := installer.InstallRuleFamily(plane, rule, rows)
	if !built || family == nil || len(addresses) != 1 || addresses[0] != (execution.FormAddress{Member: 0, Local: 0}) {
		t.Fatal("out-of-package install")
	}

	run := execution.NewRun(1, 1)
	worker := family.NewExecutor(run)
	if worker == nil {
		t.Fatal("out-of-package executor")
	}
	ticket := externalRowTicket(t, run, &fixture, 3, []carrier.State{fixture.state})
	frame, framed := execution.NewFrame(ticket)
	if !framed {
		t.Fatal("frame")
	}
	result, executed := worker.Execute(frame, ticket)
	if !executed || result.Outcome() != structure.Concrete || result.Count() != 1 {
		t.Fatalf("out-of-package execute = %v/%d/%t", result.Outcome(), result.Count(), executed)
	}
	disposition, patches, drained := run.Consume()
	if !drained || disposition != structure.Concrete || len(patches) != 1 {
		t.Fatalf("out-of-package drain = %v/%d/%t", disposition, len(patches), drained)
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches)
	if !committed {
		t.Fatal("commit")
	}
	fixture.state = next

	// The row publishes its candidate-indexed successor of 7, and its carried
	// coordinate ages through the candidate-indexed map.
	if value := externalObserve(t, &fixture, fixture.units[0]); value != 7+3*1000 {
		t.Fatalf("row coordinate = %d", value)
	}
}

func externalObserve(t testing.TB, fixture *externalFixture, unit carrier.Unit) uint64 {
	t.Helper()
	run := execution.NewRun(1, 1)
	read, readOK := execution.NewExactRead(fixture.binding, unit, 0)
	if !readOK {
		t.Fatal("observe read")
	}
	ticket := externalRowTicket(t, run, fixture, 0, []carrier.State{fixture.state})
	var scratch execution.Scratch[externalKey, uint64]
	if read.Read(ticket, &scratch) != execution.ReadAvailable {
		t.Fatal("observe cursor")
	}
	value, valueOK := scratch.Value()
	present := scratch.Present()
	if !read.Close(ticket, &scratch) || !valueOK || !present {
		t.Fatal("observe close")
	}
	_ = ticket.Submit(structure.NoCandidate)
	_, _, _ = run.Consume()
	return value
}
