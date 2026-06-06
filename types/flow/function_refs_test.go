package flow

import (
	"testing"

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
	root := constraint.PathKey("sym1.dep")
	child := constraint.PathKey("sym1.dep.get")
	sibling := constraint.PathKey("sym1.run")

	refs := WithFunctionRef(nil, root, FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRef(refs, child, FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRef(refs, sibling, FunctionRefSetOf(FunctionRef{GraphID: 3}))

	refs = WithoutFunctionRefSubtree(refs, root)
	if _, ok := FunctionRefAt(refs, root); ok {
		t.Fatalf("root identity survived subtree clear: %v", refs)
	}
	if _, ok := FunctionRefAt(refs, child); ok {
		t.Fatalf("child identity survived subtree clear: %v", refs)
	}
	if got, ok := FunctionRefAt(refs, sibling); !ok {
		t.Fatalf("sibling identity was removed")
	} else if ref, singleton := got.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("sibling identity = %s, want graph 3 singleton", got.Format())
	}
}

func TestRebaseFunctionRefsPathMovesSubtree(t *testing.T) {
	from := constraint.NewPlaceholder(0)
	to := constraint.NewPath(42, "out")
	method := from.Field("method")
	toMethod := to.Field("method")
	refs := WithFunctionRef(nil, StablePathKey(method), FunctionRefSetOf(FunctionRef{GraphID: 7}))

	rebased := RebaseFunctionRefsPath(refs, from, to)
	set, ok := FunctionRefAt(rebased, StablePathKey(toMethod))
	if !ok {
		t.Fatalf("rebased function refs missing: %#v", rebased)
	}
	ref, singleton := set.Singleton()
	if !singleton || ref.GraphID != 7 {
		t.Fatalf("rebased function refs = %s, want graph 7 singleton", set.Format())
	}
}

func TestReplaceFunctionRefSubtreePathClearsAndJoins(t *testing.T) {
	target := constraint.NewPath(43, "target")
	child := target.Field("child")
	sibling := constraint.NewPath(44, "sibling")
	out := PointState{
		FunctionRefs: WithFunctionRef(nil, StablePathKey(child), FunctionRefSetOf(FunctionRef{GraphID: 1})),
	}
	out.FunctionRefs = WithFunctionRef(out.FunctionRefs, StablePathKey(sibling), FunctionRefSetOf(FunctionRef{GraphID: 2}))
	incoming := WithFunctionRef(nil, StablePathKey(target), FunctionRefSetOf(FunctionRef{GraphID: 3}))

	if !ReplaceFunctionRefSubtreePath(&out, target, incoming) {
		t.Fatal("ReplaceFunctionRefSubtreePath reported no change")
	}
	if _, ok := FunctionRefAt(out.FunctionRefs, StablePathKey(child)); ok {
		t.Fatalf("child survived subtree replace: %#v", out.FunctionRefs)
	}
	if set, ok := FunctionRefAt(out.FunctionRefs, StablePathKey(target)); !ok {
		t.Fatalf("incoming target ref missing: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 3 {
		t.Fatalf("target ref = %s, want graph 3 singleton", set.Format())
	}
	if set, ok := FunctionRefAt(out.FunctionRefs, StablePathKey(sibling)); !ok {
		t.Fatalf("sibling was removed: %#v", out.FunctionRefs)
	} else if ref, singleton := set.Singleton(); !singleton || ref.GraphID != 2 {
		t.Fatalf("sibling ref = %s, want graph 2 singleton", set.Format())
	}
}
