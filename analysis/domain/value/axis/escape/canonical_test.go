package escape

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalEncodingMatchesEqualExhaustively(t *testing.T) {
	encoded := make([][]byte, 1<<8)
	for raw := range encoded {
		encoded[raw] = canonicalBytes(t, Value(raw))
	}
	for left := range encoded {
		for right := range encoded {
			equal := Equal(Value(left), Value(right))
			same := bytes.Equal(encoded[left], encoded[right])
			if equal != same {
				t.Fatalf("Equal/bytes mismatch for raw values %d and %d: equal=%v bytes=%v", left, right, equal, same)
			}
		}
	}
}

func TestCanonicalEncodingPropagatesCancellationWithoutAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, Key.ID(), 1); err != nil {
		t.Fatal(err)
	}
	primeCanonicalCancellation(t, &writer)
	cancel()
	if err := encodeCanonical(&writer, Fresh()); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

func BenchmarkCanonicalEncoding(b *testing.B) {
	var writer canonical.Writer
	b.ReportAllocs()
	for range b.N {
		if err := writer.Reset(context.Background(), io.Discard, Key.ID(), 1); err != nil {
			b.Fatal(err)
		}
		if err := encodeCanonical(&writer, Fresh()); err != nil {
			b.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			b.Fatal(err)
		}
	}
}

func canonicalBytes(t testing.TB, value Value) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), Key.ID(), 1); err != nil {
		t.Fatal(err)
	}
	if err := encodeCanonical(&writer, value); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func primeCanonicalCancellation(t testing.TB, writer *canonical.Writer) {
	t.Helper()
	// Reset emits domain and version. Sixty harmless events leave the first
	// helper event below the periodic checkpoint and its value event on it.
	for range 60 {
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
}
