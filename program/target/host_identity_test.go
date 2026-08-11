package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestHostPrerequisiteIDsRoundTripAndFence(t *testing.T) {
	contract := mustSeal(t, endpointIdentityTransferDependencies(1))
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer-dependencies"}})
	input, ok := contract.InputFormalID(op, 1)
	if !ok || !input.Available() {
		t.Fatal("input formal identity unavailable")
	}
	if foundOp, foundFormal, found := contract.FindInputFormalID(input); !found || foundOp != op || foundFormal != 1 {
		t.Fatalf("input inverse = %d/%d/%v", foundOp, foundFormal, found)
	}
	result, ok := contract.OutcomeResultID(op, 0, 1)
	if !ok || !result.Available() {
		t.Fatal("outcome result identity unavailable")
	}
	if foundOp, outcome, slot, found := contract.FindOutcomeResultID(result); !found || foundOp != op || outcome != 0 || slot != 1 {
		t.Fatalf("result inverse = %d/%d/%d/%v", foundOp, outcome, slot, found)
	}
	if _, ok := contract.InputFormalID(op, 2); ok {
		t.Fatal("out-of-range input formal accepted")
	}
	if _, ok := contract.OutcomeResultID(op, 0, 2); ok {
		t.Fatal("out-of-range outcome result accepted")
	}
	if _, ok := contract.InputFormalID(0, 0); ok {
		t.Fatal("zero operation accepted for input formal")
	}
	if _, ok := contract.OutcomeResultID(0, 0, 0); ok {
		t.Fatal("zero operation accepted for outcome result")
	}
}

func TestHostPrerequisiteIDsAreLocalAndReplayStable(t *testing.T) {
	base := mustSeal(t, endpointIdentityTransferDependencies(1))
	op, _ := base.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer-dependencies"}})
	input, _ := base.InputFormalID(op, 0)
	result, _ := base.OutcomeResultID(op, 0, 1)
	withUnrelated := endpointIdentityTransferDependencies(1)
	withUnrelated.Operations = append(withUnrelated.Operations, endpointIdentityOperation("aardvark-host-id"))
	changed := mustSeal(t, withUnrelated)
	op, _ = changed.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer-dependencies"}})
	if got, ok := changed.InputFormalID(op, 0); !ok || got != input {
		t.Fatal("unrelated operation changed input formal ID")
	}
	if got, ok := changed.OutcomeResultID(op, 0, 1); !ok || got != result {
		t.Fatal("unrelated operation changed outcome result ID")
	}
	mutation := mustSeal(t, endpointIdentityTransferDependencies(2))
	op, _ = mutation.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"transfer-dependencies"}})
	if got, _ := mutation.OutcomeResultID(op, 0, 1); got == result {
		t.Fatal("callback-result mutation did not change outcome result ID")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = base.InputFormalID(1, 0)
		_, _, _ = base.FindInputFormalID(input)
		_, _ = base.OutcomeResultID(1, 0, 1)
		_, _, _, _ = base.FindOutcomeResultID(result)
	}); allocations != 0 {
		t.Fatalf("host prerequisite queries allocated %v times", allocations)
	}
}

func TestInputFormalIDPermutationReplayAndForeignRangeFence(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("alpha-formal"), endpointIdentityOperation("beta-formal")}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("beta-formal"), endpointIdentityOperation("alpha-formal")}})
	leftOp, _ := left.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"alpha-formal"}})
	rightOp, _ := right.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"alpha-formal"}})
	leftID, leftOK := left.InputFormalID(leftOp, 0)
	rightID, rightOK := right.InputFormalID(rightOp, 0)
	if !leftOK || !rightOK || leftID != rightID {
		t.Fatal("input formal ID changed under authoring permutation")
	}
	foreign := mustSeal(t, Spec{Operations: []OperationSpec{endpointIdentityOperation("foreign-formal"), endpointIdentityOperation("foreign-other")}})
	foreignOp, _ := foreign.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"foreign-other"}})
	if _, ok := left.InputFormalID(foreignOp, 1); ok {
		t.Fatal("foreign out-of-range scalar coordinate accepted")
	}
	if _, _, ok := left.FindInputFormalID(keyspace.ContentID{}); ok {
		t.Fatal("zero input ID inverted")
	}
}

func TestBootAndInitialValueIDsStayNarrow(t *testing.T) {
	baseSpec := completeBootSpec("Lua 5.3", InitialMutable)
	base := mustSeal(t, baseSpec)
	boot, ok := base.BootRelationID()
	if !ok {
		t.Fatal("boot relation unavailable")
	}
	var version InitialValue
	for index := 0; index < base.InitialEntryCount(); index++ {
		_, key, value, _, found := base.InitialEntryAt(index)
		literal, keyOK := base.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			version = value
			break
		}
	}
	versionID, ok := base.InitialValueContentID(version)
	if !ok {
		t.Fatal("version value identity unavailable")
	}

	unrelated := completeBootSpec("Lua 5.3", InitialMutable)
	unrelated.Operations = append(unrelated.Operations, endpointIdentityOperation("aardvark-boot"))
	other := mustSeal(t, unrelated)
	if got, ok := other.BootRelationID(); !ok || got != boot {
		t.Fatal("unrelated operation changed boot relation")
	}
	var otherVersion InitialValue
	for index := 0; index < other.InitialEntryCount(); index++ {
		_, key, value, _, found := other.InitialEntryAt(index)
		literal, keyOK := other.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			otherVersion = value
			break
		}
	}
	if got, ok := other.InitialValueContentID(otherVersion); !ok || got != versionID {
		t.Fatal("unrelated operation changed initial value identity")
	}
	bodyMutation := completeBootSpec("Lua 5.3", InitialMutable)
	bodyMutation.Operations[0].Outcomes = append(bodyMutation.Operations[0].Outcomes, OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Tail: ValuesClosed}})
	if got, ok := mustSeal(t, bodyMutation).BootRelationID(); !ok || got != boot {
		t.Fatal("unobserved operation body changed boot relation")
	}

	rootMutation := completeBootSpec("Lua 5.3", InitialMutable)
	rootMutation.InitialRoots[0].Shape.Immutable = true
	if mutated, ok := mustSeal(t, rootMutation).BootRelationID(); !ok || mutated == boot {
		t.Fatal("root shape mutation did not change boot relation")
	}
	valueMutation := completeBootSpec("Lua 5.4", InitialMutable)
	mutated := mustSeal(t, valueMutation)
	for index := 0; index < mutated.InitialEntryCount(); index++ {
		_, key, value, _, found := mutated.InitialEntryAt(index)
		literal, keyOK := mutated.ExactKeyValue(key)
		if found && keyOK && literal.String == "_VERSION" {
			if got, ok := mutated.InitialValueContentID(value); !ok || got == versionID {
				t.Fatal("initial value mutation did not change value identity")
			}
			break
		}
	}
}
