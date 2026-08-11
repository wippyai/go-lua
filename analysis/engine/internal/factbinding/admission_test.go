package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func admissionInput(admit func(uint64, uint64) bool) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      3,
		Default:     0,
		AdmitAt:     admit,
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join:        func(left, right uint64) uint64 { return left | right },
		Widen:       func(left, right uint64) uint64 { return left | right },
		LessOrEq:    func(left, right uint64) bool { return left&right == left },
	}
}

func admissionState(t testing.TB, config testAlgebraInput[uint64, uint64]) (*Binding[uint64, uint64], carrier.State, *carrier.Composition, support.Mask, testFixture) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, state, _, composition, fixture := bindingState(t, manager, config, whole)
	return binding, state, composition, whole, fixture
}

func TestBindingRejectsDefaultNotAdmittedAtEveryDenseCoordinate(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	config := admissionInput(func(key, value uint64) bool {
		return key != 1 || value != 0
	})
	if binding, ok := bindTest(config, manager); ok || binding != nil {
		t.Fatal("binding accepted a sparse Default rejected at dense key 1")
	}
}

func TestBindRejectsZeroAlgebra(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if binding, ok := Bind((*Algebra[uint64, uint64])(nil), manager, nil); ok || binding != nil {
		t.Fatal("nil algebra bound")
	}
	if binding, ok := Bind(&Algebra[uint64, uint64]{}, manager, nil); ok || binding != nil {
		t.Fatal("zero algebra bound")
	}
}

func TestBindingAdmissionRejectsWrongStrongKey(t *testing.T) {
	binding, state, composition, whole, fixture := admissionState(t, admissionInput(func(key, value uint64) bool {
		return value == 0 || key == 0 && value == 1 || key == 1 && value == 2 || key == 2 && value == 4
	}))
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("patch")
	}
	if patch.Write(fixture.target(t, 1, carrier.StrongTarget), whole, 1) {
		t.Fatal("strong target accepted a value admitted only at another key")
	}
	if !patch.Discard() {
		t.Fatal("discard")
	}
}

func TestBindingAdmissionRejectsHeterogeneousWeakTargetAtomically(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var first, second carrier.Unit
	var firstTarget, weak carrier.Target
	config := admissionInput(func(key, value uint64) bool {
		return value == 0 || key == 0 && value == 1
	})
	config.declare = func(binding *Binding[uint64, uint64]) bool {
		var declared bool
		first, declared = binding.DeclareExact(0)
		if !declared {
			return false
		}
		second, declared = binding.DeclareExact(1)
		if !declared {
			return false
		}
		firstTarget, declared = binding.DeclareStrong(first)
		if !declared {
			return false
		}
		weak, declared = binding.DeclareWeak([]carrier.Unit{first, second})
		return declared
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
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("patch")
	}
	if patch.Write(weak, whole, 1) {
		t.Fatal("heterogeneous weak target accepted a key-local value")
	}
	// A failed weak admission occurs before its first key rewrite. A later
	// admitted exact write on the same candidate proves no prefix was staged.
	if !patch.Write(firstTarget, whole, 1) {
		t.Fatal("admitted exact write after rejected weak target")
	}
	published, ok := patch.Accept(work)
	if !ok {
		t.Fatal("publication")
	}
	next := commit(t, work, state, published)
	root, ok := next.HandleAt(0)
	if !ok {
		t.Fatal("root")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, second, whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("rejected weak target changed key 1: %d/%t/%t", got, present, valid)
	}
}

func TestBindingAdmissionAcceptsAdmittedJoinAndWiden(t *testing.T) {
	config := admissionInput(func(_ uint64, value uint64) bool { return value <= 3 })
	config.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	binding, state, composition, whole, fixture := admissionState(t, config)
	selection, ok := composition.SealWidening([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("widen scope")
	}
	baseWork := newWork(t, composition)
	write := func(value uint64) carrier.State {
		patch := binding.Begin(baseWork, state)
		if patch == nil || !patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, value) {
			t.Fatal("strong write")
		}
		published, ok := patch.Accept(baseWork)
		if !ok {
			t.Fatal("publication")
		}
		return commit(t, baseWork, state, published)
	}
	left, right := write(1), write(2)
	if _, _, ok := newWork(t, composition).Merge3Under(carrier.Widen, left, right, selection); !ok {
		t.Fatal("admitted widening result was rejected")
	}
	joinWork := newWork(t, composition)
	patch := binding.Begin(joinWork, left)
	if patch == nil || !patch.Write(fixture.target(t, 0, carrier.WeakTarget), whole, 2) {
		t.Fatal("admitted join result was rejected")
	}
	published, ok := patch.Accept(joinWork)
	if !ok {
		t.Fatal("admitted join did not publish")
	}
	if _, _, ok := joinWork.Commit(left, []carrier.Patch{published}); !ok {
		t.Fatal("admitted join did not commit")
	}
}

func TestBindingAdmissionRejectsNonAdmittedJoinResultTransactionally(t *testing.T) {
	config := admissionInput(func(_ uint64, value uint64) bool { return value != 3 })
	binding, state, composition, whole, fixture := admissionState(t, config)
	work := newWork(t, composition)
	seed := binding.Begin(work, state)
	if seed == nil || !seed.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("seed")
	}
	published, ok := seed.Accept(work)
	if !ok {
		t.Fatal("seed publication")
	}
	state = commit(t, work, state, published)
	rejected := binding.Begin(work, state)
	if rejected == nil {
		t.Fatal("join candidate")
	}
	if rejected.Write(fixture.target(t, 0, carrier.WeakTarget), whole, 2) {
		t.Fatal("join published a result rejected by target admission")
	}
	// Weak join consumes a failed candidate; the published predecessor remains
	// the only state and must still hold its original exact value.
	root, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("root")
	}
	if got, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || got != 1 {
		t.Fatalf("failed join changed published predecessor: %d/%t/%t", got, present, valid)
	}
}

func TestBindingAdmissionRejectsNonAdmittedWidenResult(t *testing.T) {
	config := admissionInput(func(_ uint64, value uint64) bool { return value != 3 })
	config.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	binding, state, composition, whole, fixture := admissionState(t, config)
	selection, ok := composition.SealWidening([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("widen scope")
	}
	write := func(value uint64) carrier.State {
		work := newWork(t, composition)
		patch := binding.Begin(work, state)
		if patch == nil || !patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, value) {
			t.Fatal("strong write")
		}
		published, accepted := patch.Accept(work)
		if !accepted {
			t.Fatal("publication")
		}
		return commit(t, work, state, published)
	}
	left, right := write(1), write(2)
	if _, _, accepted := newWork(t, composition).Merge3Under(carrier.Widen, left, right, selection); accepted {
		t.Fatal("widen published a result rejected by target admission")
	}
}
