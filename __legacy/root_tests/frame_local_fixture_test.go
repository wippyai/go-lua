package lua

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/placementplan"
)

func TestFrameLocalFixtureQualificationStats(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures/frame-local")
	if err != nil {
		t.Fatalf("discovering frame-local fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("no frame-local fixture suites found")
	}

	var plans []placementplan.Plan
	for _, suite := range suites {
		plans = append(plans, decomposableFixturePlans(t, suite)...)
	}
	merged := placementplan.Merge(plans...)
	total, frameLocal := merged.FrameLocalStats()
	if total == 0 {
		t.Fatal("frame-local fixture corpus has no allocation sites")
	}
	percent := 100 * float64(frameLocal) / float64(total)
	t.Logf("FRAMELOCAL FIXTURE CORPUS: %d/%d allocation sites qualify (%.1f%%)", frameLocal, total, percent)
	if frameLocal == 0 {
		t.Fatalf("frame-local fixture corpus has no qualifying allocation sites; entries=%s", formatPlacementEntries(merged))
	}
}
