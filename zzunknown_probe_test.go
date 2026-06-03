package lua

import (
	"strings"
	"testing"
)

// zzunknown_probe dumps the canonical-flow diagnostics for the unknown-propagation
// target fixtures so the under-inference origin can be traced. Diagnostic-only;
// kept until the unknown-propagation lane is done.
func TestZZUnknownProbe(t *testing.T) {
	targets := []string{
		"realworld/plugin-runtime-pipeline",
		"realworld/notification-delivery-runtime",
		"realworld/notification-delivery-runtime-soundness",
		"realworld/plugin-runtime-pipeline-soundness",
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
		t.Logf("=== %s (entry %s) passed=%v ===", name, entry, v.passed)
		for _, m := range v.missing {
			t.Logf("  MISS: %s", m)
		}
		for _, u := range v.unexpected {
			if strings.Contains(strings.ToLower(u), "unknown") {
				t.Logf("  FALSE+UNKNOWN: %s", u)
			} else {
				t.Logf("  FALSE+: %s", u)
			}
		}
	}
}

// TestZZSoundnessProbe verifies that no expected error is LOST on the soundness
// fixtures and the 4 known missing-error fixtures. A capture/param under-inference
// fix must never make a genuinely-unknown value precise enough to drop its error.
// Diagnostic-only; kept until the lane is done.
func TestZZSoundnessProbe(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	var soundness []namedSuite
	missingErr := map[string]bool{
		"modules/imported-not-nil-field-typeof-table-len": true,
		"regression/gradual-typing-adversarial":           true,
		"types/not-visible-outside-block":                 true,
		"types/used-before-definition":                    true,
	}
	for _, s := range suites {
		if strings.Contains(s.Name, "soundness") || missingErr[s.Name] {
			soundness = append(soundness, s)
		}
	}
	for _, s := range soundness {
		diags, entry := fixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		// A LOST expected error is the only soundness regression that matters here.
		if len(v.missing) > 0 {
			t.Logf("MISSING-ERROR in %s:", s.Name)
			for _, m := range v.missing {
				t.Logf("  MISS: %s", m)
			}
			_ = entry
		} else {
			t.Logf("OK %s (no lost expected error; passed=%v)", s.Name, v.passed)
		}
	}
}
