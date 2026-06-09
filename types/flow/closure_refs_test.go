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
	fnPath := constraint.NewPath(cfg.SymbolID(12), "fn")
	fPath := constraint.NewPath(cfg.SymbolID(1), "f")
	gPath := constraint.NewPath(cfg.SymbolID(2), "g")
	fnRefs := WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(FunctionRef{GraphID: 12}))
	a := ClosureRefOf(FunctionRef{GraphID: 1}, cellString, FunctionRefsDomain.Bottom())
	b := ClosureRefOf(FunctionRef{GraphID: 1}, cellNumber, FunctionRefsDomain.Bottom())
	c := ClosureRefOf(FunctionRef{GraphID: 2}, cellString, fnRefs)

	lattice.LawSuite[ClosureRefs]{
		Name:   "ClosureRefs",
		Domain: ClosureRefsDomain,
		Sample: []ClosureRefs{
			ClosureRefsDomain.Bottom(),
			ClosureRefsDomain.Top(),
			WithClosureRefPath(nil, fPath, ClosureRefSetOf(a)),
			WithClosureRefPath(nil, fPath, ClosureRefSetOf(b)),
			WithClosureRefPath(nil, gPath.Field("h"), ClosureRefSetOf(c)),
			ClosureRefs{
				StablePathKey(fPath): ClosureRefSetOf(a, b),
				StablePathKey(gPath): ClosureRefSetOf(c),
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
	path := constraint.NewPath(cfg.SymbolID(1), "fn")

	a := WithClosureRefPath(nil, path, ClosureRefSetOf(ClosureRefOf(ref, stringCell, nil)))
	b := WithClosureRefPath(nil, path, ClosureRefSetOf(ClosureRefOf(ref, numberCell, nil)))
	joined := ClosureRefsDomain.Join(a, b)
	set, ok := ClosureRefAtPath(joined, path)
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
	path := constraint.NewPath(cfg.SymbolID(42), "fn")
	refs := WithClosureRefPath(nil, path, ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 1}, CaptureCellsDomain.Bottom(), nil),
	))
	for graphID := uint64(2); graphID <= 8; graphID++ {
		refs = WithClosureRefPath(nil, path, ClosureRefSetOf(
			ClosureRefOf(FunctionRef{GraphID: graphID}, CaptureCellsDomain.Bottom(), nil, refs),
		))
	}

	key := ClosureRefsKeyOf(refs)
	outerSet, ok := ClosureRefAtPath(key.Refs(), path)
	if !ok {
		t.Fatalf("bounded key lost outer closure: %s", key.Format())
	}
	outer, singleton := outerSet.Singleton()
	if !singleton {
		t.Fatalf("outer closure set = %s, want singleton", outerSet.Format())
	}
	firstNestedSet, ok := ClosureRefAtPath(outer.EntryClosureRefs(), path)
	if !ok {
		t.Fatalf("bounded key lost first nested closure: %s", formatClosureRefs(outer.EntryClosureRefs()))
	}
	firstNested, singleton := firstNestedSet.Singleton()
	if !singleton {
		t.Fatalf("first nested closure set = %s, want singleton", firstNestedSet.Format())
	}
	limited, ok := ClosureRefAtPath(firstNested.EntryClosureRefs(), path)
	if !ok || !limited.IsTop() {
		t.Fatalf("second nested closure env = %s/%v, want top/true", limited.Format(), ok)
	}
}

