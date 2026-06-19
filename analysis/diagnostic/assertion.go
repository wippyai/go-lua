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
			Message: "unreachable claim state",
		}}
	}
	flags := v.Flags()
	out := make([]Evidence, 0, len(flags))
	for _, flag := range flags {
		if reason, message := assertionFlagEvidence(flag); message != "" {
			out = append(out, Evidence{
				Kind:    EvidenceUserAssertion,
				Trust:   TrustClaimed,
				Reason:  reason,
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
	_, message := assertionFlagEvidence(flag)
	return message
}

func assertionFlagEvidence(flag assertion.Flag) (EvidenceReason, string) {
	switch flag {
	case assertion.TypeClaim:
		return EvidenceReasonUserTypeAssertion, "user type assertion; not proven by analysis"
	case assertion.AnyClaim:
		return EvidenceReasonUserAssertedAny, "user asserted any; not abstract-interpreter proof"
	case assertion.NonNilClaim:
		return EvidenceReasonUserAssertedNonNil, "user asserted non-nil; nil absence not proven"
	default:
		return EvidenceReasonUnspecified, ""
	}
}
