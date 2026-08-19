package target

import (
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"strings"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestAllPublicObservablesArePermutationAndAlphaInvariant(t *testing.T) {
	leftFormal := testNewTypeParam("Element", testString)
	rightFormal := testNewTypeParam("Item", testString)
	left := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		genericAlpha(leftFormal, 2),
		providerBeta(),
	}})
	right := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		providerBeta(),
		genericAlpha(rightFormal, 1),
	}})
	assertPublicContractEqual(t, left, right)
}

func TestPublicLawTargetHasNoCallFormPlane(t *testing.T) {
	binding := vocabulary.BindingSpec{
		Namespace: vocabulary.BindingProvider,
		Owner:     []string{"network", "channel"},
		Member:    []string{"case", "receive"},
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{{
		// Program owns plain-versus-colon CallForm. A method-capable target
		// operation is authored once; the colon receiver is Input.Fixed[0].
		Bindings: []vocabulary.BindingSpec{binding},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	op, ok := contract.Operations.Lookup(binding)
	if !ok {
		t.Fatal("method-capable binding missing")
	}
	input, ok := contract.Operations.Input(op)
	if !ok || contract.Operations.ValueFormalCount(op) != 1 || contract.Operations.ValuesCount(input) != 1 {
		t.Fatalf("target input ABI = %d fixed/%d formals", contract.Operations.ValuesCount(input), contract.Operations.ValueFormalCount(op))
	}
	if value, ok := contract.Operations.ValuesAt(input, 0); !ok || value == 0 {
		t.Fatal("Input.Fixed[0] was not retained")
	}
	if _, ok := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingProvider,
		Owner:     []string{"network.channel"},
		Member:    []string{"case", "receive"},
	}); ok {
		t.Fatal("joined owner path resolved a segmented provider binding")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.Operations.Lookup(binding); !ok {
			panic("segmented provider binding disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("segmented provider Lookup allocated %f times", allocs)
	}
}

func TestBindingLookupKeepsAliasesPrefixesAndBytesExact(t *testing.T) {
	aliasLeft := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"a"}}
	aliasRight := vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"table"}, Member: []string{"unpack"}}
	prefix := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"a\x00z"}}
	longer := vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"aa"}}
	segmented := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"actor", "m\x00"}, Member: []string{"send", "x"}}
	operation := func(bindings ...vocabulary.BindingSpec) vocabulary.OperationSpec {
		return vocabulary.OperationSpec{
			Bindings: bindings,
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}
	}
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{
		operation(segmented), operation(longer), operation(aliasRight, prefix, aliasLeft),
	}})
	aliasA, ok := contract.Operations.Lookup(aliasLeft)
	if !ok {
		t.Fatal("first alias missing")
	}
	aliasB, ok := contract.Operations.Lookup(aliasRight)
	if !ok || aliasA != aliasB {
		t.Fatalf("aliases = %d/%d/%v", aliasA, aliasB, ok)
	}
	for _, binding := range []vocabulary.BindingSpec{prefix, longer, segmented} {
		if _, ok := contract.Operations.Lookup(binding); !ok {
			t.Fatalf("exact binding disappeared: %#v", binding)
		}
	}
	for _, binding := range []vocabulary.BindingSpec{
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"a\x00"}},
		{Namespace: vocabulary.BindingBuiltin, Member: []string{"a\x00zz"}},
		{Namespace: vocabulary.BindingProvider, Owner: []string{"actor", "m"}, Member: []string{"send", "x"}},
		{Namespace: vocabulary.BindingProvider, Owner: []string{"actor", "m\x00"}, Member: []string{"send"}},
	} {
		if _, ok := contract.Operations.Lookup(binding); ok {
			t.Fatalf("prefix/byte-neighbor resolved: %#v", binding)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if _, ok := contract.Operations.Lookup(segmented); !ok {
			panic("segmented binding disappeared")
		}
	}); allocs != 0 {
		t.Fatalf("exact binary Lookup allocated %f times", allocs)
	}
}

