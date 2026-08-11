package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func identityOperation(name string, input, output typ.Type) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []typ.Type{input}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{output}, Tail: ValuesClosed},
			ResultAliases: []ResultAliasSpec{{Result: 0, Source: InputSource{Kind: InputSourceValueFormal}}},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func freshOperation(name string, output typ.Type, fresh FreshKind) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{output}, Tail: ValuesClosed},
			FreshResults: []FreshResultSpec{{Result: 0, Kind: fresh}},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func producedOperation(name string, output typ.Type) Spec {
	return Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
			Input:    ValuesSpec{Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{
				Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{output}, Tail: ValuesClosed},
				Produced: []ProducedSpec{{Result: 0, Operation: 2}},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{Input: ValuesSpec{Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}, Effects: RowSpec{Tail: RowClosed}},
	}}
}

func callbackResultOperation(name string, callbackType, output typ.Type, admission Admission) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []typ.Type{callbackType}, Tail: ValuesClosed},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: admission,
			Arguments: ValuesSpec{Tail: ValuesClosed},
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeReturn, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeCancel, Values: ValuesSpec{Tail: ValuesClosed}},
			},
			Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{{
			Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{output}, Tail: ValuesClosed},
			CallbackResults: []CallbackResultSpec{{Result: 0, Callback: 1}},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestIdentityAnnotationsRejectConcreteContradictions(t *testing.T) {
	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"alias-string-number", Spec{Operations: []OperationSpec{identityOperation("alias-string-number", typ.String, typ.Number)}}},
		{"fresh-table-string", Spec{Operations: []OperationSpec{freshOperation("fresh-table-string", typ.String, FreshTable)}}},
		{"fresh-function-string", Spec{Operations: []OperationSpec{freshOperation("fresh-function-string", typ.String, FreshFunction)}}},
		{"fresh-thread-string", Spec{Operations: []OperationSpec{freshOperation("fresh-thread-string", typ.String, FreshThread)}}},
		{"produced-string", producedOperation("produced-string", typ.String)},
		{"callback-string", Spec{Operations: []OperationSpec{callbackResultOperation("callback-string", typ.String, typ.String, OrdinaryCallable)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := Seal(&item.spec); err == nil {
				t.Fatal("concrete identity contradiction sealed")
			}
		})
	}
}

func TestIdentityAnnotationsKeepProvenAndGradualCases(t *testing.T) {
	if _, err := Seal(&Spec{Operations: []OperationSpec{identityOperation("alias-integer-number", typ.Integer, typ.Number)}}); err != nil {
		t.Fatalf("integer alias to number rejected: %v", err)
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{identityOperation("alias-any-string", typ.Any, typ.String)}}); err != nil {
		t.Fatalf("Any alias to string rejected: %v", err)
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{freshOperation("fresh-table-marker", typ.BuiltinTableTopMarker(), FreshTable)}}); err != nil {
		t.Fatalf("table marker freshness rejected: %v", err)
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{freshOperation("fresh-table-record", typ.RebuildRecord(typ.RecordParts{}), FreshTable)}}); err != nil {
		t.Fatalf("record freshness rejected: %v", err)
	}
	function := typ.Func().Build()
	if _, err := Seal(&Spec{Operations: []OperationSpec{freshOperation("fresh-function", function, FreshFunction)}}); err != nil {
		t.Fatalf("function freshness rejected: %v", err)
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{freshOperation("fresh-thread-any", typ.Any, FreshThread)}}); err != nil {
		t.Fatalf("gradual thread freshness rejected: %v", err)
	}
	producedFunction := producedOperation("produced-function", function)
	if _, err := Seal(&producedFunction); err != nil {
		t.Fatalf("produced direct function rejected: %v", err)
	}
	producedAny := producedOperation("produced-any", typ.Any)
	if _, err := Seal(&producedAny); err != nil {
		t.Fatalf("produced Any rejected: %v", err)
	}
	if _, err := Seal(&Spec{Operations: []OperationSpec{callbackResultOperation("callback-function", function, function, DirectFunction)}}); err != nil {
		t.Fatalf("direct callback function rejected: %v", err)
	}

	callMeta := typ.RebuildRecord(typ.RecordParts{StaticMembers: []typ.StaticMember{{Kind: typ.StaticMemberStringIndex, Name: "__call", Type: function}}})
	ordinaryCallable := typ.RebuildRecord(typ.RecordParts{Metatable: callMeta})
	if _, err := Seal(&Spec{Operations: []OperationSpec{callbackResultOperation("callback-ordinary-callable", ordinaryCallable, ordinaryCallable, OrdinaryCallable)}}); err != nil {
		t.Fatalf("ordinary callable callback result rejected: %v", err)
	}
	producedCallableRecord := producedOperation("produced-callable-record", ordinaryCallable)
	if _, err := Seal(&producedCallableRecord); err == nil {
		t.Fatal("Produced accepted a record callable only through __call")
	}
}

