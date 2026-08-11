package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func callbackOwnerOperation(name string) OperationSpec {
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		ValuesVars: 5,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesVariable, Var: 0},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func TestCallbackOwnerCanonicalRoundTripAndForeignHandles(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackOwnerOperation("callback-owner-b"),
		callbackOwnerOperation("callback-owner-a"),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackOwnerOperation("callback-owner-a"),
		callbackOwnerOperation("callback-owner-b"),
	}})
	if got, want := publicContractSnapshot(t, left), publicContractSnapshot(t, right); got != want {
		t.Fatalf("callback owner permutation changed public contract\nleft: %s\nright: %s", got, want)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("callback owner permutation changed ContentID")
	}
	for _, binding := range []BindingSpec{
		{Namespace: BindingBuiltin, Member: []string{"callback-owner-a"}},
		{Namespace: BindingBuiltin, Member: []string{"callback-owner-b"}},
	} {
		op, found := left.Lookup(binding)
		if !found {
			t.Fatalf("callback owner operation missing: %#v", binding)
		}
		id, found := left.CallbackAt(op, 0)
		owner, ownerFound := left.CallbackOwner(id)
		if !found || !ownerFound || id == 0 || owner != op {
			t.Fatalf("callback owner round trip = %d/%d/%v/%v, want %d", id, owner, found, ownerFound, op)
		}
	}
	if _, ok := left.CallbackOwner(0); ok {
		t.Fatal("zero CallbackID resolved")
	}
	foreign := mustSeal(t, Spec{Operations: []OperationSpec{
		callbackOwnerOperation("foreign-a"),
		callbackOwnerOperation("foreign-b"),
		callbackOwnerOperation("foreign-c"),
	}})
	foreignOpaque, found := foreign.Opaque()
	if !found {
		t.Fatal("foreign opaque operation missing")
	}
	foreignID, found := foreign.CallbackAt(foreignOpaque, 0)
	if !found {
		t.Fatal("foreign out-of-range CallbackID missing")
	}
	if _, ok := left.CallbackOwner(foreignID); ok {
		t.Fatal("out-of-range foreign CallbackID resolved in this Contract")
	}
	first, found := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-owner-a"}})
	if !found {
		t.Fatal("callback owner allocation operation missing")
	}
	id, found := left.CallbackAt(first, 0)
	if !found {
		t.Fatal("callback owner allocation handle missing")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if owner, ok := left.CallbackOwner(id); !ok || owner != first {
			panic("callback owner disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("CallbackOwner allocated %f times", allocs)
	}
}
