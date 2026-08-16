package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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
