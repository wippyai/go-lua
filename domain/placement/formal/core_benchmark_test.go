package formal

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

var (
	formalSelectorRangeResult    formalSelectorRange
	formalDenseSealBenchmarkPlan routePlan
	formalDenseSealBenchmarkOK   bool
	formalPlannerBenchmarkPlan   routePlan
	formalPlannerBenchmarkOK     bool
)

// TestFormalDeliveredVectorIsConsumedWithoutACopy states what the deleted
// observation buffer used to: the derivation holds no per-invocation storage
// for the call's actuals. A member vector is a view over cells the family
// already owns, so reading every ordinal of one allocates nothing.
func TestFormalDeliveredVectorIsConsumedWithoutACopy(t *testing.T) {
	schema, values := formalSoundnessSchemas(t)
	keys := routePlanAllocationKeys(t, schema)
	if len(keys) == 0 {
		t.Skip("soundness schema exposes no allocation root")
	}
	atom, atomOK := values.Allocation(keys[0], materialization.Recent)
	fact, factOK := values.Singleton(atom)
	if !atomOK || !factOK {
		t.Fatal("allocation fact")
	}
	cells := make([]execution.MemberCell[valuedomain.Value], 16)
	for index := range cells {
		cells[index] = execution.MemberCell[valuedomain.Value]{Value: fact, Present: true}
	}
	actuals := formalActuals(t, cells)
	allocations := testing.AllocsPerRun(100, func() {
		var demands denseDemandScratch
		for ordinal := 0; ordinal < actuals.Count(); ordinal++ {
			unknown, demandOK := addFactDemandDense(schema, values, actuals, ordinal, placement.Retain, &demands)
			if !demandOK || unknown {
				t.Fatalf("ordinal %d demand = %t/%t", ordinal, demandOK, unknown)
			}
		}
		formalDenseSealBenchmarkPlan, formalDenseSealBenchmarkOK = (&routePlan{}).seal(schema, &demands)
	})
	if !formalDenseSealBenchmarkOK || formalDenseSealBenchmarkPlan.routeCount() != 1 {
		t.Fatalf("sealed plan = %t/%d, want the one root every ordinal names", formalDenseSealBenchmarkOK, formalDenseSealBenchmarkPlan.routeCount())
	}
	if allocations != 0 {
		t.Fatalf("delivered-vector reduction allocations = %f, want 0", allocations)
	}
}

// BenchmarkFormalSelectorRange measures the solve-time selector
// representation. planFor only needs the interval and flags, so no selected
// index slice is required on this path.
func BenchmarkFormalSelectorRange(b *testing.B) {
	for _, width := range []int{1, 16, 128} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			spec := vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 0}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalSelectorRangeResult = resolveFormalSelectorRange(spec, width, true)
			}
		})
	}
}

// BenchmarkFormalRoutePlanSeal exercises the planner's invocation-local
// dense demand set and authenticated Value/Pack all-root widening. The
// all-root case is included because an open Value or Pack runtime tail must
// not allocate one route per Heap root.
func BenchmarkFormalRoutePlanSeal(b *testing.B) {
	schema := routePlanFixtureSchema(b, 1024)
	keys := routePlanAllocationKeys(b, schema)
	for _, width := range []int{1, 16, 128, 1024} {
		width := width
		if width > len(keys) {
			b.Fatalf("dense route-plan fixture roots=%d, want at least %d", len(keys), width)
		}
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			var demands denseDemandScratch
			for index := width - 1; index >= 0; index-- {
				escape := placement.Retain
				if index%2 != 0 {
					escape = placement.Send
				}
				if !planAddDenseDemand(schema, keys[index], escape, index%7 == 0, &demands) {
					b.Fatalf("dense demand %d", index)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalDenseSealBenchmarkPlan, formalDenseSealBenchmarkOK = (&routePlan{}).seal(schema, &demands)
			}
			if !formalDenseSealBenchmarkOK || formalDenseSealBenchmarkPlan.routeCount() != width {
				b.Fatalf("dense sealed route plan = %t/%d, want %d", formalDenseSealBenchmarkOK, formalDenseSealBenchmarkPlan.routeCount(), width)
			}
		})
	}
	{
		var demands denseDemandScratch
		if !addUnknownAllDense(schema, &demands) {
			b.Fatal("dense all-root demand")
		}
		b.Run("all-root", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalDenseSealBenchmarkPlan, formalDenseSealBenchmarkOK = (&routePlan{}).seal(schema, &demands)
			}
			if !formalDenseSealBenchmarkOK || formalDenseSealBenchmarkPlan.routeCount() != len(keys) || !formalDenseSealBenchmarkPlan.allUnknown {
				b.Fatalf("dense all-root route plan = %t/%d/%t, want %d/true", formalDenseSealBenchmarkOK, formalDenseSealBenchmarkPlan.routeCount(), formalDenseSealBenchmarkPlan.allUnknown, len(keys))
			}
		})
	}
}

// BenchmarkFormalPlannerOpaqueDispatch covers the authenticated opaque Call
// boundary. The opaque arm has no Target/formal authority, so this benchmark
// measures the allocation-free no-route reduction after owner-fenced
// Call/Pack/Value checks. Fixture sealing is outside the timed region.
func BenchmarkFormalPlannerOpaqueDispatch(b *testing.B) {
	fixture := newOpaqueDispatchLawFixture(b, "formal-planner-benchmark")
	callFact := mustOpenDispatchValue(b, fixture.calls, fixture.key)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		formalPlannerBenchmarkPlan, formalPlannerBenchmarkOK = planFor(
			fixture.packs, fixture.calls, fixture.placement, fixture.values,
			fixture.contract, fixture.mounted, callFact, formalActuals(b, fixture.cells))
	}
	if !formalPlannerBenchmarkOK || formalPlannerBenchmarkPlan.routeCount() != 0 {
		b.Fatalf("planner result = %t/%d", formalPlannerBenchmarkOK, formalPlannerBenchmarkPlan.routeCount())
	}
}
