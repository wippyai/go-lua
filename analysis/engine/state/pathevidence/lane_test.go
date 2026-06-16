package pathevidence

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
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

	rootKey := mustLocalKey(t, root)
	childKey := mustLocalKey(t, child)
	otherKey := mustLocalKey(t, other)

	l, _ := (Lane{}).WritePathKey(reg, rootKey, present)
	l, _ = l.WritePathKey(reg, childKey, present)
	l, _ = l.WritePathKey(reg, otherKey, present)
	l, _ = l.WritePathStaticMember(childKey, present)
	l, _ = l.AddBranchProof(proof)

	out, ok := l.InvalidatePathKeySubtree(root)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected structural path key")
	}
	if got := out.ReadPathKey(reg, rootKey); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("root refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, childKey); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("child refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, otherKey); !valueDomain.Equal(got, present) {
		t.Fatalf("other refinement = %#v, want present", got)
	}
	if got, ok := out.ReadPathStaticMember(childKey); ok {
		t.Fatalf("static member = %#v, want removed", got)
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with invalidated path survived")
	}
}

func mustLocalKey(t *testing.T, key pathdom.PathKey) pathaddr.LocalKey {
	t.Helper()
	local, ok := pathaddr.LocalKeyFromPathKey(key)
	if !ok {
		t.Fatalf("LocalKeyFromPathKey(%q) failed", key)
	}
	return local
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

func TestRebaseEquivalentPathKeyStaysUnderTargetPrefix(t *testing.T) {
	from := pathdom.PathKey("sym10@1.child")
	to := pathdom.PathKey("sym20@1.leaf")

	got, ok := rebaseEquivalentPathKey(pathdom.PathKey("sym10@1.child.name"), from, to)
	if !ok || got != pathdom.PathKey("sym20@1.leaf.name") {
		t.Fatalf("rebaseEquivalentPathKey = %s/%v, want sym20@1.leaf.name/true", got, ok)
	}
	if !pathKeyInSubtree(got, to) {
		t.Fatalf("rebased key %s escaped target prefix %s", got, to)
	}
	if got, ok := rebaseEquivalentPathKey(pathdom.PathKey("sym10@1.childish.name"), from, to); ok || got != "" {
		t.Fatalf("boundary-colliding rebase = %s/%v, want rejected", got, ok)
	}
	if got, ok := rebaseEquivalentPathKey(pathdom.PathKey("sym10@1.child.name"), from, pathdom.PathKey("s20.leaf")); ok || got != "" {
		t.Fatalf("mixed local/stable rebase = %s/%v, want rejected", got, ok)
	}
}

func TestRefinementMustJoinDropsOneSidedEntry(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := product.Top()
	oneSided := Lane{
		refinements: map[pathaddr.LocalKey]product.Value{
			mustLocalKey(t, pathdom.PathKey("sym1@1.field")): present,
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
