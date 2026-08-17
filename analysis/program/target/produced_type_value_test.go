package target

import (
	"strings"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestProducedTypeValueCaptureIsTypedAndIndexed(t *testing.T) {
	contract := mustSeal(t, typeValueCaptureSpec(CaptureTypeValueFormal, 1))
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory"}})
	if !ok {
		t.Fatal("missing factory operation")
	}
	if formal, ok := contract.ProducedTypeValueCapture(op, 0, 0); !ok || formal != 1 {
		t.Fatalf("typed produced capture = %d/%v, want 1/true", formal, ok)
	}
	coexisting := typeValueCaptureSpec(CaptureTypeValueFormal, 1)
	coexisting.Operations[0].Outcomes[0].Produced[0].Captures = []CaptureSpec{
		{Kind: CaptureValueFormal, Ordinal: 0},
		{Kind: CaptureTypeValueFormal, Ordinal: 1},
		{Kind: CaptureValuesVar, Ordinal: 0},
	}
	coexisting.Operations[0].ValuesVars = 1
	coexistingContract := mustSeal(t, coexisting)
	coexistingOp, _ := coexistingContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory"}})
	if formal, ok := coexistingContract.ProducedTypeValueCapture(coexistingOp, 0, 0); !ok || formal != 1 {
		t.Fatalf("coexisting typed capture = %d/%v, want 1/true", formal, ok)
	}

	ordinary := mustSeal(t, typeValueCaptureSpec(CaptureValueFormal, 1))
	ordinaryOp, _ := ordinary.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory"}})
	if _, ok := ordinary.ProducedTypeValueCapture(ordinaryOp, 0, 0); ok {
		t.Fatal("ordinary value capture became a TypeValue capture")
	}
}

func TestProducedTypeValueCaptureRejectsInvalidAndDuplicateFormals(t *testing.T) {
	invalid := typeValueCaptureSpec(CaptureTypeValueFormal, 2)
	if _, err := testSeal(&invalid); err == nil || !strings.Contains(err.Error(), "TypeValueFormal outside scope") {
		t.Fatalf("out-of-range TypeValue capture error = %v", err)
	}

	duplicate := typeValueCaptureSpec(CaptureTypeValueFormal, 0)
	duplicate.Operations[0].Outcomes[0].Produced[0].Captures = []CaptureSpec{
		{Kind: CaptureTypeValueFormal, Ordinal: 0},
		{Kind: CaptureValueFormal, Ordinal: 1},
		{Kind: CaptureTypeValueFormal, Ordinal: 1},
	}
	if _, err := testSeal(&duplicate); err == nil || !strings.Contains(err.Error(), "more than one TypeValueFormal") {
		t.Fatalf("duplicate TypeValue capture error = %v", err)
	}
}

func TestProducedTypeValueCaptureRequiresExactFreshFunctionResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Spec)
		want   string
	}{
		{
			name: "missing",
			mutate: func(spec *Spec) {
				spec.Operations[0].Outcomes[0].FreshResults = nil
			},
			want: "lacks FreshFunction",
		},
		{
			name: "wrong kind",
			mutate: func(spec *Spec) {
				spec.Operations[0].Outcomes[0].FreshResults[0].Kind = schematype.FreshClassTable
			},
			want: "want FreshFunction",
		},
		{
			name: "duplicate result",
			mutate: func(spec *Spec) {
				fresh := spec.Operations[0].Outcomes[0].FreshResults[0]
				spec.Operations[0].Outcomes[0].FreshResults = append(spec.Operations[0].Outcomes[0].FreshResults, fresh)
			},
			want: "duplicate fresh outcome result",
		},
		{
			name: "foreign result",
			mutate: func(spec *Spec) {
				outcome := &spec.Operations[0].Outcomes[0]
				outcome.Values.Fixed = append(outcome.Values.Fixed, testAny)
				outcome.FreshResults[0].Result = 1
			},
			want: "lacks FreshFunction",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := typeValueCaptureSpec(CaptureTypeValueFormal, 1)
			test.mutate(&spec)
			if _, err := testSeal(&spec); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("TypeValue/FreshFunction error = %v, want %q", err, test.want)
			}
		})
	}

	// Ordinary Produced captures retain their established law and do not
	// acquire a nominal freshness requirement.
	ordinary := typeValueCaptureSpec(CaptureValueFormal, 1)
	ordinary.Operations[0].Outcomes[0].FreshResults = nil
	contract := mustSeal(t, ordinary)
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory"}})
	if !ok {
		t.Fatal("ordinary Produced operation disappeared")
	}
	if _, produced, ok := contract.ProducedForResult(op, 0, 0); !ok || produced != 0 {
		t.Fatal("ordinary Produced row changed without TypeValue capture")
	}
}

