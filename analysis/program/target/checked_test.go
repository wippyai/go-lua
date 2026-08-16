package target

import "testing"

func TestStoredRepresentationChecksRejectUnrepresentableRanges(t *testing.T) {
	if _, err := checkedStoredRange("test pool", maxInt(), 1); err == nil {
		t.Fatal("native or uint32 range overflow was accepted")
	}
	if _, err := checkedStoredHandle("test handle", maxInt()); err == nil {
		t.Fatal("one-based handle overflow was accepted")
	}
	if _, err := checkedStoredTotal("test pool", maxInt(), 1); err == nil {
		t.Fatal("aggregate range overflow was accepted")
	}
}
