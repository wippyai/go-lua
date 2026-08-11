package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type declaredCapabilities struct {
	exact   [2]carrier.Unit
	summary carrier.Unit
	strong  carrier.Target
	weak    carrier.Target
	targets carrier.Selector
}

func TestDeclaredCapabilitiesAreTheOnlyBindingAuthority(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var declared declaredCapabilities
	config := capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		var ok bool
		declared.exact[0], ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		declared.exact[1], ok = binding.DeclareExact(1)
		if !ok {
			return false
		}
		declared.summary, ok = binding.DeclareSummary([]uint64{0, 1})
		if !ok {
			return false
		}
		declared.strong, ok = binding.DeclareStrong(declared.exact[0])
		if !ok {
			return false
		}
		declared.weak, ok = binding.DeclareWeak([]carrier.Unit{declared.summary})
		if !ok {
			return false
		}
		declared.targets, ok = binding.DeclareTargetSelector([]carrier.Target{declared.strong, declared.weak})
		return ok
	})
	binding, ok := bindTest(config, manager)
	if !ok {
		t.Fatal("binding")
	}
	peer, ok := bindTest(capabilityInput(nil), manager)
	if !ok {
		t.Fatal("peer binding")
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
	_, exactSlot := declared.exact[0].Slot()
	_, summarySlot := declared.summary.Slot()
	_, strongSlot := declared.strong.Slot()
	_, weakSlot := declared.weak.Slot()
	_, targetsSlot := declared.targets.Slot()
	if exactSlot || summarySlot || strongSlot || weakSlot || targetsSlot ||
		binding.Begin(peerWork, peerState) != nil ||
		binding.ValidUnit(declared.exact[0]) || binding.ValidTarget(declared.strong) || binding.ValidSelector(declared.targets, carrier.TargetSelector) {
		t.Fatal("pre-bind capability was usable")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare composition")
	}
	slot := shape.Slot(0)
	targetCandidates, ok := prepared.SelectorTargets(slot, declared.targets)
	if !ok {
		t.Fatal("prepared target selector surface")
	}
	if len(targetCandidates) != 2 || !targetCandidates[0].Same(declared.strong) || !targetCandidates[1].Same(declared.weak) {
		t.Fatal("target selector candidate surface")
	}
	composition, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	if !binding.ValidUnit(declared.exact[0]) || !binding.ValidUnit(declared.exact[1]) || !binding.ValidUnit(declared.summary) || !binding.ValidTarget(declared.strong) || !binding.ValidTarget(declared.weak) || !binding.ValidSelector(declared.targets, carrier.TargetSelector) {
		t.Fatal("attached semantic capabilities were not active")
	}
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.weak, whole, 7) {
		t.Fatal("weak finite-cover write")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("weak write")
	}
	next := commit(t, work, state, candidate)
	root, _ := next.HandleAt(slot)
	for key, unit := range declared.exact {
		if got, present, valid := observedExactValue(binding, work, root, unit, whole, func(guard.Atom) bool { return false }); !valid || !present || got != 7 {
			t.Fatalf("summary cover key %d = %d/%t/%t", key, got, present, valid)
		}
	}

	var foreignTarget carrier.Target
	foreign, ok := bindTest(capabilityInput(func(other *Binding[uint64, uint64]) bool {
		unit, ok := other.DeclareExact(0)
		if !ok {
			return false
		}
		foreignTarget, ok = other.DeclareStrong(unit)
		return ok
	}), manager)
	if !ok {
		t.Fatal("foreign binding")
	}
	foreignComposition, ok := attachTestComposition(t, []carrier.FactorOperation{foreign})
	if !ok {
		t.Fatal("foreign composition")
	}
	patch = binding.Begin(work, next)
	if patch == nil || patch.Write(foreignTarget, whole, 9) {
		t.Fatal("foreign target was accepted")
	}
	if !patch.Discard() {
		t.Fatal("discard")
	}
	_ = foreignComposition
}

func TestSummaryCannotGainStrongAuthority(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		summary, ok := binding.DeclareSummary([]uint64{0, 1})
		if !ok {
			return false
		}
		_, ok = binding.DeclareStrong(summary)
		return ok
	}), manager)
	if ok {
		t.Fatal("summary acquired strong authority")
	}
}

