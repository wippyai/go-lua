package target

import (
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"strings"
	"testing"
)

func TestProducedOperationUsesOneOrdinaryOperationIdentity(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{
					Result: 0, Operation: 2,
					Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureValueFormal, Ordinal: 0}},
				}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			ValuesVars: 1,
			Input:      vocabulary.ValuesSpec{Tail: vocabulary.ValuesVariable, Var: 0},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{
					Result: 0, Operation: 3,
					Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureValuesVar, Ordinal: 0}},
				}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
	}})
	factory, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}})
	if !ok || factory != 1 || contract.Operations.BoundCount() != 1 {
		t.Fatalf("factory/bound = %d/%v/%d", factory, ok, contract.Operations.BoundCount())
	}
	first, row, ok := contract.producedForResult(factory, 0, 0)
	if !ok || first != 2 || row != 0 {
		t.Fatalf("factory result = %d/%d/%v, want 2/0/true", first, row, ok)
	}
	if kind, source, ok := contract.producedCaptureAt(factory, 0, row, 0); !ok || kind != vocabulary.CaptureValueFormal || source != 0 {
		t.Fatalf("factory capture = %d/%d/%v", kind, source, ok)
	}
	second, _, ok := contract.producedForResult(first, 0, 0)
	if !ok || second != 3 {
		t.Fatalf("produced chain = %d/%v, want 3/true", second, ok)
	}
}

func TestProducedCallbackCaptureRemapsToSealedID(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wrap"}}},
			ValuesVars: 5,
			Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesVariable, Var: 0},
			Callbacks: []vocabulary.CallbackSpec{{
				Function:  vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0},
				Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			}},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind:   flowkind.OutcomeNormal,
				Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{
					Result: 0, Operation: 2,
					Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureCallback, Ordinal: 1}},
				}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}})
	wrap, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wrap"}})
	callback, ok := contract.Operations.CallbackAt(wrap, 0)
	if !ok || callback == 0 {
		t.Fatal("missing sealed callback")
	}
	functionSource, ok := contract.callbackFunction(callback)
	if !ok || functionSource.Kind != vocabulary.InputSourceValueFormal || functionSource.Ordinal != 0 {
		t.Fatalf("callback function = %#v/%v", functionSource, ok)
	}
	_, row, ok := contract.producedForResult(wrap, 0, 0)
	if !ok {
		t.Fatal("missing wrap produced operation")
	}
	kind, source, ok := contract.producedCaptureAt(wrap, 0, row, 0)
	if !ok || kind != vocabulary.CaptureCallback || vocabulary.CallbackID(source) != callback {
		t.Fatalf("callback capture = %d/%d/%v, want %d", kind, source, ok, callback)
	}
}

func TestBindingAliasesShareOneOperation(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{
			{Namespace: vocabulary.BindingModule, Owner: []string{"string"}, Member: []string{"gfind"}},
			{Namespace: vocabulary.BindingModule, Owner: []string{"string"}, Member: []string{"gmatch"}},
		},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	gfind, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"string"}, Member: []string{"gfind"}})
	gmatch, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"string"}, Member: []string{"gmatch"}})
	if gfind == 0 || gfind != gmatch || contract.Operations.BindingCount(gfind) != 2 {
		t.Fatalf("aliases = %d/%d, binding count %d", gfind, gmatch, contract.Operations.BindingCount(gfind))
	}
}

