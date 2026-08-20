package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the receipt-native mounted return escape. Value issues the
// return-boundary operand; Placement owns the selected route-write cell.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	valueRead      engine.Read[engine.OrderedCells[valuedomain.Value]]
	placementRead  engine.Read[engine.Selection[routeTag, engine.OrderedCells[placementdomain.Placement]]]
}

// BindHot binds the exact return-boundary read and selected Placement route
// lane. The two HotOwners must project the same sealed Heap authority; the
// owner wrappers retain the shared SchemaBinding fence.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, values *valueowner.HotOwner) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || values == nil || !values.MatchesBinding(binding) || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		!values.Schema().OwnsHeapSchema(owner.Schema().Heap()) {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values}
	implementation, ok := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placementdomain.Placement, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return returnOperandContentForSchema(values.Schema(), candidate)
		},
		Fold: rule.fold,
	}, engine.HotCarrySpec[placementdomain.Placement, operand]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	valueRead, valueReadOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueRead, values.FactorRef(), func(candidate operand) (uint64, bool) {
		return returnCoordinateForSchema(values.Schema(), candidate)
	})
	if !valueReadOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementReadOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placementdomain.Placement, routeTag](implementation, fragment.placementRead, owner.FactorRef(), rule.locate)
	if !placementReadOK {
		return nil, false
	}
	rule.placementRead = placementRead
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.values == nil {
		return operand{}, false
	}
	return returnOperandForSchema(rule.values.Schema(), coords.Mount, coords.Occurrence)
}

// Implementation returns the pending Placement-owned Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule issues the sealed Program rule after the shared binding
// publishes its exact Placement implementation.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil || rule.owner == nil || rule.implementation == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func (rule *HotRule) locate(context engine.SelectorContext, operand operand) bool {
	if rule == nil || rule.values == nil || rule.owner == nil {
		return false
	}
	_, _, contentOK := returnOperandContentForSchema(rule.values.Schema(), operand)
	if !contentOK {
		return false
	}
	cells, readOK := engine.SelectorRead(context, rule.valueRead)
	if !readOK || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available || !present {
		return true
	}
	plan, planOK := routePlanFor(rule.owner.Schema(), rule.values.Schema(), fact)
	if !planOK || plan.class == routeBottom {
		return false
	}
	for _, candidate := range plan.routes {
		if !placementowner.SelectRouteTyped(rule.owner, context, candidate.key, candidate.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Placement, operand]) engine.RuleResult[placementdomain.Placement] {
	operand, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.values == nil || rule.owner == nil {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if _, _, contentOK := returnOperandContentForSchema(rule.values.Schema(), operand); !contentOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	cells, valueReadOK := engine.ReadValue(frame, rule.valueRead)
	selection, selectionOK := engine.ReadValue(frame, rule.placementRead)
	if !valueReadOK || !selectionOK || cells.Count() != 1 {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if !present || fact.IsBottom() {
		if count != 0 {
			return engine.RuleResult[placementdomain.Placement]{}
		}
		return engine.NoCandidate(frame)
	}
	plan, planOK := routePlanFor(rule.owner.Schema(), rule.values.Schema(), fact)
	if !planOK || count != len(plan.routes) {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, selection)
	}
	return engine.Routed(frame, selection, func(tag routeTag, prior engine.OrderedCells[placementdomain.Placement]) (placementdomain.Placement, bool) {
		if _, routeOK := routeAtTag(plan, tag); !routeOK || prior.Count() != 1 {
			return placementdomain.Bottom, false
		}
		current, currentPresent, currentAvailable := prior.At(0)
		if !currentAvailable {
			return placementdomain.Bottom, false
		}
		return returnValue(current, currentPresent, plan)
	})
}
