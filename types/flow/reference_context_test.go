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

func TestReferenceContextJoinAddsEveryAxis(t *testing.T) {
	leftSym := cfg.SymbolID(30)
	rightSym := cfg.SymbolID(31)
	leftPath := constraint.NewPath(leftSym, "left")
	rightPath := constraint.NewPath(rightSym, "right")
	left := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: leftSym, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, leftPath, FunctionRefSetOf(FunctionRef{GraphID: 1})),
		WithClosureRefAddress(nil, testStableAddressPath(t, leftPath), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 2}, CaptureCellsDomain.Bottom(), nil))),
	)
	right := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: rightSym, Value: product.FromType(typ.Number)}}),
		WithFunctionRefPath(nil, rightPath, FunctionRefSetOf(FunctionRef{GraphID: 3})),
		WithClosureRefAddress(nil, testStableAddressPath(t, rightPath), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 4}, CaptureCellsDomain.Bottom(), nil))),
	)

	joined := left.Join(right)
	if av, ok := joined.CaptureCells().Value(leftSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("joined left cell = %v/%v, want string", av.ProjectValue(), ok)
	}
	if av, ok := joined.CaptureCells().Value(rightSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("joined right cell = %v/%v, want number", av.ProjectValue(), ok)
	}
	if _, ok := FunctionRefAtPath(joined.FunctionRefs(), leftPath); !ok {
		t.Fatal("joined left function refs missing")
	}
	if _, ok := FunctionRefAtPath(joined.FunctionRefs(), rightPath); !ok {
		t.Fatal("joined right function refs missing")
	}
	if _, ok := ClosureRefAtPath(joined.ClosureRefs(), leftPath); !ok {
		t.Fatal("joined left closure refs missing")
	}
	if _, ok := ClosureRefAtPath(joined.ClosureRefs(), rightPath); !ok {
		t.Fatal("joined right closure refs missing")
	}
}

func TestReferenceContextRebaseCallablePathsMovesIdentityAxes(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(50), "source")
	target := constraint.NewPath(cfg.SymbolID(51), "target")
	fn := FunctionRef{GraphID: 50}
	closure := ClosureRefOf(FunctionRef{GraphID: 51}, CaptureCellsDomain.Bottom(), nil)
	references := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: source.Symbol, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, source.Field("nested"), FunctionRefSetOf(fn)),
		WithClosureRefAddress(nil, testStableAddressPath(t, source.Field("nested")), ClosureRefSetOf(closure)),
	)

	rebased := references.RebaseCallablePaths(source, target)
	if len(rebased.CaptureCells().Entries()) != 0 {
		t.Fatalf("rebased callable refs carried lexical cells: %s", rebased.CaptureCells().Format())
	}
	if set, ok := FunctionRefAtPath(rebased.FunctionRefs(), target.Field("nested")); !ok {
		t.Fatalf("rebased function refs missing: %#v", rebased.FunctionRefs())
	} else if got, singleton := set.Singleton(); !singleton || got != fn {
		t.Fatalf("rebased function refs = %s, want %v", set.Format(), fn)
	}
	if set, ok := ClosureRefAtPath(rebased.ClosureRefs(), target.Field("nested")); !ok {
		t.Fatalf("rebased closure refs missing: %#v", rebased.ClosureRefs())
	} else if got, singleton := set.Singleton(); !singleton || got.Ref != closure.Ref {
		t.Fatalf("rebased closure refs = %s, want %v", set.Format(), closure.Ref)
	}
}

func TestReferenceContextJoinRefAtAddsIdentitySets(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(52), "fn").Key()
	firstFn := FunctionRef{GraphID: 52}
	secondFn := FunctionRef{GraphID: 53}
	firstClosure := ClosureRefOf(FunctionRef{GraphID: 54}, CaptureCellsDomain.Bottom(), nil)
	secondClosure := ClosureRefOf(FunctionRef{GraphID: 55}, CaptureCellsDomain.Bottom(), nil)

	references := ReferenceContextBottom().
		JoinFunctionRefAt(path, FunctionRefSetOf(firstFn)).
		JoinFunctionRefAt(path, FunctionRefSetOf(secondFn)).
		JoinClosureRefAt(path, ClosureRefSetOf(firstClosure)).
		JoinClosureRefAt(path, ClosureRefSetOf(secondClosure))

	if set, ok := FunctionRefAt(references.FunctionRefs(), path); !ok || len(set.Refs()) != 2 {
		t.Fatalf("joined function refs = %s/%v, want two refs", set.Format(), ok)
	}
	if set, ok := ClosureRefAt(references.ClosureRefs(), path); !ok || len(set.Refs()) != 2 {
		t.Fatalf("joined closure refs = %s/%v, want two refs", set.Format(), ok)
	}
}

func TestReferenceContextKeyRoundTrip(t *testing.T) {
	sym := cfg.SymbolID(40)
	path := constraint.NewPath(sym, "dep").Field("make")
	cells := CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.String)}})
	functionRefs := WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 40}))
	closureRefs := WithClosureRefAddress(nil, testStableAddressPath(t, path), ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 41}, CaptureCellsDomain.Bottom(), nil)))

	key := ReferenceContextKeyOf(ReferenceContextOf(cells, functionRefs, closureRefs))
	if !CaptureCellsDomain.Equal(key.CaptureCells(), cells) {
		t.Fatalf("key cells = %s, want %s", key.CaptureCells().Format(), cells.Format())
	}
	if !FunctionRefsDomain.Equal(key.FunctionRefs(), functionRefs) {
		t.Fatalf("key function refs = %#v, want %#v", key.FunctionRefs(), functionRefs)
	}
	if !ClosureRefsDomain.Equal(key.ClosureRefs(), closureRefs) {
		t.Fatalf("key closure refs = %s, want %s", ClosureRefsKeyOf(key.ClosureRefs()).Format(), ClosureRefsKeyOf(closureRefs).Format())
	}

	roundTrip := key.Context()
	if !CaptureCellsDomain.Equal(roundTrip.CaptureCells(), cells) ||
		!FunctionRefsDomain.Equal(roundTrip.FunctionRefs(), functionRefs) ||
		!ClosureRefsDomain.Equal(roundTrip.ClosureRefs(), closureRefs) {
		t.Fatalf("round trip = %#v", roundTrip)
	}
}
