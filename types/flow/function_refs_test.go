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
