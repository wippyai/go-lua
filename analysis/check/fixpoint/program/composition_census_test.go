package program

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCompositionCostCensusAggregatesExistingAttribution(t *testing.T) {
	stats := &Stats{}
	key := summary.DefaultSummaryKey(ref.FromSymbol(1))
	eligible := &solveAttribution{
		stats:       stats,
		key:         bodySolveAttributionKey{bodyID: 1, function: key, phase: SolvePhaseSummary},
		composition: body.CompositionEligibility{},
	}
	rejected := &solveAttribution{
		stats:       stats,
		key:         bodySolveAttributionKey{bodyID: 2, function: key, phase: SolvePhaseMaterialize},
		composition: body.CompositionEligibility{Reason: "shape:loop"},
	}
	stats.recordBodySolve(eligible, 11)
	stats.recordBodySolve(eligible, 13)
	stats.recordBodySolve(rejected, 17)
	want := []CompositionCost{
		{Eligible: true, BodySolves: 2, PointTransfers: 24},
		{Reason: "shape:loop", BodySolves: 1, PointTransfers: 17},
	}
	if got := stats.CompositionCostCensus(); !reflect.DeepEqual(got, want) {
		t.Fatalf("census = %#v, want %#v", got, want)
	}
	attribution := stats.BodySolveAttribution()
	if len(attribution) != 2 || !attribution[0].CompositionEligible && !attribution[1].CompositionEligible {
		t.Fatalf("body attribution lost composition verdicts: %#v", attribution)
	}
}

func TestProgramRunAttributesEligibleBodyWorkWithoutChangingSolve(t *testing.T) {
	stmts := parseChunk(t, `local function f(x) return x end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fn := onlyFunctionOrigin(t, bindings).Func
	stats := &Stats{}
	result, err := RunBoundFunction(fn, bindings, Config{
		Check: body.Config{Registry: standard.Registry()},
		Stats: stats,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.rootResult == nil {
		t.Fatal("ordinary program solve did not publish its root result")
	}
	census := stats.CompositionCostCensus()
	eligibleSolves := 0
	for _, cost := range census {
		if cost.Eligible {
			eligibleSolves += cost.BodySolves
		}
	}
	if eligibleSolves == 0 {
		t.Fatalf("eligible body work was not attributed: %#v", census)
	}
}