func TestClosureRefEqualUsesCanonicalEnvironmentKeys(t *testing.T) {
	cell := CaptureCellsOf([]CaptureCell{{Symbol: cfg.SymbolID(7), Value: product.FromType(typ.String)}})
	fnPath := constraint.NewPath(cfg.SymbolID(9), "captured")
	fnRefsA := WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(FunctionRef{GraphID: 100}))
	fnRefsB := WithFunctionRefPath(nil, fnPath, FunctionRefSetOf(FunctionRef{GraphID: 100}))
	nestedA := WithClosureRefPath(nil, fnPath.Field("inner"), ClosureRefSetOf(
		ClosureRefOf(FunctionRef{GraphID: 101}, CaptureCellsDomain.Bottom(), nil),
	))
	nestedB := WithClosureRefPath(nil, fnPath.Field("inner"), ClosureRefSetOf(
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
	refs := WithClosureRefPath(nil, method, ClosureRefSetOf(closure))

	rebased := RebaseClosureRefsPath(refs, from, to)
	set, ok := ClosureRefAtPath(rebased, toMethod)
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
	refs := WithClosureRefPath(nil, child, ClosureRefSetOf(closure))
	refs = WithClosureRefPath(refs, sibling, ClosureRefSetTop())

	projected := ProjectClosureRefsByPath(refs, root)
	if _, ok := ClosureRefAtPath(projected, child); !ok {
		t.Fatalf("projection dropped child path: %#v", projected)
	}
	if _, ok := ClosureRefAtPath(projected, sibling); ok {
		t.Fatalf("projection kept sibling path: %#v", projected)
	}
}

func TestClosureRefsSubtreeStrongUpdateUsesStableAddress(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(145), "dep")
	child := root.Field("get")
	samePrintedRoot := constraint.NewPath(cfg.SymbolID(146), "dep")
	closure := ClosureRefOf(FunctionRef{GraphID: 9}, CaptureCellsDomain.Bottom(), nil)

	refs := WithClosureRefPath(nil, root, ClosureRefSetOf(closure))
	refs = WithClosureRefPath(refs, child, ClosureRefSetOf(closure))
	refs = WithClosureRefPath(refs, samePrintedRoot, ClosureRefSetOf(closure))

	rootAddr, ok := StableAddressOfPath(root)
	if !ok {
		t.Fatalf("stable address for path %s", root.Key())
	}
	refs = WithoutClosureRefSubtreeAddress(refs, rootAddr)
	if _, ok := ClosureRefAtPath(refs, root); ok {
		t.Fatalf("root closure survived subtree clear: %#v", refs)
	}
	if _, ok := ClosureRefAtPath(refs, child); ok {
		t.Fatalf("child closure survived subtree clear: %#v", refs)
	}
	if _, ok := ClosureRefAtPath(refs, samePrintedRoot); !ok {
		t.Fatalf("same printed root with different symbol was removed: %#v", refs)
	}
}

func TestClosureRefRootSymbolsUsesAddressRoots(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(55), "root")
	child := root.Field("child")
	other := constraint.NewPath(cfg.SymbolID(56), "other")
	named, ok := StableAddressOfRoot("placeholder", nil)
	if !ok {
		t.Fatal("named root address did not build")
	}
	closure := ClosureRefOf(FunctionRef{GraphID: 9}, CaptureCellsDomain.Bottom(), nil)
	refs := WithClosureRefAddress(nil, testStableAddressPath(t, root), ClosureRefSetOf(closure))
	refs = WithClosureRefAddress(refs, testStableAddressPath(t, child), ClosureRefSetTop())
	refs = WithClosureRefAddress(refs, testStableAddressPath(t, other), ClosureRefSetOf(closure))
	refs = WithClosureRefAddress(refs, named, ClosureRefSetOf(closure))

	got := ClosureRefRootSymbols(refs)
	if len(got) != 2 || got[0] != root.Symbol || got[1] != other.Symbol {
		t.Fatalf("root symbols = %v, want [%d %d]", got, root.Symbol, other.Symbol)
	}
}

func TestClosureRefAtAddressDoesNotReadNonCanonicalPathKeyEntry(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(57), "closure").Field("call")
	addr, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatal("stable address did not resolve")
	}
	nonCanonicalKey := path.Key()
	if nonCanonicalKey == addr.Key() {
		t.Fatalf("test key is already canonical: %s", nonCanonicalKey)
	}
	refs := ClosureRefs{
		nonCanonicalKey: ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 57}, CaptureCellsDomain.Bottom(), nil)),
	}

	if _, ok := ClosureRefAtAddress(refs, addr); ok {
		t.Fatalf("noncanonical path key %s was accepted as canonical address storage %s", nonCanonicalKey, addr.Key())
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
		ClosureRefs: WithClosureRefPath(nil, sourceChild, ClosureRefSetOf(closure)),
	}
	out.ClosureRefs = WithClosureRefPath(out.ClosureRefs, staleTargetGrandchild, ClosureRefSetOf(stale))
	out.ClosureRefs = WithClosureRefPath(out.ClosureRefs, sibling, ClosureRefSetOf(other))

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

