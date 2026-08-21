package returnescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the mounted return escape. Value owns the return-boundary
// operand; Placement owns the selected route-write cell.
type HotRule struct {
	implementation  *placementowner.RuleImplementation[operand]
	owner           *placementowner.HotOwner
	values          *valueowner.HotOwner
	valueAnchorRead engine.Read[engine.OrderedCells[valuedomain.Value]]
	valueRead       engine.Read[engine.Selection[valueTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead   engine.Read[engine.Selection[routeTag, engine.OrderedCells[placementdomain.Placement]]]
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
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placementdomain.Placement, operand]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	valueAnchorRead, valueAnchorOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueAnchor, values.FactorRef(), func(candidate operand) (uint64, bool) {
		return returnRootCoordinateForSchema(values.Schema(), candidate)
	})
	if !valueAnchorOK {
		return nil, false
	}
	rule.valueAnchorRead = valueAnchorRead
	valueRead, valueReadOK := placementowner.AddSelectedRuleDirectOperandRead[operand, valuedomain.Value, valueTag](implementation, fragment.valueRead, values.FactorRef(), rule.locateValues)
	if !valueReadOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementReadOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placementdomain.Placement, routeTag](implementation, fragment.placementRead, owner.FactorRef(), rule.locate)
	if !placementReadOK {
		return nil, false
	}
	rule.placementRead = placementRead
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

func (rule *HotRule) locate(context engine.SelectorContext, operand operand) bool {
	if rule == nil || rule.values == nil || rule.owner == nil {
		return false
	}
	_, _, contentOK := returnOperandContentForSchema(rule.values.Schema(), operand)
	if !contentOK {
		return false
	}
	selection, readOK := engine.SelectorRead(context, rule.valueRead)
	if !readOK {
		return false
	}
	facts, factsOK := rule.collectFacts(context, selection, operand)
	if !factsOK {
		return false
	}
	plan, planOK := routePlanForFacts(rule.owner.Schema(), rule.values.Schema(), facts, operand.boundary.HasTail())
	if !planOK || plan.class == routeBottom {
		return false
	}
	for index := 0; index < plan.routeCount(); index++ {
		candidate, candidateOK := plan.routeAt(index)
		if !candidateOK {
			return false
		}
		if !placementowner.SelectRouteTyped(rule.owner, context, candidate.key, candidate.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locateValues(context engine.SelectorContext, operand operand) bool {
	if rule == nil || rule.values == nil {
		return false
	}
	if _, _, contentOK := returnOperandContentForSchema(rule.values.Schema(), operand); !contentOK {
		return false
	}
	boundary := operand.boundary
	_, rootOK := boundary.Root()
	if !rootOK {
		return false
	}
	for index := 0; index < boundary.MemberCount(); index++ {
		member, memberOK := boundary.MemberAt(index)
		tag, tagOK := boundaryValueTag(index)
		if !memberOK || !tagOK || !valueowner.SelectRouteTyped(rule.values, context, member, tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) collectFacts(context engine.SelectorContext, selection engine.Selection[valueTag, engine.OrderedCells[valuedomain.Value]], operand operand) (returnFacts, bool) {
	if rule == nil || rule.values == nil {
		return returnFacts{}, false
	}
	boundary := operand.boundary
	expectedCount := boundary.MemberCount()
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK || count != expectedCount {
		return returnFacts{}, false
	}
	var facts returnFacts
	for index := 0; index < expectedCount; index++ {
		tag, cells, selected := engine.SelectorSelectionAt(context, selection, index)
		expectedTag, tagOK := boundaryValueTag(index)
		coordinate, coordinateOK := boundaryCoordinateForTag(boundary, tag)
		if !tagOK || tag != expectedTag || !selected || cells.Count() != 1 || !coordinateOK {
			return returnFacts{}, false
		}
		fact, present, available := cells.At(0)
		if !available || (present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact)) {
			return returnFacts{}, false
		}
		facts.append(returnFact{fact: fact, present: present, available: available})
	}
	return facts, true
}

func (rule *HotRule) collectFactsFrame(frame engine.Frame[placementdomain.Placement, operand], selection engine.Selection[valueTag, engine.OrderedCells[valuedomain.Value]], operand operand) (returnFacts, bool) {
	if rule == nil || rule.values == nil {
		return returnFacts{}, false
	}
	boundary := operand.boundary
	expectedCount := boundary.MemberCount()
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != expectedCount {
		return returnFacts{}, false
	}
	var facts returnFacts
	for index := 0; index < expectedCount; index++ {
		tag, cells, selected := engine.SelectionAt(frame, selection, index)
		expectedTag, tagOK := boundaryValueTag(index)
		coordinate, coordinateOK := boundaryCoordinateForTag(boundary, tag)
		if !tagOK || tag != expectedTag || !selected || cells.Count() != 1 || !coordinateOK {
			return returnFacts{}, false
		}
		fact, present, available := cells.At(0)
		if !available || (present && !rule.values.Schema().AdmitsCoordinate(coordinate, fact)) {
			return returnFacts{}, false
		}
		facts.append(returnFact{fact: fact, present: present, available: available})
	}
	return facts, true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Placement, operand]) engine.RuleResult[placementdomain.Placement] {
	operand, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.values == nil || rule.owner == nil {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if _, _, contentOK := returnOperandContentForSchema(rule.values.Schema(), operand); !contentOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	anchorCells, anchorReadOK := engine.ReadValue(frame, rule.valueAnchorRead)
	selection, valueReadOK := engine.ReadValue(frame, rule.valueRead)
	placementSelection, placementReadOK := engine.ReadValue(frame, rule.placementRead)
	if !anchorReadOK || !valueReadOK || !placementReadOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	facts, factsOK := rule.collectFactsFrame(frame, selection, operand)
	if !factsOK || anchorCells.Count() != 1 {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	_, _, anchorAvailable := anchorCells.At(0)
	if !anchorAvailable {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	plan, planOK := routePlanForFacts(rule.owner.Schema(), rule.values.Schema(), facts, operand.boundary.HasTail())
	if !planOK || count != plan.routeCount() {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, prior engine.OrderedCells[placementdomain.Placement]) (placementdomain.Placement, bool) {
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
