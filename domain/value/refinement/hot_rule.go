package refinement

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule executes one artifact-issued storage nilability refinement. It sees
// no Branch, Program, Flow, or diagnostic vocabulary: those proofs were
// consumed when ProgramArtifact placed the rule on the exact guarded arm.
type HotRule struct {
	implementation *valueowner.RuleImplementation[value.PresenceRefinement]
	read           engine.Read[engine.OrderedCells[value.Value]]
	owner          *valueowner.HotOwner
}

func BindHot(fragment *SchemaFragment, owner *valueowner.HotOwner) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || owner.Schema() == nil ||
		!fragment.semantic.Available() {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[value.Value]]
	implementation, read, ok := valueowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[value.Value, value.PresenceRefinement]{
		OperandContent: func(refinement value.PresenceRefinement) (value.PresenceRefinement, [32]byte, bool) {
			return hotContent(owner.Schema(), refinement)
		},
		Fold: func(frame engine.Frame[value.Value, value.PresenceRefinement]) engine.RuleResult[value.Value] {
			refinement, operandOK := engine.Operand(frame)
			_, present, targetOK := hotTarget(owner.Schema(), refinement)
			if !operandOK || !targetOK {
				return engine.RuleResult[value.Value]{}
			}
			cells, readOK := engine.ReadValue(frame, runtimeRead)
			if !readOK || cells.Count() != 1 {
				return engine.RuleResult[value.Value]{}
			}
			fact, factPresent, available := cells.At(0)
			if !available {
				return engine.RuleResult[value.Value]{}
			}
			if !factPresent {
				return engine.NoCandidate(frame)
			}
			result, resultOK := owner.Schema().FilterPresence(fact, present)
			if !resultOK {
				return engine.RuleResult[value.Value]{}
			}
			return engine.Staged(frame, result)
		},
	}, engine.HotCarrySpec[value.Value, value.PresenceRefinement]{}, func(refinement value.PresenceRefinement) (uint64, bool) {
		target, _, ok := hotTarget(owner.Schema(), refinement)
		index, indexOK := owner.Schema().CoordinateIndex(target)
		return uint64(index), ok && indexOK
	}, func(refinement value.PresenceRefinement) (uint64, bool) {
		target, _, ok := hotTarget(owner.Schema(), refinement)
		index, indexOK := owner.Schema().CoordinateIndex(target)
		return uint64(index), ok && indexOK
	})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	rule := &HotRule{implementation: implementation, read: read, owner: owner}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (value.PresenceRefinement, bool) {
	if rule == nil || rule.owner == nil || rule.owner.Schema() == nil {
		return value.PresenceRefinement{}, false
	}
	row, ok := rule.owner.Schema().PresenceRefinement(coords.Mount, coords.Occurrence)
	return row, ok && rule.owner.Schema().OwnsPresenceRefinement(row)
}

func (rule *HotRule) Implementation() (*valueowner.RuleImplementation[value.PresenceRefinement], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := valueowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := valueowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func hotContent(schema *value.Schema, row value.PresenceRefinement) (value.PresenceRefinement, [32]byte, bool) {
	id, ok := row.ID()
	if schema == nil || !schema.OwnsPresenceRefinement(row) || !ok || [32]byte(id) == ([32]byte{}) {
		return value.PresenceRefinement{}, [32]byte{}, false
	}
	return row, [32]byte(id), true
}

func hotTarget(schema *value.Schema, row value.PresenceRefinement) (value.Coordinate, bool, bool) {
	if schema == nil || !schema.OwnsPresenceRefinement(row) {
		return value.Coordinate{}, false, false
	}
	target, present, ok := row.Target()
	if !ok {
		return value.Coordinate{}, false, false
	}
	if _, coordinateOK := schema.CoordinateIndex(target); !coordinateOK {
		return value.Coordinate{}, false, false
	}
	return target, present, true
}
