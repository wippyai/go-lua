package canonical

import "testing"

func TestDigestWriterResetStartsIndependentPreimage(t *testing.T) {
	var writer DigestWriter
	if writer.Reset("canonical/test/digest", 1) != nil {
		t.Fatal("first reset")
	}
	if writer.Uint(1) != nil || writer.Finish() != nil {
		t.Fatal("first write")
	}
	first := writer.Sum()
	if writer.Reset("canonical/test/digest", 1) != nil {
		t.Fatal("second reset")
	}
	if writer.Uint(2) != nil || writer.Finish() != nil {
		t.Fatal("second write")
	}
	second := writer.Sum()
	if first == second {
		t.Fatal("reset retained the previous preimage")
	}
	var fresh DigestWriter
	if fresh.Reset("canonical/test/digest", 1) != nil || fresh.Uint(2) != nil || fresh.Finish() != nil {
		t.Fatal("fresh writer")
	}
	if fresh.Sum() != second {
		t.Fatal("reused writer diverged from a fresh writer")
	}
	if writer.Reset("canonical/test/digest", 1) != nil || writer.Uint(1) != nil || writer.Finish() != nil {
		t.Fatal("third write")
	}
	if writer.Sum() != first {
		t.Fatal("reused writer could not reproduce the first preimage")
	}
}
