package address

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

func TestStableIgnoresVersionAndExportsCanonicalKey(t *testing.T) {
	a := pathdom.NewPath(42, "node").Field("edges")
	a.Version = 1
	b := pathdom.NewPath(42, "node").Field("edges")
	b.Version = 9

	addrA, ok := StableOfPath(a)
	if !ok {
		t.Fatal("StableOfPath(a) did not resolve")
	}
	addrB, ok := StableOfPath(b)
	if !ok {
		t.Fatal("StableOfPath(b) did not resolve")
	}
	if !addrA.Equal(addrB) {
		t.Fatalf("stable addresses differ across versions: %s vs %s", addrA.Key(), addrB.Key())
	}
	if got, want := addrA.Key(), SymbolPathKey(42, a.Segments); got != want {
		t.Fatalf("stable key = %s, want %s", got, want)
	}
	if !addrA.Equal(addrB) {
		t.Fatalf("stable addresses differ across versions: %s vs %s", addrA.Key(), addrB.Key())
	}
}

func TestLocalPreservesVersionWhileStableIgnoresIt(t *testing.T) {
	a := pathdom.NewPath(7, "x")
	a.Version = 1
	b := pathdom.NewPath(7, "x")
	b.Version = 2

	localA, ok := LocalOfPath(a)
	if !ok {
		t.Fatal("LocalOfPath(a) failed")
	}
	localB, ok := LocalOfPath(b)
	if !ok {
		t.Fatal("LocalOfPath(b) failed")
	}
	if localA.Key() == localB.Key() {
		t.Fatalf("local keys should preserve version: %q", localA.Key())
	}
	if localA.SameVersion(localB) {
		t.Fatal("SameVersion accepted distinct versions")
	}
	stableA, _ := localA.Stable()
	stableB, _ := localB.Stable()
	if !stableA.Equal(stableB) {
		t.Fatalf("stable addresses should ignore versions: %q vs %q", stableA.Key(), stableB.Key())
	}
}

func TestLocalKeyOfPathPreservesPlaceholderAndReturnSlotStructure(t *testing.T) {
	tests := []struct {
		name string
		path pathdom.Path
		want pathdom.PathKey
	}{
		{
			name: "placeholder",
			path: pathdom.NewPlaceholder(0).IndexStr("item"),
			want: pathdom.PathKey("$0[\"item\"]"),
		},
		{
			name: "return slot",
			path: pathdom.Path{Root: "ret[1]"}.Field("ok"),
			want: pathdom.PathKey("ret[1].ok"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LocalKeyOfPath(tt.path)
			if !ok {
				t.Fatal("LocalKeyOfPath rejected structural path")
			}
			if got.PathKey() != tt.want {
				t.Fatalf("LocalKeyOfPath = %q, want %q", got.PathKey(), tt.want)
			}
		})
	}
}

func TestTypedLocalAndStableKeysAreNotInterchangeable(t *testing.T) {
	path := pathdom.NewPath(23, "item").Field("name")
	path.Version = 4

	local, ok := LocalOfPath(path)
	if !ok {
		t.Fatal("LocalOfPath failed")
	}
	stable, ok := StableOfPath(path)
	if !ok {
		t.Fatal("StableOfPath failed")
	}

	localKey := local.LocalKey()
	stableKey := stable.StableKey()
	if localKey == "" || stableKey == "" {
		t.Fatalf("keys should be populated: local=%q stable=%q", localKey, stableKey)
	}
	if localKey.PathKey() == stableKey.PathKey() {
		t.Fatalf("local and stable keys collided: %q", localKey.PathKey())
	}
	if _, ok := StableFromKey(localKey.PathKey()); ok {
		t.Fatalf("StableFromKey accepted local versioned key %q", localKey.PathKey())
	}
	if got, ok := StableFromKey(stableKey.PathKey()); !ok || !got.Equal(stable) {
		t.Fatalf("StableFromKey(stable) = %s/%v, want typed stable round-trip", got.Key(), ok)
	}
}

