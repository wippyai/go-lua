package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreparedStaticOwnsEntrySeedPlan(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, "function f(x: string) return x end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	if !prepared.entrySeedsPrepared {
		t.Fatal("prepared body did not record entry seed ownership")
	}
	if prepared.operationPlan == nil {
		t.Fatal("prepared body did not retain immutable operation-plan metadata")
	}
	if got, want := prepared.operationPlan.PointCount(), prepared.cfg.Graph.Size(); got != want {
		t.Fatalf("operation-plan rows=%d want %d", got, want)
	}
	if len(prepared.entrySeeds) != 1 {
		t.Fatalf("entry seed count = %d, want 1", len(prepared.entrySeeds))
	}

	entry, initial := prepared.solveEntryState(prepared.typeValues, state.State{}, nil)
	if initial != nil {
		t.Fatal("solveEntryState installed an initial wrapper without a caller initial")
	}
	value := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("entry seed type = %v/%v, want string", got, ok)
	}
}

func requireOnlyCallSitePoint(t *testing.T, result *Result) cfg.Point {
	t.Helper()
	var out cfg.Point
	for _, candidate := range result.Graph().RPO() {
		if _, ok := result.CallSiteView(candidate); !ok {
			continue
		}
		if out != 0 {
			t.Fatalf("multiple call sites: %d and %d", out, candidate)
		}
		out = candidate
	}
	if out == 0 {
		t.Fatal("call site not found")
	}
	return out
}

func markedValue(reg *axis.Registry, markKey axis.Key[markValue], mark markValue) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, mark)
}

func staticCallOutcome(value product.Value) callpayload.CallOutcomeProgram {
	evaluate := func(_ transfer.NodeContext, _ factflow.CallSiteView, _ callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{
			Index: 0,
			Value: value,
		}}}, nil
	}
	return callpayload.SealCallOutcomeProgram(
		"static test outcome", []string{"Results"}, state.LaneSet{}, state.LaneSet{}, nil, nil, evaluate,
	)
}
