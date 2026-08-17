package target

import (
	"fmt"
	"strings"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestAllPublicObservablesArePermutationAndAlphaInvariant(t *testing.T) {
	leftFormal := testNewTypeParam("Element", testString)
	rightFormal := testNewTypeParam("Item", testString)
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		genericAlpha(leftFormal, 2),
		providerBeta(),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		providerBeta(),
		genericAlpha(rightFormal, 1),
	}})
	assertPublicContractEqual(t, left, right)
}

func TestPublicLawTargetHasNoCallFormPlane(t *testing.T) {
	binding := BindingSpec{
		Namespace: BindingProvider,
		Owner:     []string{"network", "channel"},
		Member:    []string{"case", "receive"},
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{{
		// Program owns plain-versus-colon CallForm. A method-capable target
		// operation is authored once; the colon receiver is Input.Fixed[0].
		Bindings: []BindingSpec{binding},
		Input:    ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}}})
	op, ok := contract.Lookup(binding)
	if !ok {
		t.Fatal("method-capable binding missing")
	}
	input, ok := contract.Input(op)
	if !ok || contract.ValueFormalCount(op) != 1 || contract.ValuesCount(input) != 1 {
		t.Fatalf("target input ABI = %d fixed/%d formals", contract.ValuesCount(input), contract.ValueFormalCount(op))
	}
	if value, ok := contract.ValuesAt(input, 0); !ok || value == 0 {
		t.Fatal("Input.Fixed[0] was not retained")
	}
	if _, ok := contract.Lookup(BindingSpec{
		Namespace: BindingProvider,
		Owner:     []string{"network.channel"},
		Member:    []string{"case", "receive"},
	}); ok {
		t.Fatal("joined owner path resolved a segmented provider binding")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.Lookup(binding); !ok {
			panic("segmented provider binding disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("segmented provider Lookup allocated %f times", allocs)
	}
}

func TestBindingLookupKeepsAliasesPrefixesAndBytesExact(t *testing.T) {
	aliasLeft := BindingSpec{Namespace: BindingBuiltin, Member: []string{"a"}}
	aliasRight := BindingSpec{Namespace: BindingModule, Owner: []string{"table"}, Member: []string{"unpack"}}
	prefix := BindingSpec{Namespace: BindingBuiltin, Member: []string{"a\x00z"}}
	longer := BindingSpec{Namespace: BindingBuiltin, Member: []string{"aa"}}
	segmented := BindingSpec{Namespace: BindingProvider, Owner: []string{"actor", "m\x00"}, Member: []string{"send", "x"}}
	operation := func(bindings ...BindingSpec) OperationSpec {
		return OperationSpec{
			Bindings: bindings,
			Input:    ValuesSpec{Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}},
			Effects:  RowSpec{Tail: RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		operation(segmented), operation(longer), operation(aliasRight, prefix, aliasLeft),
	}})
	aliasA, ok := contract.Lookup(aliasLeft)
	if !ok {
		t.Fatal("first alias missing")
	}
	aliasB, ok := contract.Lookup(aliasRight)
	if !ok || aliasA != aliasB {
		t.Fatalf("aliases = %d/%d/%v", aliasA, aliasB, ok)
	}
	for _, binding := range []BindingSpec{prefix, longer, segmented} {
		if _, ok := contract.Lookup(binding); !ok {
			t.Fatalf("exact binding disappeared: %#v", binding)
		}
	}
	for _, binding := range []BindingSpec{
		{Namespace: BindingBuiltin, Member: []string{"a\x00"}},
		{Namespace: BindingBuiltin, Member: []string{"a\x00zz"}},
		{Namespace: BindingProvider, Owner: []string{"actor", "m"}, Member: []string{"send", "x"}},
		{Namespace: BindingProvider, Owner: []string{"actor", "m\x00"}, Member: []string{"send"}},
	} {
		if _, ok := contract.Lookup(binding); ok {
			t.Fatalf("prefix/byte-neighbor resolved: %#v", binding)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.Lookup(segmented); !ok {
			panic("segmented binding disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("exact binary Lookup allocated %f times", allocs)
	}
}

func TestOpaqueHasExactlyFourUnknownOutcomes(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, ok := contract.Opaque()
	if !ok {
		t.Fatal("opaque operation missing")
	}
	if contract.BoundOperationCount() != 0 || contract.OperationCount() != 1 {
		t.Fatal("opaque is not the sole row of an empty authored contract")
	}
	input, ok := contract.Input(opaque)
	if !ok {
		t.Fatal("opaque input missing")
	}
	if tail, variable, ok := contract.ValuesTail(input); !ok || tail != ValuesUnknown || variable != 0 {
		t.Fatalf("opaque input tail = %d/%d/%v, want unknown/0/true", tail, variable, ok)
	}
	want := [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal,
		flowkind.OutcomeThrow,
		flowkind.OutcomeYield,
		flowkind.OutcomeCancel,
	}
	if got := contract.OutcomeCount(opaque); got != len(want) {
		t.Fatalf("opaque outcomes = %d, want %d", got, len(want))
	}
	for index, kind := range want {
		gotKind, values, ok := contract.OutcomeAt(opaque, index)
		if !ok || gotKind != kind || values != input {
			t.Fatalf("opaque outcome %d = %d/%d/%v, want %d/%d/true", index, gotKind, values, ok, kind, input)
		}
		if tail, variable, ok := contract.ValuesTail(values); !ok || tail != ValuesUnknown || variable != 0 {
			t.Fatalf("opaque outcome %d values are not exactly unknown", index)
		}
	}
	if count := contract.EffectCount(opaque); count != 0 {
		t.Fatalf("opaque explicit effect count = %d, want 0", count)
	}
	if tail, variable, ok := contract.EffectTail(opaque); !ok || tail != RowUnknownOpen || variable != 0 {
		t.Fatalf("opaque effect tail = %d/%d/%v, want unknown-open/0/true", tail, variable, ok)
	}
}

func genericAlpha(formal *testRawTypeParam, beta SpecRef) OperationSpec {
	declarations, formals := testOperationTypes(formal)
	return OperationSpec{
		Bindings:    []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"alpha"}}},
		TypeFormals: formals,
		ValuesVars:  1,
		RowFormals:  1,
		Input:       ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: ValuesVariable, Var: 0},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesVariable, Var: 0}},
		},
		Effects: RowSpec{
			Occurrences: []EffectSpec{{Target: beta, ValueArgs: []ValueFormal{0}}},
			Tail:        RowVariable,
			Var:         0,
		},
	}
}