func TestParseResolverPathAndLocalKeyForVersionRoundTrip(t *testing.T) {
	key := pathdom.PathKey(`sym42@3.field["k"]`)
	sym, version, suffix, ok := ParseResolverPath(key)
	if !ok || sym != 42 || version != 3 || suffix != `.field["k"]` {
		t.Fatalf("ParseResolverPath = %d/%d/%q/%v, want 42/3/.field[\"k\"]/true", sym, version, suffix, ok)
	}
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		t.Fatal("InternFormattedSegments failed")
	}
	local, ok := LocalKeyForVersion(sym, version, segments)
	if !ok || local.PathKey() != key {
		t.Fatalf("LocalKeyForVersion = %q/%v, want %q/true", local.PathKey(), ok, key)
	}
	if _, _, _, ok := ParseResolverPath(pathdom.PathKey("s42.field")); ok {
		t.Fatal("ParseResolverPath accepted stable address spelling")
	}
	if _, _, _, ok := ParseResolverPath(pathdom.PathKey("$0.field")); ok {
		t.Fatal("ParseResolverPath accepted placeholder spelling")
	}
}

func TestStableSeparatesSymbolAndRootIdentity(t *testing.T) {
	symbolAddr, ok := StableOfPath(pathdom.NewPath(7, "x"))
	if !ok {
		t.Fatal("symbol address did not resolve")
	}
	rootAddr, ok := StableOfPath(pathdom.Path{Root: "s7"})
	if !ok {
		t.Fatal("root address did not resolve")
	}
	if symbolAddr.Equal(rootAddr) || symbolAddr.Overlaps(rootAddr) {
		t.Fatalf("symbol/root identities overlapped: %s and %s", symbolAddr.Key(), rootAddr.Key())
	}
	if symbolAddr.Key() == rootAddr.Key() {
		t.Fatalf("symbol/root keys collided: %s", symbolAddr.Key())
	}
}

func TestStablePrefixParentAndRemainderAreStructured(t *testing.T) {
	root := pathdom.NewPath(3, "graph")
	nodes := root.Field("nodes")
	entry := nodes.IndexStr("last")
	edge := root.Field("edges")

	rootAddr, _ := StableOfPath(root)
	nodesAddr, _ := StableOfPath(nodes)
	entryAddr, _ := StableOfPath(entry)
	edgeAddr, _ := StableOfPath(edge)

	if !entryAddr.HasPrefix(nodesAddr) || !entryAddr.Overlaps(nodesAddr) {
		t.Fatalf("%s should be under %s", entryAddr.Key(), nodesAddr.Key())
	}
	if !nodesAddr.HasPrefix(rootAddr) {
		t.Fatalf("%s should be under root %s", nodesAddr.Key(), rootAddr.Key())
	}
	if entryAddr.Overlaps(edgeAddr) {
		t.Fatalf("sibling addresses overlapped: %s and %s", entryAddr.Key(), edgeAddr.Key())
	}
	parent, ok := entryAddr.Parent()
	if !ok || !parent.Equal(nodesAddr) {
		t.Fatalf("Parent() = %s/%v, want %s/true", parent.Key(), ok, nodesAddr.Key())
	}

	remainder, ok := entryAddr.RemainderAfterPrefix(rootAddr)
	if !ok {
		t.Fatal("entry should be under root")
	}
	want := []segment.Segment{
		{Kind: segment.SegmentField, Name: "nodes"},
		{Kind: segment.SegmentIndexString, Name: "last"},
	}
	if !reflect.DeepEqual(remainder, want) {
		t.Fatalf("remainder = %#v, want %#v", remainder, want)
	}
	remainder[0].Name = "mutated"
	again, _ := entryAddr.RemainderAfterPrefix(rootAddr)
	if again[0].Name != "nodes" {
		t.Fatalf("remainder was not defensive: %#v", again)
	}
}

func TestSegmentBoundaryHelpersRespectWholeSegments(t *testing.T) {
	segments := []segment.Segment{
		{Kind: segment.SegmentField, Name: "field"},
		{Kind: segment.SegmentField, Name: "deep"},
	}
	prefix := []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}
	sibling := []segment.Segment{{Kind: segment.SegmentField, Name: "fieldish"}}

	if !SegmentsHasPrefix(segments, prefix) || !SegmentsHasStrictPrefix(segments, prefix) {
		t.Fatalf("segment prefix helpers rejected a real prefix: %#v / %#v", segments, prefix)
	}
	if SegmentsHasPrefix(segments, sibling) || SegmentsHasStrictPrefix(segments, sibling) {
		t.Fatalf("segment prefix helpers accepted a boundary collision: %#v / %#v", segments, sibling)
	}
}

