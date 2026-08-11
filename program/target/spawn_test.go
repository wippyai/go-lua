package target

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
)

func spawnTestOperation(name string) OperationSpec {
	return OperationSpec{
		Bindings:   []BindingSpec{{Namespace: BindingModule, Owner: []string{"coroutine"}, Member: []string{name}}},
		ValuesVars: 7,
		Input:      ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesVariable, Var: 0},
		Callbacks: []CallbackSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Admission: OrdinaryCallable, Arguments: callbackTail(1),
			Outcomes: callbackOutcomes(2, 3, 4, 5, 6), Lifecycle: CallbackRetainedRequiredOnce, Effects: RowSpec{Tail: RowClosed},
		}},
		Outcomes: []OutcomeSpec{
			{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed}},
		},
		Suspensions: []SuspensionSpec{{Yield: 0, Reentry: 1, Source: ReentryByProvider, Multiplicity: ReentryOnce}},
		Spawns: []SpawnSpec{{
			Function: InputSource{Kind: InputSourceValueFormal}, Child: 1, Yield: 0, ParentResume: 1, ChildEntry: 1,
			Alternatives: []SpawnSiblingAlternative{SpawnChildEntryThenParentResume, SpawnParentResumeThenChildEntry},
		}},
		Effects: RowSpec{Tail: RowClosed},
	}
}

func TestSpawnSealsOneTypedDetachedAuthority(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{spawnTestOperation("spawn")}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingModule, Owner: []string{"coroutine"}, Member: []string{"spawn"}})
	if !ok || contract.SpawnCount(op) != 1 {
		t.Fatalf("spawn authority = %v/%d", ok, contract.SpawnCount(op))
	}
	id, ok := contract.SpawnIDAt(op, 0)
	owner, function, child, yield, resume, entry, resumed, ok := contract.Spawn(id)
	if !ok || owner != op || function != (InputSource{Kind: InputSourceValueFormal}) || yield == resume || entry != resumed {
		t.Fatalf("spawn relation = %#v/%#v/%d/%d/%d/%d/%d/%v", owner, function, child, yield, resume, entry, resumed, ok)
	}
	if childOwner, found := contract.CallbackOwner(child); !found || childOwner != op {
		t.Fatalf("child owner = %d/%v", childOwner, found)
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if _, found := contract.CallbackOutcome(child, kind); !found {
			t.Fatalf("child lacks total %v outcome", kind)
		}
	}
	if count := contract.SpawnSiblingCount(id); count != 2 {
		t.Fatalf("sibling alternatives = %d", count)
	}
	first, firstOK := contract.SpawnSiblingAt(id, 0)
	second, secondOK := contract.SpawnSiblingAt(id, 1)
	if !firstOK || !secondOK || first == second {
		t.Fatalf("sibling alternatives = %d/%d/%v/%v", first, second, firstOK, secondOK)
	}
}

func TestSpawnRejectsIncompleteAndDuplicateAuthority(t *testing.T) {
	bad := spawnTestOperation("bad")
	bad.Spawns[0].Alternatives = bad.Spawns[0].Alternatives[:1]
	if _, err := Seal(&Spec{Operations: []OperationSpec{bad}}); err == nil {
		t.Fatal("incomplete sibling alternatives sealed")
	}
	left, right := spawnTestOperation("left"), spawnTestOperation("right")
	if _, err := Seal(&Spec{Operations: []OperationSpec{left, right}}); err == nil {
		t.Fatal("duplicate spawn authority sealed")
	}
}

func TestSpawnSiblingOrderCanonicalizesContentIdentity(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{spawnTestOperation("spawn")}})
	rightSpec := Spec{Operations: []OperationSpec{spawnTestOperation("spawn")}}
	rightSpec.Operations[0].Spawns[0].Alternatives[0], rightSpec.Operations[0].Spawns[0].Alternatives[1] = rightSpec.Operations[0].Spawns[0].Alternatives[1], rightSpec.Operations[0].Spawns[0].Alternatives[0]
	right := mustSeal(t, rightSpec)
	if left.ContentID() != right.ContentID() {
		t.Fatal("sibling alternative authoring order changed content identity")
	}
}
