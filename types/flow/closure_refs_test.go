package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestClosureRefsDomain_Laws(t *testing.T) {
	cellString := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)}})
	cellNumber := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number)}})
	fnRefs := WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(12), "fn").Key(), FunctionRefSetOf(FunctionRef{GraphID: 12}))
	a := ClosureRefOf(FunctionRef{GraphID: 1}, cellString, FunctionRefsDomain.Bottom())
	b := ClosureRefOf(FunctionRef{GraphID: 1}, cellNumber, FunctionRefsDomain.Bottom())
	c := ClosureRefOf(FunctionRef{GraphID: 2}, cellString, fnRefs)

	lattice.LawSuite[ClosureRefs]{
		Name:   "ClosureRefs",
		Domain: ClosureRefsDomain,
		Sample: []ClosureRefs{
			ClosureRefsDomain.Bottom(),
			ClosureRefsDomain.Top(),
			WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(1), "f").Key(), ClosureRefSetOf(a)),
			WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(1), "f").Key(), ClosureRefSetOf(b)),
			WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(2), "g").Field("h").Key(), ClosureRefSetOf(c)),
			ClosureRefs{
				constraint.NewPath(cfg.SymbolID(1), "f").Key(): ClosureRefSetOf(a, b),
				constraint.NewPath(cfg.SymbolID(2), "g").Key(): ClosureRefSetOf(c),
			},
		},
	}.Run(t)
}

func TestClosureRefSetJoinsCapturedEnvironmentForSameFunction(t *testing.T) {
	stringCell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}})
	numberCell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.Number)}})
	ref := FunctionRef{GraphID: 99}

	set := ClosureRefSetOf(
		ClosureRefOf(ref, stringCell, nil),
		ClosureRefOf(ref, numberCell, nil),
		ClosureRefOf(ref, stringCell, nil),
	)

	got, ok := set.Singleton()
	if !ok {
		t.Fatalf("closure set = %s, want one abstract environment for same function identity", set.Format())
	}
	cell, ok := got.EntryCells().Value(cfg.SymbolID(7))
	if !ok ||
		!product.Domain.LessOrEq(product.FromType(typ.String), cell) ||
		!product.Domain.LessOrEq(product.FromType(typ.Number), cell) {
		t.Fatalf("joined captured cell = %v/%v, want string and number included", cell.ProjectValue(), ok)
	}
}

func TestClosureRefSetJoinMergesSameFunctionContexts(t *testing.T) {
	stringCell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}})
	numberCell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.Number)}})
	ref := FunctionRef{GraphID: 99}
	path := constraint.NewPath(cfg.SymbolID(1), "fn").Key()

	a := WithClosureRef(nil, path, ClosureRefSetOf(ClosureRefOf(ref, stringCell, nil)))
	b := WithClosureRef(nil, path, ClosureRefSetOf(ClosureRefOf(ref, numberCell, nil)))
	joined := ClosureRefsDomain.Join(a, b)
	set, ok := ClosureRefAt(joined, path)
	if !ok {
		t.Fatalf("join same-function closure contexts missing set")
	}
	got, singleton := set.Singleton()
	if !singleton {
		t.Fatalf("join same-function closure contexts = %s, want singleton with joined env", set.Format())
	}
	cell, ok := got.EntryCells().Value(cfg.SymbolID(7))
	if !ok ||
		!product.Domain.LessOrEq(product.FromType(typ.String), cell) ||
		!product.Domain.LessOrEq(product.FromType(typ.Number), cell) {
		t.Fatalf("joined captured cell = %v/%v, want string and number included", cell.ProjectValue(), ok)
	}
}

func TestClosureRefsKeyBoundsNestedClosureEnvironment(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(42), "fn").Key()
	refs := WithClosureRef(nil, path, ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 1}, CaptureCellsDomain.Bottom(), nil),
	))
	for graphID := uint64(2); graphID <= 8; graphID++ {
		refs = WithClosureRef(nil, path, ClosureRefSetOf(
			ClosureRefOf(FunctionRef{GraphID: graphID}, CaptureCellsDomain.Bottom(), nil, refs),
		))
	}

	key := ClosureRefsKeyOf(refs)
	outerSet, ok := ClosureRefAt(key.Refs(), path)
	if !ok {
		t.Fatalf("bounded key lost outer closure: %s", key.Format())
	}
	outer, singleton := outerSet.Singleton()
	if !singleton {
		t.Fatalf("outer closure set = %s, want singleton", outerSet.Format())
	}
	firstNestedSet, ok := ClosureRefAt(outer.EntryClosureRefs(), path)
	if !ok {
		t.Fatalf("bounded key lost first nested closure: %s", formatClosureRefs(outer.EntryClosureRefs()))
	}
	firstNested, singleton := firstNestedSet.Singleton()
	if !singleton {
		t.Fatalf("first nested closure set = %s, want singleton", firstNestedSet.Format())
	}
	limited, ok := ClosureRefAt(firstNested.EntryClosureRefs(), path)
	if !ok || !limited.IsTop() {
		t.Fatalf("second nested closure env = %s/%v, want top/true", limited.Format(), ok)
	}
}

func TestRebaseClosureRefsMovesSubtree(t *testing.T) {
	closure := ClosureRefOf(
		FunctionRef{GraphID: 9},
		CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}}),
		nil,
	)
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(cfg.SymbolID(42), "out")
	refs := WithClosureRef(nil, from.Field("method").Key(), ClosureRefSetOf(closure))

	rebased := RebaseClosureRefs(refs, from, to)
	set, ok := ClosureRefAt(rebased, to.Field("method").Key())
	if !ok {
		t.Fatalf("rebased closure refs missing: %#v", rebased)
	}
	got, singleton := set.Singleton()
	if !singleton || !closureRefEqual(got, closure) {
		t.Fatalf("rebased closure = %s, want %s", set.Format(), ClosureRefSetOf(closure).Format())
	}
}

func TestApplyClosureRefCellEffectsUpdatesStoredEnvironment(t *testing.T) {
	sym := cfg.SymbolID(7)
	path := constraint.NewPath(cfg.SymbolID(42), "fn").Key()
	closure := ClosureRefOf(
		FunctionRef{GraphID: 9},
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
		nil,
	)
	refs := WithClosureRef(nil, path, ClosureRefSetOf(closure))

	updated := ApplyClosureRefCellEffects(refs, path, CaptureMustWrite(sym, product.FromType(typ.String)))
	set, ok := ClosureRefAt(updated, path)
	if !ok {
		t.Fatalf("updated closure refs missing: %#v", updated)
	}
	got, singleton := set.Singleton()
	if !singleton {
		t.Fatalf("updated closure refs = %s, want singleton", set.Format())
	}
	if av, ok := got.EntryCells().Value(sym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("updated closure cell = %v/%v, want string", av.ProjectValue(), ok)
	}
}
