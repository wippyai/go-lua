package body

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestPreparedPureLogicalRHSRegionsArePublished(t *testing.T) {
	tests := []struct{ name, source string }{
		{name: "literal return", source: `function f(v) return v and "x" end`},
		{name: "identifier return", source: `function f(v, fallback) return v or fallback end`},
		{name: "direct assignment producer", source: `function f(v) local x = v and "x" return x end`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := parseFunction(t, test.source)
			prepared, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
			if err != nil {
				t.Fatalf("PrepareFunction: %v", err)
			}
			count := 0
			prepared.operationPlan.ForEachStructuralExpressionRegion(func(_ factflow.ExprRef, region factflow.StructuralExpressionRegion) bool {
				count++
				assertRegionEdges(t, prepared.cfg.Graph, region)
				return true
			})
			if count != 1 {
				t.Fatalf("structural regions = %d, want 1", count)
			}
		})
	}
}

func TestPreparedStructuralExpressionRegionIdentityIsRepeatable(t *testing.T) {
	fn := parseFunction(t, `function f(v, fallback) return v and "x" or fallback end`)
	first, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("first PrepareFunction: %v", err)
	}
	second, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("second PrepareFunction: %v", err)
	}
	collect := func(prepared *Static) []any {
		var out []any
		prepared.operationPlan.ForEachStructuralExpressionRegion(func(ref factflow.ExprRef, region factflow.StructuralExpressionRegion) bool {
			out = append(out, struct {
				Ref                       factflow.ExprRef
				Branch, True, False, Join cfg.Point
				RHSOnTrue                 bool
				Owned                     []cfg.Point
			}{ref, region.Branch(), region.TrueTarget(), region.FalseTarget(), region.Join(), region.RHSOnTrue(), region.OwnedRHSPoints()})
			return true
		})
		return out
	}
	if got, want := collect(first), collect(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated structural identity differs:\nfirst=%v\nsecond=%v", got, want)
	}
}

func TestPreparedParsedIsStrPublishesExactReturnedExpressionRegion(t *testing.T) {
	fn := parseFunction(t, `function is_str(value: any): boolean
		return type(value) == "string" and value ~= ""
	end`)
	prepared, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	owner := returnedExpressionRef(t, prepared)
	region, ok := prepared.operationPlan.StructuralExpressionRegion(owner)
	if !ok {
		t.Fatalf("returned expression %d has no exact structural region", owner)
	}
	graph := prepared.cfg.Graph
	if !graph.IsBranch(region.Branch()) || !graph.IsJoin(region.Join()) {
		t.Fatalf("region branch/join = %d/%d are not exact CFG topology", region.Branch(), region.Join())
	}
	if !region.RHSOnTrue() {
		t.Fatal("logical and RHS is not owned by the true edge")
	}
	assertRegionEdges(t, graph, region)
	if got := len(region.OwnedRHSPoints()); got != 1 {
		t.Fatalf("is_str RHS region points = %v, want one evaluation point", region.OwnedRHSPoints())
	}
}

func TestPreparedNestedEffectfulLogicalRegionsOwnEveryConditionalCall(t *testing.T) {
	fn := parseFunction(t, `function f(flag)
		return flag and (left() or right())
	end`)
	prepared, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	var regions []factflow.StructuralExpressionRegion
	prepared.operationPlan.ForEachStructuralExpressionRegion(func(_ factflow.ExprRef, region factflow.StructuralExpressionRegion) bool {
		regions = append(regions, region)
		return true
	})
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want outer and nested logical", len(regions))
	}
	callPoints := preparedCallPoints(prepared)
	if len(callPoints) != 2 {
		t.Fatalf("call points = %v, want left/right", callPoints)
	}
	outer := regions[0]
	if len(regions[1].OwnedRHSPoints()) > len(outer.OwnedRHSPoints()) {
		outer = regions[1]
	}
	for _, call := range callPoints {
		if !containsPoint(outer.OwnedRHSPoints(), call) {
			t.Fatalf("outer RHS ownership %v omits conditional call %d", outer.OwnedRHSPoints(), call)
		}
	}
}

func TestPreparedLogicalRegionDoesNotCaptureUnrelatedFork(t *testing.T) {
	fn := parseFunction(t, `function f(flag)
		if flag then unrelated() end
		return flag and owned()
	end`)
	prepared, err := PrepareFunction(fn, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	owner := returnedExpressionRef(t, prepared)
	region, ok := prepared.operationPlan.StructuralExpressionRegion(owner)
	if !ok {
		t.Fatal("missing returned logical region")
	}
	calls := preparedCallPoints(prepared)
	if len(calls) != 2 {
		t.Fatalf("call points = %v, want unrelated/owned", calls)
	}
	owned := region.OwnedRHSPoints()
	if containsPoint(owned, calls[0]) || !containsPoint(owned, calls[1]) {
		t.Fatalf("RHS ownership %v captured unrelated call %d or omitted owned call %d", owned, calls[0], calls[1])
	}
	for _, point := range prepared.cfg.Graph.RPO() {
		if point != region.Branch() && prepared.cfg.Graph.IsBranch(point) && containsPoint(owned, point) {
			t.Fatalf("region captured unrelated fork %d", point)
		}
	}
}

func returnedExpressionRef(t *testing.T, prepared *Static) factflow.ExprRef {
	t.Helper()
	for _, point := range prepared.cfg.Graph.RPO() {
		ret, ok := prepared.facts.Return(point)
		if !ok {
			continue
		}
		sources := ret.Sources()
		if len(sources) == 1 && sources[0].HasExpr && sources[0].ExprRef != 0 {
			return sources[0].ExprRef
		}
	}
	t.Fatal("no returned expression ref")
	return 0
}

func preparedCallPoints(prepared *Static) []cfg.Point {
	var out []cfg.Point
	for _, point := range prepared.cfg.Graph.RPO() {
		if _, ok := prepared.facts.CallSiteView(point); ok {
			out = append(out, point)
		}
	}
	return out
}

func assertRegionEdges(t *testing.T, graph cfg.Graph, region factflow.StructuralExpressionRegion) {
	t.Helper()
	if cond, ok := graph.EdgeCond(region.Branch(), region.TrueTarget()); !ok || !cond {
		t.Fatalf("missing true edge %d -> %d", region.Branch(), region.TrueTarget())
	}
	if cond, ok := graph.EdgeCond(region.Branch(), region.FalseTarget()); !ok || cond {
		t.Fatalf("missing false edge %d -> %d", region.Branch(), region.FalseTarget())
	}
	rhsTarget := region.FalseTarget()
	if region.RHSOnTrue() {
		rhsTarget = region.TrueTarget()
	}
	if !containsPoint(region.OwnedRHSPoints(), rhsTarget) {
		t.Fatalf("owned points %v omit RHS edge target %d", region.OwnedRHSPoints(), rhsTarget)
	}
}

func containsPoint(points []cfg.Point, want cfg.Point) bool {
	for _, point := range points {
		if point == want {
			return true
		}
	}
	return false
}
