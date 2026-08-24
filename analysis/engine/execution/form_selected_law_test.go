package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// selectedFixture is a Factor with several sealed coordinates, which is what a
// selected read needs and the one-coordinate execution fixture cannot express.
// Units and their strong targets are positionally paired, exactly as the bound
// Factor pairs its route universe.
type selectedFixture struct {
	binding *factbinding.Binding[uint64, uint64]
	units   []carrier.Unit
	targets []carrier.Target
	state   carrier.State
	whole   support.Mask
	work    *carrier.Work
}

const selectedFixtureWidth = 4

func newSelectedFixture(t testing.TB) selectedFixture {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	algebra, ok := factbinding.Admit[uint64, uint64](selectedFixtureWidth, 0, lattice.Lattice[uint64]{
		Bottom:   func() uint64 { return 0 },
		Top:      func() uint64 { return ^uint64(0) },
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
	}, func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value },
		factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		t.Fatal("algebra")
	}
	units := make([]carrier.Unit, selectedFixtureWidth)
	targets := make([]carrier.Target, selectedFixtureWidth)
	binding, ok := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		// Every exact coordinate is sealed before any target: the declaration
		// phases are ordered, and a route universe is the strong target of a
		// coordinate that already exists.
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
	return selectedFixture{binding: binding, units: units, targets: targets, state: state, whole: whole, work: work}
}

func selectedContract(order ruleprogram.Order, sparse ruleprogram.Sparse) ruleplan.ReadContract {
	return ruleplan.ReadContract{
		Order:        order,
		Sparse:       sparse,
		OnOpaque:     ruleprogram.OnOpaqueRefuse,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
}

// member pairs one of the fixture's dense positions with the tag a derived
// relation issued for it, the way the Factor's own paired geometry does.
func (fixture selectedFixture) member(index int, tag uint64) RouteMember {
	return RouteMember{
		coordinate: SelectedCoordinate{Unit: fixture.units[index], Tag: tag},
		target:     fixture.targets[index],
	}
}

// canonicalMembers returns the fixture's first width members in the canonical
// order a derived member set publishes them in.
func (fixture selectedFixture) canonicalMembers(width int) []RouteMember {
	members := make([]RouteMember, 0, width)
	for index := 0; index < width; index++ {
		members = append(members, fixture.member(index, uint64(index)+1))
	}
	return members
}

// TestSelectedReadDeliversOneCellPerDerivedMemberInDeclaredOrder states the
// whole point of the J read: the derived member set is bounded and ordered, and
// every member of it comes back as its own cell carrying its own tag and its
// own authenticated support region. A selection is not one value.
func TestSelectedReadDeliversOneCellPerDerivedMemberInDeclaredOrder(t *testing.T) {
	fixture := newSelectedFixture(t)
	read, ok := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseExplicit), ReadCellPolicy[uint64]{})
	if !ok || !read.Valid() {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := fixture.canonicalMembers(3)
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, cells); status != ReadAvailable {
		t.Fatalf("observe status = %d", status)
	}
	for index, member := range coordinates {
		if cells[index].Tag != member.Tag() {
			t.Fatalf("member %d tag = %d, want %d", index, cells[index].Tag, member.Tag())
		}
		if !cells[index].Region.Equal(fixture.whole) {
			t.Fatalf("member %d region is not the observed support row", index)
		}
	}
}

// TestSelectedReadRefusesADuplicateTagUnderByTagOrder states the ByTag clause:
// a member's ordinal is the rank of its own tag, so two members carrying one tag
// admit no member order at all. Delivering them in some arbitrary order would
// hand a Fold a positional assumption the declaration does not support.
func TestSelectedReadRefusesADuplicateTagUnderByTagOrder(t *testing.T) {
	fixture := newSelectedFixture(t)
	read, ok := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderByTag, ruleprogram.SparseExplicit), ReadCellPolicy[uint64]{})
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := []RouteMember{fixture.member(0, 7), fixture.member(1, 7)}
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, cells); status != ReadRefuse {
		t.Fatalf("duplicate tag observe status = %d, want refuse", status)
	}
}

// TestSelectedReadRefusesMembersOutsideTheDeclaredOrder states that the order
// clause is verified over the derived member set rather than assumed. A member
// set that is not in its declared presentation order is a defect in the relation
// that derived it, and delivering it anyway would silently renumber members.
func TestSelectedReadRefusesMembersOutsideTheDeclaredOrder(t *testing.T) {
	fixture := newSelectedFixture(t)
	read, ok := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderByTag, ruleprogram.SparseExplicit), ReadCellPolicy[uint64]{})
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	descending := []RouteMember{fixture.member(0, 9), fixture.member(1, 4)}
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, descending, cells); status != ReadRefuse {
		t.Fatalf("unordered observe status = %d, want refuse", status)
	}
}

// TestSelectedReadDeliversTheFactorDefaultAtAnUnwrittenCoordinate states the
// sparse clause: under the Factor-default declaration every member arrives
// present, so a Fold under that contract has no absent branch to get wrong. The
// substitution belongs to the read boundary, which is why the Fold never sees
// the unwritten coordinate at all.
func TestSelectedReadDeliversTheFactorDefaultAtAnUnwrittenCoordinate(t *testing.T) {
	fixture := newSelectedFixture(t)
	const factorDefault uint64 = 41
	read, ok := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseDefault),
		NewReadCellPolicy(true, factorDefault, ^uint64(0)))
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := fixture.canonicalMembers(2)
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, cells); status != ReadAvailable {
		t.Fatalf("observe status = %d", status)
	}
	for index := range coordinates {
		if !cells[index].Present || cells[index].Value != factorDefault {
			t.Fatalf("member %d = (%d,%v), want the Factor default present", index, cells[index].Value, cells[index].Present)
		}
	}
}

