package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
)

func TestAssertionEvidenceTypeClaim(t *testing.T) {
	items := AssertionEvidence(Span{StartLine: 1, StartCol: 2}, assertion.Type())
	if len(items) != 1 {
		t.Fatalf("AssertionEvidence(type) = %d items, want 1", len(items))
	}
	item := items[0]
	if item.Kind != EvidenceUserAssertion || item.Trust != TrustClaimed {
		t.Fatalf("type claim evidence = %#v, want claimed user evidence", item)
	}
	if !strings.Contains(item.Message, "claimed by user type assertion") {
		t.Fatalf("type claim message missing claim: %q", item.Message)
	}
	if !strings.Contains(item.Message, "not proven by analysis") {
		t.Fatalf("type claim message missing non-proof: %q", item.Message)
	}
}

func TestAssertionEvidenceAnyClaimIsNotProofOrEscapeHatch(t *testing.T) {
	items := AssertionEvidence(Span{}, assertion.Any())
	if len(items) != 1 {
		t.Fatalf("AssertionEvidence(any) = %d items, want 1", len(items))
	}
	message := items[0].Message
	if !strings.Contains(message, "claimed as any") {
		t.Fatalf("any claim message missing claim: %q", message)
	}
	if !strings.Contains(message, "not abstract-interpreter proof") {
		t.Fatalf("any claim message missing non-proof: %q", message)
	}
	lower := strings.ToLower(message)
	for _, forbidden := range []string{"proven", "escape hatch"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("any claim message contains forbidden wording %q: %q", forbidden, message)
		}
	}
}

func TestAssertionEvidenceNonNilClaimIsNotNilProof(t *testing.T) {
	items := AssertionEvidence(Span{}, assertion.NonNil())
	if len(items) != 1 {
		t.Fatalf("AssertionEvidence(non-nil) = %d items, want 1", len(items))
	}
	message := items[0].Message
	if !strings.Contains(message, "claimed non-nil") {
		t.Fatalf("non-nil claim message missing claim: %q", message)
	}
	if !strings.Contains(message, "nil absence not proven") {
		t.Fatalf("non-nil claim message missing non-proof: %q", message)
	}
}

func TestAssertionEvidenceCombinedStableOrder(t *testing.T) {
	value := assertion.Of(assertion.NonNilClaim, assertion.AnyClaim, assertion.TypeClaim)
	got := FormatAssertionClaims(value)
	want := "claimed by user type assertion; not proven by analysis; " +
		"claimed as any; not abstract-interpreter proof; " +
		"claimed non-nil; nil absence not proven"
	if got != want {
		t.Fatalf("FormatAssertionClaims(combined) = %q, want %q", got, want)
	}

	items := AssertionEvidence(Span{}, value)
	if len(items) != 3 {
		t.Fatalf("AssertionEvidence(combined) = %d items, want 3", len(items))
	}
	for i, item := range items {
		if item.Kind != EvidenceUserAssertion || item.Trust != TrustClaimed {
			t.Fatalf("combined item %d = %#v, want claimed user assertion", i, item)
		}
	}
}

func TestAssertionEvidenceTopHasNoUserEvidence(t *testing.T) {
	if got := AssertionEvidence(Span{}, assertion.Top()); len(got) != 0 {
		t.Fatalf("AssertionEvidence(top) = %#v, want no evidence", got)
	}
	if got := FormatAssertionClaims(assertion.Top()); got != "" {
		t.Fatalf("FormatAssertionClaims(top) = %q, want empty", got)
	}
}

func TestAssertionEvidenceBottomRendersUnreachable(t *testing.T) {
	items := AssertionEvidence(Span{}, assertion.Bottom())
	if len(items) != 1 {
		t.Fatalf("AssertionEvidence(bottom) = %d items, want 1", len(items))
	}
	item := items[0]
	if item.Kind != EvidenceAbstractFact || item.Trust != TrustRefuted {
		t.Fatalf("bottom claim evidence = %#v, want refuted abstract fact", item)
	}
	if item.Message != "unreachable claim state" {
		t.Fatalf("bottom claim message = %q, want unreachable claim state", item.Message)
	}
	if got := FormatAssertionClaims(assertion.Bottom()); got != "unreachable claim state" {
		t.Fatalf("FormatAssertionClaims(bottom) = %q, want unreachable claim state", got)
	}
}
