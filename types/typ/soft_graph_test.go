package typ

import (
	"fmt"
	"github.com/wippyai/go-lua/types/kind"
	"testing"
)

type countedSoftLeaf struct{ visits int }

func (l *countedSoftLeaf) Kind() kind.Kind    { l.visits++; return kind.Any }
func (*countedSoftLeaf) String() string       { return "any" }
func (*countedSoftLeaf) Hash() uint64         { return 1 }
func (l *countedSoftLeaf) Equals(t Type) bool { return t == l }

func softSharedGraph(leaf Type, depth int) Type {
	node := leaf
	for range depth {
		node = &Union{Members: []Type{NewArray(node), NewMap(String, node)}}
	}
	return node
}

func TestIsSoftVisitsSharedNodesOnce(t *testing.T) {
	leaf := &countedSoftLeaf{}
	if !IsSoft(softSharedGraph(leaf, 12), SoftPlaceholderPolicy) {
		t.Fatal("shared placeholder graph is not soft")
	}
	if leaf.visits != 1 {
		t.Fatalf("leaf visited %d times, want one", leaf.visits)
	}
}

func TestIsSoftCycleKeepsConcreteEvidence(t *testing.T) {
	alias := &Alias{Name: "cycle"}
	alias.Target = alias
	if IsSoft(alias, SoftPlaceholderPolicy) {
		t.Fatal("an ungrounded cycle is not a placeholder")
	}
	shared := &Union{Members: []Type{Any, String}}
	if IsSoft(&Union{Members: []Type{shared, shared}}, SoftPlaceholderPolicy) {
		t.Fatal("sharing hides a concrete union member")
	}
}

func BenchmarkIsSoftSharedGraph(b *testing.B) {
	for _, depth := range []int{8, 12, 16} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			graph := softSharedGraph(Any, depth)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				IsSoft(graph, SoftPlaceholderPolicy)
			}
		})
	}
}
