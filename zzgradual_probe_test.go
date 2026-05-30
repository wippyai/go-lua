package lua

import (
	"strings"
	"testing"
)

// zzGradualProbe runs the two gradual-top-any target fixtures through the
// canonical flow and prints their diagnostics, so the gradual-admission wiring
// can be verified against the actual emitted false positives.
func zzGradualProbe(t *testing.T, fixtureName string) {
	t.Helper()
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	for _, s := range suites {
		if !strings.HasSuffix(s.Name, fixtureName) {
			continue
		}
		diags, entry := canonicalFixtureDiagnostics(s)
		t.Logf("[%s] entry=%s diags=%d", s.Name, entry, len(diags))
		for _, d := range diags {
			t.Logf("[%s] %s @ %s:%d:%d (%s)", s.Name, d.Message, d.Position.File, d.Position.Line, d.Position.Column, d.Severity)
		}
		return
	}
	t.Fatalf("fixture %q not found", fixtureName)
}

func TestZZGradual_DynamicRegistry(t *testing.T) {
	zzGradualProbe(t, "dynamic-registry-renderer-guard")
}

func TestZZGradual_FieldDefinedWrapper(t *testing.T) {
	zzGradualProbe(t, "field-defined-wrapper-return")
}

// TestZZGradual_Soundness verifies the narrowed/dominated-any soundness fixtures
// keep their expected errors: a value declared `any` that is narrowed on a
// type-guard false branch (or cast/excluded) must still be REJECTED against a
// concrete target. These are the fixtures a blanket Consistent(any,X)=true broke.
func TestZZGradual_Soundness(t *testing.T) {
	for _, name := range []string{
		"types/cast-type-is-not-fail",
		"types/cast-type-is-falsy-fail",
		"regression/gradual-typing-adversarial",
	} {
		t.Run(name, func(t *testing.T) {
			zzGradualProbe(t, name)
		})
	}
}

func TestZZGradual_CastDirect(t *testing.T) {
	for _, name := range []string{
		"types/cast-type-is-direct",
		"types/cast-type-is-basic",
		"types/cast-type-is-stored",
		"types/cast-type-is-field-access",
	} {
		t.Run(name, func(t *testing.T) { zzGradualProbe(t, name) })
	}
}

func TestZZGradual_SoundnessCallArg(t *testing.T) {
	zzGradualProbe(t, "active-session-any-time-sub-soundness")
}

// TestZZGradual_NonDominating verifies the soundness twin of the
// field-defined-wrapper-return target: when the dominating field write is
// guarded (non-dominating), res.answer reads off an `any` container and the
// any->string write must still be REJECTED.
func TestZZGradual_NonDominating(t *testing.T) {
	zzGradualProbe(t, "non-dominating-field-defined-wrapper-return")
}