func providerBeta() OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{
			Namespace: BindingProvider,
			Owner:     []string{"network", "channel"},
			Member:    []string{"case", "receive"},
		}},
		Input:    ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: ValuesClosed}}},
		Effects:  RowSpec{Tail: RowClosed},
	}
}

func assertPublicContractEqual(t *testing.T, left, right *Contract) {
	t.Helper()
	leftSnapshot := publicContractSnapshot(t, left)
	rightSnapshot := publicContractSnapshot(t, right)
	if leftSnapshot != rightSnapshot {
		t.Fatalf("permutation/alpha changed public Contract observations\nleft:  %s\nright: %s", leftSnapshot, rightSnapshot)
	}
}

func publicContractSnapshot(t *testing.T, contract *Contract) string {
	t.Helper()
	var out strings.Builder
	fmt.Fprintf(&out, "operations=%d,bound=%d;", contract.OperationCount(), contract.BoundOperationCount())
	opaque, opaqueOK := contract.Opaque()
	fmt.Fprintf(&out, "opaque=%d/%v;", opaque, opaqueOK)
	for index := 0; index < contract.BoundOperationCount(); index++ {
		op, ok := contract.BoundOperationAt(index)
		fmt.Fprintf(&out, "bound[%d]=%d/%v;", index, op, ok)
	}
	for index := 0; index < contract.OperationCount(); index++ {
		op, ok := contract.OperationAt(index)
		fmt.Fprintf(&out, "op[%d]=%d/%v{", index, op, ok)
		writeOperationSnapshot(t, &out, contract, op)
		out.WriteString("}")
	}
	for index := 0; index < contract.ProtocolCount(); index++ {
		protocol, ok := contract.ProtocolAt(index)
		fmt.Fprintf(&out, "protocol[%d]=%d/%v{", index, protocol, ok)
		writeProtocolSnapshot(&out, contract, protocol)
		out.WriteString("}")
	}
	return out.String()
}

