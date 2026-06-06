package transfer

import (
	"github.com/wippyai/go-lua/types/flow"
)

type relationReducer struct{}

var relations = relationReducer{}

// Point relations are transfer-local must-facts. RelationEffect is the public
// vocabulary; this reducer is the only place that mutates the relation carrier.
func (relationReducer) apply(out *flow.PointState, effect RelationEffect) bool {
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
	return !flow.PointRelationsDomain.Equal(before, out.Rel)
}
