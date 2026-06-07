package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

func TestFunctionRefSetLatticeLaws(t *testing.T) {
	a := FunctionRef{GraphID: 1}
	b := FunctionRef{GraphID: 2}
	c := FunctionRef{GraphID: 2, ParentHash: 9}

	lattice.LawSuite[FunctionRefSet]{
		Name:   "FunctionRefSet",
		Domain: FunctionRefSetDomain,
		Sample: []FunctionRefSet{
			FunctionRefSetDomain.Bottom(),
			FunctionRefSetDomain.Top(),
			FunctionRefSetOf(a),
			FunctionRefSetOf(b),
			FunctionRefSetOf(a, b),
			FunctionRefSetOf(b, c),
		},
		Format: func(s FunctionRefSet) string { return s.Format() },
	}.Run(t)
}

func TestFunctionRefSetJoinMergesCanonicalSortedSets(t *testing.T) {
	a := FunctionRefSetOf(
		FunctionRef{GraphID: 1},
		FunctionRef{GraphID: 3},
	)
	b := FunctionRefSetOf(
		FunctionRef{GraphID: 2},
		FunctionRef{GraphID: 3},
		FunctionRef{GraphID: 4, ParentHash: 9},
	)

	got := FunctionRefSetDomain.Join(a, b)
	want := FunctionRefSetOf(
		FunctionRef{GraphID: 1},
		FunctionRef{GraphID: 2},
		FunctionRef{GraphID: 3},
		FunctionRef{GraphID: 4, ParentHash: 9},
	)
	if !FunctionRefSetDomain.Equal(got, want) {
		t.Fatalf("join = %s, want %s", got.Format(), want.Format())
	}
}

func TestFunctionRefsSubtreeStrongUpdate(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(1), "dep")
	child := root.Field("get")
	samePrintedRoot := constraint.NewPath(cfg.SymbolID(2), "dep")

	refs := WithFunctionRefPath(nil, root, FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRefPath(refs, child, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRefPath(refs, samePrintedRoot, FunctionRefSetOf(FunctionRef{GraphID: 3}))

	rootAddr, ok := StableAddressOfPath(root)
	if !ok {
		t.Fatalf("stable address for path %s", root.Key())
	}
	refs = WithoutFunctionRefSubtreeAddress(refs, rootAddr)
	if _, ok := FunctionRefAtPath(refs, root); ok {
		t.Fatalf("root identity survived subtree clear: %v", refs)
	}
	if _, ok := FunctionRefAtPath(refs, child); ok {
		t.Fatalf("child identity survived subtree clear: %v", refs)
	}
	if got, ok := FunctionRefAtPath(refs, samePrintedRoot); !ok {
		t.Fatalf("same printed root with different symbol was removed")
	} else if ref, singleton := got.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("same printed root identity = %s, want graph 3 singleton", got.Format())
	}
}

func TestFunctionRefsPathAPINormalizesStructuredPath(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(41), "fn").Field("call")
	ref := FunctionRef{GraphID: 41}

	refs := WithFunctionRefPath(nil, path, FunctionRefSetOf(ref))
	set, ok := FunctionRefAtPath(refs, path)
	if !ok {
		t.Fatalf("FunctionRefAtPath missed path ref: %#v", refs)
	}
	got, singleton := set.Singleton()
	if !singleton || got != ref {
		t.Fatalf("path ref = %s, want graph 41 singleton", set.Format())
	}
}

func TestFunctionRefAtAddressDoesNotReadLegacyPathKeyEntry(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(41), "fn").Field("call")
	addr, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatal("stable address did not resolve")
	}
	legacyKey := path.Key()
	if legacyKey == addr.Key() {
		t.Fatalf("test key is already canonical: %s", legacyKey)
	}
	refs := FunctionRefs{
		legacyKey: FunctionRefSetOf(FunctionRef{GraphID: 41}),
	}

	if _, ok := FunctionRefAtAddress(refs, addr); ok {
		t.Fatalf("legacy path key %s was accepted as canonical address storage %s", legacyKey, addr.Key())
	}
}

func TestRebaseFunctionRefsPathMovesSubtree(t *testing.T) {
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(42, "out")
	method := from.Field("method")
	toMethod := to.Field("method")
	refs := WithFunctionRefPath(nil, method, FunctionRefSetOf(FunctionRef{GraphID: 7}))

	rebased := RebaseFunctionRefsPath(refs, from, to)
	set, ok := FunctionRefAtPath(rebased, toMethod)
	if !ok {
		t.Fatalf("rebased function refs missing: %#v", rebased)
	}
	ref, singleton := set.Singleton()
	if !singleton || ref.GraphID != 7 {
		t.Fatalf("rebased function refs = %s, want graph 7 singleton", set.Format())
	}
}