func writeProtocolSnapshot(out *strings.Builder, contract *Contract, protocol Protocol) {
	fmt.Fprintf(out, "acquisitions=%d,states=%d,transitions=%d,escapes=%d,callback-holders=%d;",
		contract.ProtocolAcquisitionCount(protocol), contract.StateCount(protocol),
		contract.TransitionCount(protocol), contract.EscapeCount(protocol), contract.ProtocolCallbackHolderCount(protocol))
	for index := 0; index < contract.ProtocolAcquisitionCount(protocol); index++ {
		op, outcome, result, state, ok := contract.ProtocolAcquisitionAt(protocol, index)
		fmt.Fprintf(out, "acquisition[%d]=%d/%d/%d/%d/%v;", index, op, outcome, result, state, ok)
	}
	for index := 0; index < contract.StateCount(protocol); index++ {
		state, ok := contract.StateAt(protocol, index)
		name, nameOK := contract.StateName(protocol, state)
		final, finalOK := contract.StateFinal(protocol, state)
		fmt.Fprintf(out, "state[%d]=%d/%q/%v/%v/%v;", index, state, name, final, ok, nameOK && finalOK)
	}
	for index := 0; index < contract.TransitionCount(protocol); index++ {
		op, kind, source, from, ok := contract.TransitionAt(protocol, index)
		fmt.Fprintf(out, "transition[%d]=%d/%d/%d/%d/%v;", index, op, kind, source, from, ok)
		for outcomeIndex := 0; outcomeIndex < contract.TransitionOutcomeCount(protocol, index); outcomeIndex++ {
			outcome, to, outcomeOK := contract.TransitionOutcomeAt(protocol, index, outcomeIndex)
			fmt.Fprintf(out, "transition-outcome[%d]=%d/%d/%v;", outcomeIndex, outcome, to, outcomeOK)
		}
	}
	for index := 0; index < contract.EscapeCount(protocol); index++ {
		op, kind, source, ok := contract.EscapeAt(protocol, index)
		fmt.Fprintf(out, "escape[%d]=%d/%d/%d/%v;", index, op, kind, source, ok)
	}
	for index := 0; index < contract.ProtocolCallbackHolderCount(protocol); index++ {
		op, input, callback, ok := contract.ProtocolCallbackHolderAt(protocol, index)
		fmt.Fprintf(out, "callback-holder[%d]=%d/%d/%d/%d/%v;", index, op, input.Kind, input.Ordinal, callback, ok)
	}
}