func TestIdentityAliasDoesNotReplaceFormalWithItsConstraint(t *testing.T) {
	formal := typ.NewTypeParam("T", typ.Number)
	equal := identityOperation("alias-formal-equal", formal, formal)
	equal.TypeFormals = []*typ.TypeParam{formal}
	if _, err := Seal(&Spec{Operations: []OperationSpec{equal}}); err != nil {
		t.Fatalf("T to T alias rejected: %v", err)
	}
	narrow := identityOperation("alias-formal-narrow", typ.Integer, formal)
	narrow.TypeFormals = []*typ.TypeParam{formal}
	if _, err := Seal(&Spec{Operations: []OperationSpec{narrow}}); err == nil {
		t.Fatal("integer alias was accepted as unconstrained T:number")
	}
}

func TestIdentityAnnotationsUnfoldRecursiveAndInstantiatedSemanticHeads(t *testing.T) {
	record := typ.RebuildRecord(typ.RecordParts{})
	function := typ.Func().Build()

	recursiveRecord := typ.NewRecursive("RecursiveRecord", func(self typ.Type) typ.Type { return record })
	recursiveFunction := typ.NewRecursive("RecursiveFunction", func(self typ.Type) typ.Type { return function })
	recursiveString := typ.NewRecursive("RecursiveString", func(self typ.Type) typ.Type { return typ.String })

	recordParam := typ.NewTypeParam("T", nil)
	recordGeneric := typ.NewGeneric("RecordBox", []*typ.TypeParam{recordParam}, record)
	functionParam := typ.NewTypeParam("T", nil)
	functionGeneric := typ.NewGeneric("FunctionBox", []*typ.TypeParam{functionParam}, function)
	stringParam := typ.NewTypeParam("T", nil)
	stringGeneric := typ.NewGeneric("Id", []*typ.TypeParam{stringParam}, stringParam)
	instantiatedRecord := typ.Instantiate(recordGeneric, typ.String)
	instantiatedFunction := typ.Instantiate(functionGeneric, typ.String)
	instantiatedString := typ.Instantiate(stringGeneric, typ.String)

	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"recursive-record-fresh-table", Spec{Operations: []OperationSpec{freshOperation("recursive-record-fresh-table", recursiveRecord, FreshTable)}}},
		{"instantiated-record-fresh-table", Spec{Operations: []OperationSpec{freshOperation("instantiated-record-fresh-table", instantiatedRecord, FreshTable)}}},
		{"recursive-function-fresh-function", Spec{Operations: []OperationSpec{freshOperation("recursive-function-fresh-function", recursiveFunction, FreshFunction)}}},
		{"instantiated-function-fresh-function", Spec{Operations: []OperationSpec{freshOperation("instantiated-function-fresh-function", instantiatedFunction, FreshFunction)}}},
		{"recursive-function-produced", producedOperation("recursive-function-produced", recursiveFunction)},
		{"instantiated-function-produced", producedOperation("instantiated-function-produced", instantiatedFunction)},
		{"recursive-function-direct-callback", Spec{Operations: []OperationSpec{callbackResultOperation("recursive-function-direct-callback", recursiveFunction, recursiveFunction, DirectFunction)}}},
		{"instantiated-function-direct-callback", Spec{Operations: []OperationSpec{callbackResultOperation("instantiated-function-direct-callback", instantiatedFunction, instantiatedFunction, DirectFunction)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := Seal(&item.spec); err != nil {
				t.Fatalf("semantic wrapper rejected: %v", err)
			}
		})
	}

	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"recursive-string-fresh-table", Spec{Operations: []OperationSpec{freshOperation("recursive-string-fresh-table", recursiveString, FreshTable)}}},
		{"instantiated-string-fresh-table", Spec{Operations: []OperationSpec{freshOperation("instantiated-string-fresh-table", instantiatedString, FreshTable)}}},
		{"recursive-string-produced", producedOperation("recursive-string-produced", recursiveString)},
		{"instantiated-string-produced", producedOperation("instantiated-string-produced", instantiatedString)},
		{"recursive-string-direct-callback", Spec{Operations: []OperationSpec{callbackResultOperation("recursive-string-direct-callback", recursiveString, recursiveString, DirectFunction)}}},
		{"instantiated-string-direct-callback", Spec{Operations: []OperationSpec{callbackResultOperation("instantiated-string-direct-callback", instantiatedString, instantiatedString, DirectFunction)}}},
		{"recursive-string-fresh-thread", Spec{Operations: []OperationSpec{freshOperation("recursive-string-fresh-thread", recursiveString, FreshThread)}}},
		{"instantiated-string-fresh-thread", Spec{Operations: []OperationSpec{freshOperation("instantiated-string-fresh-thread", instantiatedString, FreshThread)}}},
		{"recursive-record-fresh-thread", Spec{Operations: []OperationSpec{freshOperation("recursive-record-fresh-thread", recursiveRecord, FreshThread)}}},
		{"instantiated-record-fresh-thread", Spec{Operations: []OperationSpec{freshOperation("instantiated-record-fresh-thread", instantiatedRecord, FreshThread)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := Seal(&item.spec); err == nil {
				t.Fatal("concrete semantic wrapper contradiction sealed")
			}
		})
	}
}

