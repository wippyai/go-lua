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

// readFact is the foreign Factor's fact type. It is deliberately not the type
// the written Factor publishes: the whole point of the foreign read is that
// the two never have to agree.
type readFact int32

func readFactLattice() lattice.Lattice[readFact] {
	return lattice.Lattice[readFact]{
		Bottom:   func() readFact { return 0 },
		Top:      func() readFact { return 0 },
		Equal:    func(left, right readFact) bool { return left == right },
		Same:     func(left, right readFact) bool { return left == right },
		LessOrEq: func(left, right readFact) bool { return left <= right },
		Join: func(left, right readFact) readFact {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right readFact) readFact {
			if left > right {
				return left
			}
			return right
		},
	}
}

func writeFactLattice() lattice.Lattice[uint64] {
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

// foreignCarryFixture is two Factors in one composition: the read Factor
// carries readFact over uint32 keys, the written Factor carries uint64 over
// uint64 keys. Neither the key nor the fact type is shared.
type foreignCarryFixture struct {
	read         *factbinding.Binding[uint32, readFact]
	write        *factbinding.Binding[uint64, uint64]
	readUnits    [2]carrier.Unit
	readTargets  [2]carrier.Target
	writeUnits   [2]carrier.Unit
	writeTargets [2]carrier.Target
	state        carrier.State
	whole        support.Mask
	work         *carrier.Work
}

func newForeignCarryFixture(t testing.TB) foreignCarryFixture {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	readAlgebra, readAlgebraOK := factbinding.Admit[uint32, readFact](2, 0, readFactLattice(),
		func(_ uint32, _ readFact) bool { return true }, func(value readFact) uint64 { return uint64(value) },
		factbinding.Measure[uint32, readFact]{}, factbinding.Measure[uint32, readFact]{})
	writeAlgebra, writeAlgebraOK := factbinding.Admit[uint64, uint64](2, 0, writeFactLattice(),
		func(_ uint64, _ uint64) bool { return true }, func(value uint64) uint64 { return value },
		factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !readAlgebraOK || !writeAlgebraOK {
		t.Fatal("algebra")
	}
	fixture := foreignCarryFixture{whole: whole}
	readBinding, readBindingOK := factbinding.Bind(readAlgebra, manager, func(binding *factbinding.Binding[uint32, readFact]) bool {
		for key := uint32(0); key < 2; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			fixture.readUnits[key] = unit
		}
		for key := uint32(0); key < 2; key++ {
			target, strong := binding.DeclareStrong(fixture.readUnits[key])
			if !strong {
				return false
			}
			fixture.readTargets[key] = target
		}
		return true
	})
	writeBinding, writeBindingOK := factbinding.Bind(writeAlgebra, manager, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for key := uint64(0); key < 2; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			fixture.writeUnits[key] = unit
		}
		for key := uint64(0); key < 2; key++ {
			target, strong := binding.DeclareStrong(fixture.writeUnits[key])
			if !strong {
				return false
			}
			fixture.writeTargets[key] = target
		}
		return true
	})
	if !readBindingOK || !writeBindingOK {
		t.Fatal("binding")
	}
	prepared, preparedOK := carrier.PrepareComposition([]carrier.FactorOperation{readBinding, writeBinding})
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
	fixture.read, fixture.write, fixture.state, fixture.work = readBinding, writeBinding, state, work
	return fixture
}

// publishRead commits one fact at one coordinate of the foreign Factor.
func (fixture *foreignCarryFixture) publishRead(t testing.TB, target carrier.Target, value readFact) {
	t.Helper()
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.read, target, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !writeOK || !issued {
		t.Fatal("foreign publish write")
	}
	var scratch Scratch[uint32, readFact]
	patches := make([]carrier.Patch, 1)
	if !write.Stage(ticket, &scratch, fixture.whole, value) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("foreign publish stage")
	}
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("foreign publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("foreign publish commit")
	}
	fixture.state = next
}

// publishWrite commits one fact at one coordinate of the written Factor.
func (fixture *foreignCarryFixture) publishWrite(t testing.TB, target carrier.Target, value uint64) {
	t.Helper()
	run := NewRun(0, 1)
	write, writeOK := NewExactWrite(fixture.write, target, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, nil, 1, 4, 9, 2)
	if !writeOK || !issued {
		t.Fatal("publish write")
	}
	var scratch Scratch[uint64, uint64]
	patches := make([]carrier.Patch, 1)
	if !write.Stage(ticket, &scratch, fixture.whole, value) || !write.Close(ticket, &scratch) || !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("publish stage")
	}
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatal("publish drain")
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("publish commit")
	}
	fixture.state = next
}

