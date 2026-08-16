package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContinuationGuardChainCanonicalPrefixesAreShared(t *testing.T) {
	const subjects = 256
	const decisions = 128
	builder := newGuardChainBuilder()
	roots := make([]uint32, subjects)
	for ordinal := uint32(1); ordinal <= decisions; ordinal++ {
		builder.beginRank()
		term := keyspace.MakeTerm(keyspace.FamilySelect, ordinal)
		for subject := range roots {
			root, ok := builder.append(roots[subject], term)
			if !ok {
				t.Fatalf("append subject %d decision %d failed", subject, ordinal)
			}
			roots[subject] = root
		}
	}
	for subject := 1; subject < len(roots); subject++ {
		if roots[subject] != roots[0] {
			t.Fatalf("equal Guard set has roots %d and %d", roots[0], roots[subject])
		}
	}
	if got, want := len(builder.nodes), decisions+1; got != want {
		t.Fatalf("shared Guard nodes = %d, want %d (not subjects*decisions)", got, want)
	}
	seen := make([]bool, len(builder.nodes))
	for _, root := range roots {
		for root != 0 {
			if int(root) >= len(builder.nodes) {
				t.Fatalf("root %d leaves node arena", root)
			}
			seen[root] = true
			root = builder.nodes[root].prev
		}
	}
	for index := 1; index < len(seen); index++ {
		if !seen[index] {
			t.Fatalf("retained Guard node %d is unreachable from every subject root", index)
		}
	}
}

func TestContinuationGuardChainCanonicalTermOrderAndAllocationFreeLookup(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilySelect: 512,
		keyspace.FamilyBranch: 512,
		keyspace.FamilyLoop:   512,
	}
	builder := newGuardChainBuilder()
	root := uint32(0)
	for rank := uint32(0); rank < 1536; rank++ {
		builder.beginRank()
		term, ok := guardTermAtRank(rank, counts)
		if !ok {
			t.Fatalf("rank %d is unavailable", rank)
		}
		parent := root
		root, ok = builder.append(parent, term)
		if !ok {
			t.Fatalf("append rank %d term %08x from root %d node %#v failed", rank, uint32(term), parent, builder.nodes[parent])
		}
	}
	projection := guardProjection{nodes: builder.nodes, counts: counts}
	for index := uint32(0); index < 1536; index++ {
		got, ok := projection.at(root, 1536, index)
		want, wantOK := guardTermAtRank(index, counts)
		if !ok || !wantOK || got != want {
			t.Fatalf("GuardAt(%d) = %08x/%v, want %08x/true", index, uint32(got), ok, uint32(want))
		}
	}
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = projection.at(root, 1536, 0)
		_, _ = projection.at(root, 1536, 767)
		_, _ = projection.at(root, 1536, 1535)
	})
	if allocs != 0 {
		t.Fatalf("Guard chain lookup allocated %v objects", allocs)
	}
}

func TestContinuationGuardChainValid4095StructuralPath(t *testing.T) {
	const prefix = uint32(4095)
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilySelect: prefix}
	builder := newGuardChainBuilder()
	root := uint32(0)
	for ordinal := uint32(1); ordinal <= prefix; ordinal++ {
		builder.beginRank()
		var ok bool
		root, ok = builder.append(root, keyspace.MakeTerm(keyspace.FamilySelect, ordinal))
		if !ok {
			t.Fatalf("append prefix ordinal %d failed", ordinal)
		}
	}
	projection := guardProjection{nodes: builder.nodes, counts: counts}
	for index := uint32(0); index < prefix; index++ {
		got, ok := projection.at(root, prefix, index)
		want := keyspace.MakeTerm(keyspace.FamilySelect, index+1)
		if !ok || got != want {
			t.Fatalf("4095-prefix GuardAt(%d) = %08x/%v, want %08x/true", index, uint32(got), ok, uint32(want))
		}
	}
}

func TestContinuationGuardAncestorMaximumUint32StructuralPath(t *testing.T) {
	if guardAncestorFenwickProofBound != 32*33/2-1 {
		t.Fatalf("guard ancestor Fenwick proof bound = %d, want %d", guardAncestorFenwickProofBound, 32*33/2-1)
	}

	// A monotone structural path is enough to exercise the exact uint32 bound.
	// Its root carries the uint32 maximum, but the test allocates only the
	// links needed by the 527-step path rather than a uint32-max node arena.
	makeLinearChain := func(nodeCount int) []guardNode {
		nodes := make([]guardNode, nodeCount+1)
		for index := 1; index < len(nodes); index++ {
			count := uint32(index)
			if index == len(nodes)-1 {
				count = ^uint32(0)
			}
			nodes[index] = guardNode{prev: uint32(index - 1), count: count}
		}
		return nodes
	}

	exact := makeLinearChain(guardAncestorFenwickProofBound + 1)
	got, ok := guardAncestor(exact, uint32(len(exact)-1), 1)
	if !ok || got != 1 {
		t.Fatalf("maximum-bound Guard ancestor = %d/%v, want 1/true", got, ok)
	}

	over := makeLinearChain(guardAncestorFenwickProofBound + 2)
	got, ok = guardAncestor(over, uint32(len(over)-1), 1)
	if ok {
		t.Fatalf("Guard ancestor accepted a path requiring more than %d steps at node %d", guardAncestorFenwickProofBound, got)
	}
}

func TestContinuationGuardChainRejectsMalformedLinks(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{keyspace.FamilySelect: 2}
	builder := newGuardChainBuilder()
	builder.beginRank()
	first, ok := builder.append(0, keyspace.MakeTerm(keyspace.FamilySelect, 1))
	if !ok {
		t.Fatal("append first Guard failed")
	}
	builder.beginRank()
	second, ok := builder.append(first, keyspace.MakeTerm(keyspace.FamilySelect, 2))
	if !ok {
		t.Fatal("append second Guard failed")
	}
	cases := []struct {
		name   string
		mutate func([]guardNode)
	}{
		{name: "previous", mutate: func(nodes []guardNode) { nodes[second].prev = second }},
		{name: "jump", mutate: func(nodes []guardNode) { nodes[second].jump = second }},
		{name: "count", mutate: func(nodes []guardNode) { nodes[second].count++ }},
	}
	for _, test := range cases {
		name, mutate := test.name, test.mutate
		t.Run(name, func(t *testing.T) {
			nodes := append([]guardNode(nil), builder.nodes...)
			mutate(nodes)
			projection := guardProjection{nodes: nodes, counts: counts}
			if _, ok := projection.at(second, 2, 0); ok {
				t.Fatalf("Guard lookup accepted malformed %s link", name)
			}
		})
	}
}
