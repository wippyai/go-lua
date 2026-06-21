package pathevidence

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestInvalidateSubtreeOwnsCoupledEvidence(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	present := product.Top()
	root := pathdom.PathKey("sym1@1.table")
	child := pathdom.PathKey("sym1@1.table.field")
	other := pathdom.PathKey("sym2@1.value")

	rootKey := mustStructKey(t, ks, root)
	childKey := mustStructKey(t, ks, child)
	otherKey := mustStructKey(t, ks, other)
	proof := BranchProof{Kind: BranchProofPathEqual, Path: otherKey, Other: childKey}

	l, _ := (Lane{}).WritePathKey(reg, rootKey, present)
	l, _ = l.WritePathKey(reg, childKey, present)
	l, _ = l.WritePathKey(reg, otherKey, present)
	l, _ = l.WritePathStaticMember(childKey, present)
	l, _ = l.AddBranchProof(proof)

	out, ok := l.InvalidatePathKeySubtree(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected structural path key")
	}
	if got := out.ReadPathKey(reg, rootKey); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("root refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, childKey); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("child refinement = %#v, want bottom", got)
	}
	if got := out.ReadPathKey(reg, otherKey); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("other refinement = %#v, want bottom through alias proof", got)
	}
	if got, ok := out.ReadPathStaticMember(childKey); ok {
		t.Fatalf("static member = %#v, want removed", got)
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with invalidated path survived")
	}
}

func mustStructKey(t *testing.T, ks *keyspace.KeySpace, key pathdom.PathKey) keyspace.Key {
	t.Helper()
	structKey, ok := ks.FromPathKey(key)
	if !ok {
		t.Fatalf("FromPathKey(%q) failed", key)
	}
	return structKey
}

func mustStateKey(t *testing.T, ks *keyspace.KeySpace, key pathdom.PathKey) keyspace.Key {
	t.Helper()
	structKey, ok := ks.FromStateKey(key)
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", key)
	}
	return structKey
}

func TestEquivalentPathKeysRebaseThroughBranchProofs(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1"),
		Other: mustStateKey(t, ks, "sym20@1"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym20@1.child"),
		Other: mustStateKey(t, ks, "sym30@1.leaf"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:     BranchProofPathPresence,
		Path:     mustStateKey(t, ks, "sym10@1.child"),
		Presence: presence.Present(),
	})

	got := l.EquivalentPathKeys(ks, pathdom.PathKey("sym10@1.child.name"))
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

func TestEquivalentPathKeysStopsCyclicDescendantAliasExpansion(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1.__index"),
		Other: mustStateKey(t, ks, "sym10@1"),
	})

	got := l.EquivalentPathKeys(ks, pathdom.PathKey("sym10@1.label"))
	want := []pathdom.PathKey{
		pathdom.PathKey("sym10@1.__index.label"),
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

func TestInvalidationPrefixesStopCyclicDescendantAliasExpansion(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1.__index"),
		Other: mustStateKey(t, ks, "sym10@1"),
	})

	prefixes, ok := l.PathKeyDescendantInvalidationPrefixes(ks, pathdom.PathKey("sym10@1"))
	if !ok {
		t.Fatal("PathKeyDescendantInvalidationPrefixes failed")
	}
	for _, key := range append(append([]pathdom.PathKey{}, prefixes.Descendants...), prefixes.Subtrees...) {
		if key == pathdom.PathKey("sym10@1.__index.__index") {
			t.Fatalf("cyclic alias expansion leaked into prefixes: %#v", prefixes)
		}
	}
	if len(prefixes.Descendants) > 2 || len(prefixes.Subtrees) > 2 {
		t.Fatalf("prefixes = %#v, want finite direct aliases only", prefixes)
	}
}

func TestRebaseEquivalentPathKeyStaysUnderTargetPrefix(t *testing.T) {
	ks := keyspace.New()
	from := mustStateKey(t, ks, "sym10@1.child")
	to := mustStateKey(t, ks, "sym20@1.leaf")

	got, ok := rebaseEquivalentPathKey(ks, mustStateKey(t, ks, "sym10@1.child.name"), from, to)
	if !ok || ks.Format(got) != pathdom.PathKey("sym20@1.leaf.name") {
		t.Fatalf("rebaseEquivalentPathKey = %s/%v, want sym20@1.leaf.name/true", ks.Format(got), ok)
	}
	if !ks.HasPrefix(got, to) {
		t.Fatalf("rebased key %s escaped target prefix %s", ks.Format(got), ks.Format(to))
	}
	if got, ok := rebaseEquivalentPathKey(ks, mustStateKey(t, ks, "sym10@1.childish.name"), from, to); ok || got != (keyspace.Key{}) {
		t.Fatalf("boundary-colliding rebase = %s/%v, want rejected", ks.Format(got), ok)
	}
	stableTo, stableOK := ks.FromStableSymbol(20, nil)
	if !stableOK {
		t.Fatal("FromStableSymbol failed")
	}
	if got, ok := rebaseEquivalentPathKey(ks, mustStateKey(t, ks, "sym10@1.child.name"), from, stableTo); ok || got != (keyspace.Key{}) {
		t.Fatalf("mixed local/stable rebase = %s/%v, want rejected", ks.Format(got), ok)
	}
}

func TestRefinementMustJoinDropsOneSidedEntry(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	domain := Domain(reg)
	present := product.Top()
	oneSided := Lane{
		refinements: map[keyspace.Key]product.Value{
			mustStructKey(t, ks, pathdom.PathKey("sym1@1.field")): present,
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

func TestDomainStableAcrossRepeatedConstruction(t *testing.T) {
	reg := standard.Registry()
	top := Domain(reg).Top()
	bottom := Domain(reg).Bottom()
	domain := Domain(reg)

	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed path-evidence domain did not recognize prior top")
	}
	if !domain.Equal(bottom, domain.Bottom()) {
		t.Fatalf("reconstructed path-evidence domain did not recognize prior bottom")
	}
	if !domain.Equal(domain.Join(bottom, top), top) {
		t.Fatalf("reconstructed path-evidence domain join(bottom, top) did not produce top")
	}
}
