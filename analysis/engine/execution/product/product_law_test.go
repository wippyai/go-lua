package product

import (
	"testing"

	engineexecution "github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	carrierproduct "github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

type productFixture struct {
	binding  *factbinding.Binding[uint64, uint64]
	units    [2]carrier.Unit
	targets  [2]carrier.Target
	state    carrier.State
	whole    support.Mask
	work     *carrier.Work
	manager  *guard.Manager
	left     support.Mask
	right    support.Mask
	leftNot  support.Mask
	rightNot support.Mask
}

func newProductFixture(t *testing.T) productFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	symbols := support.New(manager)
	if symbols == nil {
		t.Fatal("support work")
	}
	left, leftOK := symbols.Literal(1, true)
	leftNot, leftNotOK := symbols.Literal(1, false)
	right, rightOK := symbols.Literal(2, true)
	rightNot, rightNotOK := symbols.Literal(2, false)
	if !leftOK || !leftNotOK || !rightOK || !rightNotOK || !symbols.Seal() {
		t.Fatal("guard regions")
	}
	algebra, ok := factbinding.Admit[uint64, uint64](2, 0, lattice.Lattice[uint64]{
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
	fixture := productFixture{whole: whole, manager: manager, left: left, right: right, leftNot: leftNot, rightNot: rightNot}
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for index := range fixture.units {
			unit, declared := binding.DeclareExact(uint64(index))
			if !declared {
				return false
			}
			fixture.units[index] = unit
		}
		for index := range fixture.targets {
			target, declared := binding.DeclareStrong(fixture.units[index])
			if !declared {
				return false
			}
			fixture.targets[index] = target
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
	fixture.binding, fixture.state, fixture.work = binding, state, work
	return fixture
}

func (fixture *productFixture) catalog(t testing.TB, inputs, outputs int) (*executioncatalog.Catalog, executioncatalog.Row) {
	t.Helper()
	catalog, ok := executioncatalog.Seal([]executioncatalog.Draft{{Rule: 1, Member: 1, Candidate: 1, InputCount: uint16(inputs), OutputCount: uint16(outputs)}})
	if !ok {
		t.Fatal("catalog")
	}
	row, ok := catalog.At(0)
	if !ok {
		t.Fatal("catalog row")
	}
	return catalog, row
}

func (fixture *productFixture) issue(t testing.TB, within support.Mask) (engineexecution.Run, engineexecution.Ticket) {
	t.Helper()
	catalog, row := fixture.catalog(t, 1, 0)
	run := engineexecution.NewRun(1, 0)
	ticket, ok := run.Issue(catalog, row, fixture.work, fixture.state, within, []carrier.State{fixture.state}, carrier.SlotCoverage{}, 1, 1, 1)
	if !ok {
		t.Fatal("issue")
	}
	return *run, ticket
}

func (fixture *productFixture) publish(t testing.TB, target carrier.Target, region support.Mask, value uint64) {
	t.Helper()
	catalog, row := fixture.catalog(t, 0, 1)
	run := engineexecution.NewRun(0, 1)
	ticket, ok := run.Issue(catalog, row, fixture.work, fixture.state, fixture.whole, nil, carrier.SlotCoverage{}, 1, 1, 1)
	if !ok {
		t.Fatal("publish issue")
	}
	write, writeOK := engineexecution.NewExactWrite(fixture.binding, target, 0)
	var scratch engineexecution.Scratch[uint64, uint64]
	if !writeOK || !write.Stage(ticket, &scratch, region, value) || !write.Close(ticket, &scratch) || !ticket.Submit(structure.Concrete) {
		t.Fatal("publish stage")
	}
	patches := make([]carrier.Patch, 1)
	outcome, count, drained := run.Drain(patches)
	if !drained || outcome != structure.Concrete || count != 1 {
		t.Fatal("publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("publish commit")
	}
	fixture.state = next
}

func (fixture *productFixture) readRows(t *testing.T, left, right engineexecution.ExactRead[uint64, uint64]) Rows[Cons[Cell[uint64], Cons[Cell[uint64], struct{}]]] {
	t.Helper()
	run, ticket := fixture.issue(t, fixture.whole)
	seed, status, ok := NewSeed(ticket)
	if !ok || status != RefineAvailable {
		t.Fatal("seed")
	}
	var leftScratch, rightScratch engineexecution.Scratch[uint64, uint64]
	var leftExt Extender[uint64, uint64, struct{}]
	leftRows, status, ok := leftExt.Extend(ticket, seed.Rows(), left, &leftScratch)
	if !ok || status != RefineAvailable {
		t.Fatal("left refinement")
	}
	var rightExt Extender[uint64, uint64, Cons[Cell[uint64], struct{}]]
	rows, status, ok := rightExt.Extend(ticket, leftRows, right, &rightScratch)
	if !ok || status != RefineAvailable {
		t.Fatalf("right refinement status=%v ok=%v left=%d", status, ok, leftRows.Count())
	}
	t.Cleanup(func() { _ = ticket.Close() })
	_ = run
	return rows
}

func TestProductChainsCrossingExactPartitions(t *testing.T) {
	fixture := newProductFixture(t)
	fixture.publish(t, fixture.targets[0], fixture.left, 10)
	fixture.publish(t, fixture.targets[0], fixture.leftNot, 11)
	fixture.publish(t, fixture.targets[1], fixture.right, 20)
	fixture.publish(t, fixture.targets[1], fixture.rightNot, 21)
	left, leftOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	right, rightOK := engineexecution.NewExactRead(fixture.binding, fixture.units[1], 0)
	if !leftOK || !rightOK {
		t.Fatal("reads")
	}
	rows := fixture.readRows(t, left, right)
	if rows.Count() != 4 {
		t.Fatalf("common refinement count = %d, want 4", rows.Count())
	}
	seen := map[uint64]map[uint64]bool{}
	for index := 0; index < rows.Count(); index++ {
		_, tuple, ok := rows.At(index)
		if !ok {
			t.Fatal("row")
		}
		leftCell := tuple.Tail().Head()
		rightCell := tuple.Head()
		if seen[leftCell.Value()] == nil {
			seen[leftCell.Value()] = map[uint64]bool{}
		}
		seen[leftCell.Value()][rightCell.Value()] = true
	}
	if len(seen) != 2 || !seen[10][20] || !seen[10][21] || !seen[11][20] || !seen[11][21] {
		t.Fatalf("crossing values = %#v", seen)
	}
}

func TestProductPreservesSparseAbsenceAcrossRefinement(t *testing.T) {
	fixture := newProductFixture(t)
	fixture.publish(t, fixture.targets[0], fixture.left, 10)
	fixture.publish(t, fixture.targets[0], fixture.leftNot, 11)
	left, leftOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	right, rightOK := engineexecution.NewExactRead(fixture.binding, fixture.units[1], 0)
	if !leftOK || !rightOK {
		t.Fatal("reads")
	}
	rows := fixture.readRows(t, left, right)
	if rows.Count() != 2 {
		t.Fatalf("sparse common refinement count = %d, want 2", rows.Count())
	}
	for index := 0; index < rows.Count(); index++ {
		_, tuple, ok := rows.At(index)
		if !ok || !tuple.Tail().Head().Present() || tuple.Head().Present() {
			t.Fatalf("sparse tuple %d = %#v", index, tuple)
		}
	}
}

func TestProductRowsRequireTicketLeaseAndRevokeAfterClose(t *testing.T) {
	fixture := newProductFixture(t)
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	_, ticketA := fixture.issue(t, fixture.whole)
	_, ticketB := fixture.issue(t, fixture.whole)
	seedA, status, ok := NewSeed(ticketA)
	if !ok || status != RefineAvailable || !seedA.Valid() {
		t.Fatal("seed A")
	}
	seedB, status, ok := NewSeed(ticketB)
	if !ok || status != RefineAvailable || !seedB.Valid() {
		t.Fatal("seed B")
	}
	var extender Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	if _, status, ok := extender.Extend(ticketB, seedA.Rows(), read, &scratch); ok || status != RefineRefuse {
		t.Fatal("seed A crossed into ticket B")
	}
	rows, status, ok := extender.Extend(ticketB, seedB.Rows(), read, &scratch)
	if !ok || status != RefineAvailable || !rows.Valid() {
		t.Fatal("seed B refinement")
	}
	if !ticketA.Close() || !ticketB.Close() {
		t.Fatal("ticket close")
	}
	if seedA.Valid() || seedA.Rows().Count() != 0 || rows.Valid() || rows.Count() != 0 {
		t.Fatal("closed ticket retained product rows")
	}
	if _, _, ok := rows.At(0); ok {
		t.Fatal("closed rows exposed a tuple")
	}
}

func TestProductRowsRejectStaleExtenderGeneration(t *testing.T) {
	fixture := newProductFixture(t)
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	_, ticket := fixture.issue(t, fixture.whole)
	seed, status, ok := NewSeed(ticket)
	if !ok || status != RefineAvailable {
		t.Fatal("seed")
	}
	var extender Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	first, status, ok := extender.Extend(ticket, seed.Rows(), read, &scratch)
	if !ok || status != RefineAvailable || !first.Valid() {
		t.Fatal("first refinement")
	}
	second, status, ok := extender.Extend(ticket, seed.Rows(), read, &scratch)
	if !ok || status != RefineAvailable || !second.Valid() {
		t.Fatal("second refinement")
	}
	if first.Valid() || first.Count() != 0 {
		t.Fatal("stale generation remained readable")
	}
	if _, _, ok := first.At(0); ok {
		t.Fatal("stale generation exposed overwritten tuple")
	}
	if !ticket.Close() {
		t.Fatal("ticket close")
	}
}

func TestProductDistinguishesEmptySeedAndZeroRows(t *testing.T) {
	fixture := newProductFixture(t)
	if (Rows[struct{}]{}).Valid() {
		t.Fatal("zero Rows was marked authenticated")
	}
	run, ticket := fixture.issue(t, fixture.whole)
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	var ext Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	if _, status, ok := ext.Extend(ticket, Rows[struct{}]{}, read, &scratch); ok || status != RefineRefuse {
		t.Fatal("zero Rows was treated as authenticated empty")
	}
	canonical, canonicalOK := carrierproduct.NewRows(fixture.whole)
	if !canonicalOK {
		t.Fatal("canonical rows")
	}
	if _, status, ok := ext.Extend(ticket, Rows[struct{}]{product: canonical, sealed: true}, read, &scratch); ok || status != RefineRefuse {
		t.Fatal("incomplete source was treated as empty")
	}
	if !ticket.Close() {
		t.Fatal("ticket close")
	}
	_ = run

	falseWork := support.New(fixture.manager)
	empty := falseWork.False()
	if !falseWork.Seal() {
		t.Fatal("empty seal")
	}
	_, emptyTicket := fixture.issue(t, empty)
	seed, status, ok := NewSeed(emptyTicket)
	if !ok || status != RefineEmpty || !seed.Rows().Valid() || seed.Rows().Count() != 0 {
		t.Fatal("authenticated empty seed")
	}
	if !emptyTicket.Close() {
		t.Fatal("empty ticket close")
	}
}

func TestProductRejectsZeroRowExactSource(t *testing.T) {
	fixture := newProductFixture(t)
	run, ticket := fixture.issue(t, fixture.whole)
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	var ext Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	// The authenticated exact Unit has no stored rows, but its canonical read
	// still emits one explicit sparse row. A fabricated zero-row source cannot
	// be manufactured through this public cursor; zero Rows above is the
	// structural boundary that protects the same law for a composed caller.
	seed, status, ok := NewSeed(ticket)
	if !ok || status != RefineAvailable {
		t.Fatal("seed")
	}
	rows, status, ok := ext.Extend(ticket, seed.Rows(), read, &scratch)
	if !ok || status != RefineAvailable || rows.Count() != 1 {
		t.Fatal("explicit sparse row")
	}
	if _, _, rowOK := rows.At(0); !rowOK {
		t.Fatal("explicit sparse row missing")
	}
	if !ticket.Close() {
		t.Fatal("ticket close")
	}
	_ = run
}

func TestProductExtenderReusesWarmBuffersAndSupportsHeterogeneousTypes(t *testing.T) {
	fixture := newProductFixture(t)
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	var ext Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	var tupleRows Rows[Cons[Cell[string], struct{}]]
	_ = tupleRows
	for invocation := 0; invocation < 2; invocation++ {
		run, ticket := fixture.issue(t, fixture.whole)
		seed, status, ok := NewSeed(ticket)
		if !ok || status != RefineAvailable {
			t.Fatal("seed")
		}
		rows, status, ok := ext.Extend(ticket, seed.Rows(), read, &scratch)
		if !ok || status != RefineAvailable || rows.Count() != 1 {
			t.Fatal("warm refinement")
		}
		if invocation == 0 && cap(ext.rows) == 0 {
			t.Fatal("no reusable tuple buffer")
		}
		if !ticket.Close() {
			t.Fatal("ticket close")
		}
		_ = run
	}
}

func TestProductStopsWhenTicketCheckpointIsRevoked(t *testing.T) {
	fixture := newProductFixture(t)
	run, ticket := fixture.issue(t, fixture.whole)
	seed, status, ok := NewSeed(ticket)
	if !ok || status != RefineAvailable {
		t.Fatal("seed should remain authenticated before refinement")
	}
	if !fixture.work.SetCheckpoint(func() bool { return false }) {
		t.Fatal("install revoked checkpoint")
	}
	read, readOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	if !readOK {
		t.Fatal("read")
	}
	var ext Extender[uint64, uint64, struct{}]
	var scratch engineexecution.Scratch[uint64, uint64]
	if _, status, ok := ext.Extend(ticket, seed.Rows(), read, &scratch); ok || status != RefineRefuse {
		t.Fatal("revoked checkpoint was laundered into a product")
	}
	if !fixture.work.SetCheckpoint(nil) {
		t.Fatal("restore checkpoint")
	}
	if !ticket.Close() {
		t.Fatal("revoked ticket close")
	}
	_ = run
}

func TestProductWarmCrossingRefinementAllocatesNothing(t *testing.T) {
	fixture := newProductFixture(t)
	fixture.publish(t, fixture.targets[0], fixture.left, 10)
	fixture.publish(t, fixture.targets[0], fixture.leftNot, 11)
	fixture.publish(t, fixture.targets[1], fixture.right, 20)
	fixture.publish(t, fixture.targets[1], fixture.rightNot, 21)
	left, leftOK := engineexecution.NewExactRead(fixture.binding, fixture.units[0], 0)
	right, rightOK := engineexecution.NewExactRead(fixture.binding, fixture.units[1], 0)
	catalog, row := fixture.catalog(t, 1, 0)
	run := engineexecution.NewRun(1, 0)
	if !leftOK || !rightOK || run == nil {
		t.Fatal("warm product fixture")
	}
	var leftScratch, rightScratch engineexecution.Scratch[uint64, uint64]
	var leftExt Extender[uint64, uint64, struct{}]
	var rightExt Extender[uint64, uint64, Cons[Cell[uint64], struct{}]]
	direct := func() bool {
		ticket, issued := run.Issue(catalog, row, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, carrier.SlotCoverage{}, 1, 1, 1)
		if !issued {
			return false
		}
		for {
			switch left.Read(ticket, &leftScratch) {
			case engineexecution.ReadAvailable:
			case engineexecution.ReadExhausted:
				return left.Close(ticket, &leftScratch) && ticket.Close()
			default:
				return false
			}
		}
	}
	refine := func(depth int) bool {
		ticket, issued := run.Issue(catalog, row, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, carrier.SlotCoverage{}, 1, 1, 1)
		if !issued {
			return false
		}
		seed, status, ok := NewSeed(ticket)
		if !ok || status != RefineAvailable {
			return false
		}
		if depth == 0 {
			return ticket.Close()
		}
		leftRows, status, ok := leftExt.Extend(ticket, seed.Rows(), left, &leftScratch)
		if !ok || status != RefineAvailable {
			return false
		}
		if depth == 1 {
			return leftRows.Count() == 2 && ticket.Close()
		}
		rows, status, ok := rightExt.Extend(ticket, leftRows, right, &rightScratch)
		return ok && status == RefineAvailable && rows.Count() == 4 && ticket.Close()
	}
	if !refine(2) || !refine(2) {
		t.Fatal("warm product preflight")
	}
	directAllocations := testing.AllocsPerRun(20, func() {
		if !direct() {
			t.Fatal("warm direct read")
		}
	})
	t.Logf("warm direct allocations=%v", directAllocations)
	for depth, name := range []string{"seed", "one", "two"} {
		allocations := testing.AllocsPerRun(20, func() {
			if !refine(depth) {
				t.Fatal("warm product " + name)
			}
		})
		t.Logf("warm product %s allocations=%v", name, allocations)
	}
	allocations := testing.AllocsPerRun(20, func() {
		if !refine(2) {
			t.Fatal("warm product refinement")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm crossing product allocated %v times", allocations)
	}
}
