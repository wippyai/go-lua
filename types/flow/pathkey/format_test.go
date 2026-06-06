package pathkey

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestSymbolRoot(t *testing.T) {
	if got := SymbolRoot(42); got != "sym42" {
		t.Fatalf("SymbolRoot(42) = %q, want %q", got, "sym42")
	}
}

func TestSymbolVersionRoot(t *testing.T) {
	if got := SymbolVersionRoot(42, 3); got != "sym42@3" {
		t.Fatalf("SymbolVersionRoot(42, 3) = %q, want %q", got, "sym42@3")
	}
}

func TestSymbolVersionKeyUsesCanonicalSegments(t *testing.T) {
	got := SymbolVersionKey(42, 3, []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: `a"b`},
		{Kind: constraint.SegmentField, Name: "value"},
	})
	want := constraint.PathKey(`sym42@3["a\"b"].value`)
	if got != want {
		t.Fatalf("SymbolVersionKey = %q, want %q", got, want)
	}
}