func TestStableFromKeyUsesSegmentBoundaries(t *testing.T) {
	root, _ := StableOfPath(pathdom.NewPath(7, "root"))
	field, _ := StableOfPath(pathdom.NewPath(7, "root").Field("foo"))
	child := SymbolPathKey(7, []segment.Segment{
		{Kind: segment.SegmentField, Name: "foo"},
		{Kind: segment.SegmentField, Name: "bar"},
	})
	siblingPrefixCollision := SymbolPathKey(7, []segment.Segment{
		{Kind: segment.SegmentField, Name: "foobar"},
	})

	if parsed, ok := StableFromKey(child); !ok || !parsed.HasPrefix(root) {
		t.Fatalf("%s should be under root %s", child, root.Key())
	}
	if parsed, ok := StableFromKey(child); !ok || !parsed.HasPrefix(field) {
		t.Fatalf("%s should be under field %s", child, field.Key())
	}
	if parsed, ok := StableFromKey(siblingPrefixCollision); !ok || parsed.HasPrefix(field) {
		t.Fatalf("%s should not be under field %s", siblingPrefixCollision, field.Key())
	}

	stale := pathdom.NewPath(7, "root").Field("foo")
	stale.Version = 1
	if _, ok := StableFromKey(stale.Key()); ok {
		t.Fatalf("StableFromKey accepted stale path key: %s", stale.Key())
	}
}

func TestStableFromKeyRoundTripsSymbolAndNamedRoots(t *testing.T) {
	symRoot, ok := SymbolRoot(11)
	if !ok {
		t.Fatal("SymbolRoot failed")
	}
	symbolAddr, ok := stableOfRootAndSuffix(symRoot, SuffixOfSegments([]segment.Segment{
		{Kind: segment.SegmentIndexString, Name: "node-id"},
		{Kind: segment.SegmentField, Name: "label"},
	}))
	if !ok {
		t.Fatal("symbol address construction failed")
	}
	parsedSymbol, ok := StableFromKey(symbolAddr.Key())
	if !ok || !parsedSymbol.Equal(symbolAddr) {
		t.Fatalf("StableFromKey(symbol) = %s/%v, want %s/true", parsedSymbol.Key(), ok, symbolAddr.Key())
	}

	root, ok := NamedRoot("$0")
	if !ok {
		t.Fatal("NamedRoot failed")
	}
	rootAddr, ok := stableOfRootAndSuffix(root, SuffixOfSegments([]segment.Segment{{Kind: segment.SegmentIndexString, Name: "node-id"}}))
	if !ok {
		t.Fatal("named root construction failed")
	}
	parsedRoot, ok := StableFromKey(rootAddr.Key())
	if !ok || !parsedRoot.Equal(rootAddr) {
		t.Fatalf("StableFromKey(root) = %s/%v, want %s/true", parsedRoot.Key(), ok, rootAddr.Key())
	}

	ambiguousRoot, ok := NamedRoot("s7")
	if !ok {
		t.Fatal("NamedRoot(ambiguous) failed")
	}
	ambiguousAddr, ok := stableOfRootAndSuffix(ambiguousRoot, SuffixOfSegments([]segment.Segment{{Kind: segment.SegmentField, Name: "value"}}))
	if !ok {
		t.Fatal("ambiguous root construction failed")
	}
	parsedAmbiguous, ok := StableFromKey(ambiguousAddr.Key())
	if !ok || !parsedAmbiguous.Equal(ambiguousAddr) {
		t.Fatalf("StableFromKey(ambiguous root) = %s/%v, want %s/true", parsedAmbiguous.Key(), ok, ambiguousAddr.Key())
	}

	retRoot, ok := NamedRoot("ret[1]")
	if !ok {
		t.Fatal("NamedRoot(ret) failed")
	}
	retAddr, ok := stableOfRootAndSuffix(retRoot, SuffixOfSegments([]segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}))
	if !ok {
		t.Fatal("ret root construction failed")
	}
	parsedRet, ok := StableFromKey(retAddr.Key())
	if !ok || !parsedRet.Equal(retAddr) {
		t.Fatalf("StableFromKey(ret) = %s/%v, want %s/true", parsedRet.Key(), ok, retAddr.Key())
	}
}

func TestStableFromKeyRejectsVersionedPathKeys(t *testing.T) {
	path := pathdom.NewPath(12, "node").Field("label")
	path.Version = 7
	staleKey := path.Key()
	if _, ok := StableFromKey(staleKey); ok {
		t.Fatalf("StableFromKey accepted versioned path key %s", staleKey)
	}
	stableKey := stableOfPathKey(t, path)
	if got, ok := StableFromKey(stableKey); !ok || got.Key() != stableKey {
		t.Fatalf("StableFromKey(stable key) = %s/%v, want %s/true", got.Key(), ok, stableKey)
	}
}

