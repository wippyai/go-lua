package target

import (
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func endpointIdentityOperation(name string) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}},
		},
		Transfers: []TransferSpec{transfer(
			TransferEndpoint{Kind: TransferEndpointInput},
			InputSource{Kind: InputSourceValueFormal},
			TransferIdentitySame,
			TransferCapabilitiesPreserveAll,
			[]TransferOutcomeSpec{
				{Outcome: 0, Possibility: TransferMayDeliver},
				{Outcome: 1, Possibility: TransferMayReject},
			},
		)},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func endpointIdentityTwoTransferOperation(name string) OperationSpec {
	operation := endpointIdentityOperation(name)
	operation.Transfers = append(operation.Transfers, transfer(
		TransferEndpoint{Kind: TransferEndpointExternal},
		InputSource{Kind: InputSourceValueFormal},
		TransferIdentityDistinct,
		TransferCapabilitiesLoseAll,
		[]TransferOutcomeSpec{
			{Outcome: 0, Possibility: TransferMayDeliver},
			{Outcome: 1, Possibility: TransferMayReject},
		},
	))
	return operation
}

func endpointIdentityOperationAndTransfer(t testing.TB, c *Contract, name string) (Operation, TransferID) {
	t.Helper()
	op, ok := c.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
	if !ok {
		t.Fatalf("operation %q absent", name)
	}
	transfer, ok := c.TransferIDAt(op, 0)
	if !ok {
		t.Fatalf("transfer %q absent", name)
	}
	return op, transfer
}

type endpointIdentitySnapshot struct {
	operation identity.ContentID
	transfer  identity.ContentID
	outcomes  [2]identity.ContentID
}

func endpointIdentitySnapshotFor(t testing.TB, c *Contract, name string) endpointIdentitySnapshot {
	t.Helper()
	op, transfer := endpointIdentityOperationAndTransfer(t, c, name)
	operation, ok := c.OperationContentID(op)
	if !ok {
		t.Fatalf("%s operation identity unavailable", name)
	}
	transferID, ok := c.TransferContentID(op, transfer)
	if !ok {
		t.Fatalf("%s transfer identity unavailable", name)
	}
	var outcomes [2]identity.ContentID
	for index := range outcomes {
		id, _, ok := c.TransferOutcomeContentID(op, transfer, index)
		if !ok {
			t.Fatalf("%s transfer outcome %d identity unavailable", name, index)
		}
		outcomes[index] = id
	}
	return endpointIdentitySnapshot{operation: operation, transfer: transferID, outcomes: outcomes}
}

func assertEndpointIdentityEqual(t testing.TB, got, want endpointIdentitySnapshot) {
	t.Helper()
	if got != want {
		t.Fatalf("endpoint identity changed: got %#v, want %#v", got, want)
	}
}

func assertEndpointIdentityChanged(t testing.TB, got, prior endpointIdentitySnapshot) {
	t.Helper()
	if got.operation == prior.operation {
		t.Fatal("operation descriptor mutation reused its identity")
	}
	if got.transfer == prior.transfer {
		t.Fatal("transfer descriptor mutation reused its identity")
	}
	for index := range got.outcomes {
		if got.outcomes[index] == prior.outcomes[index] {
			t.Fatalf("transfer outcome %d mutation reused its identity", index)
		}
	}
}

