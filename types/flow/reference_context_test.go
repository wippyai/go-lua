package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReferenceContextDomainLaws(t *testing.T) {
	leftSym := cfg.SymbolID(1)
	rightSym := cfg.SymbolID(2)
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

	lattice.LawSuite[ReferenceContext]{
		Name:   "ReferenceContext",
		Domain: ReferenceContextDomain,
		Sample: []ReferenceContext{
			ReferenceContextDomain.Bottom(),
			ReferenceContextDomain.Top(),
			left,
			right,
			left.Join(right),
			left.CallableIdentity(),
		},
	}.Run(t)
}

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

func TestReferenceContextHasCallablePathChecksIdentityAxes(t *testing.T) {
	sym := cfg.SymbolID(60)
	fnPath := constraint.NewPath(sym, "module").Field("factory")
	closurePath := constraint.NewPath(sym, "module").Field("builder")
	cellPath := constraint.NewPath(sym, "module")

	references := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(FunctionRef{GraphID: 60})),
		WithClosureRefPath(nil, closurePath, ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 61}, CaptureCellsDomain.Bottom(), nil))),
	)

	if !references.HasCallablePath(fnPath) {
		t.Fatalf("function identity path was not callable")
	}
	if !references.HasCallablePath(closurePath) {
		t.Fatalf("closure identity path was not callable")
	}
	if references.HasCallablePath(cellPath) {
		t.Fatalf("capture-cell-only path was treated as callable")
	}
	if references.HasCallablePath(constraint.Path{}) {
		t.Fatalf("empty path was treated as callable")
	}
}

func TestMergeReferenceContextWithFixedPreservesExplicitContext(t *testing.T) {
	sym := cfg.SymbolID(7)
	other := cfg.SymbolID(8)
	path := constraint.NewPath(sym, "M").Field("dep").Field("get")
	otherPath := constraint.NewPath(other, "N").Field("make")

	fixedCells := CaptureCellsOf([]CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.String)},
	})
	fallbackCells := CaptureCellsOf([]CaptureCell{
		{Symbol: sym, Value: product.FromType(typ.Nil)},
		{Symbol: other, Value: product.FromType(typ.Number)},
	})
	got := MergeReferenceContextWithFixed(
		ReferenceContextOf(fixedCells, nil, nil),
		ReferenceContextOf(fallbackCells, nil, nil),
	)
	gotCells := got.CaptureCells()
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("fixed cell overwritten: got %v/%v, want string", av.ProjectValue(), ok)
	}
	if av, ok := gotCells.Value(other); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("fallback missing cell: got %v/%v, want number", av.ProjectValue(), ok)
	}

	emptyRecord := typ.NewRecord().Build()
	recordWithMethod := typ.NewRecord().Field("render", typ.Func().Returns(typ.String).Build()).Build()
	got = MergeReferenceContextWithFixed(
		ReferenceContextOf(
			CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(emptyRecord)}}),
			nil,
			nil,
		),
		ReferenceContextOf(
			CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(recordWithMethod)}}),
			nil,
			nil,
		),
	)
	gotCells = got.CaptureCells()
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), recordWithMethod) {
		t.Fatalf("narrower fallback cell = %v/%v, want record with render", av.ProjectValue(), ok)
	}

	got = MergeReferenceContextWithFixed(
		ReferenceContextOf(
			CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
			nil,
			nil,
		),
		ReferenceContextOf(
			CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.Any)}}),
			nil,
			nil,
		),
	)
	gotCells = got.CaptureCells()
	if av, ok := gotCells.Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.Number) {
		t.Fatalf("broad fallback cell = %v/%v, want fixed number", av.ProjectValue(), ok)
	}

	fixedRefs := WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 11}))
	fallbackRefs := WithFunctionRefPath(nil, path, FunctionRefSetOf(FunctionRef{GraphID: 12}))
	fallbackRefs = WithFunctionRefPath(fallbackRefs, otherPath, FunctionRefSetOf(FunctionRef{GraphID: 13}))
	got = MergeReferenceContextWithFixed(
		ReferenceContextOf(CaptureCellsDomain.Bottom(), fixedRefs, nil),
		ReferenceContextOf(CaptureCellsDomain.Bottom(), fallbackRefs, nil),
	)
	gotRefs := got.FunctionRefs()
	if set, ok := FunctionRefAtPath(gotRefs, path); !ok {
		t.Fatal("fixed function refs missing")
	} else if got, ok := set.Singleton(); !ok || got.GraphID != 11 {
		t.Fatalf("fixed function refs overwritten: got %s, want graph 11", set.Format())
	}
	if set, ok := FunctionRefAtPath(gotRefs, otherPath); !ok {
		t.Fatal("fallback function refs missing")
	} else if got, ok := set.Singleton(); !ok || got.GraphID != 13 {
		t.Fatalf("fallback function refs = %s, want graph 13", set.Format())
	}

	fixedClosures := WithClosureRefPath(nil, path, ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 21}, fixedCells, fixedRefs),
	))
	fallbackClosures := WithClosureRefPath(nil, path, ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 22}, fallbackCells, fallbackRefs),
	))
	fallbackClosures = WithClosureRefPath(fallbackClosures, otherPath, ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 23}, fallbackCells, fallbackRefs),
	))
	got = MergeReferenceContextWithFixed(
		ReferenceContextOf(CaptureCellsDomain.Bottom(), nil, fixedClosures),
		ReferenceContextOf(CaptureCellsDomain.Bottom(), nil, fallbackClosures),
	)
	gotClosures := got.ClosureRefs()
	if set, ok := ClosureRefAtPath(gotClosures, path); !ok {
		t.Fatal("fixed closure refs missing")
	} else if got, ok := set.Singleton(); !ok || got.Ref.GraphID != 21 {
		t.Fatalf("fixed closure refs overwritten: got %s, want graph 21", set.Format())
	}
	if set, ok := ClosureRefAtPath(gotClosures, otherPath); !ok {
		t.Fatal("fallback closure refs missing")
	} else if got, ok := set.Singleton(); !ok || got.Ref.GraphID != 23 {
		t.Fatalf("fallback closure refs = %s, want graph 23", set.Format())
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

func TestReferenceContextJoinRefPathAddsIdentitySets(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(52), "fn")
	firstFn := FunctionRef{GraphID: 52}
	secondFn := FunctionRef{GraphID: 53}
	firstClosure := ClosureRefOf(FunctionRef{GraphID: 54}, CaptureCellsDomain.Bottom(), nil)
	secondClosure := ClosureRefOf(FunctionRef{GraphID: 55}, CaptureCellsDomain.Bottom(), nil)

	references := ReferenceContextBottom().
		JoinFunctionRefPath(path, FunctionRefSetOf(firstFn)).
		JoinFunctionRefPath(path, FunctionRefSetOf(secondFn)).
		JoinClosureRefPath(path, ClosureRefSetOf(firstClosure)).
		JoinClosureRefPath(path, ClosureRefSetOf(secondClosure))

	if set, ok := FunctionRefAtPath(references.FunctionRefs(), path); !ok || len(set.Refs()) != 2 {
		t.Fatalf("joined function refs = %s/%v, want two refs", set.Format(), ok)
	}
	if set, ok := ClosureRefAtPath(references.ClosureRefs(), path); !ok || len(set.Refs()) != 2 {
		t.Fatalf("joined closure refs = %s/%v, want two refs", set.Format(), ok)
	}
}

