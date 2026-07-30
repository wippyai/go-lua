package runtimekind

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalEncodingMatchesEqualAcrossEveryRawMask(t *testing.T) {
	var representatives []Value
	var encodedClasses [][]byte
	byBytes := make(map[string]int)
	for raw := 0; raw <= int(^uint16(0)); raw++ {
		value := Value{mask: uint16(raw)}
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
	if len(representatives) != len(byBytes) {
		t.Fatalf("Equal/byte class counts differ: %d/%d", len(representatives), len(byBytes))
	}
}

func equalClass(t testing.TB, value Value, representatives []Value) int {
	t.Helper()
	matched := -1
	for index, representative := range representatives {
		if !Equal(value, representative) {
			continue
		}
		if matched >= 0 {
			t.Fatalf("value %#v belongs to multiple Equal classes %#v and %#v", value, representatives[matched], representative)
		}
		matched = index
	}
	return matched
}

func TestCanonicalEncodingSeparatesEveryKnownTag(t *testing.T) {
	for _, left := range allTags {
		for _, right := range allTags {
			a, b := Singleton(left), Singleton(right)
			equal := Equal(a, b)
			same := bytes.Equal(canonicalBytes(t, a), canonicalBytes(t, b))
			if equal != same {
				t.Fatalf("tag %s/%s Equal=%v bytes=%v", left, right, equal, same)
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
	if err := encodeCanonical(&writer, Singleton(String)); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

func BenchmarkCanonicalEncoding(b *testing.B) {
	var writer canonical.Writer
	value := Top().Without(Nil, Thread)
	b.ReportAllocs()
	for range b.N {
		if err := writer.Reset(context.Background(), io.Discard, Key.ID(), 1); err != nil {
			b.Fatal(err)
		}
		if err := encodeCanonical(&writer, value); err != nil {
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
	for range 60 {
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
}