func TestOpaqueHasExactlyFourUnknownOutcomes(t *testing.T) {
	contract := mustSeal(t, Spec{})
	opaque, ok := contract.Operations.Opaque()
	if !ok {
		t.Fatal("opaque operation missing")
	}
	if contract.Operations.BoundCount() != 0 || contract.Operations.OperationCount() != 1 {
		t.Fatal("opaque is not the sole row of an empty authored contract")
	}
	input, ok := contract.Operations.Input(opaque)
	if !ok {
		t.Fatal("opaque input missing")
	}
	if tail, variable, ok := contract.Operations.ValuesTail(input); !ok || tail != vocabulary.ValuesUnknown || variable != 0 {
		t.Fatalf("opaque input tail = %d/%d/%v, want unknown/0/true", tail, variable, ok)
	}
	want := [...]flowkind.OutcomeKind{
		flowkind.OutcomeNormal,
		flowkind.OutcomeThrow,
		flowkind.OutcomeYield,
		flowkind.OutcomeCancel,
	}
	if got := contract.Operations.OutcomeCount(opaque); got != len(want) {
		t.Fatalf("opaque outcomes = %d, want %d", got, len(want))
	}
	for index, kind := range want {
		gotKind, values, ok := contract.Operations.OutcomeAt(opaque, index)
		if !ok || gotKind != kind || values != input {
			t.Fatalf("opaque outcome %d = %d/%d/%v, want %d/%d/true", index, gotKind, values, ok, kind, input)
		}
		if tail, variable, ok := contract.Operations.ValuesTail(values); !ok || tail != vocabulary.ValuesUnknown || variable != 0 {
			t.Fatalf("opaque outcome %d values are not exactly unknown", index)
		}
	}
	if count := contract.Operations.EffectCount(opaque); count != 0 {
		t.Fatalf("opaque explicit effect count = %d, want 0", count)
	}
	if tail, variable, ok := contract.Operations.EffectTail(opaque); !ok || tail != vocabulary.RowUnknownOpen || variable != 0 {
		t.Fatalf("opaque effect tail = %d/%d/%v, want unknown-open/0/true", tail, variable, ok)
	}
}

func genericAlpha(formal *testRawTypeParam, beta vocabulary.SpecRef) vocabulary.OperationSpec {
	declarations, formals := testOperationTypes(formal)
	return vocabulary.OperationSpec{
		Bindings:    []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"alpha"}}},
		TypeFormals: formals,
		ValuesVars:  1,
		RowFormals:  1,
		Input:       vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesVariable, Var: 0},
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{declarations[0]}, Tail: vocabulary.ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesVariable, Var: 0}},
		},
		Effects: vocabulary.RowSpec{
			Occurrences: []vocabulary.EffectSpec{{Target: beta, ValueArgs: []vocabulary.ValueFormal{0}}},
			Tail:        vocabulary.RowVariable,
			Var:         0,
		},
	}
}

func providerBeta() vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{
			Namespace: vocabulary.BindingProvider,
			Owner:     []string{"network", "channel"},
			Member:    []string{"case", "receive"},
		}},
		Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{testString}, Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
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
	fmt.Fprintf(&out, "operations=%d,bound=%d;", contract.Operations.OperationCount(), contract.Operations.BoundCount())
	opaque, opaqueOK := contract.Operations.Opaque()
	fmt.Fprintf(&out, "opaque=%d/%v;", opaque, opaqueOK)
	for index := 0; index < contract.Operations.BoundCount(); index++ {
		op, ok := contract.Operations.OperationAt(index)
		fmt.Fprintf(&out, "bound[%d]=%d/%v;", index, op, ok)
	}
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		op, ok := contract.Operations.OperationAt(index)
		fmt.Fprintf(&out, "op[%d]=%d/%v{", index, op, ok)
		writeOperationSnapshot(t, &out, contract, op)
		out.WriteString("}")
	}
	for index := 0; index < contract.protocols.ProtocolCount(); index++ {
		protocol, ok := contract.protocols.ProtocolAt(index)
		fmt.Fprintf(&out, "protocol[%d]=%d/%v{", index, protocol, ok)
		writeProtocolSnapshot(&out, contract, protocol)
		out.WriteString("}")
	}
	return out.String()
}

