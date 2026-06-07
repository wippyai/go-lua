package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestStableAddressIgnoresVersionAndExportsCanonicalKey(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	a.Version = 1
	b := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	b.Version = 9

	addrA, ok := StableAddressOfPath(a)
	if !ok {
		t.Fatal("StableAddressOfPath(a) did not resolve")
	}
	addrB, ok := StableAddressOfPath(b)
	if !ok {
		t.Fatal("StableAddressOfPath(b) did not resolve")
	}
	if !addrA.Equal(addrB) {
		t.Fatalf("stable addresses differ across versions: %s vs %s", addrA.Key(), addrB.Key())
	}
	if got, want := addrA.Key(), SymbolPathKey(cfg.SymbolID(42), a.Segments); got != want {
		t.Fatalf("stable key = %s, want %s", got, want)
	}
}

func TestPathIdentityKeyUsesStableAddressAndPlaceholderFallback(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	a.Version = 1
	b := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	b.Version = 9
	if got, want := PathIdentityKey(a), StablePathKey(b); got != want {
		t.Fatalf("PathIdentityKey(versioned symbol) = %s, want stable %s", got, want)
	}

	placeholder := constraint.NewPlaceholder(0).Field("item")
	if got, want := PathIdentityKey(placeholder), placeholder.Key(); got != want {
		t.Fatalf("PathIdentityKey(placeholder) = %s, want %s", got, want)
	}
}

func TestStableAddressSeparatesSymbolAndRootIdentity(t *testing.T) {
	symbol, ok := StableAddressOfPath(constraint.NewPath(cfg.SymbolID(7), "x"))
	if !ok {
		t.Fatal("symbol address did not resolve")
	}
	root, ok := StableAddressOfPath(constraint.Path{Root: "s7"})
	if !ok {
		t.Fatal("root address did not resolve")
	}
	if symbol.Equal(root) || symbol.Overlaps(root) {
		t.Fatalf("symbol/root identities overlapped: %s and %s", symbol.Key(), root.Key())
	}
}

func TestStableAddressOverlapIsStructuredPrefix(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(3), "graph")
	nodes := root.Field("nodes")
	edge := root.Field("edges")
	nodeEntry := nodes.IndexStr("last")

	rootAddr, _ := StableAddressOfPath(root)
	nodesAddr, _ := StableAddressOfPath(nodes)
	edgeAddr, _ := StableAddressOfPath(edge)
	entryAddr, _ := StableAddressOfPath(nodeEntry)

	if !entryAddr.Overlaps(nodesAddr) {
		t.Fatalf("%s should overlap ancestor %s", entryAddr.Key(), nodesAddr.Key())
	}
	if !nodesAddr.Overlaps(rootAddr) {
		t.Fatalf("%s should overlap root %s", nodesAddr.Key(), rootAddr.Key())
	}
	if entryAddr.Overlaps(edgeAddr) {
		t.Fatalf("sibling addresses overlapped: %s and %s", entryAddr.Key(), edgeAddr.Key())
	}
}

func TestStableAddressFromKeyRoundTripsSymbolAndRoot(t *testing.T) {
	symbol, _ := StableAddressOfSymbol(cfg.SymbolID(11), []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "node-id"},
		{Kind: constraint.SegmentField, Name: "label"},
	})
	parsedSymbol, ok := StableAddressFromKey(symbol.Key())
	if !ok {
		t.Fatalf("failed to parse symbol key %s", symbol.Key())
	}
	if !parsedSymbol.Equal(symbol) {
		t.Fatalf("parsed symbol = %s, want %s", parsedSymbol.Key(), symbol.Key())
	}

	root, _ := StableAddressOfRoot("$0", []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "node-id"},
	})
	parsedRoot, ok := StableAddressFromKey(root.Key())
	if !ok {
		t.Fatalf("failed to parse root key %s", root.Key())
	}
	if !parsedRoot.Equal(root) {
		t.Fatalf("parsed root = %s, want %s", parsedRoot.Key(), root.Key())
	}
}

func TestStableAddressCanonicalKeyRejectsLegacyPathKey(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(12), "node").Field("label")
	path.Version = 7
	legacyKey := path.Key()
	canonicalKey := StablePathKey(path)
	if legacyKey == canonicalKey {
		t.Fatalf("legacy key is already canonical: %s", legacyKey)
	}
	if _, ok := StableAddressFromCanonicalKey(legacyKey); ok {
		t.Fatalf("canonical decoder accepted legacy key %s", legacyKey)
	}
	if addr, ok := StableAddressFromKey(legacyKey); !ok || addr.Key() != canonicalKey {
		t.Fatalf("compat decoder = %s/%v, want canonical %s/true", addr.Key(), ok, canonicalKey)
	}
}

func TestStablePathFromKeyReturnsStructuredPath(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(12), "node").IndexStr("last").Field("label")
	key := StablePathKey(path)

	got, ok := StablePathFromKey(key)
	if !ok {
		t.Fatalf("StablePathFromKey(%s) failed", key)
	}
	if got.Symbol != path.Symbol || len(got.Segments) != len(path.Segments) {
		t.Fatalf("path = %#v, want %#v", got, path)
	}
	for i := range path.Segments {
		if got.Segments[i] != path.Segments[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got.Segments[i], path.Segments[i])
		}
	}
}

