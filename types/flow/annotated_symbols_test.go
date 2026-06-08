package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
)

func TestAnnotatedSymbolsFromMapNormalizesTruthyEntries(t *testing.T) {
	first := cfg.SymbolID(1)
	second := cfg.SymbolID(2)

	got := AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{
		first:  true,
		second: false,
		0:      true,
	})

	if !got.Contains(first) {
		t.Fatalf("Contains(%d) = false, want true", first)
	}
	if got.Contains(second) {
		t.Fatalf("Contains(%d) = true, want false", second)
	}
	if got.Contains(0) {
		t.Fatal("Contains(0) = true, want false")
	}
}

func TestAnnotatedSymbolsEqual(t *testing.T) {
	left := AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{3: true})
	right := AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{3: true, 4: false})
	if !annotatedSymbolsEqual(left, right) {
		t.Fatal("annotatedSymbolsEqual returned false for equivalent sets")
	}
}
