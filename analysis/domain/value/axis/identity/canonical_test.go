package identity

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
	ids := []ID{
		{},
		{Kind: "a", Site: "bc", Index: 1},
		{Kind: "ab", Site: "c", Index: 1},
		{Kind: "a\x00b", Site: "c", Index: 1},
		{Kind: "a", Site: "\x00bc", Index: 1},
		{Kind: "same", Site: "site", Index: 0},
		{Kind: "same", Site: "site", Index: math.MaxUint64},
	}
	values := []Value{Bottom(), Top()}
	for _, id := range ids {
		values = append(values, Singleton(id))
	}
	// Equal ignores stale ID storage in every non-singleton state, including
	// invalid private states. Keep those representations in the oracle so a
	// future equality change cannot drift away from the codec unnoticed.
	for rawState := 0; rawState <= int(^uint8(0)); rawState++ {
		for _, id := range ids {
			values = append(values, Value{state: state(rawState), id: id})
		}
	}
	assertEqualBytePartition(t, values)
}

func TestCanonicalEncodingLengthFramesIdentityFields(t *testing.T) {
	left := Singleton(ID{Kind: "ab", Site: "c", Index: 7})
	right := Singleton(ID{Kind: "a", Site: "bc", Index: 7})
	nulKind := Singleton(ID{Kind: "a\x00b", Site: "c", Index: 7})
	nulSite := Singleton(ID{Kind: "a", Site: "\x00bc", Index: 7})
	for _, pair := range [][2]Value{{left, right}, {nulKind, nulSite}} {
		if Equal(pair[0], pair[1]) {
			t.Fatal("collision fixture is unexpectedly Equal")
		}
		if bytes.Equal(canonicalBytes(t, pair[0]), canonicalBytes(t, pair[1])) {
			t.Fatalf("identity fields collided: %#v and %#v", pair[0], pair[1])
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
	value := Singleton(ID{Kind: "lua.table", Site: "cancel", Index: 1})
	if err := encodeCanonical(&writer, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

func BenchmarkCanonicalEncoding(b *testing.B) {
	var writer canonical.Writer
	value := Singleton(ID{Kind: "lua.table", Site: "lexical-body-expr-v2:0123456789abcdef", Index: 77})
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
