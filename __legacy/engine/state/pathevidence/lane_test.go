package pathevidence

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestEditLaneMatchesRepeatedPathEvidenceWrites(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	valueDomain := product.Domain(reg)
	pathKey := mustStructKey(t, ks, pathdom.PathKey("sym9@1.field"))
	staticKey := mustStructKey(t, ks, pathdom.PathKey("sym9@1.method"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)

	base, _ := (Lane{}).WritePathKey(reg, pathKey, present)
	base, _ = base.WritePathStaticMember(staticKey, present)
	edit := EditLane(reg, base)
	edit.WritePathKey(pathKey, absent)
	edit.WritePathStaticMember(staticKey, absent)
	got, changed := edit.Done()
	if !changed {
		t.Fatalf("EditLane reported unchanged")
	}

	want, _ := base.WritePathKey(reg, pathKey, absent)
	want, _ = want.WritePathStaticMember(staticKey, absent)
	if value := got.ReadPathKey(reg, pathKey); !valueDomain.Equal(value, absent) {
		t.Fatalf("edited path key = %#v, want absent", value)
	}
	if value := base.ReadPathKey(reg, pathKey); !valueDomain.Equal(value, present) {
		t.Fatalf("EditLane mutated original path key = %#v", value)
	}
	if gotMember, ok := got.ReadPathStaticMember(staticKey); !ok || gotMember != absent {
		t.Fatalf("edited static member = %#v/%v, want absent", gotMember, ok)
	}
	if wantMember, ok := want.ReadPathStaticMember(staticKey); !ok || wantMember != absent {
		t.Fatalf("repeated static member = %#v/%v, want absent", wantMember, ok)
	}
}

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

func TestInvalidateSubtreePrefixesMatchesComputedInvalidation(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	root := pathdom.PathKey("sym1@1.table")
	child := pathdom.PathKey("sym1@1.table.field")
	alias := pathdom.PathKey("sym2@1.alias.field")
	present := product.Top()
	proof := BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, child),
		Other: mustStateKey(t, ks, alias),
	}

	l, _ := (Lane{}).WritePathKey(reg, mustStructKey(t, ks, root), present)
	l, _ = l.WritePathKey(reg, mustStructKey(t, ks, child), present)
	l, _ = l.WritePathKey(reg, mustStructKey(t, ks, alias), present)
	l, _ = l.AddBranchProof(proof)

	prefixes, ok := l.PathKeySubtreeInvalidationPrefixes(ks, root)
	if !ok {
		t.Fatal("PathKeySubtreeInvalidationPrefixes rejected root")
	}
	fromPlan := l.InvalidatePathKeySubtreePrefixes(ks, prefixes)
	fromRoot, ok := l.InvalidatePathKeySubtree(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected root")
	}
	if !Domain(reg).Equal(fromPlan, fromRoot) {
		t.Fatalf("plan invalidation diverged from root invalidation")
	}
}

func TestInvalidateSubtreePrefixesRetainsTypedFormalRootIdentity(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	root, ok := ks.InternFormalRoot(formal.NewRoot(owner, 1, formal.Middle))
	if !ok {
		t.Fatal("formal root")
	}
	child, ok := ks.AppendSegment(root, segment.Segment{Kind: segment.SegmentField, Name: "field"})
	if !ok {
		t.Fatal("formal child")
	}
	lane, _ := (Lane{}).WritePathKey(reg, child, product.Top())
	prefixes, ok := lane.PathKeySubtreeInvalidationPrefixes(ks, ks.FormatReadOnly(root))
	if !ok {
		t.Fatal("formal subtree prefixes")
	}
	next, changed := lane.InvalidatePathKeySubtreePrefixesChanged(ks, prefixes)
	if !changed || !product.Equal(reg, next.ReadPathKey(reg, child), product.Bottom(reg)) {
		t.Fatal("typed formal subtree survived its canonical invalidation plan")
	}
}