func TestStableKeySeparatesAmbiguousRootsAndVersionedLocalKeys(t *testing.T) {
	ambiguousRoot := pathdom.Path{Root: "sym12", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	ambiguousStable := stableOfPathKey(t, ambiguousRoot)
	if ambiguousStable == ambiguousRoot.Key() {
		t.Fatalf("stable key collided with path key %q", ambiguousRoot.Key())
	}
	if parsed, ok := StableFromKey(ambiguousStable); !ok || parsed.Key() != ambiguousStable {
		t.Fatalf("StableFromKey(%q) = %s/%v, want round-trip", ambiguousStable, parsed.Key(), ok)
	}
	if _, ok := StableFromKey(ambiguousRoot.Key()); ok {
		t.Fatalf("StableFromKey accepted ambiguous path key %q", ambiguousRoot.Key())
	}

	versionedLocal := pathdom.NewPath(12, "sym12").Field("field")
	versionedLocal.Version = 3
	if stableOfPathKey(t, versionedLocal) == versionedLocal.Key() {
		t.Fatalf("stable key collided with versioned local key %q", versionedLocal.String())
	}
	if _, ok := StableFromKey(versionedLocal.Key()); ok {
		t.Fatalf("StableFromKey accepted versioned local key %q", versionedLocal.Key())
	}
	if parsed, ok := StableFromKey(stableOfPathKey(t, versionedLocal)); !ok || !parsed.Equal(mustStableOfPath(t, versionedLocal)) {
		t.Fatalf("StableFromKey(stable versioned key) = %s/%v, want round-trip", parsed.Key(), ok)
	}

	placeholder := pathdom.Path{Root: "$0", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	ret := pathdom.Path{Root: "ret[0]", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	for _, path := range []pathdom.Path{placeholder, ret} {
		stable := stableOfPathKey(t, path)
		parsed, ok := StableFromKey(stable)
		if !ok || !parsed.Equal(mustStableOfPath(t, path)) {
			t.Fatalf("StableFromKey(%q) = %s/%v, want round-trip", stable, parsed.Key(), ok)
		}
	}
	if stableOfPathKey(t, placeholder) == stableOfPathKey(t, ret) {
		t.Fatalf("placeholder and return-root stable keys collided: %q", stableOfPathKey(t, placeholder))
	}
}

func mustStableOfPath(t *testing.T, path pathdom.Path) Stable {
	t.Helper()
	got, ok := StableOfPath(path)
	if !ok {
		t.Fatalf("StableOfPath(%q) failed", path.String())
	}
	return got
}

func stableOfPathKey(t *testing.T, path pathdom.Path) pathdom.PathKey {
	t.Helper()
	stable := mustStableOfPath(t, path)
	return stable.Key()
}

func TestStableConstructorsAreDefensive(t *testing.T) {
	segments := []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}
	root, ok := SymbolRoot(13)
	if !ok {
		t.Fatal("SymbolRoot failed")
	}
	addr, ok := stableOfRootAndSuffix(root, SuffixOfSegments(segments))
	if !ok {
		t.Fatal("stable constructor failed")
	}
	segments[0].Name = "mutated"
	if got, want := addr.Key(), SymbolPathKey(13, []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}); got != want {
		t.Fatalf("address observed caller mutation: %s, want %s", got, want)
	}

	returned := addr.Segments()
	returned[0].Name = "mutated"
	if got, want := addr.Key(), SymbolPathKey(13, []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}); got != want {
		t.Fatalf("address observed returned segment mutation: %s, want %s", got, want)
	}
}

func TestRootAndSuffixVocabulary(t *testing.T) {
	symbolRoot, ok := SymbolRoot(9)
	if !ok {
		t.Fatal("SymbolRoot failed")
	}
	namedRoot, ok := NamedRoot("9")
	if !ok {
		t.Fatal("NamedRoot failed")
	}
	if symbolRoot.Equal(namedRoot) {
		t.Fatalf("symbol and named roots should not be equal: %#v %#v", symbolRoot, namedRoot)
	}
	if _, ok := symbolRoot.Name(); ok {
		t.Fatal("symbol root exposed a name")
	}
	if _, ok := namedRoot.Symbol(); ok {
		t.Fatal("named root exposed a symbol")
	}

	suffix := SuffixOfSegments([]segment.Segment{
		{Kind: segment.SegmentField, Name: "nodes"},
		{Kind: segment.SegmentIndexString, Name: "last"},
	})
	parent, ok := suffix.Parent()
	if !ok || parent.KeySuffix() != ".nodes" {
		t.Fatalf("suffix parent = %q/%v, want .nodes/true", parent.KeySuffix(), ok)
	}
	if !suffix.HasPrefix(parent) || !suffix.Overlaps(parent) {
		t.Fatalf("%s should overlap ancestor %s", suffix.KeySuffix(), parent.KeySuffix())
	}
	sibling := SuffixOfSegments([]segment.Segment{{Kind: segment.SegmentField, Name: "edges"}})
	if suffix.Overlaps(sibling) {
		t.Fatalf("%s should not overlap sibling %s", suffix.KeySuffix(), sibling.KeySuffix())
	}
}

