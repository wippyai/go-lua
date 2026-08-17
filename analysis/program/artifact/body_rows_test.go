package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

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

func TestProgramArtifactBodyRowsRejectReversedOutcomeRanges(t *testing.T) {
	body := valuesLawID(1)
	valid := BodyRow{id: body, context: valuesLawID(10), entry: valuesLawID(11), outcomeStart: 2, outcomeEnd: 6, sealed: true}
	if !valid.Available() || valid.OutcomeCount() != 4 {
		t.Fatal("valid Body range unavailable")
	}
	if (BodyRow{id: body, context: valuesLawID(10), entry: valuesLawID(11), outcomeStart: 6, outcomeEnd: 2, sealed: true}).Available() {
		t.Fatal("reversed Body range was available")
	}
}

func TestProgramArtifactBodyRowsRejectUnavailableEntry(t *testing.T) {
	row := BodyRow{id: valuesLawID(1), context: valuesLawID(2), entry: valuesLawID(0), sealed: true}
	if row.Available() {
		t.Fatal("body row admitted an unavailable entry")
	}
}

func TestProgramArtifactBodyRowsExposeDenseExecutableRoots(t *testing.T) {
	root := RootRow{id: valuesLawID(3), family: keyspace.FamilyBody}
	row := BodyRow{id: valuesLawID(1), context: valuesLawID(2), entry: valuesLawID(4), roots: []RootRow{root}, sealed: true}
	if !row.Available() || row.RootCount() != 1 {
		t.Fatal("body root row was not available")
	}
	got, ok := row.RootAt(0)
	if !ok || got.ID() != root.id || got.Family() == keyspace.FamilyInvalid {
		t.Fatal("body root query lost its executable root")
	}
	if _, ok := row.RootAt(1); ok {
		t.Fatal("body root query accepted its denominator")
	}
}
