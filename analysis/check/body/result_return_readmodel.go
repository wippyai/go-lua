package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r *Result) ReturnPoints() []cfg.Point {
	graph := r.Graph()
	if graph == nil {
		return nil
	}
	points := graph.RPO()
	out := make([]cfg.Point, 0, len(points))
	for _, point := range points {
		if _, ok := r.ReturnFact(point); ok {
			out = append(out, point)
		}
	}
	return out
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
	if len(sources) != 1 {
		return nil
	}
	source := sources[0]
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || !source.OpenTail || !source.Expanded {
		return nil
	}
	site, ok := r.facts.CallSite(source.CallPoint)
	if !ok {
		return nil
	}
	in, ok := r.StateAt(source.CallPoint)
	if !ok {
		return nil
	}
	graph := r.Graph()
	ctx := transfer.NodeContext{
		Graph:    graph,
		Point:    source.CallPoint,
		Registry: r.registry,
		Read:     r.boundaryRead,
	}
	if graph != nil {
		ctx.Node = graph.Node(source.CallPoint)
	}
	outcome := r.callOutcome(ctx, site, in, r.boundaryRead)
	if len(outcome.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]factflow.ReturnPresenceRelation, 0, len(outcome.ReturnPresenceRelations))
	for _, relation := range outcome.ReturnPresenceRelations {
		out = append(out, factflow.NewReturnPresenceRelation(
			relation.TriggerIndex,
			relation.TriggerPresence,
			relation.TargetIndex,
			relation.TargetPresence,
		))
	}
	return out
}
