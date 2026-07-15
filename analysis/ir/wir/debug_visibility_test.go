package wir

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestDebugLocalVisibilityNormalizesAndDetaches(t *testing.T) {
	body := NewBody("debug")
	input := []SymbolID{9, 0, 3, 9, 5, 3}
	body.SetDebugLocalVisibility(4, DebugPhaseBefore, input)
	input[0] = 99

	want := []SymbolID{3, 5, 9}
	got := body.DebugLocalVisibility(4, DebugPhaseCall)
	if !slices.Equal(got, want) {
		t.Fatalf("call visibility = %v, want %v", got, want)
	}
	got[0] = 99
	if again := body.DebugLocalVisibility(4, DebugPhaseBefore); !slices.Equal(again, want) {
		t.Fatalf("visibility after result mutation = %v, want detached %v", again, want)
	}

	canonical := []SymbolID{2, 4, 8}
	body.SetDebugLocalVisibilitySnapshot(cfg.Point(4), DebugPhaseAfter, canonical)
	if after := body.DebugLocalVisibility(4, DebugPhaseReturn); !slices.Equal(after, canonical) {
		t.Fatalf("return visibility = %v, want %v", after, canonical)
	}
}
