package diagnostic

import (
	"reflect"
	"testing"
)

func TestEvidenceTraceUsesTerminalWitnessVocabularyAndOrder(t *testing.T) {
	items := EvidenceTrace([]Evidence{
		{Kind: EvidenceMissingProof, Trust: TrustRefuted, Cause: EvidenceCause{Kind: EvidenceCauseMissingProof}, Span: Span{StartLine: 8, StartCol: 2}, Message: "guard is absent"},
		{Kind: EvidenceUserAssertion, Trust: TrustClaimed, Cause: EvidenceCause{Kind: EvidenceCauseClaim}, Span: Span{StartLine: 4, StartCol: 2}, Message: "user asserted a type"},
		{Kind: EvidenceAbstractFact, Trust: TrustProven, Cause: EvidenceCause{Kind: EvidenceCauseBirth}, Span: Span{StartLine: 2, StartCol: 2}, Message: "value was born here"},
	})
	if len(items) != 3 {
		t.Fatalf("EvidenceTrace length = %d, want 3", len(items))
	}
	if got, want := items[0].Heading, "proven"; got != want {
		t.Fatalf("first heading = %q, want %q", got, want)
	}
	if got, want := items[1].Heading, "claimed"; got != want {
		t.Fatalf("second heading = %q, want %q", got, want)
	}
	if got, want := items[2].Heading, "missing proof"; got != want {
		t.Fatalf("third heading = %q, want %q", got, want)
	}
}

func TestSourceOrderedEvidenceTraceUsesSameLineColumnsBeforeTrust(t *testing.T) {
	laterProven := Evidence{Kind: EvidenceAbstractFact, Trust: TrustProven, Span: Span{StartLine: 6, StartCol: 14}, Message: "later proven fact"}
	earlierClaim := Evidence{Kind: EvidenceUserAssertion, Trust: TrustClaimed, Span: Span{StartLine: 6, StartCol: 11}, Message: "earlier claim"}

	ordered := SourceOrderedEvidenceTrace([]Evidence{laterProven, earlierClaim}, "main.lua")
	want := []Evidence{earlierClaim, laterProven}
	if !reflect.DeepEqual(ordered, want) {
		t.Fatalf("SourceOrderedEvidenceTrace() = %#v, want source-column order %#v", ordered, want)
	}
}
