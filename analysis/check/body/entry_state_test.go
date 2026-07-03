package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestCheckFunctionSeedsGradualTopForUnannotatedParameter(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(raw) return raw end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	seeds := functionParamEntrySeeds(reg, nil, bindings, fn, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	got := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), evidence.Key)
	if !evidence.Equal(got, evidence.GradualTop()) {
		t.Fatalf("entry evidence = %s, want %s", got, evidence.GradualTop())
	}
}

func TestCheckFunctionSeedsPresentGradualTopForUntypedImplicitSelf(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local M = {}
function M:run(arg)
    return self
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	fn := functions[0]
	slot := mustParamSlot(t, bindings, fn, 0)
	if !slot.ImplicitSelf {
		t.Fatalf("slot = %#v, want implicit self", slot)
	}
	seeds := functionParamEntrySeeds(reg, nil, bindings, fn, nil)
	if len(seeds) < 1 {
		t.Fatalf("entry seeds = %d, want implicit self seed", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	value := entry.ReadValue(reg, key.SymbolValue(slot.Symbol))
	if got := product.PresenceOf(value); !presence.Equal(got, presence.Present()) {
		t.Fatalf("implicit self presence = %s, want present", got)
	}
	got := product.Get(reg, value, evidence.Key)
	if !evidence.Equal(got, evidence.GradualTop()) {
		t.Fatalf("implicit self evidence = %s, want %s", got, evidence.GradualTop())
	}
}

func TestCheckChunkSeedsGradualTopForConfiguredGlobal(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, "local value = test")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"test"}})
	id, ok := bindings.GlobalSymbol("test")
	if !ok {
		t.Fatal("configured global symbol test not bound")
	}
	seeds := configuredGlobalEntrySeeds(reg, nil, bindings, []string{"test"}, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	got := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(id)), evidence.Key)
	if !evidence.Equal(got, evidence.GradualTop()) {
		t.Fatalf("entry evidence = %s, want %s", got, evidence.GradualTop())
	}
}

func TestCheckChunkSeedsTypeWitnessForConfiguredGlobalType(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	stmts := parseChunk(t, `
local elapsed: number = time.now():sub(time.now()):milliseconds()
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"time"}})
	id, ok := bindings.GlobalSymbol("time")
	if !ok {
		t.Fatal("configured global symbol time not bound")
	}
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "milliseconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeModule := typetable.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Build()
	seeds := configuredGlobalEntrySeeds(reg, typeValues, bindings, nil, map[string]typ.Type{"time": timeModule})
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	got := entry.ReadValue(reg, key.SymbolValue(id))
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, timeModule) {
		t.Fatalf("entry type = %s, want %s", gotType, timeModule)
	}
}

func TestCheckChunkDedupesConfiguredGlobalSeedsBySlot(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	stmts := parseChunk(t, "local value = test")
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"test"}})
	id, ok := bindings.GlobalSymbol("test")
	if !ok {
		t.Fatal("configured global symbol test not bound")
	}
	seeds := configuredGlobalEntrySeeds(
		reg,
		typeValues,
		bindings,
		[]string{"test", "test"},
		map[string]typ.Type{"test": typ.String},
	)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	if seeds[0].Slot != key.SymbolValue(id) {
		t.Fatalf("seed slot = %v, want %v", seeds[0].Slot, key.SymbolValue(id))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	value := entry.ReadValue(reg, key.SymbolValue(id))
	gotType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("entry type = %s, want string", gotType)
	}
}

func BenchmarkConfiguredGlobalEntrySeeds(b *testing.B) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	stmts := parseChunk(b, `
local a = alpha
local b = beta
local c = gamma(delta)
local d = epsilon
local e = zeta
local f = eta
local g = theta
`)
	globals := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	configured := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta", "alpha", "gamma"}
	globalTypes := map[string]typ.Type{
		"delta": typ.String,
		"theta": typ.Number,
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		seeds := configuredGlobalEntrySeeds(reg, typeValues, bindings, configured, globalTypes)
		if len(seeds) != len(globals) {
			b.Fatalf("entry seeds = %d, want %d", len(seeds), len(globals))
		}
	}
}

func TestCheckFunctionSeedsExplicitTopForExplicitAnyParameter(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, "function f(raw: any) return raw end")
	bindings := bind.BindFunction(fn, bind.Options{})
	slot := mustParamSlot(t, bindings, fn, 0)
	seeds := functionParamEntrySeeds(reg, nil, bindings, fn, nil)
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
	seeds := functionParamEntrySeeds(reg, nil, bindings, fn, nil)
	if len(seeds) != 1 {
		t.Fatalf("entry seeds = %d, want 1", len(seeds))
	}
	entry := seedEntryStateValues(reg, state.State{}, seeds)
	witness := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), typewitness.Key)
	if _, ok := witness.Type(); !ok {
		t.Fatalf("entry type witness = %v, want concrete witness", witness)
	}
}

func TestCheckFunctionSeedsContextualReturnedFunctionThroughLoopBlocks(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "while",
			body: `
	while true do
		return function(value)
			return value
		end
	end`,
		},
		{
			name: "repeat",
			body: `
	repeat
		return function(value)
			return value
		end
	until true`,
		},
		{
			name: "numeric for",
			body: `
	for i = 1, 1 do
		return function(value)
			return value
		end
	end`,
		},
		{
			name: "generic for",
			body: `
	for _, item in ipairs({1}) do
		return function(value)
			return value
		end
	end`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			stmts := parseChunk(t, `
function make(): (string) -> string
`+tc.body+`
end`)
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ipairs"}})
			outers := bindings.NestedFunctions(nil)
			if len(outers) != 1 {
				t.Fatalf("outer functions = %d, want 1", len(outers))
			}
			inners := bindings.NestedFunctions(outers[0])
			if len(inners) != 1 {
				t.Fatalf("inner functions = %d, want 1", len(inners))
			}
			slot := mustParamSlot(t, bindings, inners[0], 0)
			seeds := functionParamEntrySeeds(reg, nil, bindings, inners[0], nil)
			entry := seedEntryStateValues(reg, state.State{}, seeds)
			witness := product.Get(reg, entry.ReadValue(reg, key.SymbolValue(slot.Symbol)), typewitness.Key)
			got, ok := witness.Type()
			if !ok || !typ.TypeEquals(got, typ.String) {
				t.Fatalf("contextual parameter type = %v/%v, want string", got, ok)
			}
		})
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
