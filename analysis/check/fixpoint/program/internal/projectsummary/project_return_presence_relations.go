package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/returnpresence"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type returnSourceValueReader interface {
	SourceValueAtBoundary(cfg.Point, factflow.ValueSource) (product.Value, bool)
}

func projectReturnPresenceRelations(reg *axis.Registry, result ResultReader) []summary.ReturnPresenceRelation {
	out := projectExistingReturnPresenceRelations(result)
	out = append(out, projectSolvedReturnPresenceRelations(reg, result)...)
	return out
}

func projectExistingReturnPresenceRelations(result ResultReader) []summary.ReturnPresenceRelation {
	reader, ok := result.(returnPresenceRelationReader)
	var out []summary.ReturnPresenceRelation
	if ok {
		for _, point := range result.ReturnPoints() {
			for _, relation := range reader.ReturnPresenceRelations(point) {
				out = append(out, summary.ReturnPresenceRelation{
					TriggerIndex:    relation.TriggerIndex(),
					TriggerPresence: relation.TriggerPresence(),
					TargetIndex:     relation.TargetIndex(),
					TargetPresence:  relation.TargetPresence(),
				})
			}
		}
	}
	return out
}

func projectSolvedReturnPresenceRelations(reg *axis.Registry, result ResultReader) []summary.ReturnPresenceRelation {
	sourceReader, hasSources := result.(returnValueSourceReader)
	valueReader, hasValues := result.(returnSourceValueReader)
	if reg == nil || !hasSources || !hasValues {
		return nil
	}
	arity := projectedReturnPresenceArity(result)
	if arity < 2 {
		return nil
	}
	points := make([]returnpresence.Point, 0, len(result.ReturnPoints()))
	for _, point := range result.ReturnPoints() {
		if projectedReturnPointUnreachable(reg, result, point) {
			continue
		}
		sources, ok := sourceReader.ReturnValueSources(point)
		if !ok {
			continue
		}
		points = append(points, projectedReturnPresencePointFor(reg, valueReader, point, sources, arity))
	}
	if len(points) == 0 {
		return nil
	}
	var out []summary.ReturnPresenceRelation
	for trigger := 0; trigger < arity; trigger++ {
		for target := 0; target < arity; target++ {
			if target == trigger {
				continue
			}
			for _, triggerPresence := range []presence.Value{presence.Present(), presence.Absent()} {
				targetPresence, ok := returnpresence.TargetPresence(points, trigger, triggerPresence, target)
				if !ok {
					continue
				}
				out = append(out, summary.ReturnPresenceRelation{
					TriggerIndex:    trigger,
					TriggerPresence: triggerPresence,
					TargetIndex:     target,
					TargetPresence:  targetPresence,
				})
			}
		}
	}
	return out
}

func projectedReturnPresenceArity(result ResultReader) int {
	arity := 0
	for _, point := range result.ReturnPoints() {
		pointArity, ok := resultReturnSourceArity(result, point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if reader, ok := result.(returnTypeValueReader); ok {
		if declared := len(reader.ReturnTypeValues()); declared > arity {
			arity = declared
		}
	}
	return arity
}

func projectedReturnPresencePointFor(
	reg *axis.Registry,
	reader returnSourceValueReader,
	point cfg.Point,
	sources []factflow.ValueSource,
	arity int,
) returnpresence.Point {
	out := returnpresence.Point{
		Presence: make([]presence.Value, arity),
		Known:    make([]bool, arity),
	}
	for i := 0; i < arity; i++ {
		out.Presence[i] = presence.Absent()
		out.Known[i] = true
	}
	for i := 0; i < len(sources) && i < arity; i++ {
		value, ok := reader.SourceValueAtBoundary(point, sources[i])
		if !ok || product.Equal(reg, value, product.Bottom(reg)) {
			out.Presence[i] = presence.Bottom()
			out.Known[i] = false
			continue
		}
		got, ok := returnpresence.KnownPresence(product.PresenceOf(value))
		if !ok {
			out.Presence[i] = presence.Bottom()
			out.Known[i] = false
			continue
		}
		out.Presence[i] = got
		out.Known[i] = true
	}
	return out
}

func projectedReturnPointUnreachable(reg *axis.Registry, result ResultReader, point cfg.Point) bool {
	reader, ok := result.(stateAtReader)
	if !ok {
		return false
	}
	st, ok := reader.StateAt(point)
	if !ok {
		return false
	}
	domain := state.Domain(reg)
	return domain.Equal(st, domain.Bottom())
}
