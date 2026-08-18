package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func identityOperation(name string, input, output interface{}) vocabulary.OperationSpec {
	declarations, formals := testOperationTypes(input, output)
	return vocabulary.OperationSpec{
		Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		TypeFormals: formals,
		Input:       vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[1]}, Tail: vocabulary.ValuesClosed},
			ResultAliases: []vocabulary.ResultAliasSpec{{Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}}},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func freshOperation(name string, output interface{}, fresh schematype.FreshClass) vocabulary.OperationSpec {
	declarations, formals := testOperationTypes(output)
	return vocabulary.OperationSpec{
		Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		TypeFormals: formals,
		Input:       vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed},
			FreshResults: []vocabulary.FreshResultSpec{{Result: 0, Kind: fresh}},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func producedOperation(name string, output interface{}) Spec {
	declarations, formals := testOperationTypes(output)
	return Spec{Operations: []vocabulary.OperationSpec{
		{
			Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
			TypeFormals: formals,
			Input:       vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed},
				Produced: []vocabulary.ProducedSpec{{Result: 0, Operation: 2}},
			}},
			Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		},
		{Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
	}}
}

func callbackResultOperation(name string, callbackType, output interface{}, admission schematype.CallableAdmission) vocabulary.OperationSpec {
	declarations, formals := testOperationTypes(callbackType, output)
	return vocabulary.OperationSpec{
		Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{name}}},
		TypeFormals: formals,
		Input:       vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed},
		Callbacks: []vocabulary.CallbackSpec{{
			Function: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal}, Admission: admission,
			Arguments: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeReturn, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeYield, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
				{Kind: flowkind.OutcomeCancel, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}},
			},
			Lifecycle: vocabulary.CallbackRetainedOptionalOnce, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
		Outcomes: []vocabulary.OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[1]}, Tail: vocabulary.ValuesClosed},
			CallbackResults: []vocabulary.CallbackResultSpec{{Result: 0, Callback: 1}},
		}},
		Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}
}

func TestIdentityAnnotationsRejectConcreteContradictions(t *testing.T) {
	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"alias-string-number", Spec{Operations: []vocabulary.OperationSpec{identityOperation("alias-string-number", testString, testNumber)}}},
		{"fresh-table-string", Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-table-string", testString, schematype.FreshClassTable)}}},
		{"fresh-function-string", Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-function-string", testString, schematype.FreshClassFunction)}}},
		{"fresh-thread-string", Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-thread-string", testString, schematype.FreshClassThread)}}},
		{"produced-string", producedOperation("produced-string", testString)},
		{"callback-string", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("callback-string", testString, testString, schematype.CallableAdmissionOrdinary)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := testSeal(&item.spec); err == nil {
				t.Fatal("concrete identity contradiction sealed")
			}
		})
	}
}

func TestIdentityAnnotationsKeepProvenAndGradualCases(t *testing.T) {
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{identityOperation("alias-integer-number", testInteger, testNumber)}}); err != nil {
		t.Fatalf("integer alias to number rejected: %v", err)
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{identityOperation("alias-any-string", testAny, testString)}}); err != nil {
		t.Fatalf("Any alias to string rejected: %v", err)
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-table-marker", testBuiltinTableTop(), schematype.FreshClassTable)}}); err != nil {
		t.Fatalf("table marker freshness rejected: %v", err)
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-table-record", testRawRecord(testRawRecordParts{}), schematype.FreshClassTable)}}); err != nil {
		t.Fatalf("record freshness rejected: %v", err)
	}
	function := testRawFunction()
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-function", function, schematype.FreshClassFunction)}}); err != nil {
		t.Fatalf("function freshness rejected: %v", err)
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{freshOperation("fresh-thread-any", testAny, schematype.FreshClassThread)}}); err != nil {
		t.Fatalf("gradual thread freshness rejected: %v", err)
	}
	producedFunction := producedOperation("produced-function", function)
	if _, err := testSeal(&producedFunction); err != nil {
		t.Fatalf("produced direct function rejected: %v", err)
	}
	producedAny := producedOperation("produced-any", testAny)
	if _, err := testSeal(&producedAny); err != nil {
		t.Fatalf("produced Any rejected: %v", err)
	}
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("callback-function", function, function, schematype.CallableAdmissionDirectFunction)}}); err != nil {
		t.Fatalf("direct callback function rejected: %v", err)
	}

	callMeta := testRawRecord(testRawRecordParts{StaticMembers: []testRawStaticMember{{Kind: testRawStaticMemberStringIndex, Name: "__call", Type: function}}})
	ordinaryCallable := testRawRecord(testRawRecordParts{Metatable: callMeta})
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("callback-ordinary-callable", ordinaryCallable, ordinaryCallable, schematype.CallableAdmissionOrdinary)}}); err != nil {
		t.Fatalf("ordinary callable callback result rejected: %v", err)
	}
	producedCallableRecord := producedOperation("produced-callable-record", ordinaryCallable)
	if _, err := testSeal(&producedCallableRecord); err == nil {
		t.Fatal("Produced accepted a record callable only through __call")
	}
}

