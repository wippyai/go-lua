package variantorigin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestCanonicalEncodingMatchesEqualAcrossReachableAndRawValues(t *testing.T) {
	values := []Value{
		Bottom(), Top(),
		Of(1, []int{1}), Singleton(1, 1),
		Of(7, []int{12, -2, 12}), Of(7, []int{-2, 12}),
		Join(Singleton(7, -2), Singleton(7, 12)),
		Meet(Of(7, []int{-2, 12, 99}), Of(7, []int{12, -2})),
		Widen(Singleton(7, -2), Singleton(7, 12)),
		Of(7, []int{-2, 12, 99}).NarrowCase(7, 99, false),
	}

	states := []state{bottom, concrete, top, state(3), state(math.MaxUint8)}
	families := []uint64{0, 1, 2, 23, math.MaxUint64}
	caseLists := [][]int{
		nil,
		{-1},
		{1},
		{12},
		{34},
		{1, 23},
		{3, 12},
		{math.MinInt, -128, -10, 0, 127, 128, math.MaxInt},
	}
	for _, rawState := range states {
		for _, family := range families {
			for _, cases := range caseLists {
				values = append(values, Value{
					state: rawState, family: family, cases: caseset.New(cases),
				})
			}
		}
	}

	var representatives []Value
	var encodedClasses [][]byte
	byBytes := make(map[string]int)
	for _, value := range values {
		class := canonicalEqualClass(t, value, representatives)
		if class < 0 {
			class = len(representatives)
			representatives = append(representatives, value)
			encodedClasses = append(encodedClasses, nil)
		}
		encoded := canonicalVariantBytes(t, value)
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

func TestCanonicalEncodingNormalizesOnlyAtOwnedConstruction(t *testing.T) {
	a := Of(17, []int{128, -12, 3, -12, 128})
	b := Of(17, []int{3, 128, -12})
	if !Equal(a, b) {
		t.Fatalf("source-order normalization produced unequal values: %#v/%#v", a, b)
	}
	if !bytes.Equal(canonicalVariantBytes(t, a), canonicalVariantBytes(t, b)) {
		t.Fatal("equal normalized source sets encoded differently")
	}

	adversarial := []Value{
		{state: concrete, family: 1, cases: caseset.New([]int{23})},
		{state: concrete, family: 12, cases: caseset.New([]int{3})},
		{state: concrete, family: 1, cases: caseset.New([]int{2, 34})},
		{state: concrete, family: 1, cases: caseset.New([]int{23, 4})},
		{state: concrete, family: math.MaxUint64, cases: caseset.New([]int{math.MinInt, math.MaxInt})},
	}
	for left := range adversarial {
		for right := left + 1; right < len(adversarial); right++ {
			if Equal(adversarial[left], adversarial[right]) {
				continue
			}
			if bytes.Equal(canonicalVariantBytes(t, adversarial[left]), canonicalVariantBytes(t, adversarial[right])) {
				t.Fatalf("framing collision between adversarial values %d and %d", left, right)
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
	primeVariantCanonicalCancellation(t, &writer)
	cancel()
	if err := encodeCanonical(&writer, Of(9, []int{-10, 12, 128})); !errors.Is(err, context.Canceled) {
		t.Fatalf("encodeCanonical error = %v", err)
	}
	if got, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("FinishBytes = %x, %v", got, err)
	}
}

func BenchmarkCanonicalEncoding(b *testing.B) {
	values := map[int]Value{
		0:  {state: concrete, family: 1},
		1:  Singleton(1, -1),
		4:  Of(1, []int{-128, -1, 12, 128}),
		16: Of(1, []int{-1024, -128, -12, -1, 0, 1, 2, 3, 10, 12, 127, 128, 255, 256, 1024, math.MaxInt}),
	}
	for _, count := range []int{0, 1, 4, 16} {
		value := values[count]
		b.Run(fmt.Sprintf("cases=%d", count), func(b *testing.B) {
			var writer canonical.Writer
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
		})
	}
}

func canonicalEqualClass(t testing.TB, value Value, representatives []Value) int {
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

func canonicalVariantBytes(t testing.TB, value Value) []byte {
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

func primeVariantCanonicalCancellation(t testing.TB, writer *canonical.Writer) {
	t.Helper()
	// Reset emits domain and version. Sixty harmless events leave Record at 63
	// and the raw state event at the periodic cancellation checkpoint 64.
	for range 60 {
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
}