func TestReplaceClosureRefTreePathInstallsRootAndNestedEntries(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(50), "target")
	child := target.Field("child")
	staleGrandchild := child.Field("stale")
	sibling := constraint.NewPath(cfg.SymbolID(51), "sibling")
	rootClosure := ClosureRefOf(FunctionRef{GraphID: 14}, CaptureCellsDomain.Bottom(), nil)
	childClosure := ClosureRefOf(FunctionRef{GraphID: 15}, CaptureCellsDomain.Bottom(), nil)
	stale := ClosureRefOf(FunctionRef{GraphID: 16}, CaptureCellsDomain.Bottom(), nil)
	other := ClosureRefOf(FunctionRef{GraphID: 17}, CaptureCellsDomain.Bottom(), nil)
	out := PointState{
		ClosureRefs: WithClosureRefAddress(nil, testStableAddressPath(t, staleGrandchild), ClosureRefSetOf(stale)),
	}
	out.ClosureRefs = WithClosureRefAddress(out.ClosureRefs, testStableAddressPath(t, sibling), ClosureRefSetOf(other))

	tree := ClosureRefTree{
		Root:    ClosureRefSetOf(rootClosure),
		HasRoot: true,
		Entries: []ClosureRefTreeEntry{{
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "child"}},
			Set:      ClosureRefSetOf(childClosure),
		}},
	}

	if !ReplaceClosureRefTreePath(&out, target, tree) {
		t.Fatal("ReplaceClosureRefTreePath reported no change")
	}
	if _, ok := ClosureRefAtPath(out.ClosureRefs, staleGrandchild); ok {
		t.Fatalf("stale target subtree survived tree replace: %#v", out.ClosureRefs)
	}
	if set, ok := ClosureRefAtPath(out.ClosureRefs, target); !ok {
		t.Fatalf("root closure missing: %#v", out.ClosureRefs)
	} else if got, singleton := set.Singleton(); !singleton || !closureRefEqual(got, rootClosure) {
		t.Fatalf("root closure = %s, want %s", set.Format(), ClosureRefSetOf(rootClosure).Format())
	}
	if set, ok := ClosureRefAtPath(out.ClosureRefs, child); !ok {
		t.Fatalf("nested closure missing: %#v", out.ClosureRefs)
	} else if got, singleton := set.Singleton(); !singleton || !closureRefEqual(got, childClosure) {
		t.Fatalf("nested closure = %s, want %s", set.Format(), ClosureRefSetOf(childClosure).Format())
	}
	if set, ok := ClosureRefAtPath(out.ClosureRefs, sibling); !ok {
		t.Fatalf("sibling was removed: %#v", out.ClosureRefs)
	} else if got, singleton := set.Singleton(); !singleton || !closureRefEqual(got, other) {
		t.Fatalf("sibling closure = %s, want %s", set.Format(), ClosureRefSetOf(other).Format())
	}
}

func TestReplaceClosureRefTreePathCanonicalizesEntries(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(52), "target")
	rootClosure := ClosureRefOf(FunctionRef{GraphID: 18}, CaptureCellsDomain.Bottom(), nil)
	rootEntryClosure := ClosureRefOf(FunctionRef{GraphID: 19}, CaptureCellsDomain.Bottom(), nil)
	childA := target.Field("a")
	childB := target.Field("b")
	closureA := ClosureRefOf(FunctionRef{GraphID: 20}, CaptureCellsDomain.Bottom(), nil)
	closureADuplicate := ClosureRefOf(FunctionRef{GraphID: 21}, CaptureCellsDomain.Bottom(), nil)
	closureB := ClosureRefOf(FunctionRef{GraphID: 22}, CaptureCellsDomain.Bottom(), nil)
	out := PointState{}

	tree := ClosureRefTree{
		Root:    ClosureRefSetOf(rootClosure),
		HasRoot: true,
		Entries: []ClosureRefTreeEntry{
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}}, Set: ClosureRefSetOf(closureB)},
			{Segments: nil, Set: ClosureRefSetOf(rootEntryClosure)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}}, Set: ClosureRefSetOf(closureA)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}}, Set: ClosureRefSetOf(closureADuplicate)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "dropped"}}, Set: ClosureRefSetDomain.Bottom()},
		},
	}

	if !ReplaceClosureRefTreePath(&out, target, tree) {
		t.Fatal("ReplaceClosureRefTreePath reported no change")
	}
	if got, ok := ClosureRefAtPath(out.ClosureRefs, target); !ok {
		t.Fatalf("root closures missing: %#v", out.ClosureRefs)
	} else if want := ClosureRefSetOf(rootClosure, rootEntryClosure); !ClosureRefSetDomain.Equal(got, want) {
		t.Fatalf("root closures = %s, want %s", got.Format(), want.Format())
	}
	if got, ok := ClosureRefAtPath(out.ClosureRefs, childA); !ok {
		t.Fatalf("merged child closures missing: %#v", out.ClosureRefs)
	} else if want := ClosureRefSetOf(closureA, closureADuplicate); !ClosureRefSetDomain.Equal(got, want) {
		t.Fatalf("child a closures = %s, want %s", got.Format(), want.Format())
	}
	if got, ok := ClosureRefAtPath(out.ClosureRefs, childB); !ok {
		t.Fatalf("child b closures missing: %#v", out.ClosureRefs)
	} else if want := ClosureRefSetOf(closureB); !ClosureRefSetDomain.Equal(got, want) {
		t.Fatalf("child b closures = %s, want %s", got.Format(), want.Format())
	}
	if _, ok := ClosureRefAtPath(out.ClosureRefs, target.Field("dropped")); ok {
		t.Fatalf("bottom tree entry was installed: %#v", out.ClosureRefs)
	}
}

