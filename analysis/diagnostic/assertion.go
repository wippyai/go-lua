package diagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
)

// AssertionEvidence converts assertion-axis values into diagnostic evidence.
func AssertionEvidence(span Span, v assertion.Value) []Evidence {
	if v.IsTop() {
		return nil
	}
	if v.IsBottom() {
		return []Evidence{{
			Kind:    EvidenceAbstractFact,
			Trust:   TrustRefuted,
			Span:    span,
			Message: "unreachable assertion state",
		}}
	}
	flags := v.Flags()
	out := make([]Evidence, 0, len(flags))
	for _, flag := range flags {
		if message := assertionFlagMessage(flag); message != "" {
			out = append(out, Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				Span:    span,
				Message: message,
			})
		}
	}
	return out
}

// FormatAssertionClaims renders assertion claims in stable axis order.
func FormatAssertionClaims(v assertion.Value) string {
	items := AssertionEvidence(Span{}, v)
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Message)
	}
	return strings.Join(parts, "; ")
}

func assertionFlagMessage(flag assertion.Flag) string {
	switch flag {
	case assertion.TypeAssertion:
		return "claimed by user type assertion; not proven by analysis"
	case assertion.AnyAssertion:
		return "claimed as any; not abstract-interpreter proof"
	case assertion.NonNilAssertion:
		return "claimed non-nil; nil absence not proven"
	default:
		return ""
	}
}
