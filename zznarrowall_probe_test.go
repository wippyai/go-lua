package lua

import (
	"strings"
	"testing"
)

// TestZZNarrowAll runs every narrowing/* fixture through the canonical flow and
// reports its verdict, so a discriminant-narrowing change can be checked for new
// false positives or missed errors across the whole narrowing suite. Debug probe.
func TestZZNarrowAll(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	pass, fail := 0, 0
	for _, s := range suites {
		if !strings.HasPrefix(s.Name, "narrowing/") {
			continue
		}
		diags, entry := fixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		if v.passed {
			pass++
			continue
		}
		fail++
		t.Logf("=== %s (%d missing, %d unexpected) ===", s.Name, len(v.missing), len(v.unexpected))
		for _, m := range v.missing {
			t.Logf("    MISS: %s", m)
		}
		for _, u := range v.unexpected {
			t.Logf("    FALSE+: %s", u)
		}
	}
	t.Logf("narrowing suite: %d pass, %d fail", pass, fail)
}
