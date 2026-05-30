package lua

import (
	"sort"
	"strings"
	"testing"
)

// TestZZUnknownWideProbe counts unknown-bearing false-positive diagnostics across
// the realworld OOP fixtures, the dominant under-inference lever. Diagnostic-only;
// kept until the lane is done. Run with ZCAP_OFF=1 to measure the baseline.
func TestZZUnknownWideProbe(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	totalUnknown, totalAll, totalMissing := 0, 0, 0
	perFixture := map[string]int{}
	for _, s := range suites {
		if !strings.HasPrefix(s.Name, "realworld/") {
			continue
		}
		raw, entry := canonicalFixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, raw, entry)
		totalMissing += len(v.missing)
		for _, u := range v.unexpected {
			totalAll++
			if strings.Contains(strings.ToLower(u), "unknown") {
				perFixture[s.Name]++
				totalUnknown++
			}
		}
	}
	t.Logf("TOTAL unknown-bearing FALSE+ across realworld: %d", totalUnknown)
	t.Logf("TOTAL all FALSE+ across realworld: %d", totalAll)
	t.Logf("TOTAL missing (lost errors) across realworld: %d", totalMissing)
	var names []string
	for n := range perFixture {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		t.Logf("  %s: %d", n, perFixture[n])
	}
}
