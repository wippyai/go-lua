package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
)

func TestRelationEffectSeedsAndKillsSiblingNil(t *testing.T) {
	valueSym := cfg.SymbolID(511)
	otherSym := cfg.SymbolID(512)
	errSym := cfg.SymbolID(513)
	out := PointState{}

	ApplyRelationEffect(&out, RelationEffect{
		Kind:      RelationSeedSiblingNil,
		ErrSym:    errSym,
		ValueSyms: []cfg.SymbolID{valueSym, otherSym},
	})
	if rel, ok := out.Rel.SiblingNil(errSym); !ok || len(rel.ValueSyms) != 2 {
		t.Fatalf("relation effect did not seed sibling-nil relation: %#v", out.Rel)
	}

	ApplyRelationEffect(&out, RelationEffect{
		Kind:    RelationKillSymbols,
		Symbols: []cfg.SymbolID{valueSym},
	})
	rel, ok := out.Rel.SiblingNil(errSym)
	if !ok || len(rel.ValueSyms) != 1 || rel.ValueSyms[0] != otherSym {
		t.Fatalf("relation effect kill did not remove only written symbol: %#v", out.Rel)
	}

	ApplyRelationEffect(&out, RelationEffect{
		Kind:    RelationKillSymbols,
		Symbols: []cfg.SymbolID{errSym},
	})
	if _, ok := out.Rel.SiblingNil(errSym); ok {
		t.Fatalf("relation effect kept relation after err symbol write: %#v", out.Rel)
	}
}