func TestPathRootSeparatesSemanticRootKinds(t *testing.T) {
	symbol, ok := SymbolPathRoot(cfg.SymbolID(9))
	if !ok {
		t.Fatal("symbol root did not build")
	}
	named, ok := NamedPathRoot("9")
	if !ok {
		t.Fatal("named root did not build")
	}
	if symbol.Equal(named) {
		t.Fatalf("symbol root and named root should not be equal: %#v %#v", symbol, named)
	}
	if _, ok := symbol.Name(); ok {
		t.Fatal("symbol root exposed a name")
	}
	if _, ok := named.Symbol(); ok {
		t.Fatal("named root exposed a symbol")
	}
}

func TestPathSuffixIsDefensiveAndStructural(t *testing.T) {
	segments := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "nodes"},
		{Kind: constraint.SegmentIndexString, Name: "last"},
	}
	suffix := PathSuffixOfSegments(segments)
	segments[0].Name = "mutated"

	if got := suffix.KeySuffix(); got != `.nodes["last"]` {
		t.Fatalf("suffix key = %q, want structured original", got)
	}

	returned := suffix.Segments()
	returned[1].Name = "mutated"
	if got := suffix.KeySuffix(); got != `.nodes["last"]` {
		t.Fatalf("suffix changed through returned slice: %q", got)
	}

	parent := PathSuffixOfSegments([]constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}})
	sibling := PathSuffixOfSegments([]constraint.Segment{{Kind: constraint.SegmentField, Name: "edges"}})
	if !suffix.HasPrefix(parent) || !suffix.Overlaps(parent) {
		t.Fatalf("%s should overlap ancestor %s", suffix.KeySuffix(), parent.KeySuffix())
	}
	if suffix.Overlaps(sibling) {
		t.Fatalf("%s should not overlap sibling %s", suffix.KeySuffix(), sibling.KeySuffix())
	}
}

func TestStableAddressRemainderAfterPrefixIsStructuredAndDefensive(t *testing.T) {
	root, _ := SymbolPathRoot(cfg.SymbolID(17))
	parent, _ := StableAddressOfRootAndSuffix(root, PathSuffixOfSegments([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "node"},
	}))
	child, _ := StableAddressOfRootAndSuffix(root, PathSuffixOfSegments([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "node"},
		{Kind: constraint.SegmentIndexString, Name: "id"},
	}))

	remainder, ok := child.RemainderAfterPrefix(parent)
	if !ok {
		t.Fatal("child should be under parent")
	}
	if len(remainder) != 1 || remainder[0].Kind != constraint.SegmentIndexString || remainder[0].Name != "id" {
		t.Fatalf("remainder = %#v, want [\"id\"]", remainder)
	}
	remainder[0].Name = "mutated"
	again, _ := child.RemainderAfterPrefix(parent)
	if again[0].Name != "id" {
		t.Fatalf("remainder was not defensive: %#v", again)
	}
}

func TestStableAddressKeyRelationsOwnKeyParsing(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(28), "root")
	child := root.Field("child").IndexStr("id")
	other := constraint.NewPath(cfg.SymbolID(29), "root").Field("child")

	rootAddr, _ := StableAddressOfPath(root)
	childAddr, _ := StableAddressOfPath(child)

	remainder, ok := childAddr.RemainderAfterAddressKey(StablePathKey(root))
	if !ok {
		t.Fatal("child should be under root key")
	}
	if len(remainder) != 2 || remainder[0].Name != "child" || remainder[1].Name != "id" {
		t.Fatalf("remainder = %#v, want child/id suffix", remainder)
	}
	if _, ok := childAddr.RemainderAfterAddressKey(StablePathKey(other)); ok {
		t.Fatalf("different symbol key covered child address")
	}
	if !AddressKeyOverlaps(StablePathKey(child), rootAddr) {
		t.Fatalf("child key should overlap root address")
	}
	if AddressKeyOverlaps(StablePathKey(other), rootAddr) {
		t.Fatalf("different symbol key should not overlap root address")
	}
	if _, ok := childAddr.RemainderAfterAddressKey(root.Key()); ok {
		t.Fatalf("legacy root key covered child address")
	}
	if AddressKeyOverlaps(child.Key(), rootAddr) {
		t.Fatalf("legacy child key should not overlap root address")
	}
}

func TestStableAddressOfRootAndSuffixKeepsVocabularyCanonical(t *testing.T) {
	root, _ := SymbolPathRoot(cfg.SymbolID(27))
	suffix := PathSuffixOfSegments([]constraint.Segment{
		{Kind: constraint.SegmentField, Name: "payload"},
	})
	addr, ok := StableAddressOfRootAndSuffix(root, suffix)
	if !ok {
		t.Fatal("address did not build from normalized root/suffix")
	}
	if !addr.RootIdentity().Equal(root) {
		t.Fatalf("root identity changed: %#v vs %#v", addr.RootIdentity(), root)
	}
	if !addr.Suffix().Equal(suffix) {
		t.Fatalf("suffix changed: %s vs %s", addr.Suffix().KeySuffix(), suffix.KeySuffix())
	}
	if got, want := addr.Key(), SymbolPathKey(cfg.SymbolID(27), suffix.Segments()); got != want {
		t.Fatalf("key = %s, want %s", got, want)
	}
}
