package evidence

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalEncodingMatchesEqualAcrossAdversarialCorpus(t *testing.T) {
	patterns := [][maxOrigins]Origin{
		{},
		{{Kind: OriginSource, ID: 1}},
		{3: {Kind: OriginBranch, ID: 2}},
		{
			{Kind: OriginSource, ID: 1},
			{Kind: OriginBranch, ID: 2},
			{Kind: OriginCall, ID: 3},
			{Kind: OriginAnnotation, ID: math.MaxUint64},
		},
		{
			{Kind: OriginKind(0xff), ID: 1},
			{Kind: OriginSource, ID: 0x0100},
		},
	}
	values := []Value{
		Bottom(), Top(), GradualTop(), ExplicitTop(),
		GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 1}),
		ExplicitTop().WithOrigin(Origin{Kind: OriginAnnotation, ID: math.MaxUint64}),
	}
	truncated := GradualTop()
	for index := 8; index > 0; index-- {
		truncated = truncated.WithOrigin(Origin{Kind: OriginSource, ID: uint64(index)})
	}
	values = append(values, truncated)
	for _, rawKind := range []kind{bottom, gradualTop, explicitTop, top, kind(4), kind(0xff)} {
		for _, count := range []uint8{0, 1, maxOrigins, maxOrigins + 1, 0xff} {
			for _, isTruncated := range []bool{false, true} {
				for _, items := range patterns {
					values = append(values, Value{kind: rawKind, origins: originSet{items: items, count: count, truncated: isTruncated}})
				}
			}
		}
	}
	assertEqualBytePartition(t, values)
}

func TestCanonicalEncodingIncludesInactiveOriginSlots(t *testing.T) {
	plain := GradualTop()
	stale := plain
	stale.origins.items[maxOrigins-1] = Origin{Kind: OriginCall, ID: 99}
	if Equal(plain, stale) {
		t.Fatal("inactive-slot fixture is unexpectedly Equal")
	}
	if bytes.Equal(canonicalBytes(t, plain), canonicalBytes(t, stale)) {
		t.Fatal("inactive origin storage was omitted from canonical identity")
	}
}

func TestCanonicalEncodingSeparatesOriginFieldBoundaries(t *testing.T) {
	left := Value{kind: gradualTop, origins: originSet{items: [maxOrigins]Origin{{Kind: OriginSource, ID: 0x0102}}, count: 1}}
	right := Value{kind: gradualTop, origins: originSet{items: [maxOrigins]Origin{{Kind: OriginBranch, ID: 0x02}}, count: 1}}
	if Equal(left, right) {
		t.Fatal("origin collision fixture is unexpectedly Equal")
	}
	if bytes.Equal(canonicalBytes(t, left), canonicalBytes(t, right)) {
		t.Fatal("origin kind/ID field boundary collided")
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
	value := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 1})
	if err := encodeCanonical(&writer, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

func BenchmarkCanonicalEncoding(b *testing.B) {
	var writer canonical.Writer
	value := GradualTop().WithOrigin(Origin{Kind: OriginSource, ID: 1}).WithOrigin(Origin{Kind: OriginBranch, ID: 2})
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

func assertEqualBytePartition(t testing.TB, values []Value) {
	t.Helper()
	var representatives []Value
	var encodedClasses [][]byte
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