func TestBoundOperationsStayCanonicalPrefixWithProducedChildren(t *testing.T) {
	makeSpec := func(alphaRef vocabulary.SpecRef, input []vocabulary.OperationSpec) *Contract {
		input[0].Outcomes[0].Produced = []vocabulary.ProducedSpec{{Result: 0, Operation: alphaRef}}
		return mustSeal(t, Spec{Operations: input})
	}
	alpha := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	beta := vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"beta"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	child := vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	first := makeSpec(3, []vocabulary.OperationSpec{alpha, beta, child})
	second := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{beta, child, func() vocabulary.OperationSpec {
		copy := alpha
		copy.Outcomes[0].Produced = []vocabulary.ProducedSpec{{Result: 0, Operation: 2}}
		return copy
	}()}})
	for _, contract := range []*Contract{first, second} {
		if contract.Operations.BoundCount() != 2 {
			t.Fatalf("bound operation count = %d, want 2", contract.Operations.BoundCount())
		}
		for index := 0; index < contract.Operations.BoundCount(); index++ {
			op, ok := contract.Operations.OperationAt(index)
			if !ok || op != vocabulary.Operation(index+1) {
				t.Fatalf("BoundOperationAt(%d) = %d/%v, want %d/true", index, op, ok, index+1)
			}
		}
		alphaOp, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha"}})
		betaOp, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"beta"}})
		childOp, _, found := contract.producedForResult(alphaOp, 0, 0)
		if alphaOp != 1 || betaOp != 2 || !found || childOp != 3 {
			t.Fatalf("canonical prefix = alpha:%d beta:%d child:%d/%v", alphaOp, betaOp, childOp, found)
		}
	}
}

func TestProducedAnchorsRejectAmbiguityAndCycles(t *testing.T) {
	plain := func(name string, target vocabulary.SpecRef) vocabulary.OperationSpec {
		outcome := vocabulary.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}
		if target != 0 {
			outcome.Produced = []vocabulary.ProducedSpec{{Result: 0, Operation: target}}
		}
		return vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{outcome}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}}}); err == nil {
		t.Fatal("unanchored produced-only operation accepted")
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{plain("left", 3), plain("right", 3), vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}}}); err == nil {
		t.Fatal("multiply anchored produced-only operation accepted")
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, Produced: []vocabulary.ProducedSpec{{Result: 0, Operation: 2}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}, Produced: []vocabulary.ProducedSpec{{Result: 0, Operation: 1}}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}}); err == nil {
		t.Fatal("produced cycle accepted")
	}
}

func TestDeepProducedChainSealsIteratively(t *testing.T) {
	const depth = 4096
	operations := make([]vocabulary.OperationSpec, depth)
	for index := range operations {
		operations[index] = vocabulary.OperationSpec{
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
		if index == 0 {
			operations[index].Bindings = []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"root"}}}
		}
		if index+1 < len(operations) {
			operations[index].Outcomes[0].Produced = []vocabulary.ProducedSpec{{Result: 0, Operation: vocabulary.SpecRef(index + 2)}}
		}
	}
	contract := mustSeal(t, Spec{Operations: operations})
	if contract.Operations.OperationCount() != depth+1 || contract.Operations.BoundCount() != 1 {
		t.Fatalf("deep chain operations = %d/%d", contract.Operations.OperationCount(), contract.Operations.BoundCount())
	}
	current, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"root"}})
	if !ok {
		t.Fatal("root binding missing")
	}
	for index := 1; index < depth; index++ {
		next, _, found := contract.producedForResult(current, 0, 0)
		if !found {
			t.Fatalf("chain ended at %d, want step %d", index, depth)
		}
		current = next
	}
	if contract.producedCount(current, 0) != 0 {
		t.Fatal("terminal produced operation has a successor")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, _, found := contract.producedForResult(current, 0, 0); found {
			panic("terminal unexpectedly produced")
		}
	}); allocs != 0 {
		t.Fatalf("ProducedForResult allocated %f times", allocs)
	}
}