func TestEndpointIdentityAuthenticatesOwnerAndDescriptor(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("alpha"),
		endpointIdentityOperation("beta"),
	}})
	alpha, alphaTransfer := endpointIdentityOperationAndTransfer(t, contract, "alpha")
	beta, betaTransfer := endpointIdentityOperationAndTransfer(t, contract, "beta")

	alphaOperation, ok := contract.OperationContentID(alpha)
	if !ok || !alphaOperation.Available() {
		t.Fatal("alpha operation identity unavailable")
	}
	if again, ok := contract.OperationContentID(alpha); !ok || again != alphaOperation {
		t.Fatal("operation identity is not stable")
	}
	if outcome, ok := contract.OutcomeContentID(alpha, 0); !ok || !outcome.Available() {
		t.Fatal("operation outcome identity unavailable")
	} else if again, ok := contract.OutcomeContentID(alpha, 0); !ok || again != outcome {
		t.Fatal("operation outcome identity is not stable")
	}
	betaOperation, ok := contract.OperationContentID(beta)
	if !ok || betaOperation == alphaOperation {
		t.Fatal("same operation ordinal in a distinct operation did not change identity")
	}

	alphaID, ok := contract.TransferContentID(alpha, alphaTransfer)
	if !ok || !alphaID.Available() {
		t.Fatal("alpha transfer identity unavailable")
	}
	betaID, ok := contract.TransferContentID(beta, betaTransfer)
	if !ok || betaID == alphaID {
		t.Fatal("same transfer ordinal under a distinct operation reused identity")
	}
	firstOutcome, disposition, ok := contract.TransferOutcomeContentID(alpha, alphaTransfer, 0)
	if !ok || !firstOutcome.Available() || disposition != TransferMayDeliver {
		t.Fatalf("first transfer outcome = %v/%v/%v", firstOutcome, disposition, ok)
	}
	secondOutcome, disposition, ok := contract.TransferOutcomeContentID(alpha, alphaTransfer, 1)
	if !ok || !secondOutcome.Available() || disposition != TransferMayReject || secondOutcome == firstOutcome {
		t.Fatalf("second transfer outcome = %v/%v/%v", secondOutcome, disposition, ok)
	}
	if betaOutcome, _, ok := contract.TransferOutcomeContentID(beta, betaTransfer, 0); !ok || betaOutcome == firstOutcome {
		t.Fatal("same outcome ordinal under a distinct operation reused identity")
	}

	if _, ok := contract.TransferContentID(beta, alphaTransfer); ok {
		t.Fatal("transfer accepted a mismatched owner")
	}
	if _, _, ok := contract.TransferOutcomeContentID(beta, alphaTransfer, 0); ok {
		t.Fatal("transfer outcome accepted a mismatched owner")
	}
	if _, ok := contract.OperationContentID(0); ok {
		t.Fatal("zero operation accepted")
	}
	if _, ok := contract.TransferContentID(alpha, 0); ok {
		t.Fatal("zero transfer accepted")
	}
	if _, _, ok := contract.TransferOutcomeContentID(alpha, alphaTransfer, 2); ok {
		t.Fatal("out-of-range transfer outcome accepted")
	}
}

func TestEndpointIdentityIsPermutationAndReplayInvariant(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("beta"),
		endpointIdentityOperation("alpha"),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("alpha"),
		endpointIdentityOperation("beta"),
	}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("equivalent target contracts changed content identity")
	}
	for _, name := range []string{"alpha", "beta"} {
		leftOperation, leftTransfer := endpointIdentityOperationAndTransfer(t, left, name)
		rightOperation, rightTransfer := endpointIdentityOperationAndTransfer(t, right, name)
		leftOperationID, leftOK := left.OperationContentID(leftOperation)
		rightOperationID, rightOK := right.OperationContentID(rightOperation)
		if !leftOK || !rightOK || leftOperationID != rightOperationID {
			t.Fatalf("%s operation identity changed across replay", name)
		}
		leftTransferID, leftOK := left.TransferContentID(leftOperation, leftTransfer)
		rightTransferID, rightOK := right.TransferContentID(rightOperation, rightTransfer)
		if !leftOK || !rightOK || leftTransferID != rightTransferID {
			t.Fatalf("%s transfer identity changed across replay", name)
		}
		for outcome := 0; outcome < 2; outcome++ {
			leftOutcomeID, leftDisposition, leftOK := left.TransferOutcomeContentID(leftOperation, leftTransfer, outcome)
			rightOutcomeID, rightDisposition, rightOK := right.TransferOutcomeContentID(rightOperation, rightTransfer, outcome)
			if !leftOK || !rightOK || leftDisposition != rightDisposition || leftOutcomeID != rightOutcomeID {
				t.Fatalf("%s outcome %d identity changed across replay", name, outcome)
			}
		}
	}
}

