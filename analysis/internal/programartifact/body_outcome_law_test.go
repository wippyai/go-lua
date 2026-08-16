package programartifact

import "testing"

func TestProgramArtifactBodyOutcomeScalarShapesFailClosed(t *testing.T) {
	body := valuesLawID(1)
	normal := OutcomeRow{id: valuesLawID(2), body: body, kind: OutcomeNormal, sealed: true}
	returned := OutcomeRow{id: valuesLawID(3), body: body, kind: OutcomeReturn, returnStart: 4, returnEnd: 6, sealed: true}
	broken := OutcomeRow{id: valuesLawID(4), body: body, target: valuesLawID(5), kind: OutcomeBreak, hasTarget: true, sealed: true}
	jumped := OutcomeRow{id: valuesLawID(6), body: body, target: valuesLawID(7), propagation: valuesLawID(8), kind: OutcomeGoto, hasTarget: true, hasPropagation: true, sealed: true}
	for index, row := range []OutcomeRow{normal, returned, broken, jumped} {
		if !row.Available() {
			t.Fatalf("valid Outcome row %d unavailable", index)
		}
	}
	invalid := []OutcomeRow{
		{id: valuesLawID(2), body: body, target: valuesLawID(5), kind: OutcomeNormal, hasTarget: true, sealed: true},
		{id: valuesLawID(4), body: body, kind: OutcomeBreak, sealed: true},
		{id: valuesLawID(6), body: body, propagation: valuesLawID(8), kind: OutcomeGoto, hasTarget: true, hasPropagation: true, sealed: true},
		{id: valuesLawID(2), body: body, kind: OutcomeNormal, returnEnd: 1, sealed: true},
		{id: valuesLawID(2), body: body, kind: OutcomeNormal, propagation: valuesLawID(8), sealed: true},
	}
	for index, row := range invalid {
		if row.Available() {
			t.Fatalf("invalid Outcome row %d was available", index)
		}
	}
	bodyRow := BodyRow{id: body, context: valuesLawID(10), entry: valuesLawID(11), outcomeStart: 2, outcomeEnd: 6, sealed: true}
	if !bodyRow.Available() || bodyRow.OutcomeCount() != 4 {
		t.Fatal("valid Body range unavailable")
	}
	if (BodyRow{id: body, context: valuesLawID(10), entry: valuesLawID(11), outcomeStart: 6, outcomeEnd: 2, sealed: true}).Available() {
		t.Fatal("reversed Body range was available")
	}
}

func TestProgramArtifactReturnValueKeepsSemanticValuesIdentity(t *testing.T) {
	want := valuesLawID(9)
	value := ReturnValue{id: want}
	if !value.Available() || value.ID() != want {
		t.Fatal("ReturnValue did not retain the existing Values identity")
	}
	if (ReturnValue{}).Available() {
		t.Fatal("zero ReturnValue was available")
	}
}
