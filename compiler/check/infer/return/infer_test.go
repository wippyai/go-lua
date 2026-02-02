package infer

import "testing"

func TestComputeReturnSummariesForGraph_Empty(t *testing.T) {
	inferencer := New(Config{})
	summaries, diags := inferencer.ComputeForGraph(RunContext{}, nil, nil)
	if summaries != nil {
		t.Error("nil graph should return nil summaries")
	}
	if len(diags) != 0 {
		t.Error("nil graph should return no diagnostics")
	}
}
