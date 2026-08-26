package suspension

import (
	"testing"

	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

var (
	suspensionRouteBufferLawLen  int
	suspensionRouteBufferLawOK   bool
	suspensionRouteBufferLawPlan routePlan
)

func TestSuspensionSourceCellBufferUsesBoundedInlineStorage(t *testing.T) {
	var inline [sourceCellInlineWidth]reduceroperand.MemberCell[valuedomain.Value]
	cells, ok := sourceCellBuffer(sourceCellInlineWidth, inline[:])
	if !ok || len(cells) != sourceCellInlineWidth || cap(cells) != sourceCellInlineWidth {
		t.Fatalf("inline source cells = len %d cap %d/%t, want %d/%d/true", len(cells), cap(cells), ok, sourceCellInlineWidth, sourceCellInlineWidth)
	}
	cells[0].Present = true
	if !inline[0].Present {
		t.Fatal("inline source-cell buffer did not alias caller storage")
	}

	wide, wideOK := sourceCellBuffer(sourceCellInlineWidth+1, inline[:])
	if !wideOK || len(wide) != sourceCellInlineWidth+1 || cap(wide) < len(wide) {
		t.Fatalf("wide source cells = len %d cap %d/%t, want len %d and invocation-local storage", len(wide), cap(wide), wideOK, sourceCellInlineWidth+1)
	}
}

func TestSuspensionRouteBufferKeepsDenseOrderAndDeduplicates(t *testing.T) {
	var plan routePlan
	if !plan.add(route{tag: routeTag(7)}) || !plan.add(route{tag: routeTag(2)}) || !plan.add(route{tag: routeTag(7)}) {
		t.Fatal("route insertion rejected a valid alias")
	}
	first, firstOK := plan.at(0)
	second, secondOK := plan.at(1)
	if plan.count() != 2 || !firstOK || !secondOK || first.tag != routeTag(2) || second.tag != routeTag(7) {
		t.Fatalf("ordered routes = %#v/%t, %#v/%t count=%d, want tags [2 7]", first, firstOK, second, secondOK, plan.count())
	}
	for index := 1; index <= routeInlineWidth+2; index++ {
		if !plan.add(route{tag: routeTag(index + 10)}) {
			t.Fatalf("overflow route %d rejected", index)
		}
	}
	if plan.count() != routeInlineWidth+4 || len(plan.extra) != 4 {
		t.Fatalf("overflow plan count/extra = %d/%d, want %d/4", plan.count(), len(plan.extra), routeInlineWidth+4)
	}
}

func TestSuspensionRouteBufferInlineReductionAllocatesZeroPerCall(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		suspensionRouteBufferLawPlan = routePlan{}
		_ = suspensionRouteBufferLawPlan.add(route{tag: routeTag(3)})
		_ = suspensionRouteBufferLawPlan.add(route{tag: routeTag(1)})
		suspensionRouteBufferLawLen = suspensionRouteBufferLawPlan.count()
		suspensionRouteBufferLawOK = suspensionRouteBufferLawLen == 2
	})
	if !suspensionRouteBufferLawOK || allocs != 0 {
		t.Fatalf("inline route reduction = len %d/%t allocations %f, want 2/true and zero", suspensionRouteBufferLawLen, suspensionRouteBufferLawOK, allocs)
	}
}
