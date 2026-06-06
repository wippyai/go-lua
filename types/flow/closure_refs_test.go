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

func TestClosureRefEqualUsesCanonicalEnvironmentKeys(t *testing.T) {
	cell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}})
	fnPath := constraint.NewPath(cfg.SymbolID(9), "captured")
	fnRefsA := WithFunctionRef(nil, fnPath.Key(), FunctionRefSetOf(FunctionRef{GraphID: 100}))
	fnRefsB := WithFunctionRef(nil, fnPath.Key(), FunctionRefSetOf(FunctionRef{GraphID: 100}))
	nestedA := WithClosureRef(nil, fnPath.Field("inner").Key(), ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 101}, CaptureCellsDomain.Bottom(), nil),
	))
	nestedB := WithClosureRef(nil, fnPath.Field("inner").Key(), ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 101}, CaptureCellsDomain.Bottom(), nil),
	))

	a := ClosureRefOf(FunctionRef{GraphID: 10}, cell, fnRefsA, nestedA)
	b := ClosureRefOf(FunctionRef{GraphID: 10}, cell, fnRefsB, nestedB)

	if a != b {
		t.Fatalf("semantically equal closure environments did not share canonical keys:\n  a=%s\n  b=%s",
			ClosureRefSetOf(a).Format(), ClosureRefSetOf(b).Format())
	}
	if !closureRefEqual(a, b) {
		t.Fatalf("closureRefEqual rejected canonical-key-equal closure refs")
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
	fromAddr, ok := StableAddressOfPath(from)
	if !ok {
		t.Fatal("from address")
	}
	toAddr, ok := StableAddressOfPath(to)
	if !ok {
		t.Fatal("to address")
	}
	methodAddr, ok := StableAddressOfPath(from.Field("method"))
	if !ok {
		t.Fatal("method address")
	}
	toMethodAddr, ok := StableAddressOfPath(to.Field("method"))
	if !ok {
		t.Fatal("target method address")
	}
	refs := WithClosureRefAddress(nil, methodAddr, ClosureRefSetOf(closure))

	rebased := RebaseClosureRefsAddress(refs, fromAddr, toAddr)
	set, ok := ClosureRefAtAddress(rebased, toMethodAddr)
	if !ok {
		t.Fatalf("rebased closure refs missing: %#v", rebased)
	}
	got, singleton := set.Singleton()
	if !singleton || !closureRefEqual(got, closure) {
		t.Fatalf("rebased closure = %s, want %s", set.Format(), ClosureRefSetOf(closure).Format())
	}
}

func TestRebaseClosureRefsPathMovesSubtree(t *testing.T) {
	closure := ClosureRefOf(
		FunctionRef{GraphID: 9},
		CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}}),
		nil,
	)
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(cfg.SymbolID(43), "out")
	method := from.Field("method")
	toMethod := to.Field("method")
	refs := WithClosureRef(nil, StablePathKey(method), ClosureRefSetOf(closure))

	rebased := RebaseClosureRefsPath(refs, from, to)
	set, ok := ClosureRefAt(rebased, StablePathKey(toMethod))
	if !ok {
		t.Fatalf("rebased closure refs missing: %#v", rebased)
	}
	got, singleton := set.Singleton()
	if !singleton || !closureRefEqual(got, closure) {
		t.Fatalf("rebased closure = %s, want %s", set.Format(), ClosureRefSetOf(closure).Format())
	}
	if pathSet, ok := ClosureRefAtPath(rebased, toMethod); !ok || !ClosureRefSetDomain.Equal(pathSet, set) {
		t.Fatalf("ClosureRefAtPath = %s/%v, want %s/true", pathSet.Format(), ok, set.Format())
	}
}

func TestRebaseClosureRefsTopScopesToTarget(t *testing.T) {
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(cfg.SymbolID(43), "out")

	rebased := RebaseClosureRefsPath(ClosureRefsDomain.Top(), from, to)
	if ClosureRefsDomain.Equal(rebased, ClosureRefsDomain.Top()) {
		t.Fatalf("rebased top leaked to global top")
	}
	set, ok := ClosureRefAtPath(rebased, to)
	if !ok || !set.IsTop() {
		t.Fatalf("target closure ref = %s/%v, want top/true", set.Format(), ok)
	}
	if _, ok := ClosureRefAtPath(rebased, constraint.NewPath(cfg.SymbolID(44), "other")); ok {
		t.Fatalf("rebased top leaked to unrelated path: %#v", rebased)
	}
}

func TestProjectClosureRefsByPathKeepsSubtree(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(45), "root")
	child := root.Field("child")
	sibling := constraint.NewPath(cfg.SymbolID(46), "sibling")
	closure := ClosureRefOf(FunctionRef{GraphID: 9}, CaptureCellsDomain.Bottom(), nil)
	refs := WithClosureRef(nil, StablePathKey(child), ClosureRefSetOf(closure))
	refs = WithClosureRef(refs, StablePathKey(sibling), ClosureRefSetTop())

	projected := ProjectClosureRefsByPath(refs, root)
	if _, ok := ClosureRefAtPath(projected, child); !ok {
		t.Fatalf("projection dropped child path: %#v", projected)
	}
	if _, ok := ClosureRefAtPath(projected, sibling); ok {
		t.Fatalf("projection kept sibling path: %#v", projected)
	}
}

func TestAssignClosureRefSubtreePathCopiesAndReplacesTarget(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(47), "source")
	sourceChild := source.Field("child")
	target := constraint.NewPath(cfg.SymbolID(48), "target")
	targetChild := target.Field("child")
	staleTargetGrandchild := targetChild.Field("stale")
	sibling := constraint.NewPath(cfg.SymbolID(49), "sibling")
	closure := ClosureRefOf(FunctionRef{GraphID: 11}, CaptureCellsDomain.Bottom(), nil)
	stale := ClosureRefOf(FunctionRef{GraphID: 12}, CaptureCellsDomain.Bottom(), nil)
	other := ClosureRefOf(FunctionRef{GraphID: 13}, CaptureCellsDomain.Bottom(), nil)
	out := PointState{
		ClosureRefs: WithClosureRef(nil, StablePathKey(sourceChild), ClosureRefSetOf(closure)),
	}
	out.ClosureRefs = WithClosureRef(out.ClosureRefs, StablePathKey(staleTargetGrandchild), ClosureRefSetOf(stale))
	out.ClosureRefs = WithClosureRef(out.ClosureRefs, StablePathKey(sibling), ClosureRefSetOf(other))

	if !AssignClosureRefSubtreePath(&out, source, target) {
		t.Fatal("AssignClosureRefSubtreePath reported no change")
	}
	if _, ok := ClosureRefAtPath(out.ClosureRefs, staleTargetGrandchild); ok {
		t.Fatalf("stale target subtree survived assign: %#v", out.ClosureRefs)
	}
	set, ok := ClosureRefAtPath(out.ClosureRefs, targetChild)
	if !ok {
		t.Fatalf("rebased source child missing: %#v", out.ClosureRefs)
	}
	if got, singleton := set.Singleton(); !singleton || !closureRefEqual(got, closure) {
		t.Fatalf("target child closure = %s, want %s", set.Format(), ClosureRefSetOf(closure).Format())
	}
	if set, ok := ClosureRefAtPath(out.ClosureRefs, sibling); !ok {
		t.Fatalf("sibling was removed: %#v", out.ClosureRefs)
	} else if got, singleton := set.Singleton(); !singleton || !closureRefEqual(got, other) {
		t.Fatalf("sibling closure = %s, want %s", set.Format(), ClosureRefSetOf(other).Format())
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
