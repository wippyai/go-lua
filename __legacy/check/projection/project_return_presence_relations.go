package projection

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

func projectReturnPresenceRelations(reg *axis.Registry, result ResultReader) []summary.ReturnPresenceRelation {
	out := projectExistingReturnPresenceRelations(result)
	out = append(out, filterConflictingSolvedReturnPresenceRelations(out, projectSolvedReturnPresenceRelations(reg, result))...)
	return out
}

func filterConflictingSolvedReturnPresenceRelations(existing, solved []summary.ReturnPresenceRelation) []summary.ReturnPresenceRelation {
	if len(existing) == 0 || len(solved) == 0 {
		return solved
	}
	out := solved[:0]
	for _, relation := range solved {
		if returnPresenceRelationConflicts(existing, relation) {
			continue
		}
		out = append(out, relation)
	}
	return out
}

func returnPresenceRelationConflicts(existing []summary.ReturnPresenceRelation, candidate summary.ReturnPresenceRelation) bool {
	for _, relation := range existing {
		if relation.TriggerIndex != candidate.TriggerIndex ||
			!presence.Equal(relation.TriggerPresence, candidate.TriggerPresence) ||
			relation.TargetIndex != candidate.TargetIndex {
			continue
		}
		if !presence.Equal(relation.TargetPresence, candidate.TargetPresence) {
			return true
		}
	}
	return false
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
	slots := newReturnSlotProjection(reg, result)
	if !slots.OK() || slots.arity < 2 {
		return nil
	}
	points := make([]returnpresence.Point, 0, len(slots.reachable))
	for _, point := range slots.reachable {
		sources, ok := slots.Sources(point)
		if !ok {
			continue
		}
		points = append(points, projectedReturnPresencePointFor(slots, point, sources))
	}
	if len(points) == 0 {
		return nil
	}
	var out []summary.ReturnPresenceRelation
	for trigger := 0; trigger < slots.arity; trigger++ {
		for target := 0; target < slots.arity; target++ {
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
	slots returnSlotProjection,
	point cfg.Point,
	sources []factflow.ValueSource,
) returnpresence.Point {
	out := returnpresence.Point{
		Presence: make([]presence.Value, slots.arity),
		Known:    make([]bool, slots.arity),
	}
	for i := 0; i < slots.arity; i++ {
		out.Presence[i] = presence.Absent()
		out.Known[i] = true
	}
	for i := 0; i < len(sources) && i < slots.arity; i++ {
		value, ok := slots.Value(point, sources, i)
		if !ok {
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
