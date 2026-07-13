package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestPlanOwnsDeclaredBoundaryReturns(t *testing.T) {
	values := []product.Value{product.Top()}
	plan := New(cfg.New(), factflow.FactsInput{}).WithBoundaryReturns(values)
	values = append(values, product.Top())
	got := plan.BoundaryReturns()
	if len(got) != 1 {
		t.Fatalf("boundary returns = %v, want one", got)
	}
	got = append(got, product.Top())
	if len(plan.BoundaryReturns()) != 1 {
		t.Fatal("boundary return getter exposed storage")
	}
}

func TestPlanOwnsWidthMatchedBoundaryParamContracts(t *testing.T) {
	reg := standard.Registry()
	first := typevalue.LiteralString(reg, "first")
	values := []product.Value{first, product.Top()}
	plan := New(cfg.New(), factflow.FactsInput{}).WithBoundaryParams([]symbol.ID{7, 9}).WithBoundaryParamContracts(values)
	values[0] = product.Top()
	got := plan.BoundaryParamContracts()
	if len(got) != 2 || !product.Equal(reg, got[0], first) {
		t.Fatalf("boundary param contracts = %#v", got)
	}
	got[0] = product.Top()
	if !product.Equal(reg, plan.BoundaryParamContracts()[0], first) {
		t.Fatal("boundary param contract getter exposed storage")
	}
	if mismatch := plan.WithBoundaryParamContracts([]product.Value{product.Top()}).BoundaryParamContracts(); len(mismatch) != 0 {
		t.Fatalf("width-mismatched contracts published: %#v", mismatch)
	}
}

func TestAmbiguousBoundaryParametersFailClosed(t *testing.T) {
	plan := New(cfg.New(), factflow.FactsInput{}).WithBoundaryParams([]symbol.ID{7, 7})
	if got := plan.BoundaryParams(); plan.BoundaryParamsValid() || len(got) != 0 {
		t.Fatalf("ambiguous boundary published: %v", got)
	}
}

func TestPlanOwnsOrderedBoundaryCaptureSymbols(t *testing.T) {
	captures := []symbol.ID{11, 13}
	plan := New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{7}).
		WithBoundaryCaptures(captures)
	captures[0] = 99
	if !plan.BoundaryCapturesValid() {
		t.Fatal("valid capture boundary rejected")
	}
	if got := plan.BoundaryCaptures(); len(got) != 2 || got[0] != 11 || got[1] != 13 {
		t.Fatalf("boundary captures = %v", got)
	}
	if index, ok := plan.BoundaryCaptureIndex(13); !ok || index != 1 {
		t.Fatalf("capture index = %d/%v", index, ok)
	}
	returned := plan.BoundaryCaptures()
	returned[0] = 88
	if got := plan.BoundaryCaptures()[0]; got != 11 {
		t.Fatalf("getter exposed storage: %d", got)
	}
}

func TestAmbiguousBoundaryCapturesFailClosed(t *testing.T) {
	for _, captures := range [][]symbol.ID{{0}, {11, 11}, {7}} {
		plan := New(cfg.New(), factflow.FactsInput{}).
			WithBoundaryParams([]symbol.ID{7}).
			WithBoundaryCaptures(captures)
		if plan.BoundaryCapturesValid() || len(plan.BoundaryCaptures()) != 0 {
			t.Fatalf("ambiguous capture boundary %v published as %v", captures, plan.BoundaryCaptures())
		}
	}
}

func TestPlanOwnsOrderedBoundaryGlobalSymbols(t *testing.T) {
	globals := []symbol.ID{19, 17}
	plan := New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{7}).
		WithBoundaryCaptures([]symbol.ID{11}).
		WithBoundaryGlobals(globals)
	globals[0] = 99
	if !plan.BoundaryGlobalsValid() {
		t.Fatal("valid global boundary rejected")
	}
	if got := plan.BoundaryGlobals(); len(got) != 2 || got[0] != 17 || got[1] != 19 {
		t.Fatalf("boundary globals = %v", got)
	}
	returned := plan.BoundaryGlobals()
	returned[0] = 88
	if got := plan.BoundaryGlobals()[0]; got != 17 {
		t.Fatalf("getter exposed storage: %d", got)
	}
}

func TestAmbiguousBoundaryGlobalsFailClosed(t *testing.T) {
	for _, globals := range [][]symbol.ID{{0}, {17, 17}, {7}, {11}} {
		plan := New(cfg.New(), factflow.FactsInput{}).
			WithBoundaryParams([]symbol.ID{7}).
			WithBoundaryCaptures([]symbol.ID{11}).
			WithBoundaryGlobals(globals)
		if plan.BoundaryGlobalsValid() || len(plan.BoundaryGlobals()) != 0 {
			t.Fatalf("ambiguous global boundary %v published as %v", globals, plan.BoundaryGlobals())
		}
	}
	plan := New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{7}).
		WithBoundaryCaptures([]symbol.ID{7}).
		WithBoundaryGlobals(nil)
	if plan.BoundaryGlobalsValid() {
		t.Fatal("global boundary accepted after malformed capture namespace")
	}
	plan = New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{7, 7}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	if plan.BoundaryCapturesValid() || plan.BoundaryGlobalsValid() {
		t.Fatal("downstream boundaries accepted after malformed parameter namespace")
	}
}
