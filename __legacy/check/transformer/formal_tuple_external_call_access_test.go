package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

func TestSelectFormalExternalCallProviderAccessRetainsOnlyObservedOperandRoots(t *testing.T) {
	arena := NewArena(standard.Registry())
	root := arena.Root(Root{Kind: RootParam})
	member := arena.StaticIndexValue(root, segment.Segment{Kind: segment.SegmentField, Name: "id"})
	operand := arena.LuaTypeNameValue(member)
	unobserved := arena.StaticIndexValue(root, segment.Segment{Kind: segment.SegmentField, Name: "other"})
	if root == 0 || member == 0 || operand == 0 || unobserved == 0 {
		t.Fatal("operand DAG construction failed")
	}
	access := []valueAccessTerm{{term: root, hasPoint: true}, {term: member, hasPoint: true}, {term: operand, hasPoint: true}, {term: unobserved, hasPoint: true}}
	operands := callOutcomeOperandTerms{arguments: []ValueTerm{operand, unobserved}}

	got := selectFormalExternalCallProviderAccess(arena, access, operands.selectObservation(callpayload.ObserveCallOutcomeOperands(false, false, 0)), callpayload.ObserveCallOutcomeOperands(false, false, 0))
	want := access[2:3]
	if len(got) != len(want) {
		t.Fatalf("observed operand access = %#v, want selected roots %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("observed operand access[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}