func TestDeclarationRegistrationIsCanonical(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		if _, ok := binding.DeclareExact(1); !ok {
			return false
		}
		_, ok := binding.DeclareExact(0)
		return ok
	}), manager); ok {
		t.Fatal("permuted exact registration was accepted")
	}
	if _, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		zero, ok := binding.DeclareExact(0)
		if !ok {
			return false
		}
		one, ok := binding.DeclareExact(1)
		if !ok {
			return false
		}
		if _, ok = binding.DeclareStrong(one); !ok {
			return false
		}
		_, ok = binding.DeclareStrong(zero)
		return ok
	}), manager); ok {
		t.Fatal("permuted target registration was accepted")
	}
}

func TestFailedWeakSummaryWriteLeavesItsPatchRootUntouched(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var exactZero, exactOne, summary carrier.Unit
	var strongOne carrier.Target
	var weakTarget carrier.Target
	config := capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		var ok bool
		exactZero, ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		exactOne, ok = binding.DeclareExact(1)
		if !ok {
			return false
		}
		summary, ok = binding.DeclareSummary([]uint64{0, 1})
		if !ok {
			return false
		}
		if _, ok = binding.DeclareStrong(exactZero); !ok {
			return false
		}
		strongOne, ok = binding.DeclareStrong(exactOne)
		if !ok {
			return false
		}
		weakTarget, ok = binding.DeclareWeak([]carrier.Unit{summary})
		return ok
	})
	// This intentionally proposes the incoming value even when it descends.
	// The first summary key can form a local candidate; the second rejects it.
	config.Join = func(_, incoming uint64) uint64 { return incoming }
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
	slot := shape.Slot(0)
	work := newWork(t, composition)
	seed := binding.Begin(work, state)
	if seed == nil || !seed.Write(strongOne, whole, 5) {
		t.Fatal("seed")
	}
	seedPatch, ok := seed.Accept(work)
	if !ok {
		t.Fatal("seed accept")
	}
	state = commit(t, work, state, seedPatch)

	patch := binding.Begin(work, state)
	if patch == nil || patch.Write(weakTarget, whole, 2) {
		t.Fatal("partial weak write was accepted")
	}
	_, ok = patch.Accept(work)
	if ok {
		t.Fatal("failed weak write left a publishable candidate")
	}
	root, _ := state.HandleAt(slot)
	if got, present, valid := observedExactValue(binding, work, root, exactZero, whole, func(guard.Atom) bool { return false }); !valid || present || got != 0 {
		t.Fatalf("first local key leaked = %d/%t/%t", got, present, valid)
	}
	if got, present, valid := observedExactValue(binding, work, root, exactOne, whole, func(guard.Atom) bool { return false }); !valid || !present || got != 5 {
		t.Fatalf("second key changed = %d/%t/%t", got, present, valid)
	}
}

func TestDeclaredCapabilityValidationDoesNotAllocate(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var declared declaredCapabilities
	binding, ok := bindTest(capabilityInput(func(binding *Binding[uint64, uint64]) bool {
		var ok bool
		declared.exact[0], ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		declared.strong, ok = binding.DeclareStrong(declared.exact[0])
		if !ok {
			return false
		}
		declared.targets, ok = binding.DeclareTargetSelector([]carrier.Target{declared.strong})
		return ok
	}), manager)
	if !ok {
		t.Fatal("binding")
	}
	if _, ok := attachTestComposition(t, []carrier.FactorOperation{binding}); !ok {
		t.Fatal("composition")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = binding.ValidUnit(declared.exact[0])
		_ = binding.ValidTarget(declared.strong)
		_ = binding.ValidSelector(declared.targets, carrier.TargetSelector)
	}); allocations != 0 {
		t.Fatalf("capability validation allocations/run = %g", allocations)
	}
}

func capabilityInput(declare func(*Binding[uint64, uint64]) bool) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd: 4, Default: 0, AdmitAt: func(_ uint64, _ uint64) bool { return true },
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
		declare:  declare,
	}
}
