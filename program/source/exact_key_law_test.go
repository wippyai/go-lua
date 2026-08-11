package source

import (
	"math"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestNormalizeExactKeySemanticCases(t *testing.T) {
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	textValue := func(value string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	}

	cases := []struct {
		name  string
		input keyspace.LiteralValue
		want  keyspace.LiteralValue
		ok    bool
	}{
		{
			name:  "false discards unrelated payload",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: false, Integer: 7, FloatBits: math.Float64bits(math.NaN()), String: "ignored"},
			want:  keyspace.LiteralValue{Kind: keyspace.LiteralBool},
			ok:    true,
		},
		{
			name:  "true discards unrelated payload",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true, Integer: -7, FloatBits: 99, String: "ignored"},
			want:  keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true},
			ok:    true,
		},
		{
			name:  "minimum integer",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: math.MinInt64, FloatBits: 1, String: "ignored"},
			want:  integer(math.MinInt64),
			ok:    true,
		},
		{
			name:  "maximum integer",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: math.MaxInt64, Bool: true},
			want:  integer(math.MaxInt64),
			ok:    true,
		},
		{
			name:  "positive integral float",
			input: float(1),
			want:  integer(1),
			ok:    true,
		},
		{
			name:  "negative integral float",
			input: float(-42),
			want:  integer(-42),
			ok:    true,
		},
		{
			name:  "positive signed zero",
			input: float(0),
			want:  integer(0),
			ok:    true,
		},
		{
			name:  "negative signed zero",
			input: float(math.Copysign(0, -1)),
			want:  integer(0),
			ok:    true,
		},
		{
			name:  "minimum representable integer float",
			input: float(float64(math.MinInt64)),
			want:  integer(math.MinInt64),
			ok:    true,
		},
		{
			name:  "positive out of range integer float",
			input: float(float64(math.MaxInt64)),
			want:  float(float64(math.MaxInt64)),
			ok:    true,
		},
		{
			name:  "fractional negative float",
			input: float(-1.5),
			want:  float(-1.5),
			ok:    true,
		},
		{
			name:  "fractional positive float",
			input: float(1.5),
			want:  float(1.5),
			ok:    true,
		},
		{
			name:  "negative infinity",
			input: float(math.Inf(-1)),
			want:  float(math.Inf(-1)),
			ok:    true,
		},
		{
			name:  "positive infinity",
			input: float(math.Inf(1)),
			want:  float(math.Inf(1)),
			ok:    true,
		},
		{
			name:  "empty string",
			input: textValue(""),
			want:  textValue(""),
			ok:    true,
		},
		{
			name:  "string preserves bytes",
			input: textValue("a\x00z"),
			want:  textValue("a\x00z"),
			ok:    true,
		},
		{
			name:  "quiet NaN",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: 0x7ff8000000000001},
			want:  keyspace.LiteralValue{},
			ok:    false,
		},
		{
			name:  "signaling NaN",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: 0x7ff0000000000001},
			want:  keyspace.LiteralValue{},
			ok:    false,
		},
		{
			name:  "negative NaN payload",
			input: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: 0xfff8000000000042},
			want:  keyspace.LiteralValue{},
			ok:    false,
		},
		{
			name:  "unknown kind",
			input: keyspace.LiteralValue{Kind: 0, Integer: 7, String: "ignored"},
			want:  keyspace.LiteralValue{},
			ok:    false,
		},
		{
			name:  "future kind",
			input: keyspace.LiteralValue{Kind: 255, Integer: 7, String: "ignored"},
			want:  keyspace.LiteralValue{},
			ok:    false,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := NormalizeExactKey(test.input)
			if ok != test.ok || got != test.want {
				t.Fatalf("NormalizeExactKey(%#v) = %#v/%v, want %#v/%v", test.input, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestCompareExactKeySemanticOrderAndQuotient(t *testing.T) {
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	integer := func(value int64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}
	}
	textValue := func(value string) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}
	}

	// This is the semantic canonical order: Lua's distinct key types remain
	// distinct, while bool, integer, non-integral float, and string atoms are
	// ordered by their source-owned canonical kind and value.
	ordered := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralBool, Bool: false},
		{Kind: keyspace.LiteralBool, Bool: true},
		integer(math.MinInt64),
		integer(-1),
		integer(0),
		integer(math.MaxInt64),
		float(math.Inf(-1)),
		float(-1.5),
		float(1.5),
		float(math.Inf(1)),
		textValue(""),
		textValue("a\x00z"),
		textValue("z"),
	}
	for left := range ordered {
		for right := range ordered {
			got, ok := CompareExactKey(ordered[left], ordered[right])
			if !ok {
				t.Fatalf("CompareExactKey rejected canonical pair %#v/%#v", ordered[left], ordered[right])
			}
			want := 0
			if left < right {
				want = -1
			} else if left > right {
				want = 1
			}
			if got != want {
				t.Fatalf("CompareExactKey(%#v, %#v) = %d, want %d", ordered[left], ordered[right], got, want)
			}
			reverse, reverseOK := CompareExactKey(ordered[right], ordered[left])
			if !reverseOK || reverse != -got {
				t.Fatalf("CompareExactKey antisymmetry %#v/%#v = %d/%d", ordered[left], ordered[right], got, reverse)
			}
		}
	}

	quotientCases := []struct {
		name        string
		left, right keyspace.LiteralValue
	}{
		{"integer and integral float", integer(1), float(1)},
		{"zero and positive zero", integer(0), float(0)},
		{"zero and negative zero", integer(0), float(math.Copysign(0, -1))},
		{"integer with irrelevant payload", keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7, String: "ignored"}, integer(7)},
	}
	for _, test := range quotientCases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := CompareExactKey(test.left, test.right)
			if !ok || got != 0 {
				t.Fatalf("CompareExactKey(%#v, %#v) = %d/%v, want 0/true", test.left, test.right, got, ok)
			}
		})
	}

	invalid := []keyspace.LiteralValue{
		{},
		{Kind: keyspace.LiteralFloat, FloatBits: 0x7ff8000000000001},
		{Kind: keyspace.LiteralFloat, FloatBits: 0xfff8000000000042},
		{Kind: 255},
	}
	for _, value := range invalid {
		if got, ok := CompareExactKey(value, ordered[0]); ok || got != 0 {
			t.Fatalf("CompareExactKey(%#v, valid) = %d/%v, want no order", value, got, ok)
		}
		if got, ok := CompareExactKey(ordered[0], value); ok || got != 0 {
			t.Fatalf("CompareExactKey(valid, %#v) = %d/%v, want no order", value, got, ok)
		}
	}

	// Sorting through the public comparator must produce the same semantic
	// order from every rotation of the input, independent of ingress order.
	for round := range ordered {
		rotated := append(append([]keyspace.LiteralValue(nil), ordered[round:]...), ordered[:round]...)
		sort.Slice(rotated, func(left, right int) bool {
			order, ok := CompareExactKey(rotated[left], rotated[right])
			return ok && order < 0
		})
		for index, want := range ordered {
			if rotated[index] != want {
				t.Fatalf("round %d order[%d] = %#v, want %#v", round, index, rotated[index], want)
			}
		}
	}
}