func writeProtocolSnapshot(out *strings.Builder, contract *Contract, protocol vocabulary.Protocol) {
	fmt.Fprintf(out, "acquisitions=%d,states=%d,transitions=%d,escapes=%d,callback-holders=%d;",
		contract.protocols.ProtocolAcquisitionCount(protocol), contract.protocols.StateCount(protocol),
		contract.protocols.TransitionCount(protocol), contract.protocols.EscapeCount(protocol), contract.protocols.ProtocolCallbackHolderCount(protocol))
	for index := 0; index < contract.protocols.ProtocolAcquisitionCount(protocol); index++ {
		op, outcome, result, state, ok := contract.protocols.ProtocolAcquisitionAt(protocol, index)
		fmt.Fprintf(out, "acquisition[%d]=%d/%d/%d/%d/%v;", index, op, outcome, result, state, ok)
	}
	for index := 0; index < contract.protocols.StateCount(protocol); index++ {
		state, ok := contract.protocols.StateAt(protocol, index)
		name, nameOK := contract.protocols.StateName(protocol, state)
		final, finalOK := contract.protocols.StateFinal(protocol, state)
		fmt.Fprintf(out, "state[%d]=%d/%q/%v/%v/%v;", index, state, name, final, ok, nameOK && finalOK)
	}
	for index := 0; index < contract.protocols.TransitionCount(protocol); index++ {
		op, kind, source, from, ok := contract.protocols.TransitionAt(protocol, index)
		fmt.Fprintf(out, "transition[%d]=%d/%d/%d/%d/%v;", index, op, kind, source, from, ok)
		for outcomeIndex := 0; outcomeIndex < contract.protocols.TransitionOutcomeCount(protocol, index); outcomeIndex++ {
			outcome, to, outcomeOK := contract.protocols.TransitionOutcomeAt(protocol, index, outcomeIndex)
			fmt.Fprintf(out, "transition-outcome[%d]=%d/%d/%v;", outcomeIndex, outcome, to, outcomeOK)
		}
	}
	for index := 0; index < contract.protocols.EscapeCount(protocol); index++ {
		op, kind, source, ok := contract.protocols.EscapeAt(protocol, index)
		fmt.Fprintf(out, "escape[%d]=%d/%d/%d/%v;", index, op, kind, source, ok)
	}
	for index := 0; index < contract.protocols.ProtocolCallbackHolderCount(protocol); index++ {
		op, input, callback, ok := contract.protocols.ProtocolCallbackHolderAt(protocol, index)
		fmt.Fprintf(out, "callback-holder[%d]=%d/%d/%d/%d/%v;", index, op, input.Kind, input.Ordinal, callback, ok)
	}
}

