package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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

func TestRelationPathEffectsUseSymbolPathKeys(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(21), "rows").Field("items")
	targetKey := SymbolPathKey(cfg.SymbolID(21), path.Segments)
	target, ok := LocalAddressOfPath(path)
	if !ok {
		t.Fatalf("local address was not produced")
	}

	length, ok := RelationTargetLengthParamLocalEffect(target, 2)
	if !ok {
		t.Fatalf("target-length local effect was not produced")
	}
	if length.Kind != RelationSeedTargetLengthParam || length.TargetLocal.Key() != path.Key() || length.ParamIndex != 2 {
		t.Fatalf("target-length effect = %#v, want target=%s param=2", length, path.Key())
	}

	lower, ok := RelationContainerLowerBoundPathEffect(path, 4)
	if !ok {
		t.Fatalf("container-lower path effect was not produced")
	}
	if lower.Kind != RelationSeedContainerLowerBound || lower.TargetRoot != path.Symbol || lower.TargetKey != targetKey || lower.Lower != 4 {
		t.Fatalf("container-lower effect = %#v, want root=%d key=%s lower=4", lower, path.Symbol, targetKey)
	}
}

func TestRelationPathEffectsRejectUnresolvedPaths(t *testing.T) {
	if _, ok := RelationTargetLengthParamLocalEffect(LocalAddress{}, 0); ok {
		t.Fatalf("target-length effect accepted unresolved local address")
	}
	if _, ok := RelationContainerLowerBoundPathEffect(constraint.Path{}, 1); ok {
		t.Fatalf("container-lower effect accepted unresolved path")
	}
}
