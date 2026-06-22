package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFirstDiagnosticTypeResolutionUsesFirstSuccessfulAttempt(t *testing.T) {
	calledSecond := false
	got := firstDiagnosticTypeResolution(
		diagnosticTypeResolution{Type: typ.Unknown, Source: "fallback"},
		diagnosticTypeResolutionAttempt{
			Source: "miss",
			Resolve: func() (typ.Type, bool) {
				return nil, false
			},
		},
		diagnosticTypeResolutionAttempt{
			Source:           "winner",
			UntrustedTopLike: true,
			Resolve: func() (typ.Type, bool) {
				return typ.String, true
			},
		},
		diagnosticTypeResolutionAttempt{
			Source: "late",
			Resolve: func() (typ.Type, bool) {
				calledSecond = true
				return typ.Number, true
			},
		},
	)
	if got.Type != typ.String || got.Source != "winner" || !got.UntrustedTopLike || !got.OK {
		t.Fatalf("resolution = %#v, want first successful string/untrusted winner", got)
	}
	if calledSecond {
		t.Fatalf("resolution evaluated attempts after the first success")
	}
}

func TestFirstDiagnosticTypeResolutionReturnsFallbackWhenNoAttemptMatches(t *testing.T) {
	fallback := diagnosticTypeResolution{Type: typ.Unknown, Source: "fallback", OK: true}
	got := firstDiagnosticTypeResolution(
		fallback,
		diagnosticTypeResolutionAttempt{},
		diagnosticTypeResolutionAttempt{
			Source: "miss",
			Resolve: func() (typ.Type, bool) {
				return typ.String, false
			},
		},
	)
	if got != fallback {
		t.Fatalf("resolution = %#v, want fallback %#v", got, fallback)
	}
}
