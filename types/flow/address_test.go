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