func TestEndpointIdentityExcludesUnrelatedTargetRecords(t *testing.T) {
	base := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("alpha"),
	}})
	withUnrelatedOperation := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("alpha"),
		endpointIdentityOperation("aardvark-unrelated"),
	}})
	if base.ContentID() == withUnrelatedOperation.ContentID() {
		t.Fatal("unrelated operation did not change the complete target contract")
	}
	assertEndpointIdentityEqual(t,
		endpointIdentitySnapshotFor(t, withUnrelatedOperation, "alpha"),
		endpointIdentitySnapshotFor(t, base, "alpha"),
	)

	bootBase := completeBootSpec("Lua 5.3", InitialMutable)
	bootBase.Operations = append(bootBase.Operations, endpointIdentityOperation("alpha"))
	bootChanged := completeBootSpec("Lua 5.4", InitialMutable)
	bootChanged.Operations = append(bootChanged.Operations, endpointIdentityOperation("alpha"))
	baseline := mustSeal(t, bootBase)
	withUnrelatedBootValue := mustSeal(t, bootChanged)
	if baseline.ContentID() == withUnrelatedBootValue.ContentID() {
		t.Fatal("unrelated boot value did not change the complete target contract")
	}
	assertEndpointIdentityEqual(t,
		endpointIdentitySnapshotFor(t, withUnrelatedBootValue, "alpha"),
		endpointIdentitySnapshotFor(t, baseline, "alpha"),
	)
}

func TestEndpointIdentityTracksExactLocalSemanticMutations(t *testing.T) {
	base := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("alpha")}})
	prior := endpointIdentitySnapshotFor(t, base, "alpha")

	operationMutation := endpointIdentityOperation("alpha")
	operationMutation.Input.Fixed[0] = testInteger
	assertEndpointIdentityChanged(t,
		endpointIdentitySnapshotFor(t, mustSeal(t, Spec{Operations: []OperationSpec{operationMutation}}), "alpha"),
		prior,
	)

	transferMutation := endpointIdentityOperation("alpha")
	transferMutation.Transfers[0].Identity = TransferIdentityDistinct
	assertEndpointIdentityChanged(t,
		endpointIdentitySnapshotFor(t, mustSeal(t, Spec{Operations: []OperationSpec{transferMutation}}), "alpha"),
		prior,
	)

	outcomeMutation := endpointIdentityOperation("alpha")
	outcomeMutation.Transfers[0].Outcomes[0].Possibility = TransferMayReject
	assertEndpointIdentityChanged(t,
		endpointIdentitySnapshotFor(t, mustSeal(t, Spec{Operations: []OperationSpec{outcomeMutation}}), "alpha"),
		prior,
	)
}

func TestEndpointIdentityKeepsDistinctSealedTransferDeclarations(t *testing.T) {
	first := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityTwoTransferOperation("two-transfers")}})
	second := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityTwoTransferOperation("two-transfers")}})
	firstOperation, ok := first.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"two-transfers"}})
	if !ok || first.TransferCount(firstOperation) != 2 {
		t.Fatal("first sealed transfer declarations unavailable")
	}
	secondOperation, ok := second.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"two-transfers"}})
	if !ok || second.TransferCount(secondOperation) != 2 {
		t.Fatal("second sealed transfer declarations unavailable")
	}
	for index := 0; index < 2; index++ {
		firstTransfer, ok := first.TransferIDAt(firstOperation, index)
		if !ok {
			t.Fatalf("first transfer %d unavailable", index)
		}
		secondTransfer, ok := second.TransferIDAt(secondOperation, index)
		if !ok {
			t.Fatalf("second transfer %d unavailable", index)
		}
		firstID, firstOK := first.TransferContentID(firstOperation, firstTransfer)
		secondID, secondOK := second.TransferContentID(secondOperation, secondTransfer)
		if !firstOK || !secondOK || firstID != secondID {
			t.Fatalf("transfer %d identity changed across replay", index)
		}
		if index == 0 {
			continue
		}
		priorTransfer, ok := first.TransferIDAt(firstOperation, index-1)
		if !ok {
			t.Fatalf("prior transfer %d unavailable", index-1)
		}
		priorID, ok := first.TransferContentID(firstOperation, priorTransfer)
		if !ok || priorID == firstID {
			t.Fatal("distinct sealed transfer declarations reused identity")
		}
	}
}

