package lua

import (
	"sort"
	"strings"
	"testing"
)

// TestZZOptIdxFixtureProbe runs specific target fixtures through the canonical
// flow and dumps every diagnostic, so ZNARROW traces the optional-index guard
// narrowing on the real fixture. Debug probe.
func TestZZOptIdxFixtureProbe(t *testing.T) {
	names := []string{
		"realworld/cqrs-order-runtime",
		"realworld/cqrs-order-runtime-soundness",
		"realworld/event-bus-saga-runtime-soundness",
		"realworld/notification-delivery-runtime-soundness",
		"realworld/transactional-saga-orchestrator-soundness",
		"realworld/agent-workflow-engine",
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}
	for _, name := range names {
		s, ok := byName[name]
		if !ok {
			t.Fatalf("fixture %q not found", name)
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		t.Logf("=== %s (entry %s): %d diagnostics ===", name, entry, len(diags))
		for _, d := range diags {
			t.Logf("  %s", diagSummary(d))
		}
	}
}

// TestZZSoundnessVerdictProbe judges every -soundness fixture plus the named
// missing-error fixtures against their curated expectations under the canonical
// flow, reporting each verdict's missing-error count so a soundness regression (a
// gained MISS introduced by the exit-guard change) is visible. Debug probe.
func TestZZSoundnessVerdictProbe(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	// Every -soundness fixture (must keep its expected errors) plus the 4 named
	// pre-existing missing-error fixtures (must not gain new misses).
	var names []string
	for _, s := range suites {
		if strings.Contains(s.Name, "soundness") {
			names = append(names, s.Name)
		}
	}
	names = append(names,
		"modules/imported-not-nil-field-typeof-table-len",
		"regression/gradual-typing-adversarial",
		"types/not-visible-outside-block",
		"types/used-before-definition",
	)
	sort.Strings(names)
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}
	for _, name := range names {
		s, ok := byName[name]
		if !ok {
			t.Logf("fixture %q not found (name may differ)", name)
			continue
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		t.Logf("=== %s: passed=%v missing=%d unexpected=%d ===", name, v.passed, len(v.missing), len(v.unexpected))
		for _, m := range v.missing {
			t.Logf("  MISS: %s", m)
		}
	}
}
