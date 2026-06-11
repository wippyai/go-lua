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
	if !SameStablePath(a, b) {
		t.Fatalf("SameStablePath(%s, %s) = false, want true", a.Key(), b.Key())
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

func TestStableKeyHasPrefixUsesSegmentBoundaries(t *testing.T) {
	root, _ := StableOfPath(pathdom.NewPath(7, "root"))
	field, _ := StableOfPath(pathdom.NewPath(7, "root").Field("foo"))
	child := SymbolPathKey(7, []segment.Segment{
		{Kind: segment.SegmentField, Name: "foo"},
		{Kind: segment.SegmentField, Name: "bar"},
	})
	siblingPrefixCollision := SymbolPathKey(7, []segment.Segment{
		{Kind: segment.SegmentField, Name: "foobar"},
	})

	if !StableKeyHasPrefix(child, root) {
		t.Fatalf("%s should be under root %s", child, root.Key())
	}
	if !StableKeyHasPrefix(child, field) {
		t.Fatalf("%s should be under field %s", child, field.Key())
	}
	if StableKeyHasPrefix(siblingPrefixCollision, field) {
		t.Fatalf("%s should not be under field %s", siblingPrefixCollision, field.Key())
	}

	stale := pathdom.NewPath(7, "root").Field("foo")
	stale.Version = 1
	if StableKeyHasPrefix(stale.Key(), root) {
		t.Fatalf("stale path key accepted as stable key: %s", stale.Key())
	}
}

func TestStableFromKeyRoundTripsSymbolAndNamedRoots(t *testing.T) {
	symbolAddr, _ := StableOfSymbol(11, []segment.Segment{
		{Kind: segment.SegmentIndexString, Name: "node-id"},
		{Kind: segment.SegmentField, Name: "label"},
	})
	parsedSymbol, ok := StableFromKey(symbolAddr.Key())
	if !ok || !parsedSymbol.Equal(symbolAddr) {
		t.Fatalf("StableFromKey(symbol) = %s/%v, want %s/true", parsedSymbol.Key(), ok, symbolAddr.Key())
	}

	rootAddr, _ := StableOfRoot("$0", []segment.Segment{{Kind: segment.SegmentIndexString, Name: "node-id"}})
	parsedRoot, ok := StableFromKey(rootAddr.Key())
	if !ok || !parsedRoot.Equal(rootAddr) {
		t.Fatalf("StableFromKey(root) = %s/%v, want %s/true", parsedRoot.Key(), ok, rootAddr.Key())
	}

	ambiguousRoot, _ := StableOfRoot("s7", []segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	parsedAmbiguous, ok := StableFromKey(ambiguousRoot.Key())
	if !ok || !parsedAmbiguous.Equal(ambiguousRoot) {
		t.Fatalf("StableFromKey(ambiguous root) = %s/%v, want %s/true", parsedAmbiguous.Key(), ok, ambiguousRoot.Key())
	}

	retAddr, _ := StableOfRoot("ret[1]", []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}})
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
	if got, ok := StableFromKey(StablePathKey(path)); !ok || got.Key() != StablePathKey(path) {
		t.Fatalf("StableFromKey(stable key) = %s/%v, want %s/true", got.Key(), ok, StablePathKey(path))
	}
}

func TestStablePathKeySeparatesAmbiguousRootsAndVersionedLocalKeys(t *testing.T) {
	ambiguousRoot := pathdom.Path{Root: "sym12", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	ambiguousStable := StablePathKey(ambiguousRoot)
	if ambiguousStable == ambiguousRoot.Key() {
		t.Fatalf("StablePathKey(%q) collided with path key %q", ambiguousRoot.String(), ambiguousRoot.Key())
	}
	if parsed, ok := StableFromKey(ambiguousStable); !ok || parsed.Key() != ambiguousStable {
		t.Fatalf("StableFromKey(%q) = %s/%v, want round-trip", ambiguousStable, parsed.Key(), ok)
	}
	if _, ok := StableFromKey(ambiguousRoot.Key()); ok {
		t.Fatalf("StableFromKey accepted ambiguous path key %q", ambiguousRoot.Key())
	}

	versionedLocal := pathdom.NewPath(12, "sym12").Field("field")
	versionedLocal.Version = 3
	if StablePathKey(versionedLocal) == versionedLocal.Key() {
		t.Fatalf("StablePathKey(%q) collided with versioned local key %q", versionedLocal.String(), versionedLocal.Key())
	}
	if _, ok := StableFromKey(versionedLocal.Key()); ok {
		t.Fatalf("StableFromKey accepted versioned local key %q", versionedLocal.Key())
	}
	if parsed, ok := StableFromKey(StablePathKey(versionedLocal)); !ok || !parsed.Equal(mustStableOfPath(t, versionedLocal)) {
		t.Fatalf("StableFromKey(stable versioned key) = %s/%v, want round-trip", parsed.Key(), ok)
	}

	placeholder := pathdom.Path{Root: "$0", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	ret := pathdom.Path{Root: "ret[0]", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	for _, path := range []pathdom.Path{placeholder, ret} {
		stable := StablePathKey(path)
		parsed, ok := StableFromKey(stable)
		if !ok || !parsed.Equal(mustStableOfPath(t, path)) {
			t.Fatalf("StableFromKey(%q) = %s/%v, want round-trip", stable, parsed.Key(), ok)
		}
	}
	if StablePathKey(placeholder) == StablePathKey(ret) {
		t.Fatalf("placeholder and return-root stable keys collided: %q", StablePathKey(placeholder))
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

func TestStableConstructorsAreDefensive(t *testing.T) {
	segments := []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}
	addr, ok := StableOfSymbol(13, segments)
	if !ok {
		t.Fatal("StableOfSymbol failed")
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

	ref, ok := ContainerOfPath(path)
	if !ok || !ref.IsValid() {
		t.Fatalf("ContainerOfPath = %#v/%v, want valid ref", ref, ok)
	}
	if got := ref.Root(); got != path.Symbol {
		t.Fatalf("ContainerRef root = %d, want %d", got, path.Symbol)
	}

	again, ok := ContainerOfPath(pathdom.Path{Symbol: path.Symbol, Segments: path.Segments})
	if !ok || !ref.Equal(again) {
		t.Fatalf("ContainerRef equality = %#v/%#v/%v, want equal", ref, again, ok)
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
	if ref, ok := ContainerOfPath(pathdom.Path{Root: "rows"}); ok || ref.IsValid() {
		t.Fatalf("ContainerOfPath(unresolved) = %#v/%v, want rejected", ref, ok)
	}
	if ref, ok := ContainerOfSymbol(0); ok || ref.IsValid() {
		t.Fatalf("ContainerOfSymbol(0) = %#v/%v, want rejected", ref, ok)
	}
}
