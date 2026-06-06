package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReferenceContextProjectsAllAxesTogether(t *testing.T) {
	kept := cfg.SymbolID(10)
	dropped := cfg.SymbolID(11)
	keptPath := constraint.NewPath(kept, "kept").Field("factory")
	droppedPath := constraint.NewPath(dropped, "dropped").Field("factory")
	references := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{
			{Symbol: kept, Value: product.FromType(typ.String)},
			{Symbol: dropped, Value: product.FromType(typ.Number)},
		}),
		WithFunctionRefPath(
			WithFunctionRefPath(nil, keptPath, FunctionRefSetOf(FunctionRef{GraphID: 1})),
			droppedPath,
			FunctionRefSetOf(FunctionRef{GraphID: 2}),
		),
		WithClosureRefAddress(
			WithClosureRefAddress(nil, testStableAddressPath(t, keptPath), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 3}, CaptureCellsDomain.Bottom(), nil))),
			testStableAddressPath(t, droppedPath),
			ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 4}, CaptureCellsDomain.Bottom(), nil)),
		),
	)

	projected := references.ProjectPaths(ReferencePathProjection{Subtrees: []constraint.Path{constraint.NewPath(kept, "kept")}})
	if av, ok := projected.CaptureCells().Value(kept); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("kept cell = %v/%v, want string", av.ProjectValue(), ok)
	}
	if _, ok := projected.CaptureCells().Value(dropped); ok {
		t.Fatalf("dropped cell survived projection: %s", projected.CaptureCells().Format())
	}
	if _, ok := FunctionRefAtPath(projected.FunctionRefs(), keptPath); !ok {
		t.Fatalf("kept function refs missing: %#v", projected.FunctionRefs())
	}
	if _, ok := FunctionRefAtPath(projected.FunctionRefs(), droppedPath); ok {
		t.Fatalf("dropped function refs survived projection: %#v", projected.FunctionRefs())
	}
	if _, ok := ClosureRefAtPath(projected.ClosureRefs(), keptPath); !ok {
		t.Fatalf("kept closure refs missing: %#v", projected.ClosureRefs())
	}
	if _, ok := ClosureRefAtPath(projected.ClosureRefs(), droppedPath); ok {
		t.Fatalf("dropped closure refs survived projection: %#v", projected.ClosureRefs())
	}
}

func TestReferenceContextOverlayLiveAxesOverrideSnapshot(t *testing.T) {
	sym := cfg.SymbolID(20)
	path := constraint.NewPath(sym, "module").Field("factory")
	base := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
		WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 1})),
		WithClosureRefAddress(nil, testStableAddressPath(t, path), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 2}, CaptureCellsDomain.Bottom(), nil))),
	)
	live := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 3})),
		WithClosureRefAddress(nil, testStableAddressPath(t, path), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 4}, CaptureCellsDomain.Bottom(), nil))),
	)

	overlaid := OverlayReferenceContext(base, live)
	if av, ok := overlaid.CaptureCells().Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("overlaid cell = %v/%v, want live string", av.ProjectValue(), ok)
	}
	if set, ok := FunctionRefAtPath(overlaid.FunctionRefs(), path); !ok {
		t.Fatal("overlaid function refs missing")
	} else if got, singleton := set.Singleton(); !singleton || got.GraphID != 3 {
		t.Fatalf("overlaid function refs = %s, want graph 3", set.Format())
	}
	if set, ok := ClosureRefAtPath(overlaid.ClosureRefs(), path); !ok {
		t.Fatal("overlaid closure refs missing")
	} else if got, singleton := set.Singleton(); !singleton || got.Ref.GraphID != 4 {
		t.Fatalf("overlaid closure refs = %s, want graph 4", set.Format())
	}
}