func TestProducedTypeValueFreshFunctionCanonicalRoundTrip(t *testing.T) {
	spec := func(reverse bool) Spec {
		produced := []ProducedSpec{
			{Result: 0, Operation: 2, Captures: []CaptureSpec{{Kind: CaptureTypeValueFormal, Ordinal: 0}}},
			{Result: 1, Operation: 3, Captures: []CaptureSpec{{Kind: CaptureTypeValueFormal, Ordinal: 1}}},
		}
		fresh := []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}, {Result: 1, Kind: schematype.FreshClassFunction}}
		if reverse {
			produced[0], produced[1] = produced[1], produced[0]
			fresh[0], fresh[1] = fresh[1], fresh[0]
		}
		child := func() OperationSpec {
			return OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
		}
		return Spec{Operations: []OperationSpec{
			{
				Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"factory-roundtrip"}}},
				Input:    ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed},
				Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed}, Produced: produced, FreshResults: fresh}},
				Effects:  RowSpec{Tail: RowClosed},
			},
			child(), child(),
		}}
	}
	left := mustSeal(t, spec(false))
	right := mustSeal(t, spec(true))
	if left.ContentID() != right.ContentID() {
		t.Fatal("Produced/FreshFunction authoring permutation changed Contract identity")
	}
	for _, contract := range []*Contract{left, right} {
		op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"factory-roundtrip"}})
		if !ok {
			t.Fatal("round-trip factory operation absent")
		}
		for result := uint32(0); result < 2; result++ {
			_, produced, producedOK := contract.ProducedForResult(op, 0, result)
			formal, captureOK := contract.ProducedTypeValueCapture(op, 0, produced)
			ordinal, kind, fresh, freshOK := contract.FreshResultForResult(op, 0, result)
			if !producedOK || !captureOK || uint32(formal) != result || !freshOK || kind != schematype.FreshClassFunction || fresh != int(ordinal) {
				t.Fatalf("result %d round trip = produced:%d/%v capture:%d/%v fresh:%d/%d/%d/%v", result, produced, producedOK, formal, captureOK, ordinal, kind, fresh, freshOK)
			}
		}
	}
}

func TestProducedTypeValueCaptureWideSealAndQueryStayDirect(t *testing.T) {
	const width = 2048
	input := make([]schematype.Type, width)
	output := make([]schematype.Type, width)
	produced := make([]ProducedSpec, width)
	fresh := make([]FreshResultSpec, width)
	operations := make([]OperationSpec, width+1)
	for index := range input {
		input[index], output[index] = testAny, testAny
		produced[index] = ProducedSpec{Result: uint32(index), Operation: SpecRef(index + 2), Captures: []CaptureSpec{{Kind: CaptureTypeValueFormal, Ordinal: uint32(index)}}}
		fresh[index] = FreshResultSpec{Result: uint32(index), Kind: schematype.FreshClassFunction}
		operations[index+1] = OperationSpec{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}}
	}
	operations[0] = OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"wide"}}}, Input: ValuesSpec{Fixed: input, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: output, Tail: ValuesClosed}, Produced: produced, FreshResults: fresh}}, Effects: RowSpec{Tail: RowClosed}}
	contract := mustSeal(t, Spec{Operations: operations})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"wide"}})
	for _, index := range []int{0, width - 1} {
		if formal, ok := contract.ProducedTypeValueCapture(op, 0, index); !ok || int(formal) != index {
			t.Fatalf("wide typed capture %d = %d/%v", index, formal, ok)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.ProducedTypeValueCapture(op, 0, width-1); !ok {
			panic("wide typed capture disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("wide typed capture query allocations = %f, want 0", allocs)
	}
}

func typeValueCaptureSpec(kind CaptureKind, ordinal uint32) Spec {
	return Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"factory"}}},
			Input:    ValuesSpec{Fixed: []schematype.Type{testString, testNumber}, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed},
				Produced:     []ProducedSpec{{Result: 0, Operation: 2, Captures: []CaptureSpec{{Kind: kind, Ordinal: ordinal}}}},
				FreshResults: []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}},
	}}
}
