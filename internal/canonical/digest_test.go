package canonical

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
)

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

func TestDigestWriterSumIsWriterPreimageHash(t *testing.T) {
	domain := "canonical/test/digest-writer"
	payload := []byte{0, 1, 255}
	var digest DigestWriter
	if digest.Reset(domain, 7) != nil || digest.Uint(9) != nil || digest.Count(3) != nil || digest.Bytes(payload) != nil || digest.Finish() != nil {
		t.Fatal("digest writer")
	}
	var stream Writer
	if stream.ResetBuffer(context.Background(), domain, 7) != nil || stream.Uint(9) != nil || stream.Count(3) != nil || stream.Bytes(payload) != nil {
		t.Fatal("stream writer")
	}
	framed, err := stream.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	if digest.Sum() != sha256.Sum256(framed) {
		t.Fatal("DigestWriter.Sum is not SHA-256 of Writer.FinishBytes for the same events")
	}
}

func TestCanonicalWriterAndProgramFramingDoNotShareUintTag(t *testing.T) {
	var stream Writer
	if err := stream.ResetBuffer(context.Background(), "d", 1); err != nil {
		t.Fatal(err)
	}
	if err := stream.Uint(1); err != nil {
		t.Fatal(err)
	}
	canonicalBytes, err := stream.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	var framed bytes.Buffer
	var program framing.Writer
	if err := program.Reset(&framed, "d", 1); err != nil {
		t.Fatal(err)
	}
	if err := program.Uint(1); err != nil || program.Finish() != nil {
		t.Fatal("program framing")
	}
	if bytes.Equal(canonicalBytes, framed.Bytes()) {
		t.Fatal("canonical and program framing Uint streams collided; identity schemas stay apart until a re-pin")
	}
}

func TestDigestWriterSmallPreimageAllocatesAtMostTwo(t *testing.T) {
	domain := "analysis/engine/equation/allocation"
	id := [sha256.Size]byte{1}
	payload := []byte("small")
	allocs := testing.AllocsPerRun(1000, func() {
		var writer DigestWriter
		if writer.Reset(domain, 18) != nil ||
			writer.Bytes(id[:]) != nil ||
			writer.Uint(18) != nil ||
			writer.Bytes(payload) != nil ||
			writer.Count(3) != nil ||
			writer.Uint(11) != nil ||
			writer.Finish() != nil {
			t.Fatal("digest writer")
		}
		if writer.Sum() == ([sha256.Size]byte{}) {
			t.Fatal("empty digest")
		}
	})
	if allocs > 2 {
		t.Fatalf("small DigestWriter preimage allocates too much: %.2f allocs/op", allocs)
	}
}
