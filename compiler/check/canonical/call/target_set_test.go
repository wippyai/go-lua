package call

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

func TestNewTargetSetNormalizesDirectRefsAndCopies(t *testing.T) {
	t.Parallel()

	set := NewTargetSet(
		[]summary.FuncRef{
			{GraphID: 2, ParentHash: 8},
			{GraphID: 1, ParentHash: 5},
			{GraphID: 2, ParentHash: 3},
			{GraphID: 1, ParentHash: 5},
		},
		true,
		nil,
		false,
	)

	out := set.DirectRefs()
	want := []summary.FuncRef{
		{GraphID: 1, ParentHash: 5},
		{GraphID: 2, ParentHash: 3},
		{GraphID: 2, ParentHash: 8},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("DirectRefs = %+v, want %+v", out, want)
	}

	out[0].GraphID = 99
	if got := set.DirectRefs()[0].GraphID; got == 99 {
		t.Fatalf("DirectRefs mutated through copy; got GraphID %d", got)
	}
}

func TestNewTargetSetCopiesClosureRefs(t *testing.T) {
	t.Parallel()

	cells := flow.CaptureCellsDomain.Bottom()
	set := NewTargetSet(
		nil,
		false,
		[]flow.ClosureRef{
			flow.ClosureRefOf(flow.FunctionRef{GraphID: 2}, cells, flow.FunctionRefsDomain.Bottom()),
			flow.ClosureRefOf(flow.FunctionRef{GraphID: 1}, cells, flow.FunctionRefsDomain.Bottom()),
			flow.ClosureRefOf(flow.FunctionRef{GraphID: 2}, cells, flow.FunctionRefsDomain.Bottom()),
		},
		true,
	)

	out := set.ClosureRefs()
	if len(out) != 2 {
		t.Fatalf("ClosureRefs len = %d, want 2", len(out))
	}
	if out[0].Ref.GraphID != 1 || out[1].Ref.GraphID != 2 {
		t.Fatalf("ClosureRefs order = %v then %v, want graph IDs 1 then 2", out[0].Ref, out[1].Ref)
	}

	out[0].Ref.GraphID = 99
	if got := set.ClosureRefs()[0].Ref.GraphID; got == 99 {
		t.Fatalf("ClosureRefs mutated through copy; got GraphID %d", got)
	}
}

func TestTargetSetSingleDirect(t *testing.T) {
	t.Parallel()

	set := NewTargetSet([]summary.FuncRef{{GraphID: 42, ParentHash: 1}}, false, nil, false)
	ref, ok := set.SingleDirect()
	if !ok || ref.GraphID != 42 || ref.ParentHash != 1 {
		t.Fatalf("SingleDirect() = %+v/%v, want {42,1}/true", ref, ok)
	}

	multi := NewTargetSet([]summary.FuncRef{{GraphID: 42}, {GraphID: 99}}, false, nil, false)
	if _, ok := multi.SingleDirect(); ok {
		t.Fatal("SingleDirect returned true for multiple refs")
	}
}

func TestTargetSetPrecedenceUsesFiniteClosuresOnly(t *testing.T) {
	t.Parallel()

	direct := []summary.FuncRef{{GraphID: 7, ParentHash: 1}}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 9}, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom())

	closureTopWithDirect := NewTargetSet(direct, true, nil, true)
	if closureTopWithDirect.HasFiniteClosureTargets() {
		t.Fatal("ClosureRefs top/unknown should not count as finite closure targets")
	}
	if closureTopWithDirect.UseClosureTargets() {
		t.Fatal("ClosureRefs top/unknown should not dominate finite direct targets")
	}
	if !closureTopWithDirect.UseDirectTargets() {
		t.Fatal("finite direct targets should be used when ClosureRefs is top/unknown")
	}

	staticFallbackWithClosureTop := NewTargetSet(direct, false, nil, true)
	if !staticFallbackWithClosureTop.UseDirectTargets() {
		t.Fatal("static direct fallback should remain usable when ClosureRefs is top/unknown")
	}

	finiteClosureWithDirect := NewTargetSet(direct, true, []flow.ClosureRef{closure}, true)
	if !finiteClosureWithDirect.HasFiniteClosureTargets() {
		t.Fatal("finite ClosureRefs should count as finite closure targets")
	}
	if !finiteClosureWithDirect.UseClosureTargets() {
		t.Fatal("finite closure targets should be selected")
	}
	if finiteClosureWithDirect.UseDirectTargets() {
		t.Fatal("finite closure targets should dominate direct refs")
	}
}
