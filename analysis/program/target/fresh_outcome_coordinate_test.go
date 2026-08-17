package target

import (
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestFreshOutcomeOrderingRemapsSuspensionAndResumeCoordinates(t *testing.T) {
	resumeMappings := []ResumeOutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Outcome: 0}, {Kind: flowkind.OutcomeReturn, Outcome: 0},
		{Kind: flowkind.OutcomeThrow, Outcome: 0}, {Kind: flowkind.OutcomeYield, Outcome: 0},
		{Kind: flowkind.OutcomeCancel, Outcome: 0},
	}
	contract := mustSeal(t, Spec{Operations: []OperationSpec{
		{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh-suspend"}}},
			Input:    ValuesSpec{Tail: ValuesClosed},
			// The Fresh Table case is source ordinal zero but sorts after the
			// otherwise-identical no-fresh case.
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, FreshResults: []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}},
			},
			Suspensions: []SuspensionSpec{{Yield: 2, Reentry: 0, Source: ReentryByCall, Multiplicity: ReentryOnce}},
			Effects:     RowSpec{Tail: RowClosed},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh-resume"}}},
			ValuesVars: 1,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, FreshResults: []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassThread}}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}},
			},
			Resumes: []ResumeSpec{{Source: ResumeSourceValueFormal, Carrier: 0, Arguments: callbackTail(0), Outcomes: resumeMappings}},
			Effects: RowSpec{Tail: RowClosed},
		},
		{
			Bindings:   []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"fresh-spawn"}}},
			ValuesVars: 7,
			Input:      ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesVariable, Var: 0},
			Callbacks: []CallbackSpec{{
				Function: InputSource{Kind: InputSourceValueFormal}, Admission: schematype.CallableAdmissionOrdinary, Arguments: callbackTail(1),
				Outcomes: callbackOutcomes(2, 3, 4, 5, 6), Lifecycle: CallbackRetainedRequiredOnce,
				Effects: RowSpec{Tail: RowClosed},
			}},
			Outcomes: []OutcomeSpec{
				{Kind: flowkind.OutcomeYield, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}, FreshResults: []FreshResultSpec{{Result: 0, Kind: schematype.FreshClassTable}}},
				{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}},
				{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []schematype.Type{testAny}, Tail: ValuesClosed}},
			},
			Suspensions: []SuspensionSpec{{Yield: 0, Reentry: 2, Source: ReentryByProvider, Multiplicity: ReentryOnce}},
			Spawns: []SpawnSpec{{
				Function: InputSource{Kind: InputSourceValueFormal}, Child: 1, Yield: 0, ParentResume: 2, ChildEntry: 2,
				Alternatives: []SpawnSiblingAlternative{SpawnChildEntryThenParentResume, SpawnParentResumeThenChildEntry},
			}},
			Effects: RowSpec{Tail: RowClosed},
		},
	}})

	suspend, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh-suspend"}})
	yield, reentry, _, _, ok := contract.SuspensionAt(suspend, 0)
	if !ok {
		t.Fatal("fresh suspension missing")
	}
	if kind, _, found := contract.OutcomeAt(suspend, int(yield)); !found || kind != flowkind.OutcomeYield {
		t.Fatalf("suspension yield = %d/%d/%v", yield, kind, found)
	}
	if _, kind, _, found := contract.FreshResultForResult(suspend, int(reentry), 0); !found || kind != schematype.FreshClassTable {
		t.Fatalf("suspension reentry = %d lacks remapped schematype.FreshClassTable case", reentry)
	}

	resume, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh-resume"}})
	resumeID, ok := contract.ResumeIDAt(resume, 0)
	if !ok {
		t.Fatal("fresh resume missing")
	}
	for index := 0; index < contract.ResumeOutcomeCount(resumeID); index++ {
		_, outcome, found := contract.ResumeOutcomeAt(resumeID, index)
		if !found {
			t.Fatalf("resume mapping %d missing", index)
		}
		if _, kind, _, fresh := contract.FreshResultForResult(resume, int(outcome), 0); !fresh || kind != schematype.FreshClassThread {
			t.Fatalf("resume mapping %d = outcome %d without remapped schematype.FreshClassThread case", index, outcome)
		}
	}

	spawn, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"fresh-spawn"}})
	spawnID, ok := contract.SpawnIDAt(spawn, 0)
	if !ok {
		t.Fatal("fresh spawn missing")
	}
	_, _, _, parentYield, parentResume, childEntry, resumeValues, found := contract.Spawn(spawnID)
	if !found {
		t.Fatal("fresh spawn relation unavailable")
	}
	if kind, _, found := contract.OutcomeAt(spawn, int(parentYield)); !found || kind != flowkind.OutcomeYield {
		t.Fatalf("spawn yield = %d/%d/%v", parentYield, kind, found)
	}
	if kind, values, found := contract.OutcomeAt(spawn, int(parentResume)); !found || kind != flowkind.OutcomeNormal || values != childEntry || values != resumeValues || contract.ValuesCount(values) != 0 {
		t.Fatalf("spawn resume = %d/%d/%d/%d/%v", parentResume, kind, childEntry, resumeValues, found)
	}
}
