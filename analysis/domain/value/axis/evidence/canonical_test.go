package evidence

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalEncodingMatchesFourSemanticStates(t *testing.T) {
	values := []Value{Bottom(), GradualTop(), ExplicitTop(), Top(), {kind: kind(4)}, {kind: kind(0xff)}}
	assertEqualBytePartition(t, values)
}

func TestCanonicalCodecRevisionTracksSemanticCarrierCut(t *testing.T) {
	if got := Spec().Erase().CanonicalCodecVersion(); got != 2 {
		t.Fatalf("evidence canonical codec revision = %d, want 2", got)
	}
}

func TestCanonicalDecoderRejectsLegacyOriginPayload(t *testing.T) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), Key.ID(), 2); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(canonicalValueRecord); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(uint64(gradualTop)); err != nil {
		t.Fatal(err)
	}
	// This is the first obsolete field of codec v1's origin carrier. Version 2
	// has no provenance payload and must leave it as rejected trailing data.
	if err := writer.Uint(0); err != nil {
		t.Fatal(err)
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	var reader canonical.Reader
	if err := reader.Reset(context.Background(), encoded, Key.ID(), 2); err != nil {
		t.Fatal(err)
	}
	if got, err := decodeCanonical(context.Background(), &reader); err != nil || !Equal(got, GradualTop()) {
		t.Fatalf("decodeCanonical = %s, %v; want gradual-top", got, err)
	}
	if err := reader.Finish(); !errors.Is(err, canonical.ErrTrailing) {
		t.Fatalf("legacy evidence payload finish = %v, want trailing-data rejection", err)
	}
}

func TestCanonicalEncodingPropagatesCancellationWithoutAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var writer canonical.Writer
	if err := writer.ResetBuffer(ctx, Key.ID(), 2); err != nil {
		t.Fatal(err)
	}
	primeCanonicalCancellation(t, &writer)
	cancel()
	if err := encodeCanonical(&writer, GradualTop()); !errors.Is(err, context.Canceled) {
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
		if err := writer.Reset(context.Background(), io.Discard, Key.ID(), 2); err != nil {
			b.Fatal(err)
		}
		if err := encodeCanonical(&writer, GradualTop()); err != nil {
			b.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			b.Fatal(err)
		}
	}
}

func assertEqualBytePartition(t testing.TB, values []Value) {
	t.Helper()
	var representatives []Value
	encodedClasses := make([][]byte, 0, len(values))
	byBytes := make(map[string]int)
	for _, value := range values {
		class := equalClass(t, value, representatives)
		if class < 0 {
			class = len(representatives)
			representatives = append(representatives, value)
			encodedClasses = append(encodedClasses, nil)
		}
		encoded := canonicalBytes(t, value)
		if prior := encodedClasses[class]; prior != nil && !bytes.Equal(prior, encoded) {
			t.Fatalf("Equal values %#v and %#v encoded differently", representatives[class], value)
		}
		encodedClasses[class] = encoded
		if prior, exists := byBytes[string(encoded)]; exists && prior != class {
			t.Fatalf("unequal values %#v and %#v encoded identically", representatives[prior], value)
		}
		byBytes[string(encoded)] = class
	}
}

func equalClass(t testing.TB, value Value, representatives []Value) int {
	t.Helper()
	for index, representative := range representatives {
		if Equal(value, representative) {
			return index
		}
	}
	return -1
}

func canonicalBytes(t testing.TB, value Value) []byte {
	t.Helper()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), Key.ID(), 2); err != nil {
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
	for range 60 {
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
}