func TestIdentityAnnotationsRecursiveCyclesRequireAProductiveHead(t *testing.T) {
	self := typ.NewRecursive("Self", func(self typ.Type) typ.Type { return self })
	left := typ.NewRecursivePlaceholder("Left")
	right := typ.NewRecursivePlaceholder("Right")
	left.SetBody(right)
	right.SetBody(left)
	mutualTable := typ.NewRecursivePlaceholder("MutualTable")
	mutualTable.SetBody(typ.NewRecursive("MutualTableBody", func(self typ.Type) typ.Type { return typ.RebuildRecord(typ.RecordParts{}) }))

	for _, item := range []struct {
		name string
		spec Spec
		want bool
	}{
		{"self-fresh-table", Spec{Operations: []OperationSpec{freshOperation("self-fresh-table", self, FreshTable)}}, false},
		{"mutual-fresh-table", Spec{Operations: []OperationSpec{freshOperation("mutual-fresh-table", left, FreshTable)}}, false},
		{"self-fresh-thread-opaque", Spec{Operations: []OperationSpec{freshOperation("self-fresh-thread-opaque", self, FreshThread)}}, true},
		{"mutual-table-fresh-table", Spec{Operations: []OperationSpec{freshOperation("mutual-table-fresh-table", mutualTable, FreshTable)}}, true},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := Seal(&item.spec)
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func TestIdentityAnnotationsDeepSemanticWrappersDoNotUseTheHostStack(t *testing.T) {
	const depth = 20_000
	deeplyRecursive := func(head typ.Type) typ.Type {
		for range depth {
			body := head
			head = typ.NewRecursive("", func(self typ.Type) typ.Type { return body })
		}
		return head
	}

	deepRecord := deeplyRecursive(typ.RebuildRecord(typ.RecordParts{}))
	deepFunction := deeplyRecursive(typ.Func().Build())
	for _, item := range []struct {
		name string
		spec Spec
	}{
		{"fresh-table", Spec{Operations: []OperationSpec{freshOperation("deep-fresh-table", deepRecord, FreshTable)}}},
		{"produced", producedOperation("deep-produced", deepFunction)},
		{"direct-callback", Spec{Operations: []OperationSpec{callbackResultOperation("deep-direct-callback", deepFunction, deepFunction, DirectFunction)}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := Seal(&item.spec); err != nil {
				t.Fatalf("deep semantic wrapper rejected: %v", err)
			}
		})
	}
}

func TestIdentityAnnotationsFormalBoundsAreOnlyNegativeEvidence(t *testing.T) {
	stringFormal := typ.NewTypeParam("T", typ.String)
	functionFormal := typ.NewTypeParam("F", typ.Func().Build())
	unconstrained := typ.NewTypeParam("U", nil)

	for _, item := range []struct {
		name string
		spec Spec
		want bool
	}{
		{"string-bound-fresh-thread", Spec{Operations: []OperationSpec{withTypeFormal(freshOperation("string-bound-fresh-thread", stringFormal, FreshThread), stringFormal)}}, false},
		{"function-bound-fresh-thread", Spec{Operations: []OperationSpec{withTypeFormal(freshOperation("function-bound-fresh-thread", functionFormal, FreshThread), functionFormal)}}, false},
		{"unconstrained-fresh-thread", Spec{Operations: []OperationSpec{withTypeFormal(freshOperation("unconstrained-fresh-thread", unconstrained, FreshThread), unconstrained)}}, true},
		{"function-bound-fresh-function", Spec{Operations: []OperationSpec{withTypeFormal(freshOperation("function-bound-fresh-function", functionFormal, FreshFunction), functionFormal)}}, false},
		{"function-bound-direct-callback", Spec{Operations: []OperationSpec{withTypeFormal(callbackResultOperation("function-bound-direct-callback", functionFormal, functionFormal, DirectFunction), functionFormal)}}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := Seal(&item.spec)
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func TestIdentityAnnotationsMetaIsReflectionOnly(t *testing.T) {
	meta := typ.NewMeta(typ.String)
	for _, item := range []struct {
		name  string
		fresh FreshKind
		want  bool
	}{
		{"reflection", FreshReflection, true},
		{"thread", FreshThread, false},
		{"userdata", FreshUserdata, false},
		{"error", FreshError, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := Seal(&Spec{Operations: []OperationSpec{freshOperation("meta-"+item.name, meta, item.fresh)}})
			if (err == nil) != item.want {
				t.Fatalf("Seal error = %v, want accepted=%t", err, item.want)
			}
		})
	}
}

func withTypeFormal(operation OperationSpec, formal *typ.TypeParam) OperationSpec {
	operation.TypeFormals = []*typ.TypeParam{formal}
	return operation
}
