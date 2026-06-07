package ref

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestFlowConversionRoundTrip(t *testing.T) {
	canonical := FuncRef{GraphID: 42, ParentHash: 99}
	flowRef := ToFlow(canonical)
	if flowRef.GraphID != canonical.GraphID || flowRef.ParentHash != canonical.ParentHash {
		t.Fatalf("ToFlow(%+v) = %+v", canonical, flowRef)
	}
	if got := FromFlow(flowRef); got != canonical {
		t.Fatalf("FromFlow(ToFlow(ref)) = %+v, want %+v", got, canonical)
	}
}

func TestFromFlowStructuredPath(t *testing.T) {
	path := constraint.NewPath(1, "fn")
	refs := flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(
		flow.FunctionRef{GraphID: 2, ParentHash: 3},
		flow.FunctionRef{GraphID: 4, ParentHash: 5},
	))

	got, ok := FromFlowStructuredPath(refs, path)
	if !ok {
		t.Fatal("FromFlowStructuredPath finite set ok = false")
	}
	want := []FuncRef{{GraphID: 2, ParentHash: 3}, {GraphID: 4, ParentHash: 5}}
	if len(got) != len(want) {
		t.Fatalf("FromFlowStructuredPath len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FromFlowStructuredPath[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	topRefs := flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetTop())
	if got, ok := FromFlowStructuredPath(topRefs, path); !ok || got != nil {
		t.Fatalf("FromFlowStructuredPath(top) = %+v/%v, want nil/true", got, ok)
	}
	if got, ok := FromFlowStructuredPath(refs, constraint.NewPath(9, "missing")); ok || got != nil {
		t.Fatalf("FromFlowStructuredPath(missing) = %+v/%v, want nil/false", got, ok)
	}
}

func TestUniqueSortedFuncRefs(t *testing.T) {
	t.Parallel()

	in := []FuncRef{
		{GraphID: 2, ParentHash: 8},
		{GraphID: 1, ParentHash: 5},
		{GraphID: 2, ParentHash: 3},
		{GraphID: 1, ParentHash: 5},
	}
	got := UniqueSortedFuncRefs(in)
	want := []FuncRef{
		{GraphID: 1, ParentHash: 5},
		{GraphID: 2, ParentHash: 3},
		{GraphID: 2, ParentHash: 8},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UniqueSortedFuncRefs = %+v, want %+v", got, want)
	}

	got[0].GraphID = 99
	if in[0].GraphID == 99 {
		t.Fatal("UniqueSortedFuncRefs returned storage aliasing caller input")
	}
}
