package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEntryContextKeyMatchesSummaryEntryContextKey(t *testing.T) {
	ref := summary.FuncRef{GraphID: 10, ParentHash: 20}
	cells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)},
	})
	refs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(8), "fn").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 30}))
	closures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(9), "closure").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 40}, cells, refs),
	))
	values := summary.EntryValues{
		0: product.FromType(typ.Number),
		2: product.FromType(typ.Boolean),
	}

	ctx := NewEntryContext(ref, cells, refs, closures, values, flow.BoundaryFactsDomain.Top())
	want := summary.NewKeyWithEntryContext(ref, cells, refs, closures, values)
	if got := ctx.Key(); got != want {
		t.Fatalf("Key() = %#v, want %#v", got, want)
	}
}

func TestEntryContextFromClosureWithLiveContextPreservesClosureAxes(t *testing.T) {
	ref := summary.FuncRef{GraphID: 100, ParentHash: 200}
	cells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(11), Value: product.FromType(typ.String)},
	})
	refs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(12), "captured").Key(), flow.FunctionRefSetOf(
		flow.FunctionRef{GraphID: 120, ParentHash: 1},
	))
	nestedCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(13), Value: product.FromType(typ.Number)},
	})
	nested := flow.ClosureRefOf(flow.FunctionRef{GraphID: 130}, nestedCells, nil)
	closures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(14), "factory").Field("inner").Key(), flow.ClosureRefSetOf(nested))
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: ref.GraphID, ParentHash: ref.ParentHash}, cells, refs, closures)

	live := NewEntryContext(ref, flow.CaptureCellsDomain.Bottom(), nil, nil, summary.EntryValues{1: product.FromType(typ.Boolean)}, flow.BoundaryFactsDomain.Top())
	ctx := EntryContextFromClosureWithLiveContext(closure, live)

	if ctx.Ref() != ref {
		t.Fatalf("Ref() = %#v, want %#v", ctx.Ref(), ref)
	}
	if got := ctx.CaptureCells(); !flow.CaptureCellsDomain.Equal(got, cells) {
		t.Fatalf("CaptureCells() = %s, want %s", got.Format(), cells.Format())
	}
	if got := ctx.FunctionRefs(); !flow.FunctionRefsDomain.Equal(got, refs) {
		t.Fatalf("FunctionRefs() = %#v, want %#v", got, refs)
	}
	if got := ctx.ClosureRefs(); !flow.ClosureRefsDomain.Equal(got, closures) {
		t.Fatalf("ClosureRefs() = %#v, want %#v", got, closures)
	}
	wantKey := summary.NewKeyWithEntryContext(ref, closure.EntryCells(), closure.EntryFunctionRefs(), closure.EntryClosureRefs(), ctx.EntryValues())
	if got := ctx.Key(); got != wantKey {
		t.Fatalf("Key() = %#v, want %#v", got, wantKey)
	}
}

func TestEntryContextFromClosureWithLiveContextOverridesSnapshot(t *testing.T) {
	ref := summary.FuncRef{GraphID: 11, ParentHash: 22}
	sym := cfg.SymbolID(7)
	path := constraint.NewPath(sym, "M").Field("dep").Field("get")

	oldCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.Nil)},
	})
	oldRefs := flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 30}))
	oldClosures := flow.WithClosureRef(nil, path.Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 40}, oldCells, oldRefs),
	))
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: ref.GraphID, ParentHash: ref.ParentHash}, oldCells, oldRefs, oldClosures)

	liveCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.String)},
	})
	liveRefs := flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 31}))
	liveClosures := flow.WithClosureRef(nil, path.Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 41}, liveCells, liveRefs),
	))

	live := NewEntryContext(ref, liveCells, liveRefs, liveClosures, nil, flow.BoundaryFactsDomain.Top())
	ctx := EntryContextFromClosureWithLiveContext(closure, live)
	if av, ok := ctx.CaptureCells().Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("CaptureCells()[%d] = %v/%v, want string", sym, av.ProjectValue(), ok)
	}
	if set, ok := flow.FunctionRefAt(ctx.FunctionRefs(), path.Key()); !ok {
		t.Fatal("FunctionRefs missing live path")
	} else if got, ok := set.Singleton(); !ok || got.GraphID != 31 {
		t.Fatalf("FunctionRefs live path = %s, want graph 31", set.Format())
	}
	if set, ok := flow.ClosureRefAt(ctx.ClosureRefs(), path.Key()); !ok {
		t.Fatal("ClosureRefs missing live path")
	} else if got, ok := set.Singleton(); !ok || got.Ref.GraphID != 41 {
		t.Fatalf("ClosureRefs live path = %s, want graph 41", set.Format())
	}
}

func TestEntryContextEntryValuesNoAliasAndDeterministic(t *testing.T) {
	ref := summary.FuncRef{GraphID: 200}
	stringValue := product.FromType(typ.String)
	numberValue := product.FromType(typ.Number)
	booleanValue := product.FromType(typ.Boolean)
	values := summary.EntryValues{
		2: numberValue,
		1: stringValue,
	}
	sameValues := make(summary.EntryValues)
	sameValues[1] = stringValue
	sameValues[2] = numberValue

	ctx := NewEntryContext(ref, flow.CaptureCellsDomain.Bottom(), nil, nil, values, flow.BoundaryFactsDomain.Top())
	keyBefore := ctx.Key()

	values[1] = booleanValue
	values[3] = stringValue
	got := ctx.EntryValues()
	got[2] = booleanValue
	got[4] = numberValue

	again := ctx.EntryValues()
	if !entryValueEqual(again[1], stringValue) {
		t.Fatalf("EntryValues()[1] = %s, want original string value", again[1].ProjectValue())
	}
	if !entryValueEqual(again[2], numberValue) {
		t.Fatalf("EntryValues()[2] = %s, want original number value", again[2].ProjectValue())
	}
	if _, ok := again[3]; ok {
		t.Fatalf("EntryValues() observed caller mutation at slot 3")
	}
	if _, ok := again[4]; ok {
		t.Fatalf("EntryValues() observed accessor mutation at slot 4")
	}
	if got := ctx.Key(); got != keyBefore {
		t.Fatalf("Key() changed after external mutations: before %#v after %#v", keyBefore, got)
	}

	sameCtx := NewEntryContext(ref, flow.CaptureCellsDomain.Bottom(), nil, nil, sameValues, flow.BoundaryFactsDomain.Top())
	if got, want := sameCtx.Key(), keyBefore; got != want {
		t.Fatalf("equivalent entry values produced different keys: got %#v want %#v", got, want)
	}
}

func entryValueEqual(a, b product.AbstractValue) bool {
	return product.Domain.Equal(a, b)
}