func TestInvalidateSubtreeRemovesStableStaticMemberCounterpart(t *testing.T) {
	ks := keyspace.New()
	present := product.Top()
	local := mustStructKey(t, ks, "sym1@3.process")
	stable, ok := ks.FromStableSymbol(1, ks.Segments(local))
	if !ok {
		t.Fatal("stable key failed")
	}
	l, _ := (Lane{}).WritePathStaticMember(stable, present)

	out, ok := l.InvalidatePathKeySubtree(ks, "sym1@3.process")
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected local path")
	}
	if got, ok := out.ReadPathStaticMember(stable); ok {
		t.Fatalf("stable static member survived invalidation: %#v", got)
	}
}

func TestInvalidateDescendantPrefixesMatchesComputedInvalidation(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	root := pathdom.PathKey("sym3@1.table")
	child := pathdom.PathKey("sym3@1.table.field")
	aliasChild := pathdom.PathKey("sym4@1.alias.field")
	present := product.Top()
	proof := BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, child),
		Other: mustStateKey(t, ks, aliasChild),
	}

	l, _ := (Lane{}).WritePathKey(reg, mustStructKey(t, ks, root), present)
	l, _ = l.WritePathKey(reg, mustStructKey(t, ks, child), present)
	l, _ = l.WritePathKey(reg, mustStructKey(t, ks, aliasChild), present)
	l, _ = l.AddBranchProof(proof)

	prefixes, ok := l.PathKeyDescendantInvalidationPrefixes(ks, root)
	if !ok {
		t.Fatal("PathKeyDescendantInvalidationPrefixes rejected root")
	}
	fromPlan := l.InvalidatePathKeyDescendantPrefixes(ks, prefixes)
	fromRoot, ok := l.InvalidatePathKeyDescendants(ks, root)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected root")
	}
	if !Domain(reg).Equal(fromPlan, fromRoot) {
		t.Fatalf("descendant plan invalidation diverged from root invalidation")
	}
}

func TestRekeyValueLanesKeepsStableSymbolStaticMembers(t *testing.T) {
	from := keyspace.New()
	to := keyspace.New()
	present := product.Top()
	local := mustStructKey(t, from, "sym7@1.run")
	stable, ok := from.FromStableSymbol(7, from.Segments(local))
	if !ok {
		t.Fatal("stable key failed")
	}
	l, _ := (Lane{}).WritePathStaticMember(stable, present)

	out, ok := l.RekeyValueLanes(from, to)
	if !ok {
		t.Fatal("RekeyValueLanes failed")
	}
	toStable, ok := to.FromStableSymbol(7, to.Segments(mustStructKey(t, to, "sym7@1.run")))
	if !ok {
		t.Fatal("target stable key failed")
	}
	if got, ok := out.ReadPathStaticMember(toStable); !ok || got != present {
		t.Fatalf("rekeyed stable member = %#v ok=%v, want present", got, ok)
	}
}