func writeOperationSnapshot(t *testing.T, out *strings.Builder, contract *Contract, op vocabulary.Operation) {
	t.Helper()
	input, inputOK := contract.Operations.Input(op)
	fmt.Fprintf(out, "input=%d/%v:%s;", input, inputOK, publicValuesSnapshot(t, contract, input, inputOK))
	fmt.Fprintf(out, "type-formals=%d,value-formals=%d,values-vars=%d,row-formals=%d;",
		contract.Operations.TypeFormalCount(op), contract.Operations.ValueFormalCount(op), contract.Operations.ValuesVarCount(op), contract.Operations.RowFormalCount(op))
	for index := 0; index < contract.Operations.ValuesVarCount(op); index++ {
		class, ok := contract.Operations.ValuesVarType(op, vocabulary.ValuesVar(index))
		fmt.Fprintf(out, "values-var-type[%d]=%d/%v:%s;", index, class, ok, publicTypeDigest(t, contract, class, ok))
	}
	for index := 0; index < contract.Operations.TypeFormalCount(op); index++ {
		constraint, ok := contract.Operations.TypeFormalConstraint(op, vocabulary.TypeFormal(index))
		fmt.Fprintf(out, "constraint[%d]=%d/%v:%s;", index, constraint, ok, publicTypeDigest(t, contract, constraint, ok))
	}
	for index := 0; index < contract.Operations.OutcomeCount(op); index++ {
		kind, values, ok := contract.Operations.OutcomeAt(op, index)
		fmt.Fprintf(out, "outcome[%d]=%d/%d/%v:%s;", index, kind, values, ok, publicValuesSnapshot(t, contract, values, ok))
		for callbackIndex := 0; callbackIndex < contract.Operations.CallbackResultCount(op, index); callbackIndex++ {
			result, callback, callbackOK := contract.Operations.CallbackResultAt(op, index, callbackIndex)
			fmt.Fprintf(out, "callback-result[%d]=%d/%d/%v;", callbackIndex, result, callback, callbackOK)
		}
		for aliasIndex := 0; aliasIndex < contract.Operations.ResultAliasCount(op, index); aliasIndex++ {
			result, kind, source, aliasOK := contract.Operations.ResultAliasAt(op, index, aliasIndex)
			fmt.Fprintf(out, "result-alias[%d]=%d/%d/%d/%v;", aliasIndex, result, kind, source, aliasOK)
		}
		for freshIndex := 0; freshIndex < contract.Operations.FreshResultCount(op, index); freshIndex++ {
			result, ordinal, kind, freshOK := contract.Operations.FreshResultAt(op, index, freshIndex)
			fmt.Fprintf(out, "fresh-result[%d]=%d/%d/%d/%v;", freshIndex, result, ordinal, kind, freshOK)
		}
		for producedIndex := 0; producedIndex < contract.Operations.ProducedCount(op, index); producedIndex++ {
			result, target, producedOK := contract.Operations.ProducedAt(op, index, producedIndex)
			fmt.Fprintf(out, "produced[%d]=%d/%d/%v;", producedIndex, result, target, producedOK)
			for captureIndex := 0; captureIndex < contract.Operations.ProducedCaptureCount(op, index, producedIndex); captureIndex++ {
				kind, source, captureOK := contract.Operations.ProducedCaptureAt(op, index, producedIndex, captureIndex)
				fmt.Fprintf(out, "capture[%d]=%d/%d/%v;", captureIndex, kind, source, captureOK)
			}
		}
	}
	for transfer := 0; transfer < contract.Operations.TransferCount(op); transfer++ {
		endpoint, endpointOK := contract.Operations.TransferEndpointAt(op, transfer)
		payload, payloadOK := contract.Operations.TransferPayloadAt(op, transfer)
		alias, aliasOK := contract.Operations.TransferAliasAt(op, transfer)
		identity, identityOK := contract.Operations.TransferIdentityAt(op, transfer)
		capabilities, capabilitiesOK := contract.Operations.TransferCapabilitiesAt(op, transfer)
		fmt.Fprintf(out, "transfer[%d]=endpoint:%d/%d/%v,payload:%d/%d/%v,alias:%d/%d/%v,identity:%d/%v,capabilities:%d/%v;", transfer, endpoint.Kind, endpoint.Input, endpointOK, payload.Kind, payload.Ordinal, payloadOK, alias.Kind, alias.Ordinal, aliasOK, identity, identityOK, capabilities, capabilitiesOK)
		for outcome := 0; outcome < contract.Operations.TransferOutcomeCount(op, transfer); outcome++ {
			ordinal, possibility, outcomeOK := contract.Operations.TransferOutcomeAt(op, transfer, outcome)
			fmt.Fprintf(out, "transfer-outcome[%d]=%d/%d/%v;", outcome, ordinal, possibility, outcomeOK)
		}
	}
	for index := 0; index < contract.Operations.CallbackCount(op); index++ {
		callback, callbackOK := contract.Operations.CallbackAt(op, index)
		owner, ownerOK := contract.Operations.CallbackOwner(callback)
		function, functionOK := contract.Operations.CallbackSource(callback)
		arguments, argumentsOK := contract.Operations.CallbackArguments(callback)
		admission, admissionOK := contract.Operations.CallbackAdmission(callback)
		lifecycle, lifecycleOK := contract.Operations.CallbackLifecycle(callback)
		subedge, subedgeOK := contract.Operations.CallbackSubedge(callback)
		fmt.Fprintf(out, "callback[%d]=%d/%v:owner=%d/%v,function=%d/%d/%v,args=%d/%v,admission=%d/%v,lifecycle=%d/%v;",
			index, callback, callbackOK, owner, ownerOK, function.Kind, function.Ordinal, functionOK,
			arguments, argumentsOK, admission, admissionOK, lifecycle, lifecycleOK)
		fmt.Fprintf(out, "callback-subedge=%d/%v;", subedge, subedgeOK)
		for _, kind := range []flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			values, ok := contract.Operations.CallbackOutcome(callback, kind)
			fmt.Fprintf(out, "callback-outcome[%d]=%d/%v;", kind, values, ok)
		}
		tail, rowVar, tailOK := contract.Operations.CallbackEffectTail(callback)
		fmt.Fprintf(out, "callback-effect-tail=%d/%d/%v;", tail, rowVar, tailOK)
		for effect := 0; effect < contract.Operations.CallbackEffectCount(callback); effect++ {
			target, targetOK := contract.Operations.CallbackEffectTarget(callback, effect)
			fmt.Fprintf(out, "callback-effect[%d]=target:%d/%v;", effect, target, targetOK)
			writeCallbackEffectArguments(out, contract, callback, effect)
		}
		releaseOperation, releaseInput, releaseOutcome, releaseMode, releaseOK := contract.Operations.CallbackRelease(callback)
		fmt.Fprintf(out, "callback-release=%d/%d/%d/%d/%v;", releaseOperation, releaseInput, releaseOutcome, releaseMode, releaseOK)
		zeroBehavior, zeroOutcome, zeroOK := contract.Operations.CallbackReleaseZero(callback)
		fmt.Fprintf(out, "callback-release-zero=%d/%d/%v;", zeroBehavior, zeroOutcome, zeroOK)
	}
	writeSubedgeSnapshot(out, contract, op)
	for release := 0; release < contract.Operations.CallbackReleaseCount(op); release++ {
		callback, input, outcome, mode, releaseOK := contract.Operations.CallbackReleaseAt(op, release)
		fmt.Fprintf(out, "release[%d]=%d/%d/%d/%d/%v;", release, callback, input, outcome, mode, releaseOK)
	}
	for index := 0; index < contract.Operations.EffectCount(op); index++ {
		target, targetOK := contract.Operations.EffectTarget(op, index)
		fmt.Fprintf(out, "effect[%d]=target:%d/%v;", index, target, targetOK)
		writeEffectArguments(out, contract, op, index)
	}
	for index := 0; index < contract.Operations.SuspensionCount(op); index++ {
		yield, reentry, source, multiplicity, ok := contract.Operations.SuspensionAt(op, index)
		fmt.Fprintf(out, "suspension[%d]=%d/%d/%d/%d/%v;", index, yield, reentry, source, multiplicity, ok)
	}
	for index := 0; index < contract.Operations.ResumeCount(op); index++ {
		resume, resumeOK := contract.Operations.ResumeIDAt(op, index)
		owner, source, carrier, arguments, ok := contract.Operations.Resume(resume)
		fmt.Fprintf(out, "resume[%d]=%d/%d/%d/%d/%v/%v;", index, owner, source, carrier, arguments, resumeOK, ok)
		for outcome := 0; outcome < contract.Operations.ResumeOutcomeCount(resume); outcome++ {
			kind, target, outcomeOK := contract.Operations.ResumeOutcomeAt(resume, outcome)
			fmt.Fprintf(out, "resume-outcome[%d]=%d/%d/%v;", outcome, kind, target, outcomeOK)
		}
	}
	tail, rowVar, tailOK := contract.Operations.EffectTail(op)
	fmt.Fprintf(out, "effect-tail=%d/%d/%v;", tail, rowVar, tailOK)
	writeBindingSnapshot(out, contract, op)
}