func TestClosureRefTreeFromSubtreePathProjectsRelativeEntries(t *testing.T) {
	source := constraint.NewPlaceholder(0)
	child := source.Field("child")
	other := constraint.NewPlaceholder(1).Field("child")
	rootClosure := ClosureRefOf(FunctionRef{GraphID: 18}, CaptureCellsDomain.Bottom(), nil)
	childClosure := ClosureRefOf(FunctionRef{GraphID: 19}, CaptureCellsDomain.Bottom(), nil)
	otherClosure := ClosureRefOf(FunctionRef{GraphID: 20}, CaptureCellsDomain.Bottom(), nil)
	refs := WithClosureRefPath(nil, source, ClosureRefSetOf(rootClosure))
	refs = WithClosureRefPath(refs, child, ClosureRefSetOf(childClosure))
	refs = WithClosureRefPath(refs, other, ClosureRefSetOf(otherClosure))

	tree, ok := ClosureRefTreeFromSubtreePath(refs, source)
	if !ok {
		t.Fatal("ClosureRefTreeFromSubtreePath returned no tree")
	}
	if !tree.HasRoot {
		t.Fatalf("tree root missing: %#v", tree)
	}
	if got, singleton := tree.Root.Singleton(); !singleton || !closureRefEqual(got, rootClosure) {
		t.Fatalf("tree root = %s, want %s", tree.Root.Format(), ClosureRefSetOf(rootClosure).Format())
	}
	if len(tree.Entries) != 1 {
		t.Fatalf("tree entries = %#v, want one child entry", tree.Entries)
	}
	entry := tree.Entries[0]
	if len(entry.Segments) != 1 || entry.Segments[0].Kind != constraint.SegmentField || entry.Segments[0].Name != "child" {
		t.Fatalf("tree entry segments = %#v, want relative .child", entry.Segments)
	}
	if got, singleton := entry.Set.Singleton(); !singleton || !closureRefEqual(got, childClosure) {
		t.Fatalf("tree child set = %s, want %s", entry.Set.Format(), ClosureRefSetOf(childClosure).Format())
	}
}

func TestClosureRefTreeFromSubtreePathIgnoresNonCanonicalStoredKey(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(58), "source")
	child := source.Field("child")
	closure := ClosureRefOf(FunctionRef{GraphID: 23}, CaptureCellsDomain.Bottom(), nil)
	refs := ClosureRefs{
		child.Key(): ClosureRefSetOf(closure),
	}

	if tree, ok := ClosureRefTreeFromSubtreePath(refs, source); ok {
		t.Fatalf("tree accepted noncanonical stored key %s: %#v", child.Key(), tree)
	}
}

func TestApplyClosureRefCellEffectsUpdatesStoredEnvironment(t *testing.T) {
	sym := cfg.SymbolID(7)
	path := constraint.NewPath(cfg.SymbolID(42), "fn")
	closure := ClosureRefOf(
		FunctionRef{GraphID: 9},
		CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
		nil,
	)
	refs := WithClosureRefPath(nil, path, ClosureRefSetOf(closure))

	updated := ApplyClosureRefCellEffectsPath(refs, path, CaptureMustWrite(sym, product.FromType(typ.String)))
	set, ok := ClosureRefAtPath(updated, path)
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
