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

func TestSameStablePathIgnoresVersion(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	a.Version = 1
	b := constraint.NewPath(cfg.SymbolID(42), "node").Field("edges")
	b.Version = 9
	if !SameStablePath(a, b) {
		t.Fatalf("SameStablePath(%s, %s) = false, want true", a.Key(), b.Key())
	}
	if SameStablePath(a, b.Field("label")) {
		t.Fatalf("SameStablePath accepted distinct paths: %s and %s", a.Key(), b.Field("label").Key())
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

func TestStableAddressFromCanonicalKeyRoundTripsSymbolAndRoot(t *testing.T) {
	symbol, _ := StableAddressOfSymbol(cfg.SymbolID(11), []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "node-id"},
		{Kind: constraint.SegmentField, Name: "label"},
	})
	parsedSymbol, ok := StableAddressFromCanonicalKey(symbol.Key())
	if !ok {
		t.Fatalf("failed to parse symbol key %s", symbol.Key())
	}
	if !parsedSymbol.Equal(symbol) {
		t.Fatalf("parsed symbol = %s, want %s", parsedSymbol.Key(), symbol.Key())
	}

	root, _ := StableAddressOfRoot("$0", []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "node-id"},
	})
	parsedRoot, ok := StableAddressFromCanonicalKey(root.Key())
	if !ok {
		t.Fatalf("failed to parse root key %s", root.Key())
	}
	if !parsedRoot.Equal(root) {
		t.Fatalf("parsed root = %s, want %s", parsedRoot.Key(), root.Key())
	}
}

func TestStableAddressCanonicalKeyRejectsStalePathKey(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(12), "node").Field("label")
	path.Version = 7
	staleKey := path.Key()
	canonicalKey := StablePathKey(path)
	if staleKey == canonicalKey {
		t.Fatalf("stale key is already canonical: %s", staleKey)
	}
	if _, ok := StableAddressFromCanonicalKey(staleKey); ok {
		t.Fatalf("canonical decoder accepted stale key %s", staleKey)
	}
	if got, ok := StableAddressFromCanonicalKey(canonicalKey); !ok || got.Key() != canonicalKey {
		t.Fatalf("canonical decoder = %s/%v, want %s/true", got.Key(), ok, canonicalKey)
	}
}

func TestStableAddressPublicConstructorsAreDefensive(t *testing.T) {
	segments := []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}}
	addr, ok := StableAddressOfSymbol(cfg.SymbolID(13), segments)
	if !ok {
		t.Fatal("symbol address did not build")
	}
	segments[0].Name = "mutated"
	if got, want := addr.Key(), SymbolPathKey(cfg.SymbolID(13), []constraint.Segment{{Kind: constraint.SegmentField, Name: "payload"}}); got != want {
		t.Fatalf("symbol address observed caller mutation: %s, want %s", got, want)
	}

	rootSegments := []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "k"}}
	rootAddr, ok := StableAddressOfRoot("$0", rootSegments)
	if !ok {
		t.Fatal("root address did not build")
	}
	rootSegments[0].Name = "mutated"
	if got, want := rootAddr.Key(), constraint.PathKey(`$0["k"]`); got != want {
		t.Fatalf("root address observed caller mutation: %s, want %s", got, want)
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

func TestStableAddressRelationsUseStructuredAddresses(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(28), "root")
	child := root.Field("child").IndexStr("id")
	other := constraint.NewPath(cfg.SymbolID(29), "root").Field("child")

	rootAddr, _ := StableAddressOfPath(root)
	childAddr, _ := StableAddressOfPath(child)
	otherAddr, _ := StableAddressOfPath(other)

	remainder, ok := childAddr.RemainderAfterPrefix(rootAddr)
	if !ok {
		t.Fatal("child should be under root address")
	}
	if len(remainder) != 2 || remainder[0].Name != "child" || remainder[1].Name != "id" {
		t.Fatalf("remainder = %#v, want child/id suffix", remainder)
	}
	if _, ok := childAddr.RemainderAfterPrefix(otherAddr); ok {
		t.Fatalf("different symbol address covered child address")
	}
	if !childAddr.Overlaps(rootAddr) {
		t.Fatalf("child address should overlap root address")
	}
	if otherAddr.Overlaps(rootAddr) {
		t.Fatalf("different symbol address should not overlap root address")
	}
}

func TestStableAddressKeyHasPrefixUsesStructuredBoundaries(t *testing.T) {
	root, _ := StableAddressOfPath(constraint.NewPath(cfg.SymbolID(7), "root"))
	field, _ := StableAddressOfPath(constraint.NewPath(cfg.SymbolID(7), "root").Field("foo"))
	child := SymbolPathKey(cfg.SymbolID(7), []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "foo"},
		{Kind: constraint.SegmentField, Name: "bar"},
	})
	siblingPrefixCollision := SymbolPathKey(cfg.SymbolID(7), []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "foobar"},
	})

	if !StableAddressKeyHasPrefix(child, root) {
		t.Fatalf("%s should be under root %s", child, root.Key())
	}
	if !StableAddressKeyHasPrefix(child, field) {
		t.Fatalf("%s should be under field %s", child, field.Key())
	}
	if StableAddressKeyHasPrefix(siblingPrefixCollision, field) {
		t.Fatalf("%s should not be under field %s", siblingPrefixCollision, field.Key())
	}

	otherSymbol := SymbolPathKey(cfg.SymbolID(70), nil)
	if StableAddressKeyHasPrefix(otherSymbol, root) {
		t.Fatalf("symbol prefix collision accepted: %s under %s", otherSymbol, root.Key())
	}

	stale := constraint.NewPath(cfg.SymbolID(7), "root").Field("foo")
	stale.Version = 1
	if StableAddressKeyHasPrefix(stale.Key(), root) {
		t.Fatalf("stale path key accepted as stable key: %s", stale.Key())
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
