package composite

import (
	"testing"

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
	if directory.Count() != want {
		t.Fatalf("directory holds %d placements, sealed subscriptions = %d", directory.Count(), want)
	}
	for index := 0; index < directory.Count(); index++ {
		placement, placementOK := directory.At(index)
		if !placementOK || !placement.Available() {
			t.Fatalf("directory projected an unavailable placement: %+v", placement)
		}
	}
}