func writeOperationSnapshot(t *testing.T, out *strings.Builder, contract *Contract, op Operation) {
	t.Helper()
	input, inputOK := contract.Input(op)
	fmt.Fprintf(out, "input=%d/%v:%s;", input, inputOK, publicValuesSnapshot(t, contract, input, inputOK))
	fmt.Fprintf(out, "type-formals=%d,value-formals=%d,values-vars=%d,row-formals=%d;",
		contract.TypeFormalCount(op), contract.ValueFormalCount(op), contract.ValuesVarCount(op), contract.RowFormalCount(op))
	for index := 0; index < contract.ValuesVarCount(op); index++ {
		class, ok := contract.ValuesVarType(op, ValuesVar(index))
		fmt.Fprintf(out, "values-var-type[%d]=%d/%v:%s;", index, class, ok, publicTypeDigest(t, contract, class, ok))
	}
	for index := 0; index < contract.TypeFormalCount(op); index++ {
		constraint, ok := contract.TypeFormalConstraint(op, TypeFormal(index))
		fmt.Fprintf(out, "constraint[%d]=%d/%v:%s;", index, constraint, ok, publicTypeDigest(t, contract, constraint, ok))
	}
	for index := 0; index < contract.OutcomeCount(op); index++ {
		kind, values, ok := contract.OutcomeAt(op, index)
		fmt.Fprintf(out, "outcome[%d]=%d/%d/%v:%s;", index, kind, values, ok, publicValuesSnapshot(t, contract, values, ok))
		for callbackIndex := 0; callbackIndex < contract.CallbackResultCount(op, index); callbackIndex++ {
			result, callback, callbackOK := contract.CallbackResultAt(op, index, callbackIndex)
			fmt.Fprintf(out, "callback-result[%d]=%d/%d/%v;", callbackIndex, result, callback, callbackOK)
		}
		for aliasIndex := 0; aliasIndex < contract.ResultAliasCount(op, index); aliasIndex++ {
			result, kind, source, aliasOK := contract.ResultAliasAt(op, index, aliasIndex)
			fmt.Fprintf(out, "result-alias[%d]=%d/%d/%d/%v;", aliasIndex, result, kind, source, aliasOK)
		}
		for freshIndex := 0; freshIndex < contract.FreshResultCount(op, index); freshIndex++ {
			result, ordinal, kind, freshOK := contract.FreshResultAt(op, index, freshIndex)
			fmt.Fprintf(out, "fresh-result[%d]=%d/%d/%d/%v;", freshIndex, result, ordinal, kind, freshOK)
		}
		for producedIndex := 0; producedIndex < contract.ProducedCount(op, index); producedIndex++ {
			result, target, producedOK := contract.ProducedAt(op, index, producedIndex)
			fmt.Fprintf(out, "produced[%d]=%d/%d/%v;", producedIndex, result, target, producedOK)
			for captureIndex := 0; captureIndex < contract.ProducedCaptureCount(op, index, producedIndex); captureIndex++ {
				kind, source, captureOK := contract.ProducedCaptureAt(op, index, producedIndex, captureIndex)
				fmt.Fprintf(out, "capture[%d]=%d/%d/%v;", captureIndex, kind, source, captureOK)
			}
		}
	}
	for transfer := 0; transfer < contract.TransferCount(op); transfer++ {
		endpoint, endpointOK := contract.TransferEndpointAt(op, transfer)
		payload, payloadOK := contract.TransferPayloadAt(op, transfer)
		alias, aliasOK := contract.TransferAliasAt(op, transfer)
		identity, identityOK := contract.TransferIdentityAt(op, transfer)
		capabilities, capabilitiesOK := contract.TransferCapabilitiesAt(op, transfer)
		fmt.Fprintf(out, "transfer[%d]=endpoint:%d/%d/%v,payload:%d/%d/%v,alias:%d/%d/%v,identity:%d/%v,capabilities:%d/%v;", transfer, endpoint.Kind, endpoint.Input, endpointOK, payload.Kind, payload.Ordinal, payloadOK, alias.Kind, alias.Ordinal, aliasOK, identity, identityOK, capabilities, capabilitiesOK)
		for outcome := 0; outcome < contract.TransferOutcomeCount(op, transfer); outcome++ {
			ordinal, possibility, outcomeOK := contract.TransferOutcomeAt(op, transfer, outcome)
			fmt.Fprintf(out, "transfer-outcome[%d]=%d/%d/%v;", outcome, ordinal, possibility, outcomeOK)
		}
	}
	for index := 0; index < contract.CallbackCount(op); index++ {
		callback, callbackOK := contract.CallbackAt(op, index)
		owner, ownerOK := contract.CallbackOwner(callback)
		function, functionOK := contract.CallbackFunction(callback)
		arguments, argumentsOK := contract.CallbackArguments(callback)
		admission, admissionOK := contract.CallbackAdmission(callback)
		lifecycle, lifecycleOK := contract.CallbackLifecycle(callback)
		subedge, subedgeOK := contract.CallbackSubedge(callback)
		fmt.Fprintf(out, "callback[%d]=%d/%v:owner=%d/%v,function=%d/%d/%v,args=%d/%v,admission=%d/%v,lifecycle=%d/%v;",
			index, callback, callbackOK, owner, ownerOK, function.Kind, function.Ordinal, functionOK,
			arguments, argumentsOK, admission, admissionOK, lifecycle, lifecycleOK)
		fmt.Fprintf(out, "callback-subedge=%d/%v;", subedge, subedgeOK)
		for _, kind := range []flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			values, ok := contract.CallbackOutcome(callback, kind)
			fmt.Fprintf(out, "callback-outcome[%d]=%d/%v;", kind, values, ok)
		}
		tail, rowVar, tailOK := contract.CallbackEffectTail(callback)
		fmt.Fprintf(out, "callback-effect-tail=%d/%d/%v;", tail, rowVar, tailOK)
		for effect := 0; effect < contract.CallbackEffectCount(callback); effect++ {
			target, targetOK := contract.CallbackEffectTarget(callback, effect)
			fmt.Fprintf(out, "callback-effect[%d]=target:%d/%v;", effect, target, targetOK)
			writeCallbackEffectArguments(out, contract, callback, effect)
		}
		releaseOperation, releaseInput, releaseOutcome, releaseMode, releaseOK := contract.CallbackRelease(callback)
		fmt.Fprintf(out, "callback-release=%d/%d/%d/%d/%v;", releaseOperation, releaseInput, releaseOutcome, releaseMode, releaseOK)
		zeroBehavior, zeroOutcome, zeroOK := contract.CallbackReleaseZero(callback)
		fmt.Fprintf(out, "callback-release-zero=%d/%d/%v;", zeroBehavior, zeroOutcome, zeroOK)
	}
	writeSubedgeSnapshot(out, contract, op)
	for release := 0; release < contract.CallbackReleaseCount(op); release++ {
		callback, input, outcome, mode, releaseOK := contract.CallbackReleaseAt(op, release)
		fmt.Fprintf(out, "release[%d]=%d/%d/%d/%d/%v;", release, callback, input, outcome, mode, releaseOK)
	}
	for index := 0; index < contract.EffectCount(op); index++ {
		target, targetOK := contract.EffectTarget(op, index)
		fmt.Fprintf(out, "effect[%d]=target:%d/%v;", index, target, targetOK)
		writeEffectArguments(out, contract, op, index)
	}
	for index := 0; index < contract.SuspensionCount(op); index++ {
		yield, reentry, source, multiplicity, ok := contract.SuspensionAt(op, index)
		fmt.Fprintf(out, "suspension[%d]=%d/%d/%d/%d/%v;", index, yield, reentry, source, multiplicity, ok)
	}
	for index := 0; index < contract.ResumeCount(op); index++ {
		resume, resumeOK := contract.ResumeIDAt(op, index)
		owner, source, carrier, arguments, ok := contract.Resume(resume)
		fmt.Fprintf(out, "resume[%d]=%d/%d/%d/%d/%v/%v;", index, owner, source, carrier, arguments, resumeOK, ok)
		for outcome := 0; outcome < contract.ResumeOutcomeCount(resume); outcome++ {
			kind, target, outcomeOK := contract.ResumeOutcomeAt(resume, outcome)
			fmt.Fprintf(out, "resume-outcome[%d]=%d/%d/%v;", outcome, kind, target, outcomeOK)
		}
	}
	tail, rowVar, tailOK := contract.EffectTail(op)
	fmt.Fprintf(out, "effect-tail=%d/%d/%v;", tail, rowVar, tailOK)
	writeBindingSnapshot(out, contract, op)
}

