package pathevidence

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestInvalidateSubtreeOwnsCoupledEvidence(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := product.Top()
	root := pathdom.PathKey("sym1@1.table")
	child := pathdom.PathKey("sym1@1.table.field")
	other := pathdom.PathKey("sym2@1.value")
	proof := BranchProof{Kind: BranchProofPathEqual, Path: other, Other: child}

	l, _ := (Lane{}).WritePathKey(reg, root, present)
	l, _ = l.WritePathKey(reg, child, present)
	l, _ = l.WritePathKey(reg, other, present)
	l, _ = l.WritePathStaticMember(child, present)
	l, _ = l.AddBranchProof(proof)

	out, ok := l.InvalidatePathKeySubtree(root)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected structural path key")
	}
	if got := out.ReadPathKey(reg, root); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("root refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, child); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("child refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, other); !valueDomain.Equal(got, present) {
		t.Fatalf("other refinement = %#v, want present", got)
	}
	if got, ok := out.ReadPathStaticMember(child); ok {
		t.Fatalf("static member = %#v, want removed", got)
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with invalidated path survived")
	}
}

func TestEquivalentPathKeysRebaseThroughBranchProofs(t *testing.T) {
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  pathdom.PathKey("sym10@1"),
		Other: pathdom.PathKey("sym20@1"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  pathdom.PathKey("sym20@1.child"),
		Other: pathdom.PathKey("sym30@1.leaf"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:     BranchProofPathPresence,
		Path:     pathdom.PathKey("sym10@1.child"),
		Presence: presence.Present(),
	})

	got := l.EquivalentPathKeys(pathdom.PathKey("sym10@1.child.name"))
	want := []pathdom.PathKey{
		pathdom.PathKey("sym20@1.child.name"),
		pathdom.PathKey("sym30@1.leaf.name"),
	}
	if len(got) != len(want) {
		t.Fatalf("EquivalentPathKeys len = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EquivalentPathKeys[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestRefinementMustJoinDropsOneSidedEntry(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := product.Top()
	oneSided := Lane{
		refinements: map[pathdom.PathKey]product.Value{
			pathdom.PathKey("sym1@1.field"): present,
		},
	}

	joined := domain.Join(oneSided, Lane{})
	if !domain.Equal(joined, Lane{}) {
		t.Fatalf("must join should drop a refinement absent on the other edge")
	}
	if len(joined.refinements) != 0 {
		t.Fatalf("must join kept one-sided refinement: %d", len(joined.refinements))
	}
}
