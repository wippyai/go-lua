package summary

import (
	"testing"

	"github.com/wippyai/go-lua/types/flow"
)

func TestEntryFactsKeyDistinguishesLengthRelations(t *testing.T) {
	resetEntryFactsKeyInterner()
	target := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0}
	relA := flow.BoundaryLengthRelationFact{
		Target: target,
		Source: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 1},
	}
	relB := flow.BoundaryLengthRelationFact{
		Target: target,
		Source: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 2},
	}

	keyA := entryFactsKeyOf(flow.BoundaryFactsDomain.Top().WithLengthRelations([]flow.BoundaryLengthRelationFact{relA}))
	keyB := entryFactsKeyOf(flow.BoundaryFactsDomain.Top().WithLengthRelations([]flow.BoundaryLengthRelationFact{relB}))
	if keyA == keyB {
		t.Fatalf("entry facts keys collapsed distinct length relations: %#v", keyA)
	}
	if got := keyA.Facts(); !got.HasLengthRelation(relA) {
		t.Fatalf("keyA facts = %#v, want %#v", got.LengthRelations(), relA)
	}
	if got := keyB.Facts(); !got.HasLengthRelation(relB) {
		t.Fatalf("keyB facts = %#v, want %#v", got.LengthRelations(), relB)
	}
}
