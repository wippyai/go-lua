package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r *Result) ReturnPoints() []cfg.Point {
	if r == nil {
		return nil
	}
	if r.returnPointsOK {
		return r.returnPoints
	}
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	points := graph.RPO()
	out := make([]cfg.Point, 0, len(points))
	for _, point := range points {
		if _, ok := r.facts.Return(point); ok {
			out = append(out, point)
		}
	}
	r.returnPoints = out
	r.returnPointsOK = true
	return r.returnPoints
}

func (r *Result) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	if r == nil {
		return nil, false
	}
	fact, ok := r.facts.Return(point)
	if !ok {
		return nil, false
	}
	return fact.Sources(), true
}

func (r *Result) ReturnPresenceRelations(point cfg.Point) []factflow.ReturnPresenceRelation {
	if r == nil {
		return nil
	}
	relations := r.facts.ReturnPresenceRelations(point)
	if delegated := r.openTailReturnPresenceRelations(point); len(delegated) != 0 {
		relations = append(relations, delegated...)
	}
	return relations
}

func (r *Result) openTailReturnPresenceRelations(point cfg.Point) []factflow.ReturnPresenceRelation {
	if r == nil || r.callOutcome == nil {
		return nil
	}
	ret, ok := r.facts.Return(point)
	if !ok {
		return nil
	}
	sources := ret.Sources()
	callPoint, resultSlots, ok := openTailReturnCallSources(sources)
	if !ok {
		return nil
	}
	outcome, ok := r.CallOutcomeAt(callPoint)
	if !ok {
		return nil
	}
	if len(outcome.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]factflow.ReturnPresenceRelation, 0, len(outcome.ReturnPresenceRelations))
	for _, relation := range outcome.ReturnPresenceRelations {
		triggerIndex := relation.TriggerIndex
		targetIndex := relation.TargetIndex
		if resultSlots != nil {
			var ok bool
			triggerIndex, ok = resultSlots[relation.TriggerIndex]
			if !ok {
				continue
			}
			targetIndex, ok = resultSlots[relation.TargetIndex]
			if !ok {
				continue
			}
		}
		out = append(out, factflow.NewReturnPresenceRelation(
			triggerIndex,
			relation.TriggerPresence,
			targetIndex,
			relation.TargetPresence,
		))
	}
	return out
}

func openTailReturnCallSources(sources []factflow.ValueSource) (cfg.Point, map[int]int, bool) {
	if len(sources) == 0 {
		return 0, nil, false
	}
	first := sources[0]
	if first.Kind != factflow.ValueSourceCall || !first.HasCallPoint || !first.Expanded {
		return 0, nil, false
	}
	if len(sources) == 1 {
		if !first.OpenTail {
			return 0, nil, false
		}
		return first.CallPoint, nil, true
	}
	resultSlots := make(map[int]int, len(sources))
	for slot, source := range sources {
		if source.Kind != factflow.ValueSourceCall ||
			!source.HasCallPoint ||
			source.CallPoint != first.CallPoint ||
			!source.Expanded ||
			source.ResultIndex < 0 {
			return 0, nil, false
		}
		if _, exists := resultSlots[source.ResultIndex]; exists {
			return 0, nil, false
		}
		resultSlots[source.ResultIndex] = slot
	}
	return first.CallPoint, resultSlots, true
}
