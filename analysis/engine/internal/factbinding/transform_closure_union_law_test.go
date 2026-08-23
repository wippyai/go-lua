package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestTransformClosuresStreamsTheSharedRouteClosure compares a route-only
// transform with the authored-static-plus-route transform at two route widths.
// The stage patch already performs the semantic O(R) rewrite; adding the
// one-coordinate authored closure must not introduce a second R-sized map or
// union slice on every transfer.
func TestTransformClosuresStreamsTheSharedRouteClosure(t *testing.T) {
	const (
		smallWidth = 32
		largeWidth = 1024
	)

	smallRoute, smallUnion := measureTransformClosureAllocs(t, smallWidth)
	largeRoute, largeUnion := measureTransformClosureAllocs(t, largeWidth)
	smallDelta := smallUnion - smallRoute
	largeDelta := largeUnion - largeRoute
	if largeUnion < largeRoute || largeDelta > smallDelta+2 {
		t.Fatalf("route closure union allocated a width-sized temporary: small route/union=%0.2f/%0.2f large route/union=%0.2f/%0.2f", smallRoute, smallUnion, largeRoute, largeUnion)
	}
	t.Logf("streamed route closure allocations: R=%d route/union=%0.2f/%0.2f; R=%d route/union=%0.2f/%0.2f", smallWidth, smallRoute, smallUnion, largeWidth, largeRoute, largeUnion)
}

func measureTransformClosureAllocs(t testing.TB, width int) (float64, float64) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	targets := make([]carrier.Target, width)
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      uint64(width),
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join:        func(left, right uint64) uint64 { return left | right },
		Widen:       func(left, right uint64) uint64 { return left | right },
		LessOrEq:    func(left, right uint64) bool { return left|right == right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			units := make([]carrier.Unit, width)
			for key := uint64(0); key < uint64(width); key++ {
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				units[key] = unit
			}
			for key := uint64(0); key < uint64(width); key++ {
				unit := units[key]
				target, declared := binding.DeclareStrong(unit)
				if !declared {
					return false
				}
				targets[key] = target
			}
			return true
		},
	}
	binding, _, slot, composition, _ := bindingState(t, manager, config, whole)
	seedPlan, ok := composition.SealContribution(0, []shape.Slot{slot}, nil)
	if !ok {
		t.Fatal("seed contribution plan")
	}
	source := carrier.ContributionSource{Slot: slot, Input: 0}
	carryPlan, ok := composition.SealContribution(1, []shape.Slot{slot}, []carrier.ContributionSource{source})
	if !ok {
		t.Fatal("carry contribution plan")
	}
	work := newWork(t, composition)
	seedBase, ok := work.BeginContribution(seedPlan, composition.Scope(), nil, whole)
	if !ok {
		t.Fatal("seed contribution base")
	}
	seed := binding.Begin(work, seedBase.State())
	if seed == nil {
		t.Fatal("seed patch")
	}
	for _, target := range targets {
		if !seed.Write(target, whole, 1) {
			t.Fatal("seed target")
		}
	}
	seedPatch, accepted := seed.Accept(work)
	if !accepted {
		t.Fatal("seed accept")
	}
	seedContribution, ok := work.FinishContribution(seedBase, []carrier.Patch{seedPatch})
	if !ok {
		t.Fatal("seed contribution finish")
	}
	seeded := seedContribution.State()
	seedPoint := closedPointOf(t, work, seedContribution)
	static, staticOK := binding.TransformClosure([]carrier.Target{targets[0]})
	route, routeOK := binding.TransformClosure(targets)
	if !staticOK || !routeOK {
		t.Fatal("transform closures")
	}
	identity := func(value uint64) (uint64, bool) { return value, true }
	routeOnly := []TransformClosure[uint64, uint64]{route}
	withStatic := []TransformClosure[uint64, uint64]{static, route}
	measure := func(closures []TransformClosure[uint64, uint64]) float64 {
		return testing.AllocsPerRun(20, func() {
			base, sourceCoverage := carryCoverageFor(t, work, carryPlan, source, seedPoint, whole)
			patch := binding.Begin(work, seeded)
			if patch == nil || !patch.TransformClosures(closures, sourceCoverage, whole, identity) || !patch.Discard() || !work.AbortRuleContribution(base, nil) {
				panic("transform closure allocation fixture")
			}
		})
	}
	return measure(routeOnly), measure(withStatic)
}
