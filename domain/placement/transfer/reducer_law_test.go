package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/placement"
)

// transferSendTag is the shape a route tag has when the demand it carries was
// authenticated as a Send: the owner-issued dense coordinate above the policy
// code the fold reads.
func transferSendTag() uint64 {
	return uint64(1)<<routeTagShift | uint64(placement.Send+1)
}

// TestTransferFoldRequiresAuthenticatedSendRoute is the whole of what the
// transfer reducer decides: a route the relation authenticated as a Send
// displaces its predecessor into the shared heap with retention proven.
func TestTransferFoldRequiresAuthenticatedSendRoute(t *testing.T) {
	got, outcome := TransferFold(transferSendTag(), placement.DefaultFact())
	if outcome != structure.Concrete || got.Class != placement.SharedHeap || got.RetainEscape != placement.EvidenceProven {
		t.Fatalf("send fold=%v/%v, want authenticated SharedHeap/Proven", got, outcome)
	}
}

// TestTransferFoldDoesNotFabricateUnknown states the refusal side. A
// TransferSpec is strategy-neutral: it authorizes delivery but publishes no
// Placement fact of its own, so absent, non-Send, malformed, or unauthenticated
// route evidence is no result rather than a widened one.
func TestTransferFoldDoesNotFabricateUnknown(t *testing.T) {
	cases := []uint64{
		0,
		uint64(1)<<routeTagShift | uint64(placement.Retain+1),
		uint64(1)<<routeTagShift | routeTagMask,
	}
	for _, tag := range cases {
		if got, outcome := TransferFold(tag, placement.DefaultFact()); outcome != structure.Refuse || got != placement.BottomFact() {
			t.Fatalf("tag %d fold=%v/%v, want Bottom/Refuse", tag, got, outcome)
		}
	}
	if got, outcome := TransferFold(transferSendTag(), placement.BottomFact()); outcome != structure.Refuse || got != placement.BottomFact() {
		t.Fatalf("unauthenticated cell fold=%v/%v, want Bottom/Refuse", got, outcome)
	}
}
