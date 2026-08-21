package capture

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the mounted closure-capture transport. Its operand is resolved
// from the mounted occurrence coordinates already carried by the engine.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	schema         placementdomain.Schema
	valueSchema    *valuedomain.Schema
	parentRead     engine.Read[engine.OrderedCells[placementdomain.Placement]]
	valueRead      engine.Read[engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead  engine.Read[engine.Selection[routeTag, engine.OrderedCells[placementdomain.Placement]]]
}

// BindHot binds the parent exact read, selected captured Value routes, and
// selected Placement route-write lane to the exact owner-fenced schemas.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, values *valueowner.HotOwner, valueSchema *valuedomain.Schema, schema placementdomain.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || values == nil || !values.MatchesBinding(binding) || valueSchema == nil ||
		!owner.Schema().Valid() || !values.Schema().Valid() || !valueSchema.Valid() || values.Schema() != valueSchema ||
		owner.Schema() != schema || !valueSchema.OwnsHeapSchema(schema.Heap()) ||
		!fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values, schema: schema, valueSchema: valueSchema}
	implementation, implementationOK := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placementdomain.Placement, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContentForSchema(schema, valueSchema, candidate)
		},
		Fold: func(frame engine.Frame[placementdomain.Placement, operand]) engine.RuleResult[placementdomain.Placement] {
			return rule.fold(frame)
		},
	}, engine.HotCarrySpec[placementdomain.Placement, operand]{}, nil)
	if !implementationOK || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	parentRead, parentOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.parent, owner.FactorRef(), func(candidate operand) (uint64, bool) {
		return operandCoordinateForSchema(schema, candidate)
	})
	if !parentOK {
		return nil, false
	}
	rule.parentRead = parentRead
	valueRead, valueOK := placementowner.AddSelectedRuleDirectOperandRead[operand, valuedomain.Value, routeTag](implementation, fragment.values, values.FactorRef(), rule.locateSources)
	if !valueOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placementdomain.Placement, routeTag](implementation, fragment.placements, owner.FactorRef(), rule.locateRoutes)
	if !placementOK {
		return nil, false
	}
	rule.placementRead = placementRead
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

// resolveOperand authenticates the mounted module through Heap's occurrence
// issuer, resolves the allocation occurrence to its owner-issued root, and
// then reuses the existing capture operand construction.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.owner == nil || !rule.schema.Valid() || rule.valueSchema == nil ||
		!coords.Mount.Available() || !coords.Occurrence.Available() {
		return operand{}, false
	}
	issuer, issuerOK := rule.schema.Heap().OccurrenceMountForModule(coords.Mount)
	key, keyOK := issuer.AllocationRootForOccurrence(coords.Occurrence)
	if !issuerOK || !keyOK {
		return operand{}, false
	}
	base, baseOK := rootOperandForSchema(rule.schema, key)
	if !baseOK {
		return operand{}, false
	}
	candidate, include, candidateOK := captureOperandForSchema(rule.schema, rule.valueSchema, base)
	return candidate, candidateOK && include
}

func operandCoordinateForSchema(schema placementdomain.Schema, candidate operand) (uint64, bool) {
	if !schema.Valid() || candidate.coordinate >= uint64(schema.KeyCount()) {
		return 0, false
	}
	return candidate.coordinate, true
}

func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule publishes the exact owner-resolved mounted rule after the
// shared binding seals.
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

func (rule *HotRule) locateSources(context engine.SelectorContext, candidate operand) bool {
	if rule == nil || rule.values == nil || len(candidate.sources) == 0 {
		return false
	}
	for _, item := range candidate.sources {
		if !valueowner.SelectRouteTyped(rule.values, context, item.coordinate, item.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locateRoutes(context engine.SelectorContext, candidate operand) bool {
	if rule == nil || rule.values == nil {
		return false
	}
	selection, selectionOK := engine.SelectorRead(context, rule.valueRead)
	if !selectionOK {
		return false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK {
		return false
	}
	if count != len(candidate.sources) || count == 0 {
		return false
	}
	facts := make([]sourceFact, count)
	for index := 0; index < count; index++ {
		tag, cells, selectedOK := engine.SelectorSelectionAt(context, selection, index)
		if !selectedOK || tag != candidate.sources[index].tag || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		facts[index] = sourceFact{fact: fact, present: present, available: available}
	}
	plan, planOK := routePlanForFacts(rule.schema, rule.valueSchema, facts)
	if !planOK {
		return false
	}
	for _, item := range plan.routes {
		if !placementowner.SelectRouteTyped(rule.owner, context, item.key, item.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Placement, operand]) engine.RuleResult[placementdomain.Placement] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.owner == nil || rule.values == nil || !rule.schema.Valid() || rule.valueSchema == nil {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	parentCells, parentOK := engine.ReadValue(frame, rule.parentRead)
	valueSelection, valuesOK := engine.ReadValue(frame, rule.valueRead)
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	if !parentOK || !valuesOK || !placementOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	parent, parentPresent, parentAvailable := oneOrderedCell(parentCells)
	if !parentAvailable || !parentPresent {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if !validPlacement(parent) {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	parentPlacement := parent
	_, plan, factsOK := rule.transferFacts(frame, valueSelection, candidate)
	if !factsOK {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK || count != len(plan.routes) {
		return engine.RuleResult[placementdomain.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, prior engine.OrderedCells[placementdomain.Placement]) (placementdomain.Placement, bool) {
		expected, routeOK := routeAtTag(plan, tag)
		if !routeOK || prior.Count() != 1 {
			return placementdomain.Bottom, false
		}
		current, currentPresent, currentAvailable := prior.At(0)
		if !currentAvailable || !currentPresent || !validPlacement(current) {
			return placementdomain.Bottom, false
		}
		if expected.key.Kind() != heapdomain.RootAllocation {
			return placementdomain.Bottom, false
		}
		return captureValue(parentPlacement, current), true
	})
}

func (rule *HotRule) transferFacts(frame engine.Frame[placementdomain.Placement, operand], selection engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]], candidate operand) ([]sourceFact, routePlan, bool) {
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != len(candidate.sources) || count == 0 {
		return nil, routePlan{}, false
	}
	facts := make([]sourceFact, count)
	for index := 0; index < count; index++ {
		tag, cells, selectedOK := engine.SelectionAt(frame, selection, index)
		if !selectedOK || tag != candidate.sources[index].tag || cells.Count() != 1 {
			return nil, routePlan{}, false
		}
		fact, present, available := cells.At(0)
		facts[index] = sourceFact{fact: fact, present: present, available: available}
	}
	plan, planOK := routePlanForFacts(rule.schema, rule.valueSchema, facts)
	if !planOK {
		return facts, routePlan{}, false
	}
	return facts, plan, true
}
