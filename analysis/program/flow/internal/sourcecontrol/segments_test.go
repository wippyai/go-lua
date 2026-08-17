package sourcecontrol

import "testing"

func TestSegmentsAndCarriersRequireIssuedOpaqueReferences(t *testing.T) {
	var segment Segment
	if segment.Valid(nil) {
		t.Fatal("zero Segment reported validity")
	}
	if _, _, ok := segment.Endpoints(); ok {
		t.Fatal("zero Segment exposed endpoints")
	}
	if _, ok := segment.Carrier(); ok {
		t.Fatal("zero Segment exposed a carrier")
	}
	if (CallTailProof{}).Available() || (CallTailProof{}).ValidFor(nil, 0, 0, 0) {
		t.Fatal("zero CallTail proof reported validity")
	}
	if ref, ok := ArcSegmentCarrier(ArcRef{}).ArcRef(); ok || ref.Available() {
		t.Fatal("foreign Arc carrier exposed a reference")
	}
}