func TestEndpointIdentityScopesCrossContractHandlesAndRejectsUnavailable(t *testing.T) {
	local := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("local-alpha"),
		endpointIdentityOperation("local-beta"),
	}})
	foreign := mustSeal(t, Spec{Operations: []OperationSpec{
		endpointIdentityOperation("foreign-alpha"),
		endpointIdentityOperation("foreign-beta"),
		endpointIdentityOperation("foreign-gamma"),
		endpointIdentityOperation("foreign-zeta"),
	}})
	foreignOperation, foreignTransfer := endpointIdentityOperationAndTransfer(t, foreign, "foreign-zeta")
	localOperation, localTransfer := endpointIdentityOperationAndTransfer(t, local, "local-alpha")
	foreignEqualOperation, foreignEqualTransfer := endpointIdentityOperationAndTransfer(t, foreign, "foreign-alpha")
	if localOperation != foreignEqualOperation || localTransfer != foreignEqualTransfer {
		t.Fatal("law requires numerically coincident receiver-local handles")
	}
	localID, ok := local.TransferContentID(localOperation, localTransfer)
	if !ok {
		t.Fatal("local transfer identity unavailable")
	}
	foreignID, ok := foreign.TransferContentID(foreignEqualOperation, foreignEqualTransfer)
	if !ok || foreignID == localID {
		t.Fatal("numerically equal handles across contracts reused identity")
	}
	localOutcome, _, ok := local.TransferOutcomeContentID(localOperation, localTransfer, 0)
	if !ok {
		t.Fatal("local outcome identity unavailable")
	}
	foreignOutcome, _, ok := foreign.TransferOutcomeContentID(foreignEqualOperation, foreignEqualTransfer, 0)
	if !ok || foreignOutcome == localOutcome {
		t.Fatal("numerically equal outcome handles across contracts reused identity")
	}
	if _, ok := local.OperationContentID(foreignOperation); ok {
		t.Fatal("unavailable foreign operation handle was accepted")
	}
	if _, ok := local.TransferContentID(foreignOperation, foreignTransfer); ok {
		t.Fatal("unavailable foreign transfer handle was accepted")
	}
	if _, _, ok := local.TransferOutcomeContentID(foreignOperation, foreignTransfer, 0); ok {
		t.Fatal("unavailable foreign transfer outcome handle was accepted")
	}
	if _, ok := local.OperationContentID(Operation(99)); ok {
		t.Fatal("out-of-range operation accepted")
	}
	if _, ok := local.TransferContentID(Operation(1), TransferID(99)); ok {
		t.Fatal("out-of-range transfer accepted")
	}
}

// The identity columns are sealed once.  These hot queries must stay plain
// bounded table projections; rebuilding a descriptor here would both allocate
// and make every Boundary consumer pay the whole Target serialization tax.
func TestEndpointIdentityQueriesAllocateNothing(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("allocation")}})
	op, transfer := endpointIdentityOperationAndTransfer(t, contract, "allocation")
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = contract.OperationContentID(op)
		_, _ = contract.OutcomeContentID(op, 0)
		_, _ = contract.TransferContentID(op, transfer)
		_, _, _ = contract.TransferOutcomeContentID(op, transfer, 0)
	}); allocations != 0 {
		t.Fatalf("semantic identity queries allocated %v times", allocations)
	}
}

// Target handles are intentionally dense scalar coordinates, not branded Go
// capabilities. A receiver can reject out-of-range or owner-mismatched rows,
// but it cannot discover that a numerically equal scalar came from another
// equivalent Contract. The portable ContentID is the value for cross-contract
// comparison; the Contract remains the handle owner.
func TestEndpointIdentityDocumentsReceiverLocalScalarHandles(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("same")}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("same")}})
	leftOp, leftTransfer := endpointIdentityOperationAndTransfer(t, left, "same")
	rightOp, rightTransfer := endpointIdentityOperationAndTransfer(t, right, "same")
	if leftOp != rightOp || leftTransfer != rightTransfer {
		t.Fatal("law requires coincident scalar coordinates")
	}
	leftOutcome, ok := left.OutcomeContentID(leftOp, 0)
	if !ok {
		t.Fatal("left outcome unavailable")
	}
	if got, ok := left.OutcomeContentID(rightOp, 0); !ok || got != leftOutcome {
		t.Fatal("receiver-local outcome coordinate was not interpreted locally")
	}
	leftID, ok := left.TransferContentID(leftOp, leftTransfer)
	if !ok {
		t.Fatal("left transfer unavailable")
	}
	if got, ok := left.TransferContentID(rightOp, rightTransfer); !ok || got != leftID {
		t.Fatal("receiver-local transfer coordinate was not interpreted locally")
	}
	leftTransferOutcome, _, ok := left.TransferOutcomeContentID(leftOp, leftTransfer, 0)
	if !ok {
		t.Fatal("left transfer outcome unavailable")
	}
	if got, _, ok := left.TransferOutcomeContentID(rightOp, rightTransfer, 0); !ok || got != leftTransferOutcome {
		t.Fatal("receiver-local transfer outcome coordinate was not interpreted locally")
	}
}

