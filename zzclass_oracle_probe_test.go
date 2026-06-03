package lua

import (
	"strings"
	"testing"
)

// TestZZClassOracleIsolated runs fixtureDiagnostics for a single class
// fixture in isolation, to determine whether its false positives reproduce
// standalone or only after other fixtures contaminate process-global state.
func TestZZClassOracleIsolated(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	// Each fixture run FIRST/alone to see its standalone behavior.
	order := []string{
		"modules/imported-map-of-time-record-store",
	}
	byName := map[string]namedSuite{}
	for _, s := range suites {
		byName[s.Name] = s
	}
	for _, name := range order {
		s, ok := byName[name]
		if !ok {
			continue
		}
		diags, entry := fixtureDiagnostics(s)
		t.Logf("=== %s (entry=%s) ===", s.Name, entry)
		for _, d := range diags {
			t.Logf("  %s:%d:%d [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message)
		}
		if len(diags) == 0 {
			t.Logf("  (no diagnostics)")
		}
	}
}

// TestZZClassOracleAfterContam runs a few non-class fixtures first, then the
// class fixture, to see whether prior fixtures flip it.
func TestZZClassOracleAfterContam(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	var target namedSuite
	for _, s := range suites {
		if s.Name == "modules/imported-self-method-store" {
			target = s
		}
	}
	// Run all suites that sort before the target (mirrors oracle ordering).
	for _, s := range suites {
		if s.Name >= target.Name {
			break
		}
		if strings.HasPrefix(s.Name, "modules/") || strings.HasPrefix(s.Name, "realworld/") {
			func() {
				defer func() { _ = recover() }()
				fixtureDiagnostics(s)
			}()
		}
	}
	diags, entry := fixtureDiagnostics(target)
	t.Logf("=== %s after-contam (entry=%s) ===", target.Name, entry)
	for _, d := range diags {
		t.Logf("  %s:%d:%d [%s] %s", d.Position.File, d.Position.Line, d.Position.Column, d.Code.Name(), d.Message)
	}
}
