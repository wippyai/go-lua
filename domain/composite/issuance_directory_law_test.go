package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// TestArtifactIssuanceDirectoryPlacesEveryMountedSubscription is the first
// issuance-flash law: every sealed mounted or activation Spec.Issues row
// becomes one compiler placement, and Link-lane rules contribute none.
func TestArtifactIssuanceDirectoryPlacesEveryMountedSubscription(t *testing.T) {
	if _, failure := Table(); failure.Available() {
		t.Fatalf("declaration table rejected: contributor=%d law=%d", failure.Contributor, failure.Law)
	}
	directory, ok := ArtifactIssuanceDirectory()
	if !ok {
		t.Fatal("issuance directory did not project from the sealed table")
	}
	want := 0
	for position, entry := range registry.templates {
		if entry == nil || entry.Lane() == rule.LaneLink {
			continue
		}
		if !entry.Key().Available() {
			t.Fatalf("mounted rule at position %d has no declaration key", position)
		}
		want += entry.IssuanceCount()
		if entry.Lane() == rule.LaneMounted && entry.IssuanceCount() == 0 {
			t.Fatalf("mounted rule %q declares no issuance", entry.Key())
		}
	}
	if len(directory) != want {
		t.Fatalf("directory holds %d placements, sealed subscriptions = %d", len(directory), want)
	}
	for _, placement := range directory {
		if !placement.Available() {
			t.Fatalf("directory projected an unavailable placement: %+v", placement)
		}
	}
}

// TestIssuanceStageLawsProjectTheSealedPredecessorChain is the engine-facing
// projection of the structure-table native/predecessor fields.
func TestIssuanceStageLawsProjectTheSealedPredecessorChain(t *testing.T) {
	if _, failure := Table(); failure.Available() {
		t.Fatalf("declaration table rejected: contributor=%d law=%d", failure.Contributor, failure.Law)
	}
	laws, ok := IssuanceStageLaws()
	if !ok || len(laws) != 3 {
		t.Fatalf("issuance stage laws projected %d rows", len(laws))
	}
	byStage := make(map[rows.ArtifactRuleStage]rows.ArtifactStageLaw, len(laws))
	for _, law := range laws {
		if !law.Valid() || !law.Native {
			t.Fatalf("projected non-native stage law: %+v", law)
		}
		if _, duplicate := byStage[law.Stage]; duplicate {
			t.Fatalf("duplicate stage law for %d", law.Stage)
		}
		byStage[law.Stage] = law
	}
	dispatch, dispatchOK := byStage[rows.ArtifactRuleStageCallDispatch]
	summary, summaryOK := byStage[rows.ArtifactRuleStageCallSummary]
	effect, effectOK := byStage[rows.ArtifactRuleStageCallEffect]
	if !dispatchOK || dispatch.Predecessor.Valid() {
		t.Fatalf("call-dispatch law %+v", dispatch)
	}
	if !summaryOK || summary.Predecessor != rows.ArtifactRuleStageCallDispatch {
		t.Fatalf("call-summary predecessor %d", summary.Predecessor)
	}
	if !effectOK || effect.Predecessor != rows.ArtifactRuleStageCallSummary {
		t.Fatalf("call-effect predecessor %d", effect.Predecessor)
	}
}