func writeSubedgeSnapshot(out *strings.Builder, contract *Contract, op Operation) {
	for index := 0; index < contract.SubedgeCount(op); index++ {
		edge, edgeOK := contract.SubedgeAt(op, index)
		owner, ownerOK := contract.SubedgeOwner(edge)
		role, roleOK := contract.SubedgeRole(edge)
		family, familyOK := contract.SubedgeFamily(edge)
		callee, calleeOK := contract.SubedgeCallee(edge)
		admission, admissionOK := contract.SubedgeAdmission(edge)
		arguments, argumentsOK := contract.SubedgeArguments(edge)
		ruleEntry, ruleEntryOK := contract.SubedgeRuleEntry(edge)
		fmt.Fprintf(out, "subedge[%d]=%d/%v:owner=%d/%v,role=%d/%v,family=%d/%v,callee=%d/%v,admission=%d/%v,args=%d/%v,rule-entry=%t/%v;",
			index, edge, edgeOK, owner, ownerOK, role, roleOK, family, familyOK, callee, calleeOK, admission, admissionOK, arguments, argumentsOK, ruleEntry, ruleEntryOK)
		switch callee {
		case SubedgeCalleeCallback:
			callback, callbackOK := contract.SubedgeCallback(edge)
			fmt.Fprintf(out, "subedge-callback=%d/%v;", callback, callbackOK)
		case SubedgeCalleeCapturedInitialRead:
			root, key, readOK := contract.SubedgeCapturedInitialRead(edge)
			fmt.Fprintf(out, "subedge-read=%d/%d/%v;", root, key, readOK)
		case SubedgeCalleeMetaKey:
			key, keyOK := contract.SubedgeMetaKey(edge)
			fmt.Fprintf(out, "subedge-meta=%d/%v;", key, keyOK)
		}
		failure, failureOK := contract.AdmissionFailure(edge)
		admissionRoute, admissionAdjustment, admissionResult, admissionPlacement, admissionOffset, admissionOutcome, admissionSibling, admissionDestination, admissionRouteOK := contract.AdmissionRoute(edge)
		fmt.Fprintf(out, "subedge-admission=%d/%v,route=%d/%d/%d/%d/%d/%d/%d/%d/%v;", failure, failureOK, admissionRoute, admissionAdjustment, admissionResult, admissionPlacement, admissionOffset, admissionOutcome, admissionSibling, admissionDestination, admissionRouteOK)
		for _, kind := range []flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			terminal, terminalOK := contract.SubedgeTerminal(edge, kind)
			route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := contract.SubedgeRouteAt(edge, kind)
			fmt.Fprintf(out, "subedge-terminal[%d]=%d/%v,route=%d/%d/%d/%d/%d/%d/%d/%d/%v;",
				kind, terminal, terminalOK, route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK)
		}
	}
}