func TestReferenceContextCallableIdentityDropsCaptureCells(t *testing.T) {
	sym := cfg.SymbolID(56)
	path := constraint.NewPath(sym, "fn")
	fn := FunctionRef{GraphID: 56}
	closure := ClosureRefOf(FunctionRef{GraphID: 57}, CaptureCellsDomain.Bottom(), nil)
	references := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, path, FunctionRefSetOf(fn)),
		WithClosureRefAddress(nil, testStableAddressPath(t, path), ClosureRefSetOf(closure)),
	)

	identity := references.CallableIdentity()
	if len(identity.CaptureCells().Entries()) != 0 {
		t.Fatalf("identity projection kept capture cells: %s", identity.CaptureCells().Format())
	}
	if _, ok := FunctionRefAtPath(identity.FunctionRefs(), path); !ok {
		t.Fatalf("identity projection lost function refs: %#v", identity.FunctionRefs())
	}
	if _, ok := ClosureRefAtPath(identity.ClosureRefs(), path); !ok {
		t.Fatalf("identity projection lost closure refs: %#v", identity.ClosureRefs())
	}
}

func TestReferenceContextRootSymbolsUnifiesAxes(t *testing.T) {
	cellSym := cfg.SymbolID(58)
	fnSym := cfg.SymbolID(59)
	closureSym := cfg.SymbolID(60)
	references := ReferenceContextOf(
		CaptureCellsOf([]CaptureCell{{Symbol: cellSym, Value: product.FromType(typ.String)}}),
		WithFunctionRefPath(nil, constraint.NewPath(fnSym, "make"), FunctionRefSetOf(FunctionRef{GraphID: 58})),
		WithClosureRefAddress(nil, testStableAddressPath(t, constraint.NewPath(closureSym, "make")), ClosureRefSetOf(
			ClosureRefOf(FunctionRef{GraphID: 59}, CaptureCellsDomain.Bottom(), nil),
		)),
	)

	got := references.RootSymbols()
	want := []cfg.SymbolID{cellSym, fnSym, closureSym}
	if len(got) != len(want) {
		t.Fatalf("root symbols = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("root symbols = %v, want %v", got, want)
		}
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
