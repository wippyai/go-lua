package lua

import (
	"testing"
)

// TestZZDiscUnionProbe drives the discriminated-union narrowing target fixtures
// through the canonical flow and logs every emitted diagnostic plus the curated
// verdict. Debug probe for the saga compensation include-edge and the
// plugin-supervisor exhaustiveness exclude-edge.
func TestZZDiscUnionProbe(t *testing.T) {
	targets := []string{
		"realworld/transactional-saga-orchestrator",
		"realworld/plugin-supervisor-runtime-soundness",
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}
	for _, name := range targets {
		s, ok := byName[name]
		if !ok {
			t.Logf("MISSING fixture %q", name)
			continue
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		t.Logf("=== %s passed=%v (%d missing, %d unexpected) ===", name, v.passed, len(v.missing), len(v.unexpected))
		for _, m := range v.missing {
			t.Logf("    MISS: %s", m)
		}
		for _, u := range v.unexpected {
			t.Logf("    FALSE+: %s", u)
		}
	}
}