// These are deliberately appended in source order but sort before the named
// operation at Seal.  They exercise the former global CallbackID, Operation,
// and Values leaks without changing any declared relation of the subject.
func TestEndpointIdentityIgnoresEarlierGlobalCallbackAndEffectCoordinates(t *testing.T) {
	baselineSpec := callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 1})
	baseline := mustSeal(t, baselineSpec)
	owner, ok := baseline.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"release-zero-owner"}})
	if !ok {
		t.Fatal("release owner absent")
	}
	before, ok := baseline.OperationContentID(owner)
	if !ok {
		t.Fatal("release owner ID unavailable")
	}

	withEarlier := callbackReleaseZeroSpec(CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 1})
	withEarlier.Operations = append(withEarlier.Operations, endpointIdentityOperation("aardvark-earlier"))
	afterContract := mustSeal(t, withEarlier)
	owner, ok = afterContract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"release-zero-owner"}})
	if !ok {
		t.Fatal("release owner absent after insertion")
	}
	after, ok := afterContract.OperationContentID(owner)
	if !ok || after != before {
		t.Fatal("earlier unrelated operation renumbered callback/release identity")
	}

	spawnBase := mustSeal(t, Spec{Operations: []OperationSpec{spawnTestOperation("spawn-id")}})
	spawnOp, ok := spawnBase.Lookup(BindingSpec{Namespace: BindingModule, Owner: []string{"coroutine"}, Member: []string{"spawn-id"}})
	if !ok {
		t.Fatal("spawn operation absent")
	}
	spawnID, ok := spawnBase.OperationContentID(spawnOp)
	if !ok {
		t.Fatal("spawn ID unavailable")
	}
	spawnSpec := Spec{Operations: []OperationSpec{spawnTestOperation("spawn-id"), endpointIdentityOperation("aardvark-spawn")}}
	spawnChanged := mustSeal(t, spawnSpec)
	spawnOp, ok = spawnChanged.Lookup(BindingSpec{Namespace: BindingModule, Owner: []string{"coroutine"}, Member: []string{"spawn-id"}})
	if !ok {
		t.Fatal("spawn operation absent after insertion")
	}
	if got, ok := spawnChanged.OperationContentID(spawnOp); !ok || got != spawnID {
		t.Fatal("earlier unrelated operation renumbered spawn callback/Values identity")
	}
}