func TestRekeyValueLanesImportsEveryKeyKindAcrossOppositeInternOrder(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	field := []segment.Segment{{Kind: segment.SegmentField, Name: "leaf"}}

	resolver, ok := from.FromResolverKey(11, 3, field)
	if !ok {
		t.Fatal("resolver key failed")
	}
	unversioned, ok := from.FromResolverKey(12, 0, field)
	if !ok {
		t.Fatal("unversioned key failed")
	}
	stable, ok := from.FromStableSymbol(13, field)
	if !ok {
		t.Fatal("stable key failed")
	}
	placeholder := from.FromPath(pathdom.NewPlaceholder(2).Field("leaf"))
	retSlot := from.FromPath(pathdom.Path{Root: "ret[4]", Segments: field})
	named := from.FromPath(pathdom.Path{Root: "global", Segments: field})
	namedStable, ok := pathaddr.StableOfPath(pathdom.Path{Root: "sym99", Segments: field})
	if !ok {
		t.Fatal("canonical named stable address failed")
	}
	namedCanonical, ok := from.FromStateKey(namedStable.Key())
	if !ok || !namedCanonical.Canon {
		t.Fatal("canonical named key failed")
	}
	rootless, ok := from.FromRootlessSuffix(field)
	if !ok {
		t.Fatal("rootless key failed")
	}
	keys := []keyspace.Key{resolver, unversioned, stable, placeholder, retSlot, named, namedCanonical, rootless}

	// Populate the target in the opposite structural order with unrelated roots
	// and suffixes. Import must never copy either dense intern id.
	for i := len(keys) - 1; i >= 0; i-- {
		padding := []segment.Segment{{Kind: segment.SegmentField, Name: "padding"}, {Kind: segment.SegmentIndexInt, Index: i + 1}}
		if _, ok := to.FromRootlessSuffix(padding); !ok {
			t.Fatalf("target padding %d failed", i)
		}
		_ = to.FromPath(pathdom.Path{Root: "padding-root", Segments: padding})
	}

	present := product.Top()
	lane := Lane{}
	for index, key := range keys {
		lane, _ = lane.WritePathKey(reg, key, present)
		lane, _ = lane.WritePathStaticMember(key, present)
		lane, _ = lane.AddBranchProof(BranchProof{Kind: BranchProofPathPresence, Path: key, Presence: presence.Present()})
		target := keys[(index+1)%len(keys)]
		lane, _ = lane.AddPathPresenceImplication(NewPathPresenceImplication(key, presence.Present(), target, presence.Present()))
	}
	lane, _ = lane.AddBranchProof(BranchProof{Kind: BranchProofPathEqual, Path: named, Other: unversioned})
	rekeyed, ok := lane.RekeyValueLanes(from, to)
	if !ok {
		t.Fatal("RekeyValueLanes failed")
	}
	if len(rekeyed.refinements) != len(keys) || len(rekeyed.staticMembers) != len(keys) {
		t.Fatalf("rekey dropped key kinds: refinements=%d static=%d want=%d", len(rekeyed.refinements), len(rekeyed.staticMembers), len(keys))
	}
	if len(rekeyed.proofs) != len(keys)+1 || len(rekeyed.pathPresenceImplications) != len(keys) {
		t.Fatalf("rekey dropped proof keys: proofs=%d implications=%d want=%d/%d", len(rekeyed.proofs), len(rekeyed.pathPresenceImplications), len(keys)+1, len(keys))
	}

	want := make(map[string]keyspace.Key, len(keys))
	for _, key := range keys {
		want[string(from.FormatReadOnly(key))] = key
	}
	for key := range rekeyed.refinements {
		spelling := string(to.FormatReadOnly(key))
		source, ok := want[spelling]
		if !ok {
			t.Fatalf("rekey produced unexpected key %s (%+v)", spelling, key)
		}
		if key.Kind != source.Kind || key.Sym != source.Sym || key.Ver != source.Ver || key.Canon != source.Canon {
			t.Fatalf("rekey changed structural namespace for %s: source=%+v target=%+v", spelling, source, key)
		}
		if got, ok := rekeyed.ReadPathStaticMember(key); !ok || got != present {
			t.Fatalf("static member missing for rekeyed %s: value=%#v ok=%v", spelling, got, ok)
		}
	}
	for proof := range rekeyed.proofs {
		if to.FormatReadOnly(proof.Path) == "" || (proof.Other.Kind != keyspace.KindInvalid && to.FormatReadOnly(proof.Other) == "") {
			t.Fatalf("proof retained foreign key after rekey: %+v", proof)
		}
	}
	for implication := range rekeyed.pathPresenceImplications {
		if to.FormatReadOnly(implication.Trigger) == "" || to.FormatReadOnly(implication.Target) == "" ||
			(implication.TriggerOther.Kind != keyspace.KindInvalid && to.FormatReadOnly(implication.TriggerOther) == "") {
			t.Fatalf("implication retained foreign key after rekey: %+v", implication)
		}
	}
}

