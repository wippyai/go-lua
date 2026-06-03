package lua

import (
	"testing"
)

// TestZZPluginSupervisorCanonical runs only the plugin-supervisor-runtime
// fixture through the canonical flow and prints its diagnostics. Diagnostic
// probe for the Output-vs-RenderOutput false positive at main.lua:223.
func TestZZPluginSupervisorCanonical(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	var target namedSuite
	found := false
	for _, s := range suites {
		if s.Name == "realworld/plugin-supervisor-runtime" {
			target = s
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fixture not found")
	}
	diags, entry := fixtureDiagnostics(target)
	t.Logf("entry=%s, %d diagnostics", entry, len(diags))
	for _, d := range diags {
		t.Logf("  %s", diagSummary(d))
	}
}