func TestIdentityAliasDoesNotReplaceFormalWithItsConstraint(t *testing.T) {
	formal := testNewTypeParam("T", testNumber)
	equal := identityOperation("alias-formal-equal", formal, formal)
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{equal}}); err != nil {
		t.Fatalf("T to T alias rejected: %v", err)
	}
	narrow := identityOperation("alias-formal-narrow", testInteger, formal)
	if _, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{narrow}}); err == nil {
		t.Fatal("integer alias was accepted as unconstrained T:number")
	}
}

func TestIdentityAnnotationsUnfoldRecursiveAndInstantiatedSemanticHeads(t *testing.T) {
	record := testRawRecord(testRawRecordParts{})
	function := testRawFunction()

	recursiveRecord := testRawRecursive("RecursiveRecord", func(self testRawType) testRawType { return record })
	recursiveFunction := testRawRecursive("RecursiveFunction", func(self testRawType) testRawType { return function })
	recursiveString := testRawRecursive("RecursiveString", func(self testRawType) testRawType { return testRawString })

	recordParam := testNewTypeParam("T", nil)
	recordGeneric := testRawGeneric("RecordBox", []*testRawTypeParam{recordParam}, record)
	functionParam := testNewTypeParam("T", nil)
	functionGeneric := testRawGeneric("FunctionBox", []*testRawTypeParam{functionParam}, function)
	stringParam := testNewTypeParam("T", nil)
	stringGeneric := testRawGeneric("Id", []*testRawTypeParam{stringParam}, stringParam)
	instantiatedRecord := testRawInstantiate(recordGeneric, testRawString)
	instantiatedFunction := testRawInstantiate(functionGeneric, testRawString)
	instantiatedString := testRawInstantiate(stringGeneric, testRawString)

	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"recursive-record-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("recursive-record-fresh-table", recursiveRecord, schematype.FreshClassTable)}}},
		{"instantiated-record-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("instantiated-record-fresh-table", instantiatedRecord, schematype.FreshClassTable)}}},
		{"recursive-function-fresh-function", Spec{Operations: []vocabulary.OperationSpec{freshOperation("recursive-function-fresh-function", recursiveFunction, schematype.FreshClassFunction)}}},
		{"instantiated-function-fresh-function", Spec{Operations: []vocabulary.OperationSpec{freshOperation("instantiated-function-fresh-function", instantiatedFunction, schematype.FreshClassFunction)}}},
		{"recursive-function-produced", producedOperation("recursive-function-produced", recursiveFunction)},
		{"instantiated-function-produced", producedOperation("instantiated-function-produced", instantiatedFunction)},
		{"recursive-function-direct-callback", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("recursive-function-direct-callback", recursiveFunction, recursiveFunction, schematype.CallableAdmissionDirectFunction)}}},
		{"instantiated-function-direct-callback", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("instantiated-function-direct-callback", instantiatedFunction, instantiatedFunction, schematype.CallableAdmissionDirectFunction)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := testSeal(&item.spec); err != nil {
				t.Fatalf("semantic wrapper rejected: %v", err)
			}
		})
	}

	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"recursive-string-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("recursive-string-fresh-table", recursiveString, schematype.FreshClassTable)}}},
		{"instantiated-string-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("instantiated-string-fresh-table", instantiatedString, schematype.FreshClassTable)}}},
		{"recursive-string-produced", producedOperation("recursive-string-produced", recursiveString)},
		{"instantiated-string-produced", producedOperation("instantiated-string-produced", instantiatedString)},
		{"recursive-string-direct-callback", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("recursive-string-direct-callback", recursiveString, recursiveString, schematype.CallableAdmissionDirectFunction)}}},
		{"instantiated-string-direct-callback", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("instantiated-string-direct-callback", instantiatedString, instantiatedString, schematype.CallableAdmissionDirectFunction)}}},
		{"recursive-string-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{freshOperation("recursive-string-fresh-thread", recursiveString, schematype.FreshClassThread)}}},
		{"instantiated-string-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{freshOperation("instantiated-string-fresh-thread", instantiatedString, schematype.FreshClassThread)}}},
		{"recursive-record-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{freshOperation("recursive-record-fresh-thread", recursiveRecord, schematype.FreshClassThread)}}},
		{"instantiated-record-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{freshOperation("instantiated-record-fresh-thread", instantiatedRecord, schematype.FreshClassThread)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := testSeal(&item.spec); err == nil {
				t.Fatal("concrete semantic wrapper contradiction sealed")
			}
		})
	}
}

