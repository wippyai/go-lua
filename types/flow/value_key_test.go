package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestValueKey_ConstructorsAreStableAndTyped(t *testing.T) {
	if got := SymbolValueKey(cfg.SymbolID(42)); got != ValueKey("s42") {
		t.Fatalf("SymbolValueKey(42) = %q, want s42", got)
	}
	if got := ReturnSlotValueKey(3); got != ValueKey("r3") {
		t.Fatalf("ReturnSlotValueKey(3) = %q, want r3", got)
	}
}

func TestParseSymbolPathKey(t *testing.T) {
	segments := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "users"},
		{Kind: constraint.SegmentIndexString, Name: `a"b\c`},
		{Kind: constraint.SegmentIndexInt, Index: 3},
	}
	key := SymbolPathKey(cfg.SymbolID(42), segments)
	sym, got, ok := ParseSymbolPathKey(key)
	if !ok || sym != cfg.SymbolID(42) {
		t.Fatalf("ParseSymbolPathKey(%q) = %d/%v, want 42/true", key, sym, ok)
	}
	if len(got) != len(segments) {
		t.Fatalf("segments len = %d, want %d", len(got), len(segments))
	}
	for i := range segments {
		if got[i] != segments[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], segments[i])
		}
	}
}

func TestParseSymbolPathKeyRejectsNonSymbolKeys(t *testing.T) {
	if sym, segs, ok := ParseSymbolPathKey("ret[0].field"); ok || sym != 0 || segs != nil {
		t.Fatalf("ParseSymbolPathKey(non-symbol) = %d/%#v/%v, want false", sym, segs, ok)
	}
	if sym, segs, ok := ParseSymbolPathKey("s42[bad]"); ok || sym != 0 || segs != nil {
		t.Fatalf("ParseSymbolPathKey(bad segment) = %d/%#v/%v, want false", sym, segs, ok)
	}
}

func TestParseSymbolValueKey(t *testing.T) {
	sym, ok := ParseSymbolValueKey(SymbolValueKey(cfg.SymbolID(42)))
	if !ok || sym != cfg.SymbolID(42) {
		t.Fatalf("ParseSymbolValueKey(SymbolValueKey(42)) = %d/%v, want 42/true", sym, ok)
	}
	if sym, ok := ParseSymbolValueKey(ReturnSlotValueKey(0)); ok || sym != 0 {
		t.Fatalf("ParseSymbolValueKey(ReturnSlotValueKey(0)) = %d/%v, want 0/false", sym, ok)
	}
	if sym, ok := ParseSymbolValueKey(ValueKey("s42.field")); ok || sym != 0 {
		t.Fatalf("ParseSymbolValueKey(s42.field) = %d/%v, want 0/false", sym, ok)
	}
}
