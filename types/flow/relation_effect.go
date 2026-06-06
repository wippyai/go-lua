package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

type RelationEffectKind uint8

const (
	RelationSeedSiblingNil RelationEffectKind = iota + 1
	RelationKillSymbols
	RelationSeedTargetLengthParam
	RelationSeedContainerLowerBound
	RelationKillLengthTargets
	RelationSeedGuardedType
)

// RelationEffect is the canonical reducer payload for point-local relation facts.
// Relations are must-facts in PointState.Rel, so producers seed and kill them
// through this reducer rather than editing the relation axis directly.
type RelationEffect struct {
	Kind          RelationEffectKind
	ErrSym        cfg.SymbolID
	ValueSyms     []cfg.SymbolID
	Symbols       []cfg.SymbolID
	TargetRoot    cfg.SymbolID
	TargetKey     constraint.PathKey
	ParamIndex    int
	Lower         int64
	GuardSym      cfg.SymbolID
	TargetSym     cfg.SymbolID
	GuardOnTruthy bool
	TargetType    typ.Type
}

func ApplyRelationEffect(out *PointState, effect RelationEffect) bool {
	if out == nil {
		return false
	}
	before := out.Rel
	switch effect.Kind {
	case RelationSeedSiblingNil:
		out.Rel = out.Rel.WithSiblingNil(effect.ErrSym, effect.ValueSyms)
	case RelationKillSymbols:
		out.Rel = out.Rel.KillSymbols(effect.Symbols...)
	case RelationSeedTargetLengthParam:
		out.Rel = out.Rel.WithTargetLengthParam(effect.TargetRoot, effect.TargetKey, effect.ParamIndex)
	case RelationSeedContainerLowerBound:
		out.Rel = out.Rel.WithContainerLowerBound(effect.TargetRoot, effect.TargetKey, effect.Lower)
	case RelationKillLengthTargets:
		out.Rel = out.Rel.KillLengthTargets(effect.Symbols...)
	case RelationSeedGuardedType:
		out.Rel = out.Rel.WithGuardedType(effect.GuardSym, effect.TargetSym, effect.GuardOnTruthy, effect.TargetType)
	default:
		return false
	}
	return !PointRelationsDomain.Equal(before, out.Rel)
}
