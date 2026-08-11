package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// testAlgebraInput is test-only authored input for the public two-phase API.
// Production exposes only Admit followed by Bind.
type testAlgebraInput[K scalar.Key, V any] struct {
	KeyEnd      uint64
	Default     V
	AdmitAt     func(K, V) bool
	Equal       func(V, V) bool
	Same        func(V, V) bool
	Fingerprint func(V) uint64
	Join        func(V, V) V
	Widen       func(V, V) V
	Narrow      func(V, V) V
	LessOrEq    func(V, V) bool
	WidenRank   Measure[K, V]
	NarrowRank  Measure[K, V]
	declare     func(*Binding[K, V]) bool
}

func bindTest[K scalar.Key, V any](input testAlgebraInput[K, V], manager *guard.Manager) (*Binding[K, V], bool) {
	algebra, ok := Admit(input.KeyEnd, input.Default, lattice.Lattice[V]{Bottom: func() V { return input.Default }, Top: func() V { return input.Default }, Equal: input.Equal, Same: input.Same, Join: input.Join, Widen: input.Widen, Narrow: input.Narrow, LessOrEq: input.LessOrEq}, input.AdmitAt, input.Fingerprint, input.WidenRank, input.NarrowRank)
	if !ok {
		return nil, false
	}
	return Bind(algebra, manager, input.declare)
}

// testFixture retains only the capabilities a test declared while its Binding
// was sealed.  Tests must pass this value explicitly: a Binding is not a
// test-only capability registry, and no test may recover declarations by
// inspecting Binding storage.
type testFixture struct {
	exact  []carrier.Unit
	strong []carrier.Target
	weak   []carrier.Target
}

func newTestFixture(keyEnd uint64) testFixture {
	return testFixture{
		exact:  make([]carrier.Unit, keyEnd),
		strong: make([]carrier.Target, keyEnd),
		weak:   make([]carrier.Target, keyEnd),
	}
}

func (fixture *testFixture) declareAllExact(binding *Binding[uint64, uint64]) bool {
	if fixture == nil || len(fixture.exact) == 0 || len(fixture.strong) != len(fixture.exact) || len(fixture.weak) != len(fixture.exact) {
		return false
	}
	for key := range fixture.exact {
		unit, ok := binding.DeclareExact(uint64(key))
		if !ok {
			return false
		}
		fixture.exact[key] = unit
	}
	for key, unit := range fixture.exact {
		target, ok := binding.DeclareStrong(unit)
		if !ok {
			return false
		}
		fixture.strong[key] = target
	}
	for key, unit := range fixture.exact {
		target, ok := binding.DeclareWeak([]carrier.Unit{unit})
		if !ok {
			return false
		}
		fixture.weak[key] = target
	}
	return true
}

func (fixture testFixture) target(t testing.TB, key uint64, mode carrier.TargetMode) carrier.Target {
	t.Helper()
	if key >= uint64(len(fixture.exact)) {
		t.Fatalf("fixture key %d is outside declared surface", key)
		return carrier.Target{}
	}
	switch mode {
	case carrier.StrongTarget:
		return fixture.strong[key]
	case carrier.WeakTarget:
		return fixture.weak[key]
	default:
		t.Fatalf("unknown fixture target mode %d", mode)
		return carrier.Target{}
	}
}

func (fixture testFixture) unit(t testing.TB, key uint64) carrier.Unit {
	t.Helper()
	if key >= uint64(len(fixture.exact)) {
		t.Fatalf("fixture key %d is outside declared surface", key)
		return carrier.Unit{}
	}
	return fixture.exact[key]
}