func TestContainerRefOwnsSymbolContainerIdentity(t *testing.T) {
	path := pathdom.NewPath(31, "rows").Field("items")
	key, ok := SymbolPathKeyOf(path)
	if !ok {
		t.Fatal("SymbolPathKeyOf failed")
	}
	ref, ok := ContainerRefFromKey(path.Symbol, key)
	if !ok {
		t.Fatal("ContainerRefFromKey failed")
	}
	if !ref.IsValid() {
		t.Fatalf("ContainerRef = %#v, want valid ref", ref)
	}
	if got := ref.Root(); got != path.Symbol {
		t.Fatalf("ContainerRef root = %d, want %d", got, path.Symbol)
	}

	again, ok := ContainerRefOfPath(path)
	if !ok {
		t.Fatal("ContainerRefOfPath failed")
	}
	if !ref.Equal(again) {
		t.Fatalf("ContainerRef equality = %#v/%#v, want equal", ref, again)
	}
	gotStable, ok := ref.Stable()
	if !ok {
		t.Fatal("ContainerRef.Stable failed")
	}
	wantStable, _ := StableOfPath(path)
	if !gotStable.Equal(wantStable) {
		t.Fatalf("ContainerRef.Stable = %s, want %s", gotStable.Key(), wantStable.Key())
	}
}

func TestContainerRefRejectsUnresolvedPaths(t *testing.T) {
	if key, ok := SymbolPathKeyOf(pathdom.Path{Root: "rows"}); ok || key != "" {
		t.Fatalf("SymbolPathKeyOf(unresolved) = %q/%v, want rejected", key, ok)
	}
	ref := ContainerRef{}
	if ref.IsValid() {
		t.Fatalf("zero ContainerRef should be invalid: %#v", ref)
	}
	if ref, ok := ContainerRefFromKey(31, ""); ok || ref.IsValid() {
		t.Fatalf("ContainerRefFromKey(empty) = %#v/%v, want rejected", ref, ok)
	}
	if stable, ok := ref.Stable(); ok || stable.Key() != "" {
		t.Fatalf("zero ContainerRef.Stable = %#v/%v, want rejected", stable, ok)
	}
}

func TestContainerRefConstructorsValidateSymbolKeyPair(t *testing.T) {
	path := pathdom.NewPath(31, "rows").Field("items")
	ref, ok := ContainerRefOfPath(path)
	if !ok {
		t.Fatal("ContainerRefOfPath failed")
	}
	if got, want := ref.Key(), SymbolPathKey(31, path.Segments); got != want {
		t.Fatalf("ContainerRefOfPath key = %s, want %s", got, want)
	}

	if ref, ok := ContainerRefFromKey(32, ref.Key()); ok || ref.IsValid() {
		t.Fatalf("ContainerRefFromKey accepted mismatched root/key: %#v/%v", ref, ok)
	}
	versioned := path
	versioned.Version = 1
	if ref, ok := ContainerRefFromKey(31, versioned.Key()); ok || ref.IsValid() {
		t.Fatalf("ContainerRefFromKey accepted versioned path key: %#v/%v", ref, ok)
	}

	namedRoot, ok := StableOfPath(pathdom.Path{Root: "rows"}.Field("items"))
	if !ok {
		t.Fatal("StableOfPath(named root) failed")
	}
	if ref, ok := ContainerRefOfStable(namedRoot); ok || ref.IsValid() {
		t.Fatalf("ContainerRefOfStable accepted named root: %#v/%v", ref, ok)
	}
}

