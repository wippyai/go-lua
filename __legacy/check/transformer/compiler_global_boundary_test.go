package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPlanCompilerBindsParamCaptureGlobalNamespacesDistinctly(t *testing.T) {
	const param, capture, global = symbol.ID(7), symbol.ID(11), symbol.ID(17)
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{param}).
		WithBoundaryCaptures([]symbol.ID{capture}).
		WithBoundaryGlobals([]operationplan.BoundaryGlobal{{Symbol: global}})
	prepared, err := NewPlanCompiler().Prepare(standard.Registry(), cfg.New(), plan, Shape{Params: 1, Captures: 1, Globals: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[symbol.ID]Root{
		param: {Kind: RootParam, Index: 0}, capture: {Kind: RootCapture, Index: 0}, global: {Kind: RootGlobal, Index: 0},
	}
	seen := make(map[ValueTerm]symbol.ID, len(want))
	for id, root := range want {
		term := prepared.base.locals[id]
		if term == 0 || term != prepared.builder.Arena().Root(root) {
			t.Fatalf("symbol %d term = %d, want root %+v", id, term, root)
		}
		if other, duplicate := seen[term]; duplicate {
			t.Fatalf("symbols %d and %d alias term %d", other, id, term)
		}
		seen[term] = id
	}
}

func TestPlanCompilerRejectsMalformedOrWidthMismatchedGlobalBoundary(t *testing.T) {
	tests := []struct {
		name  string
		plan  *operationplan.Plan
		shape Shape
		want  string
	}{
		{
			name: "parameter overlap",
			plan: operationplan.New(cfg.New(), factflow.FactsInput{}).
				WithBoundaryParams([]symbol.ID{7}).WithBoundaryCaptures(nil).WithBoundaryGlobals([]operationplan.BoundaryGlobal{{Symbol: 7}}),
			shape: Shape{Params: 1}, want: "global boundary is malformed",
		},
		{
			name: "capture overlap",
			plan: operationplan.New(cfg.New(), factflow.FactsInput{}).
				WithBoundaryParams(nil).WithBoundaryCaptures([]symbol.ID{11}).WithBoundaryGlobals([]operationplan.BoundaryGlobal{{Symbol: 11}}),
			shape: Shape{Captures: 1}, want: "global boundary is malformed",
		},
		{
			name: "shape width",
			plan: operationplan.New(cfg.New(), factflow.FactsInput{}).
				WithBoundaryParams(nil).WithBoundaryCaptures(nil).WithBoundaryGlobals([]operationplan.BoundaryGlobal{{Symbol: 17}}),
			shape: Shape{}, want: "global symbols 1 != shape globals 0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPlanCompiler().Prepare(standard.Registry(), cfg.New(), test.plan, test.shape)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Prepare error = %v, want %q", err, test.want)
			}
		})
	}
}