func writeSubedgeSnapshot(out *strings.Builder, contract *Contract, op vocabulary.Operation) {
	for index := 0; index < contract.Operations.SubedgeCount(op); index++ {
		edge, edgeOK := contract.Operations.SubedgeAt(op, index)
		owner, ownerOK := contract.Operations.SubedgeOwner(edge)
		role, roleOK := contract.Operations.SubedgeRole(edge)
		family, familyOK := contract.Operations.SubedgeFamily(edge)
		callee, calleeOK := contract.Operations.SubedgeCallee(edge)
		admission, admissionOK := contract.Operations.SubedgeAdmission(edge)
		arguments, argumentsOK := contract.Operations.SubedgeArguments(edge)
		ruleEntry, ruleEntryOK := contract.Operations.SubedgeRuleEntry(edge)
		fmt.Fprintf(out, "subedge[%d]=%d/%v:owner=%d/%v,role=%d/%v,family=%d/%v,callee=%d/%v,admission=%d/%v,args=%d/%v,rule-entry=%t/%v;",
			index, edge, edgeOK, owner, ownerOK, role, roleOK, family, familyOK, callee, calleeOK, admission, admissionOK, arguments, argumentsOK, ruleEntry, ruleEntryOK)
		switch callee {
		case vocabulary.SubedgeCalleeCallback:
			callback, callbackOK := contract.Operations.SubedgeCallback(edge)
			fmt.Fprintf(out, "subedge-callback=%d/%v;", callback, callbackOK)
		case vocabulary.SubedgeCalleeCapturedInitialRead:
			root, key, readOK := contract.Operations.SubedgeCapturedInitialRead(edge)
			fmt.Fprintf(out, "subedge-read=%d/%d/%v;", root, key, readOK)
		case vocabulary.SubedgeCalleeMetaKey:
			key, keyOK := contract.Operations.SubedgeMetaKey(edge)
			fmt.Fprintf(out, "subedge-meta=%d/%v;", key, keyOK)
		}
		failure, failureOK := contract.Operations.SubedgeAdmissionFailure(edge)
		admissionRoute, admissionAdjustment, admissionResult, admissionPlacement, admissionOffset, admissionOutcome, admissionSibling, admissionDestination, admissionRouteOK := contract.Operations.SubedgeAdmissionRoute(edge)
		fmt.Fprintf(out, "subedge-admission=%d/%v,route=%d/%d/%d/%d/%d/%d/%d/%d/%v;", failure, failureOK, admissionRoute, admissionAdjustment, admissionResult, admissionPlacement, admissionOffset, admissionOutcome, admissionSibling, admissionDestination, admissionRouteOK)
		for _, kind := range []flowkind.OutcomeKind{
			flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow,
			flowkind.OutcomeYield, flowkind.OutcomeCancel,
		} {
			terminal, terminalOK := contract.Operations.SubedgeTerminal(edge, kind)
			route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK := contract.Operations.SubedgeRouteAt(edge, kind)
			fmt.Fprintf(out, "subedge-terminal[%d]=%d/%v,route=%d/%d/%d/%d/%d/%d/%d/%d/%v;",
				kind, terminal, terminalOK, route, adjustment, result, placement, offset, outcome, sibling, destination, routeOK)
		}
	}
}

