package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// TestQueryZeroRowFixturePublishesCoveredAbsence fixes SQ10's solve-level
// absence case. The empty root has a declared query subject but contributes no
// value-summary row, so the sealed column proves the subject absent rather
// than treating it as a key it never declared.
func TestQueryZeroRowFixturePublishesCoveredAbsence(t *testing.T) {
	solve := solveThroughReceipts(t, fixtureLink(t, "core/query-zero-row"))
	plan, opens := snapshot.OpenQuery[identity.ContentID, engine.Answer](&solve.published, solve.queryFamily)
	if !opens {
		t.Fatal("the zero-row fixture publishes no query column")
	}
	coveredAbsence := false
	for _, summary := range solve.summaries {
		if summary.answer.Rows != 0 {
			continue
		}
		if _, status := snapshot.Query(&solve.published, plan, summary.subject); status != snapshot.ReadProvenAbsent {
			t.Fatalf("zero-row summary subject %x = %s, want proven-absent", summary.subject[:4], status)
		}
		coveredAbsence = true
	}
	if !coveredAbsence {
		t.Fatal("the zero-row fixture produced no covered value-summary absence")
	}
}
