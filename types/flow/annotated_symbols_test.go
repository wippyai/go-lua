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

func TestAnnotatedSymbolsCloneAndSymbols(t *testing.T) {
	got := AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{9: true, 3: true}).Clone()
	if !got.Contains(9) || !got.Contains(3) {
		t.Fatalf("Clone lost symbols: %#v", got.Symbols())
	}

	got.Add(1)
	symbols := got.Symbols()
	want := []cfg.SymbolID{1, 3, 9}
	if len(symbols) != len(want) {
		t.Fatalf("Symbols() = %#v, want %#v", symbols, want)
	}
	for i := range want {
		if symbols[i] != want[i] {
			t.Fatalf("Symbols() = %#v, want %#v", symbols, want)
		}
	}
}
