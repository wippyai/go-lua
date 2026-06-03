package lua

import (
	"testing"
)

// TestZZThreeFPProbe drives the three single-error false-positive fixtures
// through the canonical flow and dumps every diagnostic. Read-only diagnostic
// probe for the root-cause investigation. Keep until the three are fixed.
func TestZZThreeFPProbe(t *testing.T) {
	names := []string{
		"realworld/metatable-oop",
		"realworld/lookup-table-cast",
		"regression/gradual-typing-adversarial",
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
		diags, entry := fixtureDiagnostics(s)
		t.Logf("=== %s (entry %s): %d diagnostics ===", name, entry, len(diags))
		for _, d := range diags {
			t.Logf("  %s", diagSummary(d))
		}
	}
}