// observeWrite reads one committed coordinate of the written Factor back.
func (fixture *foreignCarryFixture) observeWrite(t testing.TB, unit carrier.Unit) (uint64, bool) {
	t.Helper()
	run := NewRun(1, 1)
	read, readOK := NewExactRead(fixture.write, unit, 0)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !readOK || !issued {
		t.Fatal("observe read")
	}
	var scratch Scratch[uint64, uint64]
	if read.Read(ticket, &scratch) != ReadAvailable {
		t.Fatal("observe cursor")
	}
	value, valueOK := scratch.Value()
	present := scratch.Present()
	if !read.Close(ticket, &scratch) {
		t.Fatal("observe close")
	}
	_ = run.Submit(&ticket, structure.NoCandidate)
	_, _, _, _ = run.Consume()
	return value, valueOK && present
}

// foreignCarryReducer reads the foreign Factor's fact and publishes the
// written Factor's. The signature is the whole statement: the fold's judgment
// is a map between two domains, not an endomorphism on one.
type foreignCarryReducer struct{ outcome structure.ReductionOutcome }

func (reducer foreignCarryReducer) Reduce(read readFact, present bool) (uint64, structure.ReductionOutcome) {
	if !present {
		return 0, structure.NoCandidate
	}
	return uint64(read) * 10, reducer.outcome
}

// TestAWTFoldReadsOneFactorAndWritesAnother is the heterogeneous primitive.
// A rule whose join is foreign reads a fact its own Factor never publishes and
// writes a fact the read Factor cannot represent. The fold therefore carries
// the read fact and the written fact as separate types: the row publishes the
// reduced value at its own coordinate, the carried coordinates of the WRITTEN
// Factor age through the owner's map, and the foreign Factor is left untouched.
func TestAWTFoldReadsOneFactorAndWritesAnother(t *testing.T) {
	fixture := newForeignCarryFixture(t)
	fixture.publishRead(t, fixture.readTargets[0], 7)
	fixture.publishWrite(t, fixture.writeTargets[0], 3)
	fixture.publishWrite(t, fixture.writeTargets[1], 5)

	foreign, foreignOK := NewForeignFactor(fixture.read, RouteTable{})
	if !foreignOK {
		t.Fatal("foreign read side")
	}
	read, readOK := ForeignExactRead[uint32, readFact](foreign, fixture.readUnits[0], 0)
	write, writeOK := NewCarryWrite(fixture.write, fixture.writeTargets[0], 0, []carrier.Target{fixture.writeTargets[1]}, ageCarry)
	if !readOK || !writeOK {
		t.Fatal("sealed foreign carry row")
	}
	run := NewRun(1, 1)
	ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, []carrier.State{fixture.state}, 1, 4, 9, 2)
	if !issued {
		t.Fatal("issue foreign carry invocation")
	}
	var reads Scratch[uint32, readFact]
	var writes Scratch[uint64, uint64]
	outcome := FoldCarry(ticket, foreignCarryReducer{outcome: structure.Concrete}, read, &reads, write, &writes)
	if outcome != structure.Concrete {
		t.Fatalf("foreign carry fold = %v, want Concrete", outcome)
	}
	if !run.Submit(&ticket, outcome) {
		t.Fatal("submit foreign carry invocation")
	}
	patches := make([]carrier.Patch, 1)
	disposition, count, drained := run.Drain(patches)
	if !drained || disposition != structure.Concrete || count != 1 {
		t.Fatalf("foreign carry drain = %v/%d/%t, want one patch", disposition, count, drained)
	}
	next, _, committed := fixture.work.Commit(fixture.state, patches[:count])
	if !committed {
		t.Fatal("commit foreign carry patch")
	}
	fixture.state = next

	if value, present := fixture.observeWrite(t, fixture.writeUnits[0]); !present || value != 70 {
		t.Fatalf("row coordinate = %d/%t, want the reduced 70", value, present)
	}
	if value, present := fixture.observeWrite(t, fixture.writeUnits[1]); !present || value != 105 {
		t.Fatalf("carried coordinate = %d/%t, want the aged 105", value, present)
	}
}

