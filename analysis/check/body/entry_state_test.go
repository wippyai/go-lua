package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestCheckFunctionSeedsGradualTopForUnannotatedParameter(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(raw) return raw end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	seeds := functionParamEntrySeeds(reg, bindings, fn, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	got := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), evidence.Key)
	if !evidence.Equal(got, evidence.GradualTop()) {
		t.Fatalf("entry evidence = %s, want %s", got, evidence.GradualTop())
	}
}

func TestCheckFunctionSeedsExplicitTopForExplicitAnyParameter(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(raw: any) return raw end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	seeds := functionParamEntrySeeds(reg, bindings, fn, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	got := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), evidence.Key)
	if !evidence.Equal(got, evidence.ExplicitTop()) {
		t.Fatalf("entry evidence = %s, want %s", got, evidence.ExplicitTop())
	}
}

func TestCheckFunctionSeedsTypeWitnessForDeclaredParameter(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
type User = { id: string, retries: number }
function f(user: User)
	return user
end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	fn := functions[0]
	slot := mustParamSlot(t, bindings, fn, 0)
	seeds := functionParamEntrySeeds(reg, bindings, fn, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	witness := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), typewitness.Key)
	if _, ok := witness.Type(); !ok {
		t.Fatalf("entry type witness = %v, want concrete witness", witness)
	}
}

func TestCheckFunctionAssertionsDoNotCreateGradualTopFromExplicitAny(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(raw: any)
	local y = raw as any
	local z = raw :: any
	local n = raw!
	return y, z, n
end`)

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}

	for _, tc := range []struct {
		name  string
		index int
	}{
		{name: "as any", index: 0},
		{name: ":: any", index: 1},
		{name: "non-nil", index: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := fn.Stmts[tc.index].(*ast.LocalAssignStmt)
			sym := mustLocalAt(t, result, local, 0)
			got := product.Get(reg, exit.ReadValue(reg, key.SymbolValue(sym)), evidence.Key)
			if !evidence.Equal(got, evidence.ExplicitTop()) {
				t.Fatalf("exit evidence = %s, want %s", got, evidence.ExplicitTop())
			}
		})
	}
}