func TestBindingStagesDefaultSparseFactThroughCarrier(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, slot, composition, fixture := bindingState(t, manager, testAlgebraInput[uint64, uint64]{
		KeyEnd:      8,
		Default:     3,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
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
		LessOrEq: func(left, right uint64) bool { return left <= right },
	}, whole)
	work := newWork(t, composition)
	base, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("base")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), base, fixture.unit(t, 2), whole, func(guard.Atom) bool { return false }); !valid || present || got != 3 {
		t.Fatalf("sparse default = %d/%t/%t, want 3/false/true", got, present, valid)
	}
	seed := binding.Begin(work, state)
	if seed == nil || !seed.Write(fixture.target(t, 2, carrier.StrongTarget), whole, 5) {
		t.Fatal("seed patch")
	}
	sourcePatch, ok := seed.Accept(work)
	if !ok {
		t.Fatal("seed did not publish a semantic change")
	}
	state = commit(t, work, state, sourcePatch)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(fixture.target(t, 2, carrier.WeakTarget), whole, 2) {
		t.Fatal("join patch")
	}
	nextPatch, ok := patch.Accept(work)
	if !ok {
		t.Fatal("joined patch did not publish a semantic change")
	}
	state = commit(t, work, state, nextPatch)
	next, ok := state.HandleAt(slot)
	if !ok {
		t.Fatal("next")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), next, fixture.unit(t, 2), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 5 {
		t.Fatalf("joined value = %d/%t/%t, want 5/true/true", got, present, valid)
	}

	noChange := binding.Begin(work, state)
	if noChange == nil || !noChange.Write(fixture.target(t, 2, carrier.StrongTarget), whole, 5) {
		t.Fatal("no-op patch")
	}
	returned, ok := noChange.Accept(work)
	if !ok {
		t.Fatal("semantic no-op did not retain its exact predecessor")
	}
	if committed := commit(t, work, state, returned); !work.EqualUnder(committed, state) {
		t.Fatal("no-op carrier commit changed state")
	}
}