func TestIdentityAnnotationsRecursiveCyclesRequireAProductiveHead(t *testing.T) {
	self := testRawRecursive("Self", func(self testRawType) testRawType { return self })
	left := testRawRecursivePlaceholder("Left")
	right := testRawRecursivePlaceholder("Right")
	left.SetBody(right)
	right.SetBody(left)
	mutualTable := testRawRecursivePlaceholder("MutualTable")
	mutualTable.SetBody(testRawRecursive("MutualTableBody", func(self testRawType) testRawType { return testRawRecord(testRawRecordParts{}) }))

	for _, item := range []struct {
		name string
		spec Spec
		want bool
	}{
		{"self-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("self-fresh-table", self, schematype.FreshClassTable)}}, false},
		{"mutual-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("mutual-fresh-table", left, schematype.FreshClassTable)}}, false},
		{"self-fresh-thread-opaque", Spec{Operations: []vocabulary.OperationSpec{freshOperation("self-fresh-thread-opaque", self, schematype.FreshClassThread)}}, true},
		{"mutual-table-fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("mutual-table-fresh-table", mutualTable, schematype.FreshClassTable)}}, true},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := testSeal(&item.spec)
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func TestIdentityAnnotationsDeepSemanticWrappersDoNotUseTheHostStack(t *testing.T) {
	const depth = 20_000
	deeplyRecursive := func(head testRawType) testRawType {
		for range depth {
			body := head
			head = testRawRecursive("", func(self testRawType) testRawType { return body })
		}
		return head
	}

	deepRecord := deeplyRecursive(testRawRecord(testRawRecordParts{}))
	deepFunction := deeplyRecursive(testRawFunction())
	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"fresh-table", Spec{Operations: []vocabulary.OperationSpec{freshOperation("deep-fresh-table", deepRecord, schematype.FreshClassTable)}}},
		{"produced", producedOperation("deep-produced", deepFunction)},
		{"direct-callback", Spec{Operations: []vocabulary.OperationSpec{callbackResultOperation("deep-direct-callback", deepFunction, deepFunction, schematype.CallableAdmissionDirectFunction)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := testSeal(&item.spec); err != nil {
				t.Fatalf("deep semantic wrapper rejected: %v", err)
			}
		})
	}
}

func TestIdentityAnnotationsFormalBoundsAreOnlyNegativeEvidence(t *testing.T) {
	stringFormal := testNewTypeParam("T", testString)
	functionFormal := testNewTypeParam("F", testFunction())
	unconstrained := testNewTypeParam("U", nil)

	for _, item := range []struct {
		name string
		spec Spec
		want bool
	}{
		{"string-bound-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{withTypeFormal(freshOperation("string-bound-fresh-thread", stringFormal, schematype.FreshClassThread), stringFormal)}}, false},
		{"function-bound-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{withTypeFormal(freshOperation("function-bound-fresh-thread", functionFormal, schematype.FreshClassThread), functionFormal)}}, false},
		{"unconstrained-fresh-thread", Spec{Operations: []vocabulary.OperationSpec{withTypeFormal(freshOperation("unconstrained-fresh-thread", unconstrained, schematype.FreshClassThread), unconstrained)}}, true},
		{"function-bound-fresh-function", Spec{Operations: []vocabulary.OperationSpec{withTypeFormal(freshOperation("function-bound-fresh-function", functionFormal, schematype.FreshClassFunction), functionFormal)}}, false},
		{"function-bound-direct-callback", Spec{Operations: []vocabulary.OperationSpec{withTypeFormal(callbackResultOperation("function-bound-direct-callback", functionFormal, functionFormal, schematype.CallableAdmissionDirectFunction), functionFormal)}}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := testSeal(&item.spec)
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func TestIdentityAnnotationsMetaIsReflectionOnly(t *testing.T) {
	meta := testMeta(testString)
	for _, item := range []struct {
		name  string
		fresh schematype.FreshClass
		want  bool
	}{
		{"reflection", schematype.FreshClassReflection, true},
		{"thread", schematype.FreshClassThread, false},
		{"userdata", schematype.FreshClassUserdata, false},
		{"error", schematype.FreshClassError, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := testSeal(&Spec{Operations: []vocabulary.OperationSpec{freshOperation("meta-"+item.name, meta, item.fresh)}})
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func withTypeFormal(operation vocabulary.OperationSpec, formal *testRawTypeParam) vocabulary.OperationSpec {
	return operation
}
