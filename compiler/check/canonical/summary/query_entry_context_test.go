package summary

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeEntryAxesWithFixedPreservesExplicitContext(t *testing.T) {
	sym := cfg.SymbolID(7)
	other := cfg.SymbolID(8)
	path := constraint.NewPath(sym, "M").Field("dep").Field("get")
	otherPath := constraint.NewPath(other, "N").Field("make")

	fixedCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.String)},
	})
	fallbackCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.Nil)},
		{Symbol: other, Value: product.FromType(typ.Number)},
	})
	gotCells := mergeCaptureCellsWithFixed(fixedCells, fallbackCells)
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("fixed cell overwritten: got %v/%v, want string", av.ProjectValue(), ok)
	}
	if av, ok := gotCells.Value(other); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("fallback missing cell: got %v/%v, want number", av.ProjectValue(), ok)
	}
	emptyRecord := typ.NewRecord().Build()
	recordWithMethod := typ.NewRecord().Field("render", typ.Func().Returns(typ.String).Build()).Build()
	gotCells = mergeCaptureCellsWithFixed(
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(emptyRecord)}}),
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(recordWithMethod)}}),
	)
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), recordWithMethod) {
		t.Fatalf("narrower fallback cell = %v/%v, want record with render", av.ProjectValue(), ok)
	}
	gotCells = mergeCaptureCellsWithFixed(
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Any)}}),
	)
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("broad fallback cell = %v/%v, want fixed number", av.ProjectValue(), ok)
	}

	fixedRefs := flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 11}))
	fallbackRefs := flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 12}))
	fallbackRefs = flow.WithFunctionRefPath(fallbackRefs, otherPath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 13}))
	gotRefs := mergeFunctionRefsWithFixed(fixedRefs, fallbackRefs)
	if set, ok := flow.FunctionRefAtPath(gotRefs, path); !ok {
		t.Fatal("fixed function refs missing")
	} else if got, ok := set.Singleton(); !ok || got.GraphID != 11 {
		t.Fatalf("fixed function refs overwritten: got %s, want graph 11", set.Format())
	}
	if set, ok := flow.FunctionRefAtPath(gotRefs, otherPath); !ok {
		t.Fatal("fallback function refs missing")
	} else if got, ok := set.Singleton(); !ok || got.GraphID != 13 {
		t.Fatalf("fallback function refs = %s, want graph 13", set.Format())
	}

	fixedClosures := flow.WithClosureRefPath(nil, path, flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 21}, fixedCells, fixedRefs),
	))
	fallbackClosures := flow.WithClosureRefPath(nil, path, flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 22}, fallbackCells, fallbackRefs),
	))
	fallbackClosures = flow.WithClosureRefPath(fallbackClosures, otherPath, flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 23}, fallbackCells, fallbackRefs),
	))
	gotClosures := mergeClosureRefsWithFixed(fixedClosures, fallbackClosures)
	if set, ok := flow.ClosureRefAtPath(gotClosures, path); !ok {
		t.Fatal("fixed closure refs missing")
	} else if got, ok := set.Singleton(); !ok || got.Ref.GraphID != 21 {
		t.Fatalf("fixed closure refs overwritten: got %s, want graph 21", set.Format())
	}
	if set, ok := flow.ClosureRefAtPath(gotClosures, otherPath); !ok {
		t.Fatal("fallback closure refs missing")
	} else if got, ok := set.Singleton(); !ok || got.Ref.GraphID != 23 {
		t.Fatalf("fallback closure refs = %s, want graph 23", set.Format())
	}
}

func TestMergeEntryValuesWithFixedPreservesExplicitSlots(t *testing.T) {
	fixed := EntryValues{
		0: product.FromType(typ.String),
	}
	fallback := EntryValues{
		0: product.FromType(typ.Number),
		1: product.FromType(typ.Boolean),
	}

	got := mergeEntryValuesWithFixed(fixed, fallback)

	if !product.Equal(got[0], fixed[0]) {
		t.Fatalf("slot 0 = %s, want explicit fixed %s", got[0].ProjectValue(), fixed[0].ProjectValue())
	}
	if !product.Equal(got[1], fallback[1]) {
		t.Fatalf("slot 1 = %s, want fallback %s", got[1].ProjectValue(), fallback[1].ProjectValue())
	}
}