func TestBranchProofsSnapshotOrdersByFormattedKeys(t *testing.T) {
	ks := keyspace.New()
	proofs := []BranchProof{
		{
			Kind:  BranchProofPathEqual,
			Path:  mustStateKey(t, ks, pathdom.PathKey("sym2@1.z")),
			Other: mustStateKey(t, ks, pathdom.PathKey("sym2@1.a")),
		},
		{
			Kind:     BranchProofPathPresence,
			Path:     mustStateKey(t, ks, pathdom.PathKey("sym10@1")),
			Presence: presence.Present(),
		},
		{
			Kind:     BranchProofPathPresence,
			Path:     mustStateKey(t, ks, pathdom.PathKey("sym2@1.a")),
			Presence: presence.Maybe(),
		},
	}
	l := Lane{}
	for _, proof := range proofs {
		var changed bool
		l, changed = l.AddBranchProof(proof)
		if !changed {
			t.Fatalf("AddBranchProof(%#v) reported unchanged", proof)
		}
	}

	got := l.BranchProofsSnapshot(ks).Proofs
	want := []BranchProof{proofs[1], proofs[2], proofs[0]}
	if len(got) != len(want) {
		t.Fatalf("snapshot proof count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("proof[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestBranchProofsSnapshotKeepsLexicalPresenceTieBreak(t *testing.T) {
	ks := keyspace.New()
	pathKey := mustStateKey(t, ks, pathdom.PathKey("sym12@1"))
	present := BranchProof{Kind: BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	absent := BranchProof{Kind: BranchProofPathPresence, Path: pathKey, Presence: presence.Absent()}
	l := Lane{}
	var changed bool
	l, changed = l.AddBranchProof(present)
	if !changed {
		t.Fatal("adding present proof reported unchanged")
	}
	l, changed = l.AddBranchProof(absent)
	if !changed {
		t.Fatal("adding absent proof reported unchanged")
	}
	got := l.BranchProofsSnapshot(ks).Proofs
	if len(got) != 2 || got[0] != absent || got[1] != present {
		t.Fatalf("presence tie-break order = %#v, want [%#v %#v]", got, absent, present)
	}
}

func TestBranchProofPresenceReturnsOnlyUnambiguousProof(t *testing.T) {
	ks := keyspace.New()
	pathKey := mustStateKey(t, ks, pathdom.PathKey("sym10@1.value"))
	otherKey := mustStateKey(t, ks, pathdom.PathKey("sym11@1.value"))
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:     BranchProofPathPresence,
		Path:     pathKey,
		Presence: presence.Present(),
	})
	if got, ok := l.BranchProofPresence(pathKey); !ok || !presence.Equal(got, presence.Present()) {
		t.Fatalf("BranchProofPresence = %v/%v, want present", got, ok)
	}
	if got, ok := l.BranchProofPresence(otherKey); ok || !presence.Equal(got, presence.Bottom()) {
		t.Fatalf("unproven BranchProofPresence = %v/%v, want bottom/false", got, ok)
	}
	l, _ = l.AddBranchProof(BranchProof{
		Kind:     BranchProofPathPresence,
		Path:     pathKey,
		Presence: presence.Absent(),
	})
	if got, ok := l.BranchProofPresence(pathKey); ok || !presence.Equal(got, presence.Bottom()) {
		t.Fatalf("conflicting BranchProofPresence = %v/%v, want bottom/false", got, ok)
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
	// Enumeration is over finite terms already tracked by the lane. These
	// presence facts make the two descendant representatives explicit; the
	// equality proof then relates them by exact constructor congruence.
	l, _ = l.AddBranchProofs([]BranchProof{
		{Kind: BranchProofPathPresence, Path: mustStateKey(t, ks, "sym20@1.child.name"), Presence: presence.Present()},
		{Kind: BranchProofPathPresence, Path: mustStateKey(t, ks, "sym30@1.leaf.name"), Presence: presence.Present()},
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
	start := mustStateKey(t, ks, "sym10@1.child.name")
	target := mustStateKey(t, ks, "sym30@1.leaf.name")
	if !l.HasEquivalentKeyspaceKey(ks, start, target) {
		t.Fatalf("HasEquivalentKeyspaceKey(%s, %s) = false, want true", ks.Format(start), ks.Format(target))
	}
	if l.HasEquivalentKeyspaceKey(ks, start, start) {
		t.Fatalf("HasEquivalentKeyspaceKey(%s, itself) = true, want false", ks.Format(start))
	}
	if l.HasEquivalentKeyspaceKey(ks, start, mustStateKey(t, ks, "sym99@1.child.name")) {
		t.Fatal("HasEquivalentKeyspaceKey reported an unrelated key")
	}
}

func TestEquivalentKeyspaceKeysNormalizesLargeObservationSetAgainstEqualityEndpoints(t *testing.T) {
	ks := keyspace.New()
	left := mustStateKey(t, ks, "sym10@1")
	right := mustStateKey(t, ks, "sym20@1")
	rightChild := mustStateKey(t, ks, "sym20@1.child")
	proofs := make([]BranchProof, 0, 1026)
	proofs = append(proofs,
		BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right},
		BranchProof{Kind: BranchProofPathPresence, Path: rightChild, Presence: presence.Present()},
	)
	// These are tracked observations, not equations. Their prefixes must not be
	// inserted into the equality endpoint DAG or change its quotient.
	for index := 0; index < 1024; index++ {
		proofs = append(proofs, BranchProof{
			Kind:     BranchProofPathPresence,
			Path:     mustStateKey(t, ks, pathdom.PathKey(fmt.Sprintf("sym%d@1.payload.field", 1000+index))),
			Presence: presence.Present(),
		})
	}
	lane, changed := (Lane{}).AddBranchProofs(proofs)
	if !changed {
		t.Fatal("AddBranchProofs reported unchanged")
	}
	start := mustStateKey(t, ks, "sym10@1.child")
	got := lane.EquivalentKeyspaceKeys(ks, start)
	if len(got) != 1 || got[0] != rightChild {
		t.Fatalf("EquivalentKeyspaceKeys = %#v, want [%s]", got, ks.Format(rightChild))
	}
	if lane.HasEquivalentKeyspaceKey(ks, start, mustStateKey(t, ks, "sym1000@1.payload.field")) {
		t.Fatal("unrelated tracked observation entered the equality quotient")
	}
}

func TestEqualityProofMayRebaseFromRequiresSharedStructuralRoot(t *testing.T) {
	ks := keyspace.New()
	local := mustStateKey(t, ks, "sym10@1.child")
	if !equalityProofMayRebaseFrom(local, mustStateKey(t, ks, "sym10@1.parent")) {
		t.Fatal("same resolver root should pass the rebase prefilter")
	}
	if equalityProofMayRebaseFrom(local, mustStateKey(t, ks, "sym10@2.parent")) {
		t.Fatal("different resolver version should fail the rebase prefilter")
	}
	if equalityProofMayRebaseFrom(local, mustStateKey(t, ks, "sym20@1.parent")) {
		t.Fatal("different resolver symbol should fail the rebase prefilter")
	}
	stable, ok := ks.FromStableSymbol(10, nil)
	if !ok {
		t.Fatal("FromStableSymbol failed")
	}
	if equalityProofMayRebaseFrom(local, stable) {
		t.Fatal("different structural address spaces should fail the rebase prefilter")
	}
}

func TestEquivalentPathKeysDoesNotInventCyclicDescendantTerms(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1.__index"),
		Other: mustStateKey(t, ks, "sym10@1"),
	})

	start := mustStateKey(t, ks, "sym10@1.label")
	target := mustStateKey(t, ks, "sym10@1.__index.label")
	if got := l.EquivalentKeyspaceKeys(ks, start); len(got) != 0 {
		t.Fatalf("EquivalentKeyspaceKeys invented untracked terms: %#v", got)
	}
	if !l.HasEquivalentKeyspaceKey(ks, start, target) {
		t.Fatalf("ground congruence did not prove %s = %s", ks.Format(start), ks.Format(target))
	}
}

func TestEquivalentPathCongruenceClosesRepeatedLengthGrowthWithoutLimit(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1"),
		Other: mustStateKey(t, ks, "sym20@1.child"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym20@1"),
		Other: mustStateKey(t, ks, "sym10@1.child"),
	})

	start := mustStateKey(t, ks, "sym10@1.label")
	// The removed sum-once limit was start depth 1 + two positive deltas = 3.
	// Proving this depth-11 target requires repeatedly closing the same two
	// equations over target subterms; no fixed expansion count can justify it.
	target := mustStateKey(t, ks, "sym10@1.child.child.child.child.child.child.child.child.child.child.label")
	if !l.HasEquivalentKeyspaceKey(ks, start, target) {
		t.Fatalf("ground congruence did not prove repeated length growth to %s", ks.Format(target))
	}
	if got := l.EquivalentKeyspaceKeys(ks, start); len(got) != 0 {
		t.Fatalf("EquivalentKeyspaceKeys invented members of an infinite class: %#v", got)
	}
}

func TestEquivalentPathCongruenceIsProofOrderDeterministic(t *testing.T) {
	ks := keyspace.New()
	proofs := []BranchProof{
		{Kind: BranchProofPathEqual, Path: mustStateKey(t, ks, "sym10@1"), Other: mustStateKey(t, ks, "sym20@1.child")},
		{Kind: BranchProofPathEqual, Path: mustStateKey(t, ks, "sym20@1"), Other: mustStateKey(t, ks, "sym10@1.child")},
		{Kind: BranchProofPathPresence, Path: mustStateKey(t, ks, "sym10@1.child.child.label"), Presence: presence.Present()},
	}
	forward, _ := (Lane{}).AddBranchProofs(proofs)
	reverse, _ := (Lane{}).AddBranchProofs([]BranchProof{proofs[2], proofs[1], proofs[0]})
	start := mustStateKey(t, ks, "sym10@1.label")
	forwardKeys := forward.EquivalentKeyspaceKeys(ks, start)
	reverseKeys := reverse.EquivalentKeyspaceKeys(ks, start)
	if len(forwardKeys) != len(reverseKeys) {
		t.Fatalf("proof order changed result cardinality: %v vs %v", forwardKeys, reverseKeys)
	}
	for index := range forwardKeys {
		if forwardKeys[index] != reverseKeys[index] {
			t.Fatalf("proof order changed result[%d]: %s vs %s", index, ks.Format(forwardKeys[index]), ks.Format(reverseKeys[index]))
		}
	}
}

func TestEquivalentPathCongruenceKeepsFullWidthSymbolRootsDistinct(t *testing.T) {
	ks := keyspace.New()
	left := mustStateKey(t, ks, "sym4294967297@1.value")
	right := mustStateKey(t, ks, "sym2@1.value")
	unrelated := mustStateKey(t, ks, "sym8589934593@1.value")
	lane, _ := (Lane{}).AddBranchProof(BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right})
	if !lane.HasEquivalentKeyspaceKey(ks, left, right) {
		t.Fatal("full-width symbol equality was not closed")
	}
	if lane.HasEquivalentKeyspaceKey(ks, unrelated, right) {
		t.Fatal("symbol roots with equal low 32 bits were conflated")
	}
}

func TestEquivalentRootKeysFollowExactEndpointChainOnly(t *testing.T) {
	ks := keyspace.New()
	l, _ := (Lane{}).AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym10@1"),
		Other: mustStateKey(t, ks, "sym20@1.child"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym20@1.child"),
		Other: mustStateKey(t, ks, "sym30@1"),
	})
	l, _ = l.AddBranchProof(BranchProof{
		Kind:  BranchProofPathEqual,
		Path:  mustStateKey(t, ks, "sym20@1"),
		Other: mustStateKey(t, ks, "sym40@1.deep"),
	})

	got := l.EquivalentRootKeys(ks, pathdom.PathKey("sym10@1"))
	want := []pathdom.PathKey{pathdom.PathKey("sym30@1")}
	if len(got) != len(want) {
		t.Fatalf("EquivalentRootKeys len = %d (%#v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if formatted := ks.Format(got[i]); formatted != want[i] {
			t.Fatalf("EquivalentRootKeys[%d] = %s, want %s", i, formatted, want[i])
		}
	}
}

func TestEquivalentRootKeysDoesNotExpandDescendantAliases(t *testing.T) {
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

	got := l.EquivalentRootKeys(ks, pathdom.PathKey("sym10@1.child.name"))
	if len(got) != 0 {
		t.Fatalf("EquivalentRootKeys expanded descendant aliases: %#v", got)
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

func TestDomainSameUsesAllPersistentSublaneIdentities(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	path := mustStructKey(t, ks, pathdom.PathKey("sym1@1.field"))
	shared, _ := (Lane{}).WritePathKey(reg, path, product.Top())
	domain := Domain(reg)
	if domain.Same == nil || !domain.Same(shared, shared) {
		t.Fatal("shared path-evidence lane lost persistent identity")
	}
	clone := shared.Clone()
	if !domain.Equal(shared, clone) {
		t.Fatal("cloned path-evidence lane changed semantics")
	}
	if domain.Same(shared, clone) {
		t.Fatal("distinct equal path-evidence maps were treated as the same representation")
	}
}