func TestRebaseFunctionRefsTopScopesToTarget(t *testing.T) {
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(42, "out")

	rebased := RebaseFunctionRefsPath(FunctionRefsDomain.Top(), from, to)
	if FunctionRefsDomain.Equal(rebased, FunctionRefsDomain.Top()) {
		t.Fatalf("rebased top leaked to global top")
	}
	set, ok := FunctionRefAtPath(rebased, to)
	if !ok || !set.IsTop() {
		t.Fatalf("target ref = %s/%v, want top/true", set.Format(), ok)
	}
	if _, ok := FunctionRefAtPath(rebased, constraint.NewPath(43, "other")); ok {
		t.Fatalf("rebased top leaked to unrelated path: %#v", rebased)
	}
}

func TestProjectFunctionRefsByPathKeepsSubtree(t *testing.T) {
	root := constraint.NewPath(45, "root")
	child := root.Field("child")
	sibling := constraint.NewPath(46, "sibling")
	refs := WithFunctionRefPath(nil, child, FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRefPath(refs, sibling, FunctionRefSetOf(FunctionRef{GraphID: 2}))

	projected := ProjectFunctionRefsByPath(refs, root)
	if _, ok := FunctionRefAtPath(projected, child); !ok {
		t.Fatalf("projection dropped child path: %#v", projected)
	}
	if _, ok := FunctionRefAtPath(projected, sibling); ok {
		t.Fatalf("projection kept sibling path: %#v", projected)
	}
}

func TestProjectFunctionRefsByPathIgnoresLegacyStoredKey(t *testing.T) {
	root := constraint.NewPath(45, "root")
	child := root.Field("child")
	refs := FunctionRefs{
		child.Key(): FunctionRefSetOf(FunctionRef{GraphID: 11}),
	}

	projected := ProjectFunctionRefsByPath(refs, root)
	if _, ok := FunctionRefAtPath(projected, child); ok {
		t.Fatalf("projection accepted legacy stored key %s: %#v", child.Key(), projected)
	}
}

func TestRebaseFunctionRefsPathIgnoresLegacyStoredKey(t *testing.T) {
	source := constraint.NewPath(46, "source")
	target := constraint.NewPath(47, "target")
	child := source.Field("child")
	refs := FunctionRefs{
		child.Key(): FunctionRefSetOf(FunctionRef{GraphID: 12}),
	}

	rebased := RebaseFunctionRefsPath(refs, source, target)
	if _, ok := FunctionRefAtPath(rebased, target.Field("child")); ok {
		t.Fatalf("rebase accepted legacy stored key %s: %#v", child.Key(), rebased)
	}
}

func TestFunctionRefRootSymbolsUsesAddressRoots(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(51), "root")
	child := root.Field("child")
	other := constraint.NewPath(cfg.SymbolID(52), "other")
	named, ok := StableAddressOfRoot("placeholder", nil)
	if !ok {
		t.Fatal("named root address did not build")
	}
	refs := WithFunctionRefPath(nil, root, FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRefPath(refs, child, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRefPath(refs, other, FunctionRefSetOf(FunctionRef{GraphID: 3}))
	refs = WithFunctionRefAddress(refs, named, FunctionRefSetOf(FunctionRef{GraphID: 4}))

	got := FunctionRefRootSymbols(refs)
	if len(got) != 2 || got[0] != root.Symbol || got[1] != other.Symbol {
		t.Fatalf("root symbols = %v, want [%d %d]", got, root.Symbol, other.Symbol)
	}
}

func TestReplaceFunctionRefSubtreePathClearsAndJoins(t *testing.T) {
	target := constraint.NewPath(43, "target")
	child := target.Field("child")
	sibling := constraint.NewPath(44, "sibling")
	out := PointState{
		FunctionRefs: WithFunctionRefPath(nil, child, FunctionRefSetOf(FunctionRef{GraphID: 1})),
	}
	out.FunctionRefs = WithFunctionRefPath(out.FunctionRefs, sibling, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	incoming := WithFunctionRefPath(nil, target, FunctionRefSetOf(FunctionRef{GraphID: 3}))

	if !ReplaceFunctionRefSubtreePath(&out, target, incoming) {
		t.Fatal("ReplaceFunctionRefSubtreePath reported no change")
	}
	if _, ok := FunctionRefAtPath(out.FunctionRefs, child); ok {
		t.Fatalf("child survived subtree replace: %#v", out.FunctionRefs)
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, target); !ok {
		t.Fatalf("incoming target ref missing: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("target ref = %s, want graph 3 singleton", set.Format())
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, sibling); !ok {
		t.Fatalf("sibling was removed: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 2 {
		t.Fatalf("sibling ref = %s, want graph 2 singleton", set.Format())
	}
}

func TestReplaceFunctionRefTreePathInstallsRootAndNestedEntries(t *testing.T) {
	target := constraint.NewPath(48, "target")
	child := target.Field("child")
	staleGrandchild := child.Field("stale")
	sibling := constraint.NewPath(49, "sibling")
	out := PointState{
		FunctionRefs: WithFunctionRefPath(nil, staleGrandchild, FunctionRefSetOf(FunctionRef{GraphID: 1})),
	}
	out.FunctionRefs = WithFunctionRefPath(out.FunctionRefs, sibling, FunctionRefSetOf(FunctionRef{GraphID: 2}))

	tree := FunctionRefTree{
		Root:    FunctionRefSetOf(FunctionRef{GraphID: 3}),
		HasRoot: true,
		Entries: []FunctionRefTreeEntry{{
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "child"}},
			Set:      FunctionRefSetOf(FunctionRef{GraphID: 4}),
		}},
	}

	if !ReplaceFunctionRefTreePath(&out, target, tree) {
		t.Fatal("ReplaceFunctionRefTreePath reported no change")
	}
	if _, ok := FunctionRefAtPath(out.FunctionRefs, staleGrandchild); ok {
		t.Fatalf("stale target subtree survived tree replace: %#v", out.FunctionRefs)
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, target); !ok {
		t.Fatalf("root ref missing: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("root ref = %s, want graph 3 singleton", set.Format())
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, child); !ok {
		t.Fatalf("nested ref missing: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 4 {
		t.Fatalf("nested ref = %s, want graph 4 singleton", set.Format())
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, sibling); !ok {
		t.Fatalf("sibling was removed: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 2 {
		t.Fatalf("sibling ref = %s, want graph 2 singleton", set.Format())
	}
}

func TestReplaceFunctionRefTreePathCanonicalizesEntries(t *testing.T) {
	target := constraint.NewPath(52, "target")
	rootRef := FunctionRef{GraphID: 1}
	rootEntryRef := FunctionRef{GraphID: 2}
	childA := target.Field("a")
	childB := target.Field("b")
	refA := FunctionRef{GraphID: 3}
	refADuplicate := FunctionRef{GraphID: 4}
	refB := FunctionRef{GraphID: 5}
	out := PointState{}

	tree := FunctionRefTree{
		Root:    FunctionRefSetOf(rootRef),
		HasRoot: true,
		Entries: []FunctionRefTreeEntry{
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}}, Set: FunctionRefSetOf(refB)},
			{Segments: nil, Set: FunctionRefSetOf(rootEntryRef)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}}, Set: FunctionRefSetOf(refA)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}}, Set: FunctionRefSetOf(refADuplicate)},
			{Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "dropped"}}, Set: FunctionRefSetDomain.Bottom()},
		},
	}

	if !ReplaceFunctionRefTreePath(&out, target, tree) {
		t.Fatal("ReplaceFunctionRefTreePath reported no change")
	}
	if got, ok := FunctionRefAtPath(out.FunctionRefs, target); !ok {
		t.Fatalf("root refs missing: %#v", out.FunctionRefs)
	} else if want := FunctionRefSetOf(rootRef, rootEntryRef); !FunctionRefSetDomain.Equal(got, want) {
		t.Fatalf("root refs = %s, want %s", got.Format(), want.Format())
	}
	if got, ok := FunctionRefAtPath(out.FunctionRefs, childA); !ok {
		t.Fatalf("merged child refs missing: %#v", out.FunctionRefs)
	} else if want := FunctionRefSetOf(refA, refADuplicate); !FunctionRefSetDomain.Equal(got, want) {
		t.Fatalf("child a refs = %s, want %s", got.Format(), want.Format())
	}
	if got, ok := FunctionRefAtPath(out.FunctionRefs, childB); !ok {
		t.Fatalf("child b refs missing: %#v", out.FunctionRefs)
	} else if want := FunctionRefSetOf(refB); !FunctionRefSetDomain.Equal(got, want) {
		t.Fatalf("child b refs = %s, want %s", got.Format(), want.Format())
	}
	if _, ok := FunctionRefAtPath(out.FunctionRefs, target.Field("dropped")); ok {
		t.Fatalf("bottom tree entry was installed: %#v", out.FunctionRefs)
	}
}

