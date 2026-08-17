package artifact

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestBodyOutcomeCompilerKeepsClosedOutcomeVocabulary(t *testing.T) {
	for input, want := range map[flowkind.OutcomeKind]OutcomeKind{
		flowkind.OutcomeNormal: OutcomeNormal,
		flowkind.OutcomeReturn: OutcomeReturn,
		flowkind.OutcomeThrow:  OutcomeThrow,
		flowkind.OutcomeBreak:  OutcomeBreak,
		flowkind.OutcomeGoto:   OutcomeGoto,
		flowkind.OutcomeYield:  OutcomeYield,
		flowkind.OutcomeCancel: OutcomeCancel,
	} {
		got, ok := outcomeKind(input)
		if !ok || got != want {
			t.Fatalf("outcomeKind(%d) = %d/%v, want %d/true", input, got, ok, want)
		}
	}
	if _, ok := outcomeKind(flowkind.OutcomeKind(0)); ok {
		t.Fatal("invalid outcome kind was admitted")
	}
}
