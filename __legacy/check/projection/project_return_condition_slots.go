package projection

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func projectReturnConditionSlotRefinements(
	reg *axis.Registry,
	result ResultReader,
) []summary.ReturnConditionSlotRefinement {
	slots := newReturnSlotProjection(reg, result)
	if !slots.OK() || slots.arity < 2 {
		return nil
	}
	if len(slots.reachable) == 0 {
		return nil
	}
	var out []summary.ReturnConditionSlotRefinement
	for trigger := 0; trigger < slots.arity; trigger++ {
		out = append(out, projectReturnConditionSlotRefinementsFor(slots, trigger, true)...)
		out = append(out, projectReturnConditionSlotRefinementsFor(slots, trigger, false)...)
	}
	return out
}

func projectReturnConditionSlotRefinementsFor(
	slots returnSlotProjection,
	trigger int,
	returnValue bool,
) []summary.ReturnConditionSlotRefinement {
	values := make([]product.Value, slots.arity)
	seen := make([]bool, slots.arity)
	triggerSeen := false
	for _, point := range slots.reachable {
		sources, ok := slots.Sources(point)
		if !ok {
			return nil
		}
		triggerValue, ok := slots.Value(point, sources, trigger)
		if !ok {
			return nil
		}
		truthy, ok := definiteLuaTruthiness(slots.reg, triggerValue)
		if !ok {
			return nil
		}
		if truthy != returnValue {
			continue
		}
		triggerSeen = true
		for target := 0; target < slots.arity; target++ {
			if target == trigger {
				continue
			}
			targetValue, ok := slots.Value(point, sources, target)
			if !ok {
				return nil
			}
			targetValue = slots.ValueWithDeclaredContract(targetValue, target)
			if seen[target] {
				values[target] = product.Join(slots.reg, values[target], targetValue)
			} else {
				values[target] = targetValue
				seen[target] = true
			}
		}
	}
	if !triggerSeen {
		return nil
	}
	out := make([]summary.ReturnConditionSlotRefinement, 0, slots.arity-1)
	for target := 0; target < slots.arity; target++ {
		if target == trigger || !seen[target] {
			continue
		}
		out = append(out, summary.ReturnConditionSlotRefinement{
			ReturnIndex: trigger,
			ReturnValue: returnValue,
			TargetIndex: target,
			Value:       values[target],
		})
	}
	return out
}

func reachableReturnPoints(reg *axis.Registry, result ResultReader) []cfg.Point {
	var out []cfg.Point
	for _, point := range result.ReturnPoints() {
		if projectedReturnPointUnreachable(reg, result, point) {
			continue
		}
		out = append(out, point)
	}
	return out
}

func definiteLuaTruthiness(reg *axis.Registry, value product.Value) (bool, bool) {
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return false, true
	}
	canTruthy := valuerefine.CanBeTruthy(reg, value)
	canFalsy := valuerefine.CanBeFalsy(reg, value)
	if canTruthy == canFalsy {
		return false, false
	}
	return canTruthy, true
}
