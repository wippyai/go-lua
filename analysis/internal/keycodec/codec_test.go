package keycodec

import (
	"strconv"
	"testing"
)

func TestPrefixedDecimalKey(t *testing.T) {
	if got := PrefixedDecimalKey('s', 42, `.field["k"]`); got != `s42.field["k"]` {
		t.Fatalf("PrefixedDecimalKey = %q, want compact key", got)
	}
	if got := PrefixedDecimalKey('r', 0, ""); got != "r0" {
		t.Fatalf("PrefixedDecimalKey zero = %q, want r0", got)
	}
}

func TestParseUnsignedDecimal(t *testing.T) {
	if got, ok := ParseUnsignedDecimal("18446744073709551615"); !ok || got != ^uint64(0) {
		t.Fatalf("ParseUnsignedDecimal(max) = %d/%v, want uint64 max/true", got, ok)
	}
	for _, input := range []string{"", "12x", "184467440737095516150"} {
		if got, ok := ParseUnsignedDecimal(input); ok || got != 0 {
			t.Fatalf("ParseUnsignedDecimal(%q) = %d/%v, want false", input, got, ok)
		}
	}
}

func TestParsePositiveIntAfterAt(t *testing.T) {
	if got, next, ok := ParsePositiveIntAfterAt("sym42@3.field", len("sym42@")); !ok || got != 3 || next != len("sym42@3") {
		t.Fatalf("ParsePositiveIntAfterAt = %d/%d/%v, want 3/7/true", got, next, ok)
	}
	maxInt := int(^uint(0) >> 1)
	overflow := "@" + strconv.FormatInt(int64(maxInt), 10) + "0"
	if got, _, ok := ParsePositiveIntAfterAt(overflow, 1); ok || got != 0 {
		t.Fatalf("ParsePositiveIntAfterAt(overflow) = %d/%v, want false", got, ok)
	}
	for _, input := range []string{"@", "@0", "@x"} {
		if got, _, ok := ParsePositiveIntAfterAt(input, 1); ok || got != 0 {
			t.Fatalf("ParsePositiveIntAfterAt(%q) = %d/%v, want false", input, got, ok)
		}
	}
}