func writeEffectArguments(out *strings.Builder, contract *Contract, op vocabulary.Operation, effect int) {
	fmt.Fprintf(out, "values(%d)=", contract.Operations.EffectValueArgumentCount(op, effect))
	for index := 0; index < contract.Operations.EffectValueArgumentCount(op, effect); index++ {
		value, ok := contract.Operations.EffectValueArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "types(%d)=", contract.Operations.EffectTypeArgumentCount(op, effect))
	for index := 0; index < contract.Operations.EffectTypeArgumentCount(op, effect); index++ {
		value, ok := contract.Operations.EffectTypeArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "Values(%d)=", contract.Operations.EffectValuesArgumentCount(op, effect))
	for index := 0; index < contract.Operations.EffectValuesArgumentCount(op, effect); index++ {
		value, ok := contract.Operations.EffectValuesArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "rows(%d)=", contract.Operations.EffectRowArgumentCount(op, effect))
	for index := 0; index < contract.Operations.EffectRowArgumentCount(op, effect); index++ {
		value, ok := contract.Operations.EffectRowArgumentAt(op, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
}

func writeCallbackEffectArguments(out *strings.Builder, contract *Contract, callback vocabulary.CallbackID, effect int) {
	fmt.Fprintf(out, "values(%d)=", contract.Operations.CallbackEffectValueArgumentCount(callback, effect))
	for index := 0; index < contract.Operations.CallbackEffectValueArgumentCount(callback, effect); index++ {
		value, ok := contract.Operations.CallbackEffectValueArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "types(%d)=", contract.Operations.CallbackEffectTypeArgumentCount(callback, effect))
	for index := 0; index < contract.Operations.CallbackEffectTypeArgumentCount(callback, effect); index++ {
		value, ok := contract.Operations.CallbackEffectTypeArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "Values(%d)=", contract.Operations.CallbackEffectValuesArgumentCount(callback, effect))
	for index := 0; index < contract.Operations.CallbackEffectValuesArgumentCount(callback, effect); index++ {
		value, ok := contract.Operations.CallbackEffectValuesArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
	fmt.Fprintf(out, "rows(%d)=", contract.Operations.CallbackEffectRowArgumentCount(callback, effect))
	for index := 0; index < contract.Operations.CallbackEffectRowArgumentCount(callback, effect); index++ {
		value, ok := contract.Operations.CallbackEffectRowArgumentAt(callback, effect, index)
		fmt.Fprintf(out, "%d/%v,", value, ok)
	}
}

func writeBindingSnapshot(out *strings.Builder, contract *Contract, op vocabulary.Operation) {
	fmt.Fprintf(out, "bindings=%d;", contract.Operations.BindingCount(op))
	for bindingIndex := 0; bindingIndex < contract.Operations.BindingCount(op); bindingIndex++ {
		namespace, namespaceOK := contract.Operations.BindingNamespaceAt(op, bindingIndex)
		ownerCount := contract.Operations.BindingOwnerCountAt(op, bindingIndex)
		memberCount := contract.Operations.BindingMemberCountAt(op, bindingIndex)
		fmt.Fprintf(out, "binding[%d]=%d/%v,owner=%d,member=%d;", bindingIndex, namespace, namespaceOK, ownerCount, memberCount)
		if !namespaceOK {
			continue
		}
		binding := vocabulary.BindingSpec{Namespace: namespace}
		for index := 0; index < ownerCount; index++ {
			part, ok := contract.Operations.BindingOwnerAt(op, bindingIndex, index)
			fmt.Fprintf(out, "owner[%d]=%q/%v;", index, part, ok)
			binding.Owner = append(binding.Owner, part)
		}
		for index := 0; index < memberCount; index++ {
			part, ok := contract.Operations.BindingMemberAt(op, bindingIndex, index)
			fmt.Fprintf(out, "member[%d]=%q/%v;", index, part, ok)
			binding.Member = append(binding.Member, part)
		}
		lookup, lookupOK := contract.Operations.Lookup(binding)
		fmt.Fprintf(out, "lookup=%d/%v;", lookup, lookupOK)
	}
}

func publicValuesSnapshot(t *testing.T, contract *Contract, values vocabulary.Values, valuesOK bool) string {
	t.Helper()
	if !valuesOK {
		return "invalid"
	}
	var out strings.Builder
	tail, variable, tailOK := contract.Operations.ValuesTail(values)
	tailType, tailTypeOK := contract.Operations.ValuesTailType(values)
	fmt.Fprintf(&out, "count=%d,suffix=%d,tail=%d/%d/%v,tail-type=%d/%v:%s:[", contract.Operations.ValuesCount(values), contract.Operations.ValuesSuffixCount(values), tail, variable, tailOK, tailType, tailTypeOK, publicTypeDigest(t, contract, tailType, tailTypeOK))
	for index := 0; index < contract.Operations.ValuesCount(values); index++ {
		value, ok := contract.Operations.ValuesAt(values, index)
		fmt.Fprintf(&out, "%d/%v:%s,", value, ok, publicTypeDigest(t, contract, value, ok))
	}
	out.WriteString("];suffix[")
	for index := 0; index < contract.Operations.ValuesSuffixCount(values); index++ {
		value, ok := contract.Operations.ValuesSuffixAt(values, index)
		fmt.Fprintf(&out, "%d/%v:%s,", value, ok, publicTypeDigest(t, contract, value, ok))
	}
	out.WriteByte(']')
	return out.String()
}

func publicTypeDigest(t *testing.T, contract *Contract, value vocabulary.Type, ok bool) string {
	t.Helper()
	if !ok {
		return "invalid"
	}
	declaration, declarationOK := contract.Operations.TypeDeclaration(value)
	return fmt.Sprintf("%x/%v", declaration.Digest(), declarationOK)
}
