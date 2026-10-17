package returns

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func sharedScanGraph(depth int) typ.Type {
	var node typ.Type = typ.String
	for range depth {
		node = &typ.Tuple{Elements: []typ.Type{node, node}}
	}
	return node
}

func TestScanTypeVisitsSharedNodesOnce(t *testing.T) {
	root := sharedScanGraph(12)
	visits := make(map[typ.Type]int)
	if scanType(root, nil, func(node typ.Type) (bool, bool) {
		visits[node]++
		return false, true
	}) {
		t.Fatal("scan reports a match without a matching predicate")
	}
	for node, count := range visits {
		if count != 1 {
			t.Errorf("%T visited %d times, want one visit per graph node", node, count)
		}
	}
	if len(visits) != 13 {
		t.Fatalf("visits %d distinct nodes, want 13", len(visits))
	}
}

func BenchmarkScanTypeSharedGraph(b *testing.B) {
	for _, depth := range []int{8, 12, 16} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			root := sharedScanGraph(depth)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				scanType(root, nil, func(typ.Type) (bool, bool) { return false, true })
			}
		})
	}
}

func TestScanTypeTerminatesOnCycles(t *testing.T) {
	node := &typ.Alias{Name: "cycle"}
	node.Target = node
	visits := 0
	if scanType(node, nil, func(typ.Type) (bool, bool) { visits++; return false, true }) {
		t.Fatal("cycle reports a match without a matching predicate")
	}
	if visits != 1 {
		t.Fatalf("cycle visited %d times, want one", visits)
	}
}

func TestScanTypeKeepsPredicateControl(t *testing.T) {
	child := &typ.Tuple{Elements: []typ.Type{typ.String}}
	root := &typ.Tuple{Elements: []typ.Type{child, typ.Number}}
	if scanType(root, nil, func(node typ.Type) (bool, bool) { return node == typ.String, node != child }) {
		t.Fatal("scan descends into a pruned subtree")
	}
	if !scanType(root, nil, func(node typ.Type) (bool, bool) { return node == typ.Number, true }) {
		t.Fatal("scan misses a reachable matching node")
	}
}
