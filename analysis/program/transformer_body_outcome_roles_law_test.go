package program

import "testing"

func TestTransformerBodyOutcomeSemanticPathRejectsEquivalentReplaySplices(t *testing.T) {
	left := transformerStorageSpliceProgram(t, "transformer-body-outcome-replay.lua")
	replay := transformerStorageSpliceProgram(t, "transformer-body-outcome-replay.lua")
	leftInput, replayInput := left.TransformerInput(), replay.TransformerInput()
	leftBody, leftBodyOK := leftInput.BodyAt(0)
	replayBody, replayBodyOK := replayInput.BodyAt(0)
	leftOutcome, leftOutcomeOK := leftBody.OutcomeAt(0)
	replayOutcome, replayOutcomeOK := replayBody.OutcomeAt(0)
	if !leftBodyOK || !replayBodyOK || !leftOutcomeOK || !replayOutcomeOK ||
		leftOutcome.PathID() != replayOutcome.PathID() || !leftOutcome.PathID().Available() {
		t.Fatal("equivalent replay did not retain its semantic Outcome path")
	}
	if leftInput.OwnsBodyOutcome(replayOutcome) || replayInput.OwnsBodyOutcome(leftOutcome) {
		t.Fatal("equivalent replay crossed exact Body Outcome ownership")
	}
	spliced := leftOutcome
	spliced.body = replayOutcome.body
	if spliced.Available() || leftInput.OwnsBodyOutcome(spliced) || spliced.PathID().Available() {
		t.Fatal("Body Outcome accepted an equivalent replay Body splice")
	}
	if target, targetKind, targeted := leftOutcome.TargetPath(); targeted || target.Available() || targetKind != OutcomeTargetInvalid {
		t.Fatal("targetless mandatory Outcome fabricated a semantic target")
	}
	if next, propagated := leftOutcome.Propagation(); propagated || next.Available() {
		t.Fatal("terminal root Outcome fabricated a propagation successor")
	}
}