func TestProducedTypeValueCaptureIsTypedAndIndexed(t *testing.T) {
	contract := mustSeal(t, typeValueCaptureSpec(vocabulary.CaptureTypeValueFormal, 1))
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}})
	if !ok {
		t.Fatal("missing factory operation")
	}
	if formal, ok := contract.producedTypeValueCapture(op, 0, 0); !ok || formal != 1 {
		t.Fatalf("typed produced capture = %d/%v, want 1/true", formal, ok)
	}
	coexisting := typeValueCaptureSpec(vocabulary.CaptureTypeValueFormal, 1)
	coexisting.Operations[0].Outcomes[0].Produced[0].Captures = []vocabulary.CaptureSpec{
		{Kind: vocabulary.CaptureValueFormal, Ordinal: 0},
		{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: 1},
		{Kind: vocabulary.CaptureValuesVar, Ordinal: 0},
	}
	coexisting.Operations[0].ValuesVars = 1
	coexistingContract := mustSeal(t, coexisting)
	coexistingOp, _ := coexistingContract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}})
	if formal, ok := coexistingContract.producedTypeValueCapture(coexistingOp, 0, 0); !ok || formal != 1 {
		t.Fatalf("coexisting typed capture = %d/%v, want 1/true", formal, ok)
	}

	ordinary := mustSeal(t, typeValueCaptureSpec(vocabulary.CaptureValueFormal, 1))
	ordinaryOp, _ := ordinary.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}})
	if _, ok := ordinary.producedTypeValueCapture(ordinaryOp, 0, 0); ok {
		t.Fatal("ordinary value capture became a TypeValue capture")
	}
}

func TestProducedTypeValueCaptureRejectsInvalidAndDuplicateFormals(t *testing.T) {
	invalid := typeValueCaptureSpec(vocabulary.CaptureTypeValueFormal, 2)
	if _, err := testSeal(&invalid); err == nil || !strings.Contains(err.Error(), "TypeValueFormal outside scope") {
		t.Fatalf("out-of-range TypeValue capture error = %v", err)
	}

	duplicate := typeValueCaptureSpec(vocabulary.CaptureTypeValueFormal, 0)
	duplicate.Operations[0].Outcomes[0].Produced[0].Captures = []vocabulary.CaptureSpec{
		{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: 0},
		{Kind: vocabulary.CaptureValueFormal, Ordinal: 1},
		{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: 1},
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
			spec := typeValueCaptureSpec(vocabulary.CaptureTypeValueFormal, 1)
			test.mutate(&spec)
			if _, err := testSeal(&spec); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("TypeValue/FreshFunction error = %v, want %q", err, test.want)
			}
		})
	}

	// Ordinary Produced captures retain their established law and do not
	// acquire a nominal freshness requirement.
	ordinary := typeValueCaptureSpec(vocabulary.CaptureValueFormal, 1)
	ordinary.Operations[0].Outcomes[0].FreshResults = nil
	contract := mustSeal(t, ordinary)
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}})
	if !ok {
		t.Fatal("ordinary Produced operation disappeared")
	}
	if _, produced, ok := contract.producedForResult(op, 0, 0); !ok || produced != 0 {
		t.Fatal("ordinary Produced row changed without TypeValue capture")
	}
}

