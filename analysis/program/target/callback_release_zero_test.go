package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestCallbackReleaseZeroPolicyIsRequiredAndExact(t *testing.T) {
	for _, want := range []struct {
		behavior CallbackReleaseZeroBehavior
		outcome  uint32
	}{
		{CallbackReleaseZeroSuppress, 0},
		{CallbackReleaseZeroThrow, 1},
		{CallbackReleaseZeroIdempotent, 0},
	} {
		contract := mustSeal(t, callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: want.behavior, Outcome: want.outcome}))
		owner, ownerOK := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"release-zero-owner"}})
		callback, callbackOK := contract.CallbackAt(owner, 0)
		got, outcome, ok := contract.CallbackReleaseZero(callback)
		if !ownerOK || !callbackOK || !ok || got != want.behavior || outcome != want.outcome {
			t.Fatalf("zero policy = %d/%d/%v, want %d/%d", got, outcome, ok, want.behavior, want.outcome)
		}
	}

	for _, test := range []struct {
		name string
		zero CallbackReleaseZeroSpec
	}{
		{"missing", CallbackReleaseZeroSpec{}},
		{"suppress outcome", CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroSuppress, Outcome: 1}},
		{"throw normal", CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 0}},
		{"idempotent throw", CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroIdempotent, Outcome: 1}},
		{"outcome outside scope", CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := callbackReleaseZeroSpec(test.zero)
			if contract, err := Seal(&spec); err == nil || contract != nil {
				t.Fatal("invalid zero-holder policy was published")
			}
		})
	}
}

func TestCallbackReleaseZeroPolicyAffectsContentID(t *testing.T) {
	suppress := mustSeal(t, callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroSuppress}))
	throw := mustSeal(t, callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 1}))
	idempotent := mustSeal(t, callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroIdempotent, Outcome: 0}))
	if suppress.ContentID() == throw.ContentID() || throw.ContentID() == idempotent.ContentID() || suppress.ContentID() == idempotent.ContentID() {
		t.Fatal("zero-holder release policy was omitted from ContentID")
	}
}

func callbackReleaseZeroSpec(zero CallbackReleaseZeroSpec) Spec {
	closed := ValuesSpec{Tail: ValuesClosed}
	return Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"release-zero-owner"}}},
			Input:    ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}},
			Callbacks: []CallbackSpec{{
				Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable,
				Arguments: closed,
				Outcomes: []TerminalSpec{
					{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed},
					{Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed},
				},
				Lifecycle: CallbackRetainedOptionalOnce,
				Effects:   RowSpec{Tail: RowClosed},
				Release:   &CallbackReleaseSpec{Operation: 2, Input: 0, Outcome: 0, Mode: CallbackReleaseOne, Zero: zero},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"release-zero-target"}}},
			Input:    ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: closed},
				{Kind: flowkind.OutcomeThrow, Values: closed},
			},
			Effects: RowSpec{Tail: RowClosed},
		},
	}}
}