// TestAForeignReadIsSealedAtItsOwnTypes states what the erased handle protects.
// The read side is recovered at the types the read Factor was bound with, and
// a handle asked for any other pair is refused rather than reinterpreted: a
// binding is not a byte range that two domains can both claim.
func TestAForeignReadIsSealedAtItsOwnTypes(t *testing.T) {
	fixture := newForeignCarryFixture(t)
	foreign, foreignOK := NewForeignFactor(fixture.read, RouteTable{})
	if !foreignOK {
		t.Fatal("foreign read side")
	}
	if _, ok := ForeignExactRead[uint32, readFact](foreign, fixture.readUnits[0], 0); !ok {
		t.Fatal("the read Factor's own types were refused")
	}
	if _, ok := ForeignExactRead[uint64, uint64](foreign, fixture.readUnits[0], 0); ok {
		t.Fatal("a foreign read was sealed at another Factor's types")
	}
	if _, ok := ForeignExactRead[uint32, readFact](foreign, fixture.writeUnits[0], 0); ok {
		t.Fatal("a Unit minted by another binding was sealed as a foreign read")
	}
	if _, ok := NewForeignFactor[uint32, readFact](nil, RouteTable{}); ok {
		t.Fatal("an absent binding was sealed as a read side")
	}
}

// TestAnExactProductConsumesEveryOwnerIssuedJoin states the information-loss
// boundary behind binary folds. The Program chooses join ordinals; the Units
// are those the read Factor issued, retained in the sealed row in that same
// order. A family cannot substitute a Unit or invent a third read.
func TestAnExactProductConsumesEveryOwnerIssuedJoin(t *testing.T) {
	fixture := newForeignCarryFixture(t)
	foreign, foreignOK := NewForeignFactor(fixture.read, RouteTable{})
	row, classified := DeclaredForm(planCompiledExactProductRule(t))
	if !foreignOK || !classified || row.Form != FormExact {
		t.Fatal("exact-product setup")
	}
	var bound bool
	row, bound = row.BindExact(0, fixture.readUnits[0])
	if !bound {
		t.Fatal("bind first exact join")
	}
	row, bound = row.BindExact(1, fixture.readUnits[1])
	if !bound {
		t.Fatal("bind second exact join")
	}
	if _, ok := ForeignRowExactRead[uint32, readFact](foreign, row, 0); !ok {
		t.Fatal("first owner-issued exact join was unavailable")
	}
	if _, ok := ForeignRowExactRead[uint32, readFact](foreign, row, 1); !ok {
		t.Fatal("second owner-issued exact join was unavailable")
	}
	if _, ok := ForeignRowExactRead[uint32, readFact](foreign, row, 2); ok {
		t.Fatal("an undeclared third exact join was admitted")
	}
	if _, ok := row.BindExact(1, fixture.readUnits[1]); ok {
		t.Fatal("one exact join accepted a second Unit")
	}
	if _, ok := ForeignRowExactRead[uint64, uint64](foreign, row, 0); ok {
		t.Fatal("an exact join was reinterpreted at foreign types")
	}
}

// TestAWarmForeignCarryAllocatesNothing holds the heterogeneous fold to the
// same budget as the homogeneous one. Two lanes are two pieces of reusable
// storage, not two allocations: the read lane belongs to the Factor being read
// and the write lane to the Factor being written, and both live for the
// worker's lifetime.
func TestAWarmForeignCarryAllocatesNothing(t *testing.T) {
	fixture := newForeignCarryFixture(t)
	fixture.publishRead(t, fixture.readTargets[0], 7)
	fixture.publishWrite(t, fixture.writeTargets[0], 3)
	fixture.publishWrite(t, fixture.writeTargets[1], 5)
	foreign, foreignOK := NewForeignFactor(fixture.read, RouteTable{})
	if !foreignOK {
		t.Fatal("foreign read side")
	}
	read, readOK := ForeignExactRead[uint32, readFact](foreign, fixture.readUnits[0], 0)
	write, writeOK := NewCarryWrite(fixture.write, fixture.writeTargets[0], 0, []carrier.Target{fixture.writeTargets[1]}, ageCarry)
	if !readOK || !writeOK {
		t.Fatal("sealed foreign carry row")
	}
	run := NewRun(1, 1)
	var reads Scratch[uint32, readFact]
	var writes Scratch[uint64, uint64]
	inputs := []carrier.State{fixture.state}
	invoke := func() bool {
		ticket, issued := issueExecutionRow(run, fixture.work, fixture.state, fixture.whole, inputs, 1, 4, 9, 2)
		if !issued {
			return false
		}
		outcome := FoldCarry(ticket, foreignCarryReducer{outcome: structure.NoSelection}, read, &reads, write, &writes)
		if !run.Submit(&ticket, outcome) {
			return false
		}
		disposition, patches, _, drained := run.Consume()
		return drained && disposition == structure.NoSelection && len(patches) == 0
	}
	measureWarmInvocation(t, invoke, 0)
}