func TestEndpointIdentityUsesSymbolicEffectAndProducedAnchors(t *testing.T) {
	effectBase := mustSeal(t, deltaEffects(2))
	effectOp, ok := effectBase.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-a"}})
	if !ok {
		t.Fatal("effect source absent")
	}
	beforeEffect, ok := effectBase.OperationContentID(effectOp)
	if !ok {
		t.Fatal("effect identity unavailable")
	}
	effectWithEarlier := deltaEffects(2)
	effectWithEarlier.Operations = append(effectWithEarlier.Operations, endpointIdentityOperation("aardvark-effect"))
	effectChanged := mustSeal(t, effectWithEarlier)
	effectOp, _ = effectChanged.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-a"}})
	if got, ok := effectChanged.OperationContentID(effectOp); !ok || got != beforeEffect {
		t.Fatal("earlier operation renumbered effect target identity")
	}
	effectMutation := mustSeal(t, deltaEffects(3))
	effectOp, _ = effectMutation.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"effect-a"}})
	if got, ok := effectMutation.OperationContentID(effectOp); !ok || got == beforeEffect {
		t.Fatal("effect target mutation did not change declaration identity")
	}

	producedBase := mustSeal(t, deltaProduced(0))
	parent, ok := producedBase.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"produced"}})
	if !ok {
		t.Fatal("produced parent absent")
	}
	child, _, ok := producedBase.ProducedForResult(parent, 0, 0)
	if !ok {
		t.Fatal("produced child absent")
	}
	parentID, parentOK := producedBase.OperationContentID(parent)
	childID, childOK := producedBase.OperationContentID(child)
	if !parentOK || !childOK || parentID == childID {
		t.Fatal("produced path did not mint distinct operation identities")
	}
	producedReplay := mustSeal(t, deltaProduced(0))
	replayParent, _ := producedReplay.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"produced"}})
	replayChild, _, _ := producedReplay.ProducedForResult(replayParent, 0, 0)
	if got, _ := producedReplay.OperationContentID(replayParent); got != parentID {
		t.Fatal("produced parent replay changed identity")
	}
	if got, _ := producedReplay.OperationContentID(replayChild); got != childID {
		t.Fatal("produced child replay changed identity")
	}
	producedMutation := mustSeal(t, deltaProduced(1))
	mutationParent, _ := producedMutation.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"produced"}})
	if got, _ := producedMutation.OperationContentID(mutationParent); got == parentID {
		t.Fatal("produced capture mutation did not change parent declaration identity")
	}

	callbackBaseSpec := Spec{Operations: []OperationSpec{deltaCallbackOperation("callback-result", 0, 1, true)}}
	callbackBase := mustSeal(t, callbackBaseSpec)
	callbackOp, _ := callbackBase.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-result"}})
	callbackID, _ := callbackBase.OperationContentID(callbackOp)
	callbackEarlier := Spec{Operations: []OperationSpec{deltaCallbackOperation("callback-result", 0, 1, true), endpointIdentityOperation("aardvark-callback-result")}}
	callbackChanged := mustSeal(t, callbackEarlier)
	callbackOp, _ = callbackChanged.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-result"}})
	if got, ok := callbackChanged.OperationContentID(callbackOp); !ok || got != callbackID {
		t.Fatal("earlier operation renumbered callback-result identity")
	}
	callbackMutation := mustSeal(t, Spec{Operations: []OperationSpec{deltaCallbackOperation("callback-result", 0, 2, true)}})
	callbackOp, _ = callbackMutation.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"callback-result"}})
	if got, _ := callbackMutation.OperationContentID(callbackOp); got == callbackID {
		t.Fatal("callback-result selector mutation did not change declaration identity")
	}
}

func endpointIdentityCapturedRootOperation(name string) OperationSpec {
	empty := ValuesSpec{Tail: ValuesClosed}
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    empty,
		Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: empty}},
		Subedges: []SubedgeSpec{{
			Role: 1, Family: SubedgeFamilyCall, Admission: OrdinaryCallable,
			Callee:    SubedgeCalleeSpec{Kind: SubedgeCalleeCapturedInitialRead, Read: CapturedInitialReadSpec{Root: "GlobalEnvRoot", Key: bootLiteralKey("assert")}},
			Arguments: empty, RuleEntry: true,
			Outcomes: []TerminalSpec{
				{Kind: flowkind.OutcomeNormal, Values: empty}, {Kind: flowkind.OutcomeReturn, Values: empty}, {Kind: flowkind.OutcomeThrow, Values: empty},
				{Kind: flowkind.OutcomeYield, Values: empty}, {Kind: flowkind.OutcomeCancel, Values: empty},
			},
			AdmissionFailure: AdmissionFailureSpec{Values: empty, Route: AdmissionRouteSpec{Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0}},
			Routes: []SubedgeRouteSpec{
				{Kind: flowkind.OutcomeNormal, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
				{Kind: flowkind.OutcomeReturn, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
				{Kind: flowkind.OutcomeThrow, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
				{Kind: flowkind.OutcomeYield, Route: RoutePropagateYield, Adjustment: AdjustmentPreserve, Result: empty},
				{Kind: flowkind.OutcomeCancel, Route: RouteOutcome, Adjustment: AdjustmentPreserve, Result: empty, Placement: PlacementFixed, Outcome: 0},
			},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestEndpointIdentityCapturedRootUsesRootIdentityNotCoordinate(t *testing.T) {
	baseSpec := completeBootSpec("Lua 5.3", InitialMutable)
	baseSpec.Operations = append(baseSpec.Operations, endpointIdentityCapturedRootOperation("root-read"))
	base := mustSeal(t, baseSpec)
	op, ok := base.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"root-read"}})
	if !ok {
		t.Fatal("captured-root operation absent")
	}
	before, ok := base.OperationContentID(op)
	if !ok {
		t.Fatal("captured-root identity unavailable")
	}
	withEarlierRoot := completeBootSpec("Lua 5.3", InitialMutable)
	withEarlierRoot.InitialRoots = append(withEarlierRoot.InitialRoots, InitialRootSpec{Identity: "AardvarkRoot", Shape: BootShapeSpec{Aggregate: BootAggregateTable, Value: InitialValueSpec{Kind: InitialValueRoot, Root: "AardvarkRoot"}}})
	withEarlierRoot.Operations = append(withEarlierRoot.Operations, endpointIdentityCapturedRootOperation("root-read"))
	changed := mustSeal(t, withEarlierRoot)
	op, _ = changed.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"root-read"}})
	if got, ok := changed.OperationContentID(op); !ok || got != before {
		t.Fatal("unrelated earlier root renumbered captured-root identity")
	}
}