func TestStructuralKeyVersionedPrefixRelationsUseSegmentBoundaries(t *testing.T) {
	prefix := mustStructuralKey(t, pathdom.PathKey("sym40@3.field"))

	for _, key := range []pathdom.PathKey{
		pathdom.PathKey("sym40@3.field"),
		pathdom.PathKey("sym40@3.field.deep"),
	} {
		if got := mustStructuralKey(t, key); !got.HasPrefix(prefix) {
			t.Fatalf("%s should be under %s", key, prefix.PathKey())
		}
	}

	for _, key := range []pathdom.PathKey{
		pathdom.PathKey("sym40@3"),
		pathdom.PathKey("sym40@3.fieldish"),
		pathdom.PathKey("sym40.field.deep"),
		pathdom.PathKey("sym40@4.field.deep"),
		pathdom.PathKey("sym41@3.field.deep"),
	} {
		if got, ok := StructuralKeyFromPathKey(key); ok && got.HasPrefix(prefix) {
			t.Fatalf("%s should not be under %s", key, prefix.PathKey())
		}
	}
}

func TestStructuralKeyStablePrefixRelationsUseSegmentBoundaries(t *testing.T) {
	placeholder := mustStructuralKey(t, pathdom.PathKey("$0.field"))
	placeholderChild := mustStructuralKey(t, pathdom.PathKey("$0.field.deep"))
	placeholderSibling := mustStructuralKey(t, pathdom.PathKey("$0.fieldish"))
	retChild := mustStructuralKey(t, pathdom.PathKey("ret[1].field.deep"))

	if !placeholderChild.HasPrefix(placeholder) || !placeholderChild.HasStrictPrefix(placeholder) {
		t.Fatalf("%s should be a strict descendant of %s", placeholderChild.PathKey(), placeholder.PathKey())
	}
	if placeholder.HasStrictPrefix(placeholder) {
		t.Fatalf("%s should not be a strict descendant of itself", placeholder.PathKey())
	}
	if placeholderSibling.HasPrefix(placeholder) {
		t.Fatalf("%s should not be under %s", placeholderSibling.PathKey(), placeholder.PathKey())
	}
	if retChild.HasPrefix(placeholder) {
		t.Fatalf("%s should not be under %s", retChild.PathKey(), placeholder.PathKey())
	}
}

func TestStructuralKeyRejectsInvalidSpellings(t *testing.T) {
	for _, key := range []pathdom.PathKey{
		pathdom.PathKey(""),
		pathdom.PathKey(".field"),
		pathdom.PathKey("sym1@1[bad]"),
		pathdom.PathKey("sym1@1."),
		pathdom.PathKey("sym1.field"),
		pathdom.PathKey("ret[1"),
	} {
		if got, ok := StructuralKeyFromPathKey(key); ok || got.PathKey() != "" {
			t.Fatalf("StructuralKeyFromPathKey(%q) = %s/%v, want rejected", key, got.PathKey(), ok)
		}
	}
}

func TestRebasePathKeyPreservesStructuralBoundaries(t *testing.T) {
	got, ok := RebasePathKey(
		pathdom.PathKey("sym10@1.child.name"),
		pathdom.PathKey("sym10@1.child"),
		pathdom.PathKey("sym20@1.leaf"),
	)
	if !ok || got != pathdom.PathKey("sym20@1.leaf.name") {
		t.Fatalf("RebasePathKey(versioned) = %s/%v, want sym20@1.leaf.name/true", got, ok)
	}

	if got, ok := RebasePathKey(
		pathdom.PathKey("sym10@1.childish.name"),
		pathdom.PathKey("sym10@1.child"),
		pathdom.PathKey("sym20@1.leaf"),
	); ok || got != "" {
		t.Fatalf("RebasePathKey(boundary collision) = %s/%v, want rejected", got, ok)
	}

	got, ok = RebasePathKey(
		pathdom.PathKey("$0.child.name"),
		pathdom.PathKey("$0.child"),
		pathdom.PathKey("ret[1].leaf"),
	)
	if !ok || got != pathdom.PathKey("ret[1].leaf.name") {
		t.Fatalf("RebasePathKey(stable) = %s/%v, want ret[1].leaf.name/true", got, ok)
	}

	if got, ok := RebasePathKey(
		pathdom.PathKey("sym10@1.child.name"),
		pathdom.PathKey("sym10@1.child"),
		pathdom.PathKey("s20.leaf"),
	); ok || got != "" {
		t.Fatalf("RebasePathKey(mixed local/stable) = %s/%v, want rejected", got, ok)
	}
}

func mustStructuralKey(t *testing.T, key pathdom.PathKey) StructuralKey {
	t.Helper()
	got, ok := StructuralKeyFromPathKey(key)
	if !ok {
		t.Fatalf("StructuralKeyFromPathKey(%q) failed", key)
	}
	return got
}
