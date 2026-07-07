package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/returnpresence"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func (l *lowerer) addReturnPresenceRelations(input *factflow.FactsInput, graph cfg.Graph) {
	if input == nil || graph == nil || l == nil || l.wir == nil {
		return
	}
	points := wirReturnFactPoints(graph, l.wir)
	arity := l.returnPresenceArity(input, points)
	if arity < 2 {
		return
	}
	if relations := l.inferReturnSourcePresenceRelations(input, points, arity); len(relations) != 0 {
		for _, point := range points {
			appendReturnPresenceRelations(input.ReturnPresenceRelations, point, relations...)
		}
	}
}

func wirReturnFactPoints(graph cfg.Graph, body *wir.Body) []cfg.Point {
	if graph == nil || body == nil {
		return nil
	}
	var points []cfg.Point
	for _, point := range graph.RPO() {
		if body.HasInstruction(point, wir.OpReturn) {
			points = append(points, point)
		}
	}
	return points
}

func (l *lowerer) returnPresenceArity(input *factflow.FactsInput, points []cfg.Point) int {
	arity := 0
	for _, point := range points {
		if fact, ok := input.Returns[point]; ok {
			if n := len(fact.Sources()); n > arity {
				arity = n
			}
		}
	}
	if l != nil && l.wir != nil {
		if n := l.wir.DeclaredReturnArity(); n > arity {
			arity = n
		}
		return arity
	}
	return arity
}

func (l *lowerer) inferReturnSourcePresenceRelations(
	input *factflow.FactsInput,
	points []cfg.Point,
	arity int,
) []factflow.ReturnPresenceRelation {
	if len(points) == 0 {
		return nil
	}
	presencePoints := make([]returnpresence.Point, 0, len(points))
	for _, point := range points {
		fact, ok := input.Returns[point]
		if !ok {
			continue
		}
		presencePoints = append(presencePoints, l.returnPresencePoint(input, fact.Sources(), arity))
	}
	if len(presencePoints) == 0 {
		return nil
	}
	var out []factflow.ReturnPresenceRelation
	for trigger := 0; trigger < arity; trigger++ {
		for target := 0; target < arity; target++ {
			if target == trigger {
				continue
			}
			for _, triggerPresence := range []presence.Value{presence.Present(), presence.Absent()} {
				targetPresence, ok := returnpresence.TargetPresence(presencePoints, trigger, triggerPresence, target)
				if !ok {
					continue
				}
				out = append(out, factflow.NewReturnPresenceRelation(
					trigger,
					triggerPresence,
					target,
					targetPresence,
				))
			}
		}
	}
	return out
}

func (l *lowerer) returnPresencePoint(
	input *factflow.FactsInput,
	sources []factflow.ValueSource,
	arity int,
) returnpresence.Point {
	point := returnpresence.Point{
		Presence: make([]presence.Value, arity),
		Known:    make([]bool, arity),
	}
	for i := 0; i < arity; i++ {
		point.Presence[i] = presence.Absent()
		point.Known[i] = true
	}
	for i := 0; i < len(sources) && i < arity; i++ {
		sourcePresence, ok := l.returnSourcePresence(input, sources[i])
		if !ok {
			point.Presence[i] = presence.Maybe()
			point.Known[i] = false
			continue
		}
		point.Presence[i] = sourcePresence
		point.Known[i] = true
	}
	return point
}

func (l *lowerer) returnSourcePresence(input *factflow.FactsInput, source factflow.ValueSource) (presence.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceNil:
		return presence.Absent(), true
	case factflow.ValueSourceLiteral:
		return presence.Present(), true
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return presence.Bottom(), false
		}
		value, ok := l.expressionValues[source.ExprRef]
		if !ok {
			return presence.Bottom(), false
		}
		return returnpresence.KnownPresence(product.PresenceOf(value))
	case factflow.ValueSourceCall:
		if !source.HasCallPoint || source.ResultIndex < 0 {
			return presence.Bottom(), false
		}
		for _, result := range input.CallResultValues[source.CallPoint].Values() {
			if result.Index() != source.ResultIndex {
				continue
			}
			return returnpresence.KnownPresence(product.PresenceOf(result.Value()))
		}
		return presence.Bottom(), false
	default:
		return presence.Bottom(), false
	}
}

func appendReturnPresenceRelations(
	relations map[cfg.Point]factflow.ReturnPresenceRelationSet,
	point cfg.Point,
	added ...factflow.ReturnPresenceRelation,
) {
	if len(added) == 0 {
		return
	}
	existing := relations[point].Relations()
	existing = append(existing, added...)
	relations[point] = factflow.NewReturnPresenceRelationSet(existing...)
}