func TestBindingEnforcesTypedWidenAndNarrowConvergenceLaws(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	newState := func(widen func(uint64, uint64) uint64, widenRank func(uint64, uint64, int) uint64, narrow func(uint64, uint64) uint64, narrowRank func(uint64, uint64, int) uint64) (*Binding[uint64, uint64], carrier.State, shape.Slot, *carrier.Composition, testFixture) {
		return bindingState(t, manager, testAlgebraInput[uint64, uint64]{
			KeyEnd:      4,
			Default:     0,
			AdmitAt:     func(_ uint64, _ uint64) bool { return true },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
			Join: func(left, right uint64) uint64 {
				if left > right {
					return left
				}
				return right
			},
			Widen:      widen,
			Narrow:     narrow,
			LessOrEq:   func(left, right uint64) bool { return left <= right },
			WidenRank:  Measure[uint64, uint64]{Width: 1, At: widenRank},
			NarrowRank: Measure[uint64, uint64]{Width: 1, At: narrowRank},
		}, whole)
	}
	write := func(work *carrier.Work, binding *Binding[uint64, uint64], fixture testFixture, state carrier.State, slot shape.Slot, value uint64) carrier.State {
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(fixture.target(t, 1, carrier.StrongTarget), whole, value) {
			t.Fatal("write")
		}
		next, ok := patch.Accept(work)
		if !ok {
			t.Fatal("publish")
		}
		return commit(t, work, state, next)
	}

	binding, base, slot, composition, fixture := newState(
		func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return ^value },
		func(left, right uint64) uint64 {
			if left < right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return value },
	)
	widenSelection := scopedWidenScope(t, composition, fixture, 1)
	narrowSelection, ok := composition.SealNarrowing([]carrier.Target{fixture.target(t, 1, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow selection")
	}
	two := write(newWork(t, composition), binding, fixture, base, slot, 2)
	three := write(newWork(t, composition), binding, fixture, base, slot, 3)
	work := newWork(t, composition)
	widened, widenedChanges, ok := work.Merge3Under(carrier.Widen, two, three, widenSelection)
	if !ok {
		t.Fatal("lawful widening rejected")
	}
	if widenedChanges.FactorCount() != 1 {
		t.Fatal("lawful widening omitted its Factor delta")
	}
	widenedRoot, _ := widened.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, newWork(t, composition), widenedRoot, fixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 3 {
		t.Fatalf("widened = %d/%t/%t, want 3/true/true", got, present, valid)
	}
	narrowed, narrowedChanges, ok := work.Merge3Under(carrier.Narrow, three, two, narrowSelection)
	if !ok {
		t.Fatal("lawful narrowing rejected")
	}
	if narrowedChanges.FactorCount() != 1 {
		t.Fatal("lawful narrowing omitted its Factor delta")
	}
	narrowedRoot, _ := narrowed.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, newWork(t, composition), narrowedRoot, fixture.unit(t, 1), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 2 {
		t.Fatalf("narrowed = %d/%t/%t, want 2/true/true", got, present, valid)
	}

	badRank, badBase, badSlot, badComposition, badRankFixture := newState(
		func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		func(uint64, uint64, int) uint64 { return 0 },
		func(left, right uint64) uint64 {
			if left < right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return value },
	)
	badRankSelection := scopedWidenScope(t, badComposition, badRankFixture, 1)
	if _, _, ok := newWork(t, badComposition).Merge3Under(carrier.Widen, write(newWork(t, badComposition), badRank, badRankFixture, badBase, badSlot, 2), write(newWork(t, badComposition), badRank, badRankFixture, badBase, badSlot, 3), badRankSelection); ok {
		t.Fatal("non-descending strict widen was accepted")
	}

	badBounds, badBoundsBase, badBoundsSlot, badBoundsComposition, badBoundsFixture := newState(
		func(left, _ uint64) uint64 { return left },
		func(_ uint64, value uint64, _ int) uint64 { return ^value },
		func(left, right uint64) uint64 {
			if left < right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return value },
	)
	badBoundsSelection := scopedWidenScope(t, badBoundsComposition, badBoundsFixture, 1)
	if _, _, ok := newWork(t, badBoundsComposition).Merge3Under(carrier.Widen, write(newWork(t, badBoundsComposition), badBounds, badBoundsFixture, badBoundsBase, badBoundsSlot, 2), write(newWork(t, badBoundsComposition), badBounds, badBoundsFixture, badBoundsBase, badBoundsSlot, 3), badBoundsSelection); ok {
		t.Fatal("non-upper-bound widen was accepted")
	}

	badNarrow, badNarrowBase, badNarrowSlot, badNarrowComposition, badNarrowFixture := newState(
		func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return ^value },
		func(left, right uint64) uint64 {
			if left == right {
				return left
			}
			return 1
		},
		func(_ uint64, value uint64, _ int) uint64 { return value },
	)
	badNarrowSelection, ok := badNarrowComposition.SealNarrowing([]carrier.Target{badNarrowFixture.target(t, 1, carrier.StrongTarget)})
	if !ok {
		t.Fatal("bad-narrow selection")
	}
	if _, _, ok := newWork(t, badNarrowComposition).Merge3Under(carrier.Narrow, write(newWork(t, badNarrowComposition), badNarrow, badNarrowFixture, badNarrowBase, badNarrowSlot, 3), write(newWork(t, badNarrowComposition), badNarrow, badNarrowFixture, badNarrowBase, badNarrowSlot, 2), badNarrowSelection); ok {
		t.Fatal("narrow below desired operand was accepted")
	}

	badNarrowRank, badNarrowRankBase, badNarrowRankSlot, badNarrowRankComposition, badNarrowRankFixture := newState(
		func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		func(_ uint64, value uint64, _ int) uint64 { return ^value },
		func(left, right uint64) uint64 {
			if left < right {
				return left
			}
			return right
		},
		func(uint64, uint64, int) uint64 { return 0 },
	)
	badNarrowRankSelection, ok := badNarrowRankComposition.SealNarrowing([]carrier.Target{badNarrowRankFixture.target(t, 1, carrier.StrongTarget)})
	if !ok {
		t.Fatal("bad-narrow-rank selection")
	}
	if _, _, ok := newWork(t, badNarrowRankComposition).Merge3Under(carrier.Narrow, write(newWork(t, badNarrowRankComposition), badNarrowRank, badNarrowRankFixture, badNarrowRankBase, badNarrowRankSlot, 3), write(newWork(t, badNarrowRankComposition), badNarrowRank, badNarrowRankFixture, badNarrowRankBase, badNarrowRankSlot, 2), badNarrowRankSelection); ok {
		t.Fatal("non-descending strict narrow was accepted")
	}
}

