package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/workplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

var _ workplan.Rows = (*Plan)(nil)

func TestPointWorkProjectsPhasesWithoutPayloads(t *testing.T) {
	graph := cfg.New()
	point := graph.Exit()
	tests := []struct {
		name  string
		input factflow.FactsInput
		want  workplan.PointWork
	}{
		{name: "empty", input: factflow.FactsInput{}, want: 0},
		{
			name:  "node",
			input: factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{point: {}}},
			want:  workplan.Node,
		},
		{
			name:  "edge",
			input: factflow.FactsInput{BranchEdgeReachability: map[cfg.Point]factflow.BranchEdgeReachability{point: {}}},
			want:  workplan.Edge,
		},
		{
			name:  "composite node and edge stages",
			input: factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{point: {}}},
			want:  workplan.Node | workplan.Edge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(graph, tt.input).PointWork(point)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("work = %b, want %b", got, tt.want)
			}
		})
	}
}

func TestPointWorkRejectsPointsOutsidePlan(t *testing.T) {
	plan := New(cfg.New(), factflow.FactsInput{})
	if _, err := plan.PointWork(cfg.Point(plan.PointCount())); err == nil {
		t.Fatal("out-of-range point accepted")
	}
	var nilPlan *Plan
	if _, err := nilPlan.PointWork(0); err == nil {
		t.Fatal("nil plan accepted")
	}
}