func TestEndpointIdentitySubedgeCallbackSelectorLocalityAndMutation(t *testing.T) {
	base := mustSeal(t, Spec{Operations: []OperationSpec{callbackLifecycleOperation("subedge-callback", CallbackSyncOptionalOnce, CallbackSyncOptionalOnce)}})
	op, _ := base.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"subedge-callback"}})
	before, _ := base.OperationContentID(op)
	withEarlier := mustSeal(t, Spec{Operations: []OperationSpec{callbackLifecycleOperation("subedge-callback", CallbackSyncOptionalOnce, CallbackSyncOptionalOnce), endpointIdentityOperation("aardvark-subedge")}})
	op, _ = withEarlier.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"subedge-callback"}})
	if got, ok := withEarlier.OperationContentID(op); !ok || got != before {
		t.Fatal("earlier operation renumbered subedge callback selector")
	}
	mutated := callbackLifecycleOperation("subedge-callback", CallbackSyncOptionalOnce, CallbackSyncOptionalOnce)
	mutated.Subedges[0].Callee.Callback = 2
	mutated.Subedges[1].Callee.Callback = 1
	mutation := mustSeal(t, Spec{Operations: []OperationSpec{mutated}})
	op, _ = mutation.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"subedge-callback"}})
	if got, _ := mutation.OperationContentID(op); got == before {
		t.Fatal("subedge callback selector mutation did not change identity")
	}
}

func endpointIdentityTransferDependencies(callback CallbackRef) Spec {
	closed := ValuesSpec{Tail: ValuesClosed}
	callbacks := []CallbackSpec{
		{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 0}, Admission: OrdinaryCallable, Arguments: closed, Outcomes: []TerminalSpec{{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed}, {Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed}}, Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
		{Function: InputSource{Kind: InputSourceValueFormal, Ordinal: 1}, Admission: OrdinaryCallable, Arguments: closed, Outcomes: []TerminalSpec{{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed}, {Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed}}, Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}},
	}
	return Spec{Operations: []OperationSpec{
		{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"transfer-dependencies"}}}, Input: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed}, Callbacks: callbacks, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny, testAny}, Tail: ValuesClosed}, Produced: []ProducedSpec{{Result: 0, Operation: 2, Captures: []CaptureSpec{{Kind: CaptureCallback, Ordinal: 1}}}}, CallbackResults: []CallbackResultSpec{{Result: 1, Callback: callback}}}, {Kind: flowkind.OutcomeThrow, Values: closed}}, Transfers: []TransferSpec{transfer(TransferEndpoint{Kind: TransferEndpointExternal}, InputSource{Kind: InputSourceValueFormal}, TransferIdentitySame, TransferCapabilitiesPreserveAll, []TransferOutcomeSpec{{Outcome: 0, Possibility: TransferMayDeliver}, {Outcome: 1, Possibility: TransferMayReject}})}, Effects: RowSpec{Tail: RowClosed}},
		{Input: closed, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}}, Effects: RowSpec{Tail: RowClosed}},
	}}
}

