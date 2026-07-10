package diagnostic

import (
	"strings"
	"testing"
)

func TestTrustKindString(t *testing.T) {
	tests := []struct {
		kind TrustKind
		want string
	}{
		{TrustProven, "proven"},
		{TrustClaimed, "claimed"},
		{TrustRefuted, "refuted"},
		{TrustUnknown, "unknown"},
		{TrustKind(99), "trust(unknown)"},
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Fatalf("TrustKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestEvidenceKindString(t *testing.T) {
	tests := []struct {
		kind EvidenceKind
		want string
	}{
		{EvidenceAbstractFact, "abstract fact"},
		{EvidenceUserAssertion, "user assertion"},
		{EvidenceMissingProof, "missing proof"},
		{EvidencePrecisionBoundary, "unvalidated value"},
		{EvidenceKind(99), "evidence(unknown)"},
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Fatalf("EvidenceKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestEvidenceReasonString(t *testing.T) {
	tests := []struct {
		reason EvidenceReason
		want   string
	}{
		{EvidenceReasonUnspecified, "unspecified"},
		{EvidenceReasonBoundaryValidationMissing, "boundary validation missing"},
		{EvidenceReasonIndexReadValidationMissing, "index read validation missing"},
		{EvidenceReasonExplicitBoundaryValidation, "explicit boundary validation"},
		{EvidenceReasonUserTypeAssertion, "user type assertion"},
		{EvidenceReasonUserAssertedAny, "user asserted any"},
		{EvidenceReasonUserAssertedNonNil, "user asserted non-nil"},
		{EvidenceReason(99), "reason(unknown)"},
	}

	for _, tt := range tests {
		if got := tt.reason.String(); got != tt.want {
			t.Fatalf("EvidenceReason(%d).String() = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestNewExplanationCopiesInput(t *testing.T) {
	items := []Evidence{{
		Kind:    EvidenceUserAssertion,
		Trust:   TrustClaimed,
		Message: "original",
	}}
	explanation := NewExplanation(items...)

	items[0].Message = "mutated input"
	got := explanation.Evidence()
	if len(got) != 1 || got[0].Message != "original" {
		t.Fatalf("NewExplanation did not copy input: %#v", got)
	}

	got[0].Message = "mutated output"
	got = explanation.Evidence()
	if len(got) != 1 || got[0].Message != "original" {
		t.Fatalf("Explanation.Evidence did not return copy: %#v", got)
	}
}

func TestExplanationStringStable(t *testing.T) {
	explanation := NewExplanation(
		Evidence{
			Kind:    EvidenceUserAssertion,
			Trust:   TrustClaimed,
			Span:    Span{StartLine: 3, StartCol: 4},
			Message: "claimed by user",
		},
		Evidence{
			Kind:  EvidenceMissingProof,
			Trust: TrustUnknown,
		},
	)

	got := explanation.String()
	if !strings.Contains(got, "3:4: claimed by user") {
		t.Fatalf("Explanation.String() missing span/message: %q", got)
	}
	if !strings.Contains(got, "required proof was not found") {
		t.Fatalf("Explanation.String() missing fallback: %q", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("Explanation.String() should be newline-separated: %q", got)
	}
}

func TestExplanationStringIncludesEvidenceFile(t *testing.T) {
	explanation := NewExplanation(
		Evidence{
			Kind:    EvidenceAbstractFact,
			Trust:   TrustProven,
			File:    "protocol.lua",
			Span:    Span{StartLine: 7, StartCol: 3},
			Message: "Receipt.content is string?",
		},
		Evidence{
			Kind:    EvidenceMissingProof,
			Trust:   TrustUnknown,
			File:    "main.lua",
			Message: "no guard on this path proves content is present",
		},
	)

	got := explanation.String()
	if !strings.Contains(got, "protocol.lua:7:3: Receipt.content is string?") {
		t.Fatalf("Explanation.String() missing file/span evidence: %q", got)
	}
	if !strings.Contains(got, "main.lua: no guard on this path proves content is present") {
		t.Fatalf("Explanation.String() missing file-only evidence: %q", got)
	}
}

func TestExplanationStringEmpty(t *testing.T) {
	if got := NewExplanation().String(); got != "" {
		t.Fatalf("empty Explanation.String() = %q, want empty", got)
	}
}
