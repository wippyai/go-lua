package cfgfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestLoopKind(t *testing.T) {
	tests := []struct {
		kind LoopKind
		name string
	}{
		{LoopKindUnknown, "LoopKindUnknown"},
		{LoopKindConditional, "LoopKindConditional"},
		{LoopKindNumericFor, "LoopKindNumericFor"},
		{LoopKindGenericFor, "LoopKindGenericFor"},
	}

	for i, tt := range tests {
		if int(tt.kind) != i {
			t.Errorf("%s: expected value %d, got %d", tt.name, i, tt.kind)
		}
	}
}

func TestLoopFactCopiesSlices(t *testing.T) {
	var meta Metadata
	vars := []symbol.ID{1}
	locals := []symbol.ID{2}
	modified := []symbol.ID{3}

	meta.SetLoop(3, LoopFact{
		Vars:                 vars,
		Locals:               locals,
		DirectModifiedOuters: modified,
		Preheader:            cfg.Point(1),
		HasPreheader:         true,
	})
	vars[0] = 10
	locals[0] = 20
	modified[0] = 30

	fact, ok := meta.Loop(3)
	if !ok {
		t.Fatal("missing loop fact")
	}
	if fact.Vars[0] != 1 || fact.Locals[0] != 2 || fact.DirectModifiedOuters[0] != 3 {
		t.Fatalf("loop fact was mutated through input slices: %+v", fact)
	}

	fact.Vars[0] = 30
	fact.DirectModifiedOuters[0] = 40
	again, _ := meta.Loop(3)
	if again.Vars[0] != 1 || again.DirectModifiedOuters[0] != 3 {
		t.Fatalf("loop fact was mutated through getter slice: %+v", again)
	}
}
