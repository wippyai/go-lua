package operationplan

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"testing"
)

func TestPlanOwnsOrderedBoundaryParameterSymbols(t *testing.T) {
	params := []symbol.ID{7, 9}
	plan := New(cfg.New(), factflow.FactsInput{}).WithBoundaryParams(params)
	params[0] = 99
	if got := plan.BoundaryParams(); len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Fatalf("boundary params = %v", got)
	}
	if index, ok := plan.BoundaryParamIndex(9); !ok || index != 1 {
		t.Fatalf("param index = %d/%v", index, ok)
	}
	returned := plan.BoundaryParams()
	returned[0] = 88
	if got := plan.BoundaryParams()[0]; got != 7 {
		t.Fatalf("getter exposed storage: %d", got)
	}
}

func TestAmbiguousBoundaryParametersFailClosed(t *testing.T) {
	plan := New(cfg.New(), factflow.FactsInput{}).WithBoundaryParams([]symbol.ID{7, 7})
	if got := plan.BoundaryParams(); len(got) != 0 {
		t.Fatalf("ambiguous boundary published: %v", got)
	}
}
