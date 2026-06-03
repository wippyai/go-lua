package lua

import (
	"testing"
)

// TestZZNilUnionProbe drives the nil-union / Result-discriminant narrowing target
// fixtures through the canonical flow and logs every emitted diagnostic plus the
// curated verdict, so the per-edge narrowing gap each fixture trips can be read
// directly. Debug probe.
func TestZZNilUnionProbe(t *testing.T) {
	targets := []string{
		"realworld/transactional-saga-orchestrator",
		"realworld/transactional-saga-orchestrator-soundness",
		"realworld/event-bus-saga-runtime",
		"realworld/event-bus-saga-runtime-soundness",
		"realworld/error-handling-chain",
		"realworld/result-type-narrowing",
		"narrowing/union-discriminated-literal",
		"narrowing/equality-discriminant",
		"narrowing/else-branch-wrong-type",
		"narrowing/boolean-discriminant",
		"narrowing/discriminator-wrong-method",
		"narrowing/union-timeout-check-pattern",
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
		diags, entry := fixtureDiagnostics(s)
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