func TestJoinFunctionRefTreePathPublishesWithoutClearingTarget(t *testing.T) {
	target := constraint.NewPath(50, "target")
	existingChild := target.Field("existing")
	publishedChild := target.Field("published")
	out := PointState{
		FunctionRefs: WithFunctionRefPath(nil, existingChild, FunctionRefSetOf(FunctionRef{GraphID: 1})),
	}
	tree := FunctionRefTree{
		Entries: []FunctionRefTreeEntry{{
			Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "published"}},
			Set:      FunctionRefSetOf(FunctionRef{GraphID: 2}),
		}},
	}

	if !JoinFunctionRefTreePath(&out, target, tree) {
		t.Fatal("JoinFunctionRefTreePath reported no change")
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, existingChild); !ok {
		t.Fatalf("existing child was cleared: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 1 {
		t.Fatalf("existing child ref = %s, want graph 1 singleton", set.Format())
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, publishedChild); !ok {
		t.Fatalf("published child missing: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 2 {
		t.Fatalf("published child ref = %s, want graph 2 singleton", set.Format())
	}
}

func TestFunctionRefTreeFromSubtreePathProjectsRelativeEntries(t *testing.T) {
	source := constraint.NewPlaceholder(0)
	child := source.Field("child")
	other := constraint.NewPlaceholder(1).Field("child")
	refs := WithFunctionRefPath(nil, source, FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRefPath(refs, child, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRefPath(refs, other, FunctionRefSetOf(FunctionRef{GraphID: 3}))

	tree, ok := FunctionRefTreeFromSubtreePath(refs, source)
	if !ok {
		t.Fatal("FunctionRefTreeFromSubtreePath returned no tree")
	}
	if !tree.HasRoot {
		t.Fatalf("tree root missing: %#v", tree)
	}
	if ref, singleton := tree.Root.Singleton(); !singleton || ref.GraphID != 1 {
		t.Fatalf("tree root = %s, want graph 1 singleton", tree.Root.Format())
	}
	if len(tree.Entries) != 1 {
		t.Fatalf("tree entries = %#v, want one child entry", tree.Entries)
	}
	entry := tree.Entries[0]
	if len(entry.Segments) != 1 || entry.Segments[0].Kind != constraint.SegmentField || entry.Segments[0].Name != "child" {
		t.Fatalf("tree entry segments = %#v, want relative .child", entry.Segments)
	}
	if ref, singleton := entry.Set.Singleton(); !singleton || ref.GraphID != 2 {
		t.Fatalf("tree child set = %s, want graph 2 singleton", entry.Set.Format())
	}
}

func TestAssignFunctionRefSubtreePathCopiesAndReplacesTarget(t *testing.T) {
	source := constraint.NewPath(45, "source")
	sourceChild := source.Field("child")
	target := constraint.NewPath(46, "target")
	targetChild := target.Field("child")
	staleTargetGrandchild := targetChild.Field("stale")
	sibling := constraint.NewPath(47, "sibling")
	out := PointState{
		FunctionRefs: WithFunctionRefPath(nil, sourceChild, FunctionRefSetOf(FunctionRef{GraphID: 1})),
	}
	out.FunctionRefs = WithFunctionRefPath(out.FunctionRefs, staleTargetGrandchild, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	out.FunctionRefs = WithFunctionRefPath(out.FunctionRefs, sibling, FunctionRefSetOf(FunctionRef{GraphID: 3}))

	if !AssignFunctionRefSubtreePath(&out, source, target) {
		t.Fatal("AssignFunctionRefSubtreePath reported no change")
	}
	if _, ok := FunctionRefAtPath(out.FunctionRefs, staleTargetGrandchild); ok {
		t.Fatalf("stale target subtree survived assign: %#v", out.FunctionRefs)
	}
	set, ok := FunctionRefAtPath(out.FunctionRefs, targetChild)
	if !ok {
		t.Fatalf("rebased source child missing: %#v", out.FunctionRefs)
	}
	if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 1 {
		t.Fatalf("target child ref = %s, want graph 1 singleton", set.Format())
	}
	if set, ok := FunctionRefAtPath(out.FunctionRefs, sibling); !ok {
		t.Fatalf("sibling was removed: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("sibling ref = %s, want graph 3 singleton", set.Format())
	}
}
