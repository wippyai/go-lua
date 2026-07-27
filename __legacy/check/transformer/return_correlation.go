package transformer

import (
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/__legacy/analysis/domain/value/returnpresence"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	checkprojection "github.com/wippyai/go-lua/analysis/check/internal/projection"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// inferReturnRowCorrelations projects the same two cross-slot families as the
// canonical concrete Summary projector, but from already-specialized feasible
// relation rows. Rows are retained until this transaction completes precisely
// so correlations are not lost by the final component-wise Summary join.
func inferReturnRowCorrelations(reg *axis.Registry, rows []summary.Summary, rawReturns [][]product.Value, declared []product.Value) ([]summary.ReturnConditionSlotRefinement, []summary.ReturnPresenceRelation, bool) {
	if reg == nil || len(rows) != len(rawReturns) {
		return nil, nil, false
	}
	arity := len(declared)
	for rowIndex, row := range rows {
		if len(row.Returns) > arity {
			arity = len(row.Returns)
		}
		if len(rawReturns[rowIndex]) > arity {
			arity = len(rawReturns[rowIndex])
		}
	}
	if arity < 2 || len(rows) == 0 {
		return nil, nil, true
	}
	raw := make([][]product.Value, len(rows))
	projected := make([][]product.Value, len(rows))
	points := make([]returnpresence.Point, len(rows))
	for rowIndex, row := range rows {
		raw[rowIndex] = make([]product.Value, arity)
		projected[rowIndex] = make([]product.Value, arity)
		points[rowIndex] = returnpresence.Point{Presence: make([]presence.Value, arity), Known: make([]bool, arity)}
		for slot := 0; slot < arity; slot++ {
			rawValue := product.Absent(reg)
			if slot < len(rawReturns[rowIndex]) {
				rawValue = rawReturns[rowIndex][slot]
				if product.Equal(reg, rawValue, product.Bottom(reg)) {
					return nil, nil, false
				}
			}
			raw[rowIndex][slot] = rawValue
			projectedValue := product.Absent(reg)
			if slot < len(row.Returns) {
				projectedValue = row.Returns[slot]
				if product.Equal(reg, projectedValue, product.Bottom(reg)) {
					return nil, nil, false
				}
			} else if slot < len(declared) {
				projectedValue = checkprojection.WithDeclaredContractPreservingPresence(reg, projectedValue, declared[slot])
			}
			projected[rowIndex][slot] = projectedValue
			got, known := returnpresence.KnownPresence(product.PresenceOf(rawValue))
			points[rowIndex].Presence[slot] = got
			points[rowIndex].Known[slot] = known
		}
	}

	var conditions []summary.ReturnConditionSlotRefinement
	for trigger := 0; trigger < arity; trigger++ {
		for _, truthy := range []bool{true, false} {
			values := make([]product.Value, arity)
			seen := make([]bool, arity)
			triggerSeen, exact := false, true
			for rowIndex := range raw {
				rowTruthy, ok := definiteReturnTruthiness(reg, raw[rowIndex][trigger])
				if !ok {
					exact = false
					break
				}
				if rowTruthy != truthy {
					continue
				}
				triggerSeen = true
				for target := 0; target < arity; target++ {
					if target == trigger {
						continue
					}
					if seen[target] {
						values[target] = product.Join(reg, values[target], projected[rowIndex][target])
					} else {
						values[target] = projected[rowIndex][target]
						seen[target] = true
					}
				}
			}
			if !exact || !triggerSeen {
				continue
			}
			for target := 0; target < arity; target++ {
				if target != trigger && seen[target] {
					conditions = append(conditions, summary.ReturnConditionSlotRefinement{
						ReturnIndex: trigger, ReturnValue: truthy, TargetIndex: target, Value: values[target],
					})
				}
			}
		}
	}

	var relations []summary.ReturnPresenceRelation
	for trigger := 0; trigger < arity; trigger++ {
		for target := 0; target < arity; target++ {
			if target == trigger {
				continue
			}
			for _, triggerPresence := range []presence.Value{presence.Present(), presence.Absent()} {
				targetPresence, ok := returnpresence.TargetPresence(points, trigger, triggerPresence, target)
				if ok {
					relations = append(relations, summary.ReturnPresenceRelation{
						TriggerIndex: trigger, TriggerPresence: triggerPresence,
						TargetIndex: target, TargetPresence: targetPresence,
					})
				}
			}
		}
	}
	return conditions, relations, true
}

func definiteReturnTruthiness(reg *axis.Registry, value product.Value) (bool, bool) {
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
