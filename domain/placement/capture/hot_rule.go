package capture

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema/structure"
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
	parentRead     engine.Read[engine.OrderedCells[placementdomain.Fact]]
	valueRead      engine.Read[engine.Selection[RouteTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead  engine.Read[engine.Selection[RouteTag, engine.OrderedCells[placementdomain.Fact]]]
}

// Capture source rows are normally short. Keep the authored-order fact plane
// and its physical-row coverage bitmap on the invocation stack; genuinely
// wide captures spill explicitly and are never retained by HotRule.
const captureFactInlineCapacity = 8

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
	implementation, implementationOK := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placementdomain.Fact, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return operandContentForSchema(schema, valueSchema, candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold: func(frame engine.Frame[placementdomain.Fact, operand]) engine.RuleResult[placementdomain.Fact] {
			return rule.fold(frame)
		},
	}, engine.HotCarrySpec[placementdomain.Fact, operand]{}, nil)
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
	valueRead, valueOK := placementowner.AddSelectedRuleDirectOperandRead[operand, valuedomain.Value, RouteTag](implementation, fragment.values, values.FactorRef(), rule.locateSources)
	if !valueOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placementdomain.Fact, RouteTag](implementation, fragment.placements, owner.FactorRef(), rule.locateRoutes)
	if !placementOK {
		return nil, false
	}
	rule.placementRead = placementRead
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
	var factsInline [captureFactInlineCapacity]SourceFact
	var seenInline [captureFactInlineCapacity]bool
	var facts []SourceFact
	var seen []bool
	if count <= captureFactInlineCapacity {
		facts = factsInline[:count]
		seen = seenInline[:count]
	} else {
		facts = make([]SourceFact, count)
		seen = make([]bool, count)
	}
	for physical := 0; physical < count; physical++ {
		tag, cells, selectedOK := engine.SelectorSelectionAt(context, selection, physical)
		logical, logicalOK := sourceOrdinal(candidate, tag)
		if !logicalOK || seen[logical] || !selectedOK || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		facts[logical] = SourceFact{fact: fact, present: present, available: available}
		seen[logical] = true
	}
	for _, present := range seen {
		if !present {
			return false
		}
	}
	plan, planOK := DeriveCaptureRoutes(rule.schema, rule.valueSchema, facts)
	if !planOK {
		return false
	}
	for index := 0; index < plan.RouteCount(); index++ {
		item, itemOK := plan.RouteAt(index)
		if !itemOK {
			return false
		}
		if !placementowner.SelectRouteTyped(rule.owner, context, item.key, item.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Fact, operand]) engine.RuleResult[placementdomain.Fact] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.owner == nil || rule.values == nil || !rule.schema.Valid() || rule.valueSchema == nil {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	parentCells, parentOK := engine.ReadValue(frame, rule.parentRead)
	valueSelection, valuesOK := engine.ReadValue(frame, rule.valueRead)
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	if !parentOK || !valuesOK || !placementOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	parent, parentPresent, parentAvailable := oneOrderedCell(parentCells)
	if !parentAvailable || !parentPresent {
		return engine.NoCandidate(frame)
	}
	parent, parentFactOK := placementdomain.AuthenticateFactCell(parent, parentPresent, parentAvailable)
	if !parentFactOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	plan, factsOK := rule.transferFacts(frame, valueSelection, candidate)
	if !factsOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK || count != plan.RouteCount() {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag RouteTag, prior engine.OrderedCells[placementdomain.Fact]) (placementdomain.Fact, bool) {
		expected, routeOK := RouteAtTag(plan, tag)
		if !routeOK || prior.Count() != 1 {
			return placementdomain.BottomFact(), false
		}
		current, currentPresent, currentAvailable := prior.At(0)
		current, currentOK := placementdomain.AuthenticateFactCell(current, currentPresent, currentAvailable)
		if !currentOK {
			return placementdomain.BottomFact(), false
		}
		// The declared fold is the judgment: it is handed the destination this
		// route publishes at and the tag it was correlated by, and it decides
		// what the capture means from them.
		key, _ := expected.Coordinates()
		result, outcome := CaptureFold(parent, key, uint64(tag), current)
		return result, outcome == structure.Concrete
	})
}

func (rule *HotRule) transferFacts(frame engine.Frame[placementdomain.Fact, operand], selection engine.Selection[RouteTag, engine.OrderedCells[valuedomain.Value]], candidate operand) (RoutePlan, bool) {
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK || count != len(candidate.sources) || count == 0 {
		return RoutePlan{}, false
	}
	var factsInline [captureFactInlineCapacity]SourceFact
	var seenInline [captureFactInlineCapacity]bool
	var facts []SourceFact
	var seen []bool
	if count <= captureFactInlineCapacity {
		facts = factsInline[:count]
		seen = seenInline[:count]
	} else {
		facts = make([]SourceFact, count)
		seen = make([]bool, count)
	}
	for physical := 0; physical < count; physical++ {
		tag, cells, selectedOK := engine.SelectionAt(frame, selection, physical)
		logical, logicalOK := sourceOrdinal(candidate, tag)
		if !logicalOK || seen[logical] || !selectedOK || cells.Count() != 1 {
			return RoutePlan{}, false
		}
		fact, present, available := cells.At(0)
		facts[logical] = SourceFact{fact: fact, present: present, available: available}
		seen[logical] = true
	}
	for _, present := range seen {
		if !present {
			return RoutePlan{}, false
		}
	}
	plan, planOK := DeriveCaptureRoutes(rule.schema, rule.valueSchema, facts)
	if !planOK {
		return RoutePlan{}, false
	}
	return plan, true
}
