package body

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

func TestExecutionFactoryReportsItsApplicationSessionCancellation(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(value) return value end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx, session := cancellation.Attach(ctx)
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	if err := factory.Err(); err != nil {
		t.Fatalf("live factory error = %v", err)
	}
	cancel()
	if err := factory.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled factory error = %v, want context.Canceled", err)
	}
}

func TestExecutionFactoryOwnsReplacementCallContext(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(value) return value end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	callCtx := factory.CallOutcomeContext()
	if callCtx.LexicalBodyID != prepared.StableLexicalBodyID() || callCtx.Sources == nil || callCtx.KeySpace != factory.KeySpace() || callCtx.TypeValues != factory.typeValues {
		t.Fatal("replacement call context does not retain its exact prepared/session ownership")
	}
	if callCtx.CalleeValue == nil || callCtx.ReceiverCallable == nil || callCtx.ReturnPresenceRelationsPath == nil || callCtx.PathValue == nil || callCtx.DynamicRead == nil || callCtx.DynamicTableRead == nil {
		t.Fatal("replacement call context omitted a canonical body call adapter")
	}
}

func TestExecutionFactoryEntrySeedPlanOwnsParamsConfiguredAndModuleGlobals(t *testing.T) {
	reg := standard.Registry()
	ambient := manifest.New("ambient")
	ambient.SetExport(typ.String)
	prepared, err := PrepareFunction(parseFunction(t, `
function f(value: string)
	return value, configured, ambient
end
`), Config{
		Registry: reg,
		Globals:  []string{"configured", "ambient"},
		GlobalTypes: map[string]typ.Type{
			"configured": typ.Number,
		},
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{ambient}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}

	plan := factory.EntrySeedPlan()
	if plan.Empty() || plan.Len() < 3 {
		t.Fatalf("entry seed plan = %d/%t, want param plus two globals", plan.Len(), plan.Empty())
	}
	paramSlots := prepared.bindings.ParamSlots(prepared.function)
	if len(paramSlots) != 1 {
		t.Fatalf("param slots = %d, want 1", len(paramSlots))
	}
	configured, configuredOK := prepared.bindings.GlobalSymbol("configured")
	ambientSymbol, ambientOK := prepared.bindings.GlobalSymbol("ambient")
	if !configuredOK || !ambientOK {
		t.Fatal("prepared globals are absent from binding ownership")
	}
	for name, slot := range map[string]key.Value{
		"param":      key.SymbolValue(paramSlots[0].Symbol),
		"configured": key.SymbolValue(configured),
		"ambient":    key.SymbolValue(ambientSymbol),
	} {
		seeded, err := plan.Apply(reg, state.State{})
		if err != nil {
			t.Fatal(err)
		}
		if got := seeded.ReadValue(reg, slot); product.Domain(reg).Equal(got, product.Bottom(reg)) {
			t.Fatalf("%s entry seed remained Bottom", name)
		}
	}

	actual := product.Top()
	paramSlot := key.SymbolValue(paramSlots[0].Symbol)
	seeded, err := plan.Apply(reg, state.State{}.WriteValue(reg, paramSlot, actual))
	if err != nil {
		t.Fatal(err)
	}
	if got := seeded.ReadValue(reg, paramSlot); !product.Equal(reg, got, actual) {
		t.Fatal("entry seed plan replaced route-supplied parameter")
	}
	// Re-fetching the authority must not share mutable plan storage with callers.
	if other := factory.EntrySeedPlan(); other.Len() != plan.Len() {
		t.Fatalf("re-fetched entry seed plan size = %d, want %d", other.Len(), plan.Len())
	}
}

func TestExecutionFactoryEntrySeedPlanDistinguishesPreparedEmptyFromMissing(t *testing.T) {
	fn := parseFunction(t, `function f() return 1 end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	plan := factory.EntrySeedPlan()
	if !plan.Valid() || !plan.Empty() || plan.Len() != 0 {
		t.Fatalf("prepared empty plan = valid:%t empty:%t len:%d", plan.Valid(), plan.Empty(), plan.Len())
	}
	if ((*ExecutionFactory)(nil)).EntrySeedPlan().Valid() {
		t.Fatal("nil execution factory minted entry-seed authority")
	}
}

func TestExecutionFactoryFreezesInitialStateCallbackOnce(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareFunction(parseFunction(t, `
function f(value)
	local copy = value
	return copy
end
`), Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	points := cfg.RPOReadOnly(factory.Graph())
	if len(points) < 3 {
		t.Fatalf("prepared graph has %d points, want entry plus non-entry coordinates", len(points))
	}
	nonEntry := points[1]
	if nonEntry == factory.Graph().Entry() {
		nonEntry = points[2]
	}
	entryState := state.State{}.WriteValue(reg, key.Value(901), product.Top())
	nonEntryState := state.State{}.WriteValue(reg, key.Value(902), product.Top())
	calls := make(map[cfg.Point]int, len(points))
	plan, err := factory.FreezeInitialStatePlan(func(point cfg.Point) (state.State, bool) {
		calls[point]++
		switch point {
		case factory.Graph().Entry():
			return entryState, true
		case nonEntry:
			return nonEntryState, true
		default:
			return state.State{}, false
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ValidFor(prepared.StableLexicalBodyID(), factory.Graph().ID(), factory.Graph().Size()) || plan.Len() != 2 {
		t.Fatalf("initial plan = valid:%t len:%d", plan.ValidFor(prepared.StableLexicalBodyID(), factory.Graph().ID(), factory.Graph().Size()), plan.Len())
	}
	for _, point := range points {
		if calls[point] != 1 {
			t.Fatalf("initial callback calls at %d = %d, want exactly 1", point, calls[point])
		}
	}
	if got, ok := plan.At(state.InitialCoordinate(nonEntry)); !ok || !product.Equal(reg, got.ReadValue(reg, key.Value(902)), product.Top()) {
		t.Fatal("non-entry initial coordinate was not frozen")
	}
	empty, err := factory.FreezeInitialStatePlan(nil)
	if err != nil || !empty.ValidFor(prepared.StableLexicalBodyID(), factory.Graph().ID(), factory.Graph().Size()) || !empty.Empty() {
		t.Fatalf("nil initial plan = %#v/%v", empty, err)
	}
}

func TestExecutionFactoryInitialStateFreezeCancellationPublishesNothing(t *testing.T) {
	prepared, err := PrepareFunction(parseFunction(t, `function f(value) return value end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx, session := cancellation.Attach(ctx)
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	plan, err := factory.FreezeInitialStatePlan(func(cfg.Point) (state.State, bool) {
		calls++
		cancel()
		return state.State{}, true
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("freeze error = %v, want context.Canceled", err)
	}
	if plan.Valid() || calls != 1 {
		t.Fatalf("canceled freeze published plan:%t after %d calls", plan.Valid(), calls)
	}
}
