package program

import "testing"

func TestOutcomeQueriesPreserveSealedTerminalDenominator(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-outcome-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	body, ok := published.BodyAt(0)
	if !ok {
		t.Fatal("BodyAt(0) unavailable")
	}
	if body.OutcomeCount() == 0 {
		t.Fatal("OutcomeCount discarded the sealed Body outcome denominator")
	}
	for index := 0; index < body.OutcomeCount(); index++ {
		outcome, outcomeOK := body.OutcomeAt(index)
		if !outcomeOK || !outcome.Available() || !outcome.BelongsTo(body) {
			t.Fatalf("OutcomeAt(%d) = %#v/%v; want an owned outcome proof", index, outcome, outcomeOK)
		}
		if _, kindOK := outcome.Kind(); !kindOK {
			t.Fatalf("OutcomeAt(%d) has no sealed kind", index)
		}
	}
	if _, ok := body.OutcomeAt(body.OutcomeCount()); ok {
		t.Fatal("OutcomeAt accepted the denominator boundary")
	}
	if _, ok := body.OutcomeAt(-1); ok {
		t.Fatal("OutcomeAt accepted a negative index")
	}
	if _, ok := published.Outcome(0); ok {
		t.Fatal("Program.Outcome accepted the zero term")
	}
	if _, ok := body.Return(); ok {
		t.Fatal("Return fabricated a terminal outcome")
	}
	normal, normalOK := body.Normal()
	if !normalOK || !normal.Available() || !normal.BelongsTo(body) {
		t.Fatal("Normal did not expose the sealed normal outcome")
	}
}