func TestProducedTypeValueFreshFunctionCanonicalRoundTrip(t *testing.T) {
	spec := func(reverse bool) Spec {
		produced := []vocabulary.ProducedSpec{
			{Result: 0, Operation: 2, Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: 0}}},
			{Result: 1, Operation: 3, Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: 1}}},
		}
		fresh := []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}, {Result: 1, Kind: schematype.FreshClassFunction}}
		if reverse {
			produced[0], produced[1] = produced[1], produced[0]
			fresh[0], fresh[1] = fresh[1], fresh[0]
		}
		child := func() vocabulary.OperationSpec {
			return vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
		}
		return Spec{Operations: []vocabulary.OperationSpec{
			{
				Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory-roundtrip"}}},
				Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed},
				Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed}, Produced: produced, FreshResults: fresh}},
				Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
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
		op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory-roundtrip"}})
		if !ok {
			t.Fatal("round-trip factory operation absent")
		}
		for result := uint32(0); result < 2; result++ {
			_, produced, producedOK := contract.producedForResult(op, 0, result)
			formal, captureOK := contract.producedTypeValueCapture(op, 0, produced)
			ordinal, kind, fresh, freshOK := contract.freshResultForResult(op, 0, result)
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
	produced := make([]vocabulary.ProducedSpec, width)
	fresh := make([]vocabulary.FreshResultSpec, width)
	operations := make([]vocabulary.OperationSpec, width+1)
	for index := range input {
		input[index], output[index] = testAny, testAny
		produced[index] = vocabulary.ProducedSpec{Result: uint32(index), Operation: vocabulary.SpecRef(index + 2), Captures: []vocabulary.CaptureSpec{{Kind: vocabulary.CaptureTypeValueFormal, Ordinal: uint32(index)}}}
		fresh[index] = vocabulary.FreshResultSpec{Result: uint32(index), Kind: schematype.FreshClassFunction}
		operations[index+1] = vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	}
	operations[0] = vocabulary.OperationSpec{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide"}}}, Input: vocabulary.ValuesSpec{Fixed: input, Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: output, Tail: vocabulary.ValuesClosed}, Produced: produced, FreshResults: fresh}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	contract := mustSeal(t, Spec{Operations: operations})
	op, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide"}})
	for _, index := range []int{0, width - 1} {
		if formal, ok := contract.producedTypeValueCapture(op, 0, index); !ok || int(formal) != index {
			t.Fatalf("wide typed capture %d = %d/%v", index, formal, ok)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.producedTypeValueCapture(op, 0, width-1); !ok {
			panic("wide typed capture disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("wide typed capture query allocations = %f, want 0", allocs)
	}
}

func typeValueCaptureSpec(kind vocabulary.CaptureKind, ordinal uint32) Spec {
	return Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"factory"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString, testNumber}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				Produced:     []vocabulary.ProducedSpec{{Result: 0, Operation: 2, Captures: []vocabulary.CaptureSpec{{Kind: kind, Ordinal: ordinal}}}},
				FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassFunction}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}}
}

func TestFreshResultsSealDenselyAndAllowConjunctiveRelations(t *testing.T) {
	child := vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh"}}},
		ValuesVars: 1,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0),
			Outcomes: callbackOutcomes(0, 0, 0, 0, 0), Lifecycle: vocabulary.CallbackRetainedOptionalOnce,
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed},
				FreshResults: []vocabulary.FreshResultSpec{{Result: 1, Kind: schematype.FreshClassFunction}, {Result: 0, Kind: schematype.FreshClassTable}},
				Produced:     []vocabulary.ProducedSpec{{Result: 0, Operation: 2}},
			},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
				FreshResults:    []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassError}},
				CallbackResults: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}},
			},
		},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}, child}})
	op, _ := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh"}})
	if count := contract.FreshResultCount(op, 0); count != 2 {
		t.Fatalf("normal fresh count = %d, want 2", count)
	}
	for index, want := range []struct {
		result  uint32
		ordinal uint32
		kind    schematype.FreshClass
	}{{0, 0, schematype.FreshClassTable}, {1, 1, schematype.FreshClassFunction}} {
		result, ordinal, kind, found := contract.FreshResultAt(op, 0, index)
		if !found || result != want.result || ordinal != want.ordinal || kind != want.kind {
			t.Fatalf("FreshResultAt(%d) = %d/%d/%d/%v, want %d/%d/%d/true", index, result, ordinal, kind, found, want.result, want.ordinal, want.kind)
		}
	}
	if ordinal, kind, index, found := contract.freshResultForResult(op, 0, 1); !found || ordinal != 1 || kind != schematype.FreshClassFunction || index != 1 {
		t.Fatalf("FreshResultForResult = %d/%d/%d/%v, want 1/function/1/true", ordinal, kind, index, found)
	}
	if _, _, _, found := contract.FreshResultAt(op, 0, 2); found {
		t.Fatal("FreshResultAt accepted outside range")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = contract.FreshResultCount(op, 0)
		_, _, _, _ = contract.FreshResultAt(op, 0, 1)
		_, _, _, _ = contract.freshResultForResult(op, 0, 1)
	}); allocations != 0 {
		t.Fatalf("FreshResult queries allocated %f times", allocations)
	}
}

