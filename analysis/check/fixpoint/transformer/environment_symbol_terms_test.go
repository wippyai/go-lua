package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBindEnvironmentSymbolSealsScalarAndRootPathVocabulary(t *testing.T) {
	arena := NewArena(standard.Registry())
	id := symbol.ID(701)
	value := arena.bindEnvironmentSymbol(id)
	path := arena.EnvironmentPath(id)
	if value == 0 || path == 0 {
		t.Fatal("environment binding must seal both scalar and root-path terms")
	}

	arena.Seal()
	sealedValue, exact := arena.environmentValue(id)
	if !exact || sealedValue != value {
		t.Fatalf("sealed scalar term = %d/%t, want %d/true", sealedValue, exact, value)
	}
	if sealedPath := arena.EnvironmentPath(id); sealedPath != path {
		t.Fatalf("sealed path term = %d, want %d", sealedPath, path)
	}
	if missing, exact := arena.environmentValue(symbol.ID(702)); exact || missing != 0 {
		t.Fatalf("missing symbol scalar = %d/%t, want 0/false", missing, exact)
	}
	if missing := arena.EnvironmentPath(symbol.ID(702)); missing != 0 {
		t.Fatalf("missing symbol path = %d, want 0", missing)
	}
}