// TestSelectedReadWidenedDeliversTopAtEveryMember states that widening
// dominates every other substitution. A read whose declared contract widens
// on an opaque alternative delivers the Factor's Top at every coordinate,
// because Top is the sound over-approximation of anything the unobserved
// alternative could have written - and it does so uniformly, so no Fold can
// branch on which member was opaque.
func TestSelectedReadWidenedDeliversTopAtEveryMember(t *testing.T) {
	fixture := newSelectedFixture(t)
	const factorTop = ^uint64(0)
	propagating := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseDefault,
		OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	read, ok := NewSelectedRead(fixture.binding, 0, propagating, NewReadCellPolicy(true, 41, factorTop))
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := fixture.canonicalMembers(3)
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, cells); status != ReadAvailable {
		t.Fatalf("observe status = %d", status)
	}
	for index := range coordinates {
		if !cells[index].Present || cells[index].Value != factorTop {
			t.Fatalf("member %d = %d, want the Factor Top at every member", index, cells[index].Value)
		}
	}
}

// TestSelectedReadOnOpaqueGovernsWideningNotTheCaller states the OnOpaque
// clause itself: a PropagateAuthenticated contract widens the sealed read
// even when the caller's own policy was never widened, and a Refuse contract
// never widens even when the caller's policy was. The declared clause is the
// one authority over the widened arm; a caller-sealed policy has no
// competing vote on it.
func TestSelectedReadOnOpaqueGovernsWideningNotTheCaller(t *testing.T) {
	fixture := newSelectedFixture(t)
	const factorDefault, factorTop = uint64(41), ^uint64(0)

	propagating := ruleplan.ReadContract{
		Order:        ruleprogram.OrderCanonical,
		Sparse:       ruleprogram.SparseDefault,
		OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
		Multiplicity: ruleprogram.MultiplicityOne,
	}
	// The caller's policy is deliberately never widened. A declared
	// PropagateAuthenticated contract must widen it anyway.
	read, ok := NewSelectedRead(fixture.binding, 0, propagating, NewReadCellPolicy(true, factorDefault, factorTop))
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := fixture.canonicalMembers(3)
	cells := make([]SelectedCell[uint64], selectedFixtureWidth)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, cells); status != ReadAvailable {
		t.Fatalf("observe status = %d", status)
	}
	for index := range coordinates {
		if !cells[index].Present || cells[index].Value != factorTop {
			t.Fatalf("member %d = %d, want the declared OnOpaque clause to widen a caller policy that never was", index, cells[index].Value)
		}
	}

	// The reverse: a Refuse contract never widens even when the caller's own
	// policy was already widened before it was sealed.
	refusing := selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseDefault)
	read, ok = NewSelectedRead(fixture.binding, 0, refusing, NewReadCellPolicy(true, factorDefault, factorTop).Widen())
	if !ok {
		t.Fatal("selected read")
	}
	ticket = issueSelected(t, NewRun(1, 1), fixture, fixture.state)
	var otherScratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &otherScratch, coordinates, cells); status != ReadAvailable {
		t.Fatalf("observe status = %d", status)
	}
	for index := range coordinates {
		if !cells[index].Present || cells[index].Value != factorDefault {
			t.Fatalf("member %d = %d, want the declared Refuse clause to hold the Factor default, not the caller's own widened policy", index, cells[index].Value)
		}
	}
}

// TestSelectedReadRefusesAContractItCannotCarry states that the contract is a
// declaration this read either honours or refuses. A many-valued read is not
// one cell per member, so it is refused at seal rather than truncated at solve.
func TestSelectedReadRefusesAContractItCannotCarry(t *testing.T) {
	fixture := newSelectedFixture(t)
	many := selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseExplicit)
	many.Multiplicity = ruleprogram.MultiplicityMany
	if _, ok := NewSelectedRead(fixture.binding, 0, many, ReadCellPolicy[uint64]{}); ok {
		t.Fatal("a many-valued contract was admitted by a one-cell-per-member read")
	}
	unavailable := selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseExplicit)
	unavailable.Order = ruleprogram.OrderInvalid
	if _, ok := NewSelectedRead(fixture.binding, 0, unavailable, ReadCellPolicy[uint64]{}); ok {
		t.Fatal("an unavailable order clause was admitted")
	}
}

// TestSelectedReadRefusesAMemberSetWiderThanItsSealedStorage states that the
// member set is bounded by the seal. Storage is sized once from the declared
// denominator, so a wider derived set is a refusal rather than a reallocation on
// the solve path.
func TestSelectedReadRefusesAMemberSetWiderThanItsSealedStorage(t *testing.T) {
	fixture := newSelectedFixture(t)
	read, ok := NewSelectedRead(fixture.binding, 0, selectedContract(ruleprogram.OrderCanonical, ruleprogram.SparseExplicit), ReadCellPolicy[uint64]{})
	if !ok {
		t.Fatal("selected read")
	}
	run := NewRun(1, 1)
	ticket := issueSelected(t, run, fixture, fixture.state)
	coordinates := fixture.canonicalMembers(3)
	narrow := make([]SelectedCell[uint64], 2)
	var scratch SelectedScratch[uint64, uint64]
	if status := read.Observe(ticket, &scratch, coordinates, narrow); status != ReadRefuse {
		t.Fatalf("overwide observe status = %d, want refuse", status)
	}
}

func issueSelected(t testing.TB, run *Run, fixture selectedFixture, inputs ...carrier.State) Ticket {
	t.Helper()
	ticket, ok := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, len(run.outputs), 1, 17, 3)
	if !ok {
		t.Fatal("issue")
	}
	return ticket
}
