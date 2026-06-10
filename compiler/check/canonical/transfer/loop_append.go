package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/flow"
)

func indexLoopAppendLengthFacts(facts []input.LoopAppendLengthFact) map[cfg.Point][]input.LoopAppendLengthFact {
	if len(facts) == 0 {
		return nil
	}
	out := make(map[cfg.Point][]input.LoopAppendLengthFact)
	for _, fact := range facts {
		if fact.Point == 0 || fact.TargetLocal.Key() == "" {
			continue
		}
		if fact.Count <= 0 && fact.ParamIndex < 0 {
			continue
		}
		out[fact.Point] = append(out[fact.Point], fact)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (t *Transfer) applyLoopAppendLengthFacts(out *flow.PointState, facts []input.LoopAppendLengthFact) bool {
	if out == nil || len(facts) == 0 {
		return false
	}
	if flow.PointNumericIsUnsat(out) {
		return false
	}
	changed := false
	for _, fact := range facts {
		if fact.Count > 0 {
			if op, ok := flow.NumericLenGeConstLocalOp(fact.TargetLocal, fact.Count); ok {
				changed = flow.ApplyNumericEffect(out, flow.NumericEffect{
					Ops: []flow.NumericOp{op},
				}) || changed
			}
		}
		if fact.ParamIndex >= 0 {
			if effect, ok := flow.RelationTargetLengthParamLocalEffect(fact.TargetLocal, fact.ParamIndex); ok {
				changed = flow.ApplyRelationEffect(out, effect) || changed
			}
		}
	}
	return changed
}
