package artifact

import "testing"

func TestOutcomeRowsRejectInvalidKindAndExposeClosedReturnValues(t *testing.T) {
	if (OutcomeRow{id: valuesLawID(1), body: valuesLawID(2), kind: OutcomeKind(0), sealed: true}).Available() {
		t.Fatal("invalid outcome kind was admitted")
	}
	value := ReturnValue{id: valuesLawID(3)}
	if !value.Available() || value.ID() != valuesLawID(3) {
		t.Fatal("valid return value unavailable")
	}
}