func TestExactKeyPublicQueriesAllocateZero(t *testing.T) {
	left := keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1.5)}
	right := keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "stable"}
	var normalized keyspace.LiteralValue
	var normalizeOK bool
	var order int
	var compareOK bool
	allocations := testing.AllocsPerRun(1000, func() {
		normalized, normalizeOK = NormalizeExactKey(left)
		order, compareOK = CompareExactKey(left, right)
	})
	if allocations != 0 {
		t.Fatalf("NormalizeExactKey/CompareExactKey allocated %v times", allocations)
	}
	if !normalizeOK || normalized.Kind != keyspace.LiteralFloat || normalized.FloatBits != left.FloatBits {
		t.Fatalf("allocation probe normalization = %#v/%v", normalized, normalizeOK)
	}
	if !compareOK || order >= 0 {
		t.Fatalf("allocation probe comparison = %d/%v, want negative/true", order, compareOK)
	}
}

func TestExactKeyArtifactByteOrderParity(t *testing.T) {
	float := func(value float64) keyspace.LiteralValue {
		return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(value)}
	}
	values := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralBool, Bool: false},
		{Kind: keyspace.LiteralBool, Bool: true},
		{Kind: keyspace.LiteralInteger, Integer: -7},
		{Kind: keyspace.LiteralInteger, Integer: 7},
		float(1),
		float(-1.5),
		float(math.Inf(1)),
		{Kind: keyspace.LiteralString, String: ""},
		{Kind: keyspace.LiteralString, String: "a\x00z"},
		{Kind: keyspace.LiteralString, String: "z"},
	}
	byteRef := func(value keyspace.LiteralValue) exactKeyRef {
		ref := exactKeyRefFromLiteral(value)
		if value.Kind == keyspace.LiteralString {
			ref.text = ""
			ref.bytes = []byte(value.String)
		}
		return ref
	}
	for left := range values {
		for right := range values {
			publicOrder, publicOK := CompareExactKey(values[left], values[right])
			leftRef, leftOK := normalizeExactKeyRef(byteRef(values[left]))
			rightRef, rightOK := normalizeExactKeyRef(byteRef(values[right]))
			if publicOK != (leftOK && rightOK) {
				t.Fatalf("byte normalization validity differs for %#v/%#v: public=%v bytes=%v/%v", values[left], values[right], publicOK, leftOK, rightOK)
			}
			if !publicOK {
				continue
			}
			byteOrder := compareCanonicalExactKey(leftRef, rightRef)
			if byteOrder != publicOrder {
				t.Fatalf("byte order for %#v/%#v = %d, public = %d", values[left], values[right], byteOrder, publicOrder)
			}
			publicLeft, _ := NormalizeExactKey(values[left])
			publicRight, _ := NormalizeExactKey(values[right])
			if got := exactKeyRefValue(leftRef, true); got != publicLeft {
				t.Fatalf("byte normalization %#v = %#v, public = %#v", values[left], got, publicLeft)
			}
			if got := exactKeyRefValue(rightRef, true); got != publicRight {
				t.Fatalf("byte normalization %#v = %#v, public = %#v", values[right], got, publicRight)
			}
		}
	}

	left := exactKeyRef{kind: keyspace.LiteralString, bytes: []byte("artifact-left")}
	right := exactKeyRef{kind: keyspace.LiteralString, bytes: []byte("artifact-right")}
	var leftNormalized, rightNormalized exactKeyRef
	var leftOK, rightOK bool
	var order int
	allocations := testing.AllocsPerRun(1000, func() {
		leftNormalized, leftOK = normalizeExactKeyRef(left)
		rightNormalized, rightOK = normalizeExactKeyRef(right)
		order = compareCanonicalExactKey(leftNormalized, rightNormalized)
	})
	if allocations != 0 {
		t.Fatalf("artifact byte normalization/order allocated %v times", allocations)
	}
	if !leftOK || !rightOK || order >= 0 {
		t.Fatalf("artifact byte allocation probe = %#v/%v, %#v/%v, order=%d", leftNormalized, leftOK, rightNormalized, rightOK, order)
	}
}