func writeEffectArguments(out *strings.Builder, contract *Contract, op Operation, effect int) {
	fmt.Fprintf(out, "values(%d)=", contract.EffectValueArgumentCount(op, effect))
	for index := 0; index < contract.EffectValueArgumentCount(op, effect); index++ {
		value, ok := contract.EffectValueArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "types(%d)=", contract.EffectTypeArgumentCount(op, effect))
	for index := 0; index < contract.EffectTypeArgumentCount(op, effect); index++ {
		value, ok := contract.EffectTypeArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "Values(%d)=", contract.EffectValuesArgumentCount(op, effect))
	for index := 0; index < contract.EffectValuesArgumentCount(op, effect); index++ {
		value, ok := contract.EffectValuesArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "rows(%d)=", contract.EffectRowArgumentCount(op, effect))
	for index := 0; index < contract.EffectRowArgumentCount(op, effect); index++ {
		value, ok := contract.EffectRowArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
}

func writeCallbackEffectArguments(out *strings.Builder, contract *Contract, callback CallbackID, effect int) {
	fmt.Fprintf(out, "values(%d)=", contract.CallbackEffectValueArgumentCount(callback, effect))
	for index := 0; index < contract.CallbackEffectValueArgumentCount(callback, effect); index++ {
		value, ok := contract.CallbackEffectValueArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "types(%d)=", contract.CallbackEffectTypeArgumentCount(callback, effect))
	for index := 0; index < contract.CallbackEffectTypeArgumentCount(callback, effect); index++ {
		value, ok := contract.CallbackEffectTypeArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "Values(%d)=", contract.CallbackEffectValuesArgumentCount(callback, effect))
	for index := 0; index < contract.CallbackEffectValuesArgumentCount(callback, effect); index++ {
		value, ok := contract.CallbackEffectValuesArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "rows(%d)=", contract.CallbackEffectRowArgumentCount(callback, effect))
	for index := 0; index < contract.CallbackEffectRowArgumentCount(callback, effect); index++ {
		value, ok := contract.CallbackEffectRowArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
}

func writeBindingSnapshot(out *strings.Builder, contract *Contract, op Operation) {
	fmt.Fprintf(out, "bindings=%d;", contract.BindingCount(op))
	for bindingIndex := 0; bindingIndex < contract.BindingCount(op); bindingIndex++ {
		namespace, namespaceOK := contract.BindingNamespaceAt(op, bindingIndex)
		ownerCount := contract.BindingOwnerCountAt(op, bindingIndex)
		memberCount := contract.BindingMemberCountAt(op, bindingIndex)
		fmt.Fprintf(out, "binding[%d]=%d/%v,owner=%d,member=%d;", bindingIndex, namespace, namespaceOK, ownerCount, memberCount)
		if !namespaceOK {
			continue
		}
		binding := BindingSpec{Namespace: namespace}
		for index := 0; index < ownerCount; index++ {
			part, ok := contract.BindingOwnerAt(op, bindingIndex, index)
			fmt.Fprintf(out, "owner[%d]=%q/%v;", index, part, ok)
			binding.Owner = append(binding.Owner, part)
		}
		for index := 0; index < memberCount; index++ {
			part, ok := contract.BindingMemberAt(op, bindingIndex, index)
			fmt.Fprintf(out, "member[%d]=%q/%v;", index, part, ok)
			binding.Member = append(binding.Member, part)
		}
		lookup, lookupOK := contract.Lookup(binding)
		fmt.Fprintf(out, "lookup=%d/%v;", lookup, lookupOK)
	}
}

func publicValuesSnapshot(t *testing.T, contract *Contract, values Values, valuesOK bool) string {
	t.Helper()
	if !valuesOK {
		return "invalid"
	}
	var out strings.Builder
	tail, variable, tailOK := contract.ValuesTail(values)
	tailType, tailTypeOK := contract.ValuesTailType(values)
	fmt.Fprintf(&out, "count=%d,suffix=%d,tail=%d/%d/%v,tail-type=%d/%v:%s:[", contract.ValuesCount(values), contract.ValuesSuffixCount(values), tail, variable, tailOK, tailType, tailTypeOK, publicTypeDigest(t, contract, tailType, tailTypeOK))
	for index := 0; index < contract.ValuesCount(values); index++ {
		value, ok := contract.ValuesAt(values, index)
		fmt.Fprintf(&out, "%d/%v:%s,", value, ok, publicTypeDigest(t, contract, value, ok))
	}
	out.WriteString("];suffix[")
	for index := 0; index < contract.ValuesSuffixCount(values); index++ {
		value, ok := contract.ValuesSuffixAt(values, index)
		fmt.Fprintf(&out, "%d/%v:%s,", value, ok, publicTypeDigest(t, contract, value, ok))
	}
	out.WriteByte(']')
	return out.String()
}

func publicTypeDigest(t *testing.T, contract *Contract, value Type, ok bool) string {
	t.Helper()
	if !ok {
		return "invalid"
	}
	declaration, declarationOK := contract.TypeDeclaration(value)
	return fmt.Sprintf("%x/%v", declaration.Digest(), declarationOK)
}
