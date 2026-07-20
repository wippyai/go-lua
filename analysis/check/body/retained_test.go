package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func assertStructuralCallbackCallsOnce(t *testing.T, prepared *Static, label string, calls map[cfg.Point]int) {
	t.Helper()
	for _, point := range prepared.cfg.Graph.RPO() {
		if calls[point] != 1 {
			t.Fatalf("%s callback calls at point %d = %d, want 1", label, point, calls[point])
		}
	}
}

func retainedCallPoint(t *testing.T, prepared *Static) cfg.Point {
	t.Helper()
	var points []cfg.Point
	for _, point := range prepared.cfg.Graph.RPO() {
		if _, ok := prepared.facts.CallSiteView(point); ok {
			points = append(points, point)
		}
	}
	if len(points) != 1 {
		t.Fatalf("call points = %v, want one", points)
	}
	return points[0]
}

func assertRetainedResultEqual(t *testing.T, got, want *Result) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("nil result: got=%v want=%v", got == nil, want == nil)
	}
	if got.ResultVersion() != want.ResultVersion() {
		t.Fatalf("ResultVersion=%d, want %d", got.ResultVersion(), want.ResultVersion())
	}
	domain := state.Domain(got.registry)
	for _, point := range got.cfg.Graph.RPO() {
		if !domain.Equal(got.flow[point], want.flow[point]) {
			t.Fatalf("flow differs at point %d", point)
		}
		if got.published.pointReachable[point] != want.published.pointReachable[point] {
			t.Fatalf("reachability differs at point %d", point)
		}
		gotNode, gotOK := got.published.nodeOutputs[point]
		wantNode, wantOK := want.published.nodeOutputs[point]
		if gotOK != wantOK || (gotOK && !domain.Equal(gotNode, wantNode)) {
			t.Fatalf("published node differs at point %d", point)
		}
	}
}