func TestFreshResultsRejectAliasesDuplicatesAndInvalidKinds(t *testing.T) {
	base := vocabulary.OperationSpec{
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: vocabulary.ValuesClosed}, FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name string
		edit func(*vocabulary.OperationSpec)
	}{
		{"duplicate fixed result", func(op *vocabulary.OperationSpec) {
			op.Outcomes[0].FreshResults = append(op.Outcomes[0].FreshResults, vocabulary.FreshResultSpec{Result: 0, Kind: schematype.FreshClassFunction})
		}},
		{"outside fixed prefix", func(op *vocabulary.OperationSpec) { op.Outcomes[0].FreshResults[0].Result = 2 }},
		{"invalid kind", func(op *vocabulary.OperationSpec) { op.Outcomes[0].FreshResults[0].Kind = schematype.FreshClassInvalid }},
		{"result alias overlap", func(op *vocabulary.OperationSpec) {
			op.Outcomes[0].ResultAliases = []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}}
			op.Input.Fixed = []schematype.Type{testAny}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Input.Fixed = append([]schematype.Type(nil), base.Input.Fixed...)
			op.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
			op.Outcomes[0].FreshResults = append([]vocabulary.FreshResultSpec(nil), base.Outcomes[0].FreshResults...)
			test.edit(&op)
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid FreshResult relation")
			}
		})
	}
}

func TestFreshResultDistinguishesSameShapeCasesAndProducedAnchors(t *testing.T) {
	caseResult := func(kind schematype.FreshClass, child vocabulary.SpecRef) vocabulary.OutcomeSpec {
		return vocabulary.OutcomeSpec{
			Kind:         flowkind.OutcomeNormal,
			Values:       vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: kind}},
			Produced:     []vocabulary.ProducedSpec{{Result: 0, Operation: child}},
		}
	}
	child := func() vocabulary.OperationSpec {
		return vocabulary.OperationSpec{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}
	}
	makeSpec := func(outcomes []vocabulary.OutcomeSpec) Spec {
		return Spec{Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-cases"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: outcomes, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}, child(), child()}}
	}
	left := mustSeal(t, makeSpec([]vocabulary.OutcomeSpec{caseResult(schematype.FreshClassTable, 2), caseResult(schematype.FreshClassFunction, 3)}))
	right := mustSeal(t, makeSpec([]vocabulary.OutcomeSpec{caseResult(schematype.FreshClassFunction, 3), caseResult(schematype.FreshClassTable, 2)}))
	assertPublicContractEqual(t, left, right)
	if left.ContentID() != right.ContentID() {
		t.Fatal("FreshResult outcome permutation changed ContentID")
	}
	op, _ := left.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"fresh-cases"}})
	if count := left.Operations.OutcomeCount(op); count != 2 {
		t.Fatalf("same-shape Fresh outcomes = %d, want 2", count)
	}
}

func TestResultAliasesCanonicalizeAndRemainConjunctive(t *testing.T) {
	contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"aliases"}}},
			Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny, testString}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{Result: 0, Operation: 2}},
				ResultAliases: []vocabulary.ResultAliasSpec{
					{Result: 1, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}},
					{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}},
				},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"aliases"}})
	if !ok || contract.resultAliasCount(op, 0) != 2 {
		t.Fatalf("result aliases missing: op=%d ok=%v count=%d", op, ok, contract.resultAliasCount(op, 0))
	}
	result, kind, source, ok := contract.resultAliasAt(op, 0, 0)
	if !ok || result != 0 || kind != vocabulary.InputSourceValueFormal || source != 0 {
		t.Fatalf("first alias = %d/%d/%d/%v", result, kind, source, ok)
	}
	kind, source, row, ok := contract.resultAliasForResult(op, 0, 1)
	if !ok || kind != vocabulary.InputSourceValueFormal || source != 1 || row != 1 {
		t.Fatalf("alias lookup = %d/%d/%d/%v", kind, source, row, ok)
	}
	if _, _, _, ok := contract.resultAliasForResult(op, 0, 2); ok {
		t.Fatal("unknown result alias found")
	}
	if got := testing.AllocsPerRun(100, func() { _, _, _, _ = contract.resultAliasForResult(op, 0, 1) }); got != 0 {
		t.Fatalf("ResultAliasForResult allocated %f times", got)
	}
}

