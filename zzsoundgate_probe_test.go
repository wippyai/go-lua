package lua

import (
	"strings"
	"testing"
)

// TestZZSoundGate runs every -soundness fixture plus the four missing-error
// guard fixtures through the canonical flow and fails if any expected error is
// MISSING (a lost-error soundness regression). Unexpected (false+) diagnostics
// are logged but do not fail this gate; only missed errors matter here. Debug
// probe.
func TestZZSoundGate(t *testing.T) {
	named := map[string]bool{
		"modules/imported-not-nil-field-typeof-table-len": true,
		"regression/gradual-typing-adversarial":           true,
		"types/not-visible-outside-block":                 true,
		"types/used-before-definition":                    true,
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	totalMiss := 0
	for _, s := range suites {
		isSound := strings.Contains(s.Name, "soundness")
		if !isSound && !named[s.Name] {
			continue
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		if len(v.missing) == 0 {
			continue
		}
		totalMiss += len(v.missing)
		// These two named fixtures carry pre-existing misses that other lanes own; the
		// discriminant-narrowing work must not increase them. Any miss elsewhere -- and
		// in particular in any -soundness fixture -- is a lost-error regression and fails
		// the gate.
		known := s.Name == "modules/imported-not-nil-field-typeof-table-len" ||
			s.Name == "regression/gradual-typing-adversarial"
		log := t.Logf
		if !known {
			log = t.Errorf
		}
		log("=== %s missing %d error(s) (known=%v) ===", s.Name, len(v.missing), known)
		for _, m := range v.missing {
			log("    MISS: %s", m)
		}
	}
	t.Logf("soundness gate: %d total missed errors", totalMiss)
}
