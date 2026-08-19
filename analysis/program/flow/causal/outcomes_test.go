package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
)

func TestOutcomePhaseClassifierExcludesOnlyNormal(t *testing.T) {
	if outcomePhaseOutcome(kind.OutcomeNormal) {
		t.Fatal("Normal selected an Outcome phase")
	}
	for _, outcomeKind := range []kind.OutcomeKind{kind.OutcomeReturn, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel, kind.OutcomeBreak, kind.OutcomeGoto} {
		if !outcomePhaseOutcome(outcomeKind) {
			t.Fatalf("%v did not select an Outcome phase", outcomeKind)
		}
	}
}

// syntheticResult is deliberately assembled from the published typed rows.
// It keeps these laws independent of the upstream fixture builders while
// exercising the final authority's closed storage and query contracts.