func TestEndpointTransferOutcomeCommitsProducedAndCallbackResult(t *testing.T) {
	base := mustSeal(t, endpointIdentityTransferDependencies(1))
	op, transfer := endpointIdentityOperationAndTransfer(t, base, "transfer-dependencies")
	before, _, ok := base.TransferOutcomeContentID(op, transfer, 0)
	if !ok {
		t.Fatal("dependent transfer outcome unavailable")
	}
	changed := mustSeal(t, endpointIdentityTransferDependencies(2))
	op, transfer = endpointIdentityOperationAndTransfer(t, changed, "transfer-dependencies")
	if got, _, ok := changed.TransferOutcomeContentID(op, transfer, 0); !ok || got == before {
		t.Fatal("transfer outcome omitted produced/callback-result dependency")
	}
}

func endpointIdentityEffectCycle(reverse bool) Spec {
	a, b := endpointIdentityOperation("cycle-a"), endpointIdentityOperation("cycle-b")
	a.Effects = RowSpec{Occurrences: []EffectSpec{{Target: 2, ValueArgs: []ValueFormal{0}}}, Tail: RowClosed}
	b.Effects = RowSpec{Occurrences: []EffectSpec{{Target: 1, ValueArgs: []ValueFormal{0}}}, Tail: RowClosed}
	if reverse {
		a, b = b, a
		a.Effects.Occurrences[0].Target, b.Effects.Occurrences[0].Target = 2, 1
	}
	return Spec{Operations: []OperationSpec{a, b}}
}

func endpointIdentityReleaseCycle(reverse bool) Spec {
	closed := ValuesSpec{Tail: ValuesClosed}
	makeOp := func(name string, target SpecRef) OperationSpec {
		return OperationSpec{Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}}, Input: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeThrow, Values: closed}}, Callbacks: []CallbackSpec{{Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: closed, Outcomes: []TerminalSpec{{Kind: flowkind.OutcomeNormal, Values: closed}, {Kind: flowkind.OutcomeReturn, Values: closed}, {Kind: flowkind.OutcomeThrow, Values: closed}, {Kind: flowkind.OutcomeYield, Values: closed}, {Kind: flowkind.OutcomeCancel, Values: closed}}, Lifecycle: CallbackRetainedOptionalOnce, Effects: RowSpec{Tail: RowClosed}, Release: &CallbackReleaseSpec{Operation: target, Input: 0, Outcome: 0, Mode: CallbackReleaseOne, Zero: CallbackReleaseZeroSpec{Behavior: CallbackReleaseZeroThrow, Outcome: 1}}}}, Effects: RowSpec{Tail: RowClosed}}
	}
	a, b := makeOp("release-cycle-a", 2), makeOp("release-cycle-b", 1)
	if reverse {
		a, b = b, a
		a.Callbacks[0].Release.Operation, b.Callbacks[0].Release.Operation = 2, 1
	}
	return Spec{Operations: []OperationSpec{a, b}}
}

func TestEndpointIdentityCyclesAreFiniteAndPermutationStable(t *testing.T) {
	for _, cycle := range []struct {
		name  string
		build func(bool) Spec
	}{{"effects", endpointIdentityEffectCycle}, {"releases", endpointIdentityReleaseCycle}} {
		t.Run(cycle.name, func(t *testing.T) {
			left, right := mustSeal(t, cycle.build(false)), mustSeal(t, cycle.build(true))
			for _, name := range []string{"cycle-a", "cycle-b", "release-cycle-a", "release-cycle-b"} {
				leftOp, leftOK := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
				rightOp, rightOK := right.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{name}})
				if !leftOK && !rightOK {
					continue
				}
				if !leftOK || !rightOK {
					t.Fatalf("%s missing across replay", name)
				}
				leftID, _ := left.OperationContentID(leftOp)
				rightID, _ := right.OperationContentID(rightOp)
				if leftID != rightID {
					t.Fatalf("%s identity changed across cycle replay", name)
				}
			}
		})
	}
}