func TestResultAliasCoexistsWithCallbackResult(t *testing.T) {
	contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{
		Bindings:   []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-alias"}}},
		ValuesVars: 5,
		Input:      vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
		Callbacks:  []vocabulary.CallbackSpec{{Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}}},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind:            flowkind.OutcomeNormal,
			Values:          vocabulary.ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: vocabulary.ValuesClosed},
			CallbackResults: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}},
			ResultAliases:   []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"callback-alias"}})
	if !ok || contract.callbackResultCount(op, 0) != 1 || contract.resultAliasCount(op, 0) != 1 {
		t.Fatalf("callback/alias conjunction missing: op=%d ok=%v callbacks=%d aliases=%d", op, ok, contract.callbackResultCount(op, 0), contract.resultAliasCount(op, 0))
	}
}

func TestResultAliasesRejectInvalidAndDuplicate(t *testing.T) {
	base := vocabulary.OperationSpec{
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
	for _, test := range []struct {
		name    string
		aliases []vocabulary.ResultAliasSpec
	}{
		{"result outside prefix", []vocabulary.ResultAliasSpec{{Result: 1, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}}},
		{"Values variable source", []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar}}}},
		{"formal outside scope", []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}}}},
		{"duplicate result", []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}, {Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			op := base
			op.Outcomes = append([]vocabulary.OutcomeSpec(nil), base.Outcomes...)
			op.Outcomes[0].ResultAliases = test.aliases
			if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{op}}); err == nil {
				t.Fatal("Seal accepted invalid result alias")
			}
		})
	}
}

func TestResultAliasPermutationAndWideLookup(t *testing.T) {
	const width = 1024
	fixed := make([]schematype.Type, width)
	aliases := make([]vocabulary.ResultAliasSpec, width)
	for index := range fixed {
		fixed[index] = testAny
		aliases[width-index-1] = vocabulary.ResultAliasSpec{
			Result: uint32(width - index - 1), Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(width - index - 1)},
		}
	}
	seal := func(aliasRows []vocabulary.ResultAliasSpec) *Contract {
		contract, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide-alias"}}},
			Input:    vocabulary.ValuesSpec{Fixed: fixed, Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: fixed, Tail: vocabulary.ValuesClosed}, ResultAliases: aliasRows}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return contract
	}
	left := seal(aliases)
	sorted := append([]vocabulary.ResultAliasSpec(nil), aliases...)
	for index := range sorted {
		sorted[index] = vocabulary.ResultAliasSpec{Result: uint32(index), Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(index)}}
	}
	right := seal(sorted)
	assertPublicContractEqual(t, left, right)
	op, _ := left.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"wide-alias"}})
	if kind, source, row, ok := left.resultAliasForResult(op, 0, width-1); !ok || kind != vocabulary.InputSourceValueFormal || source != width-1 || row != width-1 {
		t.Fatalf("wide alias lookup = %d/%d/%d/%v", kind, source, row, ok)
	}
	if got := testing.AllocsPerRun(100, func() { _, _, _, _ = left.resultAliasForResult(op, 0, width-1) }); got != 0 {
		t.Fatalf("wide ResultAliasForResult allocated %f times", got)
	}
}