func TestFailedCompositionConsumesCandidateBindings(t *testing.T) {
	firstManager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	secondManager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	config := func() testAlgebraInput[uint64, uint64] {
		return testAlgebraInput[uint64, uint64]{
			KeyEnd:      1,
			AdmitAt:     func(_ uint64, _ uint64) bool { return true },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
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
			LessOrEq: func(left, right uint64) bool { return left <= right },
		}
	}
	first, ok := bindTest(config(), firstManager)
	if !ok {
		t.Fatal("first binding")
	}
	second, ok := bindTest(config(), secondManager)
	if !ok {
		t.Fatal("second binding")
	}
	if prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{first, second}); ok || prepared != nil {
		t.Fatal("mixed guard universes sealed a composition")
	}
	if prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{first}); ok || prepared != nil {
		t.Fatal("partially bound candidate was reused after failed seal")
	}
}

type rejectedPreflight struct{ called bool }

func (operation *rejectedPreflight) Preflight() (carrier.SlotOperation, bool) {
	if operation == nil || operation.called {
		return nil, false
	}
	operation.called = true
	return nil, false
}

func TestLaterPreflightFailureLeavesPriorDeclarationsUnattached(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var retained carrier.Unit
	config := func(declare func(*Binding[uint64, uint64]) bool) testAlgebraInput[uint64, uint64] {
		return testAlgebraInput[uint64, uint64]{
			KeyEnd:      1,
			Default:     0,
			AdmitAt:     func(_ uint64, _ uint64) bool { return true },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
			Join:        func(left, right uint64) uint64 { return left | right },
			Widen:       func(left, right uint64) uint64 { return left | right },
			LessOrEq:    func(left, right uint64) bool { return left&right == left },
			declare:     declare,
		}
	}
	candidate, ok := bindTest(config(func(binding *Binding[uint64, uint64]) bool {
		var declared bool
		retained, declared = binding.DeclareExact(0)
		return declared
	}), manager)
	if !ok {
		t.Fatal("candidate")
	}
	peer, ok := bindTest(config(nil), manager)
	if !ok {
		t.Fatal("peer")
	}
	peerComposition, ok := attachTestComposition(t, []carrier.FactorOperation{peer})
	if !ok {
		t.Fatal("peer composition")
	}
	peerState, ok := carrier.NewState(peerComposition, peerComposition.Scope(), whole)
	if !ok {
		t.Fatal("peer state")
	}
	peerWork := newWork(t, peerComposition)
	if _, attached := retained.Slot(); attached || candidate.Begin(peerWork, peerState) != nil {
		t.Fatal("pre-slot declaration was usable")
	}
	trailing, ok := bindTest(config(nil), manager)
	if !ok {
		t.Fatal("trailing")
	}
	later := &rejectedPreflight{}
	if prepared, sealed := carrier.PrepareComposition([]carrier.FactorOperation{candidate, later, trailing}); sealed || prepared != nil || !later.called {
		t.Fatal("later preflight failure sealed a composition")
	}
	peerWork = newWork(t, peerComposition)
	if _, attached := retained.Slot(); attached || candidate.ValidUnit(retained) || candidate.Begin(peerWork, peerState) != nil {
		t.Fatal("failed candidate attached a retained declaration")
	}
	if prepared, sealed := carrier.PrepareComposition([]carrier.FactorOperation{candidate}); sealed || prepared != nil {
		t.Fatal("failed candidate was reusable")
	}
	if prepared, sealed := carrier.PrepareComposition([]carrier.FactorOperation{trailing}); sealed || prepared != nil {
		t.Fatal("candidate after a failed preflight was reusable")
	}
}

func bindingState(t testing.TB, manager *guard.Manager, config testAlgebraInput[uint64, uint64], whole support.Mask) (*Binding[uint64, uint64], carrier.State, shape.Slot, *carrier.Composition, testFixture) {
	t.Helper()
	fixture := newTestFixture(config.KeyEnd)
	if config.declare == nil {
		config.declare = fixture.declareAllExact
	}
	binding, ok := bindTest(config, manager)
	if !ok {
		t.Fatal("binding")
	}
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	return binding, state, 0, composition, fixture
}

func commit(t testing.TB, work *carrier.Work, state carrier.State, patch carrier.Patch) carrier.State {
	t.Helper()
	next, _, ok := work.Commit(state, []carrier.Patch{patch})
	if !ok {
		t.Fatal("carrier commit")
	}
	return next
}

func newWork(t testing.TB, composition *carrier.Composition) *carrier.Work {
	t.Helper()
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("carrier work")
	}
	return work
}
