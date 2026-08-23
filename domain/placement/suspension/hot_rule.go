package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the Link suspension consumer. Program joins have
// already been sealed into Catalog; runtime keeps only the owner-fenced
// operand and exact Placement write implementation.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	catalog        *Catalog
	valueAnchor    engine.Read[engine.OrderedCells[valuedomain.Value]]
	valueRead      engine.Read[engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]]]
	placementRead  engine.Read[engine.Selection[routeTag, engine.OrderedCells[placementdomain.Fact]]]
}

// BindHot binds the Value-aware suspension bridge to one shared hot
// transaction. The catalog is sealed from the exact Placement/Heap and
// Value/Heap owner tuple; runtime selected reads then project
// Cell/Value/Values atoms onto exact Heap roots. Direct root rows use the same
// route lane. Every Value source is authenticated at catalog seal time;
// runtime cannot replace an unavailable or mismatched read with a wider route.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, values *valueowner.HotOwner, valueSchema *valuedomain.Schema, schema placementdomain.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || !fragment.route || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || valueSchema == nil ||
		!owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() || !valueSchema.Valid() || values.Schema() != valueSchema ||
		owner.Schema() != schema || !valueSchema.OwnsHeapSchema(schema.Heap()) {
		return nil, false
	}
	catalog, catalogOK := SealCatalog(schema, valueSchema)
	if !catalogOK {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values, catalog: catalog}
	implementation, implementationOK := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placementdomain.Fact, operand]{
		OperandContent: func(candidate operand) (operand, [32]byte, bool) {
			return rule.operandContent(candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placementdomain.Fact, operand]{}, nil)
	if !implementationOK || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	valueAnchor, anchorOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueAnchor, values.FactorRef(), rule.anchorCoordinate)
	if !anchorOK {
		return nil, false
	}
	rule.valueAnchor = valueAnchor
	valueRead, valueOK := placementowner.AddSelectedRuleDirectOperandRead[operand, valuedomain.Value, routeTag](implementation, fragment.valueRead, values.FactorRef(), rule.locateValues)
	if !valueOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placementdomain.Fact, routeTag](implementation, fragment.placementRead, owner.FactorRef(), rule.locateRoutes)
	if !placementOK {
		return nil, false
	}
	rule.placementRead = placementRead
	return rule, true
}

// anchorCoordinate supplies only the exact predecessor required by the
// engine's selected-read topology. It is never consulted by route planning:
// source rows are still selected from every catalog-authenticated Value
// coordinate. Prefer a row's first authenticated source, then the mounted
// Value coordinate belonging to an allocation root.
func (rule *HotRule) anchorCoordinate(candidate operand) (uint64, bool) {
	if rule == nil || rule.owner == nil || rule.values == nil || rule.values.Schema() == nil {
		return 0, false
	}
	schema := rule.values.Schema()
	if len(candidate.sources) != 0 {
		index, ok := schema.CoordinateIndex(candidate.sources[0].coordinate)
		return uint64(index), ok
	}
	if candidate.key.Kind() == heapdomain.RootAllocation {
		if valueID, idOK := rule.owner.Schema().Heap().AllocationRootValueID(candidate.key); idOK {
			if coordinate, coordinateOK := schema.CoordinateForID(valueID); coordinateOK {
				index, indexOK := schema.CoordinateIndex(coordinate)
				if indexOK {
					return uint64(index), true
				}
			}
		}
	}
	return 0, false
}

func (rule *HotRule) operandContent(candidate operand) (operand, [32]byte, bool) {
	if rule == nil || rule.owner == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return operand{}, [32]byte{}, false
	}
	canonical, ok := rule.catalog.operandForID(candidate.id)
	if !ok || canonical.id != candidate.id || canonical.state != candidate.state || canonical.key != candidate.key {
		return operand{}, [32]byte{}, false
	}
	return canonical, [32]byte(canonical.id), true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.catalog == nil {
		return operand{}, false
	}
	return rule.catalog.operandForID(coords.Occurrence)
}

func (rule *HotRule) Catalog() *Catalog {
	if rule == nil {
		return nil
	}
	return rule.catalog
}

func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

func (rule *HotRule) locateValues(context engine.SelectorContext, candidate operand) bool {
	if rule == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return false
	}
	canonical, _, contentOK := rule.operandContent(candidate)
	if !contentOK {
		return false
	}
	if canonical.key.Kind() == heapdomain.RootAllocation {
		return true
	}
	for _, item := range canonical.sources {
		if !valueowner.SelectRouteTyped(rule.values, context, item.coordinate, item.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locateRoutes(context engine.SelectorContext, candidate operand) bool {
	if rule == nil || rule.owner == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return false
	}
	canonical, _, contentOK := rule.operandContent(candidate)
	if !contentOK {
		return false
	}
	if canonical.key.Kind() == heapdomain.RootAllocation {
		index, indexOK := rule.owner.Schema().Heap().AllocationKeyIndex(canonical.key)
		if !indexOK || index < 0 {
			return false
		}
		return placementowner.SelectRouteTyped(rule.owner, context, canonical.key, routeTag(uint64(index)+1))
	}
	selection, selectionOK := engine.SelectorRead(context, rule.valueRead)
	if !selectionOK {
		return false
	}
	count, countOK := engine.SelectorSelectionCount(context, selection)
	if !countOK {
		return false
	}
	if count != len(canonical.sources) {
		return false
	}
	var factsInline [sourceFactInlineWidth]sourceFact
	facts, factsOK := sourceFactBuffer(count, factsInline[:])
	if !factsOK {
		return false
	}
	for index := 0; index < count; index++ {
		tag, cells, selectedOK := engine.SelectorSelectionAt(context, selection, index)
		if !selectedOK || index >= len(canonical.sources) || tag != canonical.sources[index].tag || cells.Count() != 1 {
			return false
		}
		fact, present, available := cells.At(0)
		facts[index] = sourceFact{fact: fact, present: present, available: available}
	}
	plan, planOK := routePlanForFacts(rule.owner.Schema(), rule.values.Schema(), facts)
	if !planOK {
		return false
	}
	for index := 0; index < plan.count(); index++ {
		item, itemOK := plan.at(index)
		if !itemOK || !placementowner.SelectRouteTyped(rule.owner, context, item.key, item.tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placementdomain.Fact, operand]) engine.RuleResult[placementdomain.Fact] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.owner == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	canonical, _, contentOK := rule.operandContent(candidate)
	if !contentOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	valueSelection, valuesOK := engine.ReadValue(frame, rule.valueRead)
	placementSelection, placementOK := engine.ReadValue(frame, rule.placementRead)
	if !valuesOK || !placementOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	count, countOK := engine.SelectionCount(frame, placementSelection)
	if !countOK {
		return engine.RuleResult[placementdomain.Fact]{}
	}

	var plan routePlan
	var planOK bool
	if canonical.key.Kind() == heapdomain.RootAllocation {
		selectedCount, selectedOK := engine.SelectionCount(frame, valueSelection)
		if !selectedOK || selectedCount != 0 {
			return engine.RuleResult[placementdomain.Fact]{}
		}
		index, indexOK := rule.owner.Schema().Heap().AllocationKeyIndex(canonical.key)
		if !indexOK || index < 0 {
			return engine.RuleResult[placementdomain.Fact]{}
		}
		planOK = plan.add(route{key: canonical.key, tag: routeTag(uint64(index) + 1)})
		plan.class = routeExact
	} else {
		selectedCount, selectedOK := engine.SelectionCount(frame, valueSelection)
		if !selectedOK || selectedCount != len(canonical.sources) {
			return engine.RuleResult[placementdomain.Fact]{}
		}
		var factsInline [sourceFactInlineWidth]sourceFact
		facts, factsOK := sourceFactBuffer(selectedCount, factsInline[:])
		if !factsOK {
			return engine.RuleResult[placementdomain.Fact]{}
		}
		for index := 0; index < selectedCount; index++ {
			tag, cells, selectedOK := engine.SelectionAt(frame, valueSelection, index)
			if !selectedOK || index >= len(canonical.sources) || tag != canonical.sources[index].tag || cells.Count() != 1 {
				return engine.RuleResult[placementdomain.Fact]{}
			}
			fact, present, available := cells.At(0)
			facts[index] = sourceFact{fact: fact, present: present, available: available}
		}
		plan, planOK = routePlanForFacts(rule.owner.Schema(), rule.values.Schema(), facts)
	}
	if !planOK || count != plan.count() {
		return engine.RuleResult[placementdomain.Fact]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, placementSelection)
	}
	return engine.Routed(frame, placementSelection, func(tag routeTag, prior engine.OrderedCells[placementdomain.Fact]) (placementdomain.Fact, bool) {
		_, routeOK := routeAtTag(plan, tag)
		if !routeOK || prior.Count() != 1 {
			return placementdomain.BottomFact(), false
		}
		current, currentPresent, currentAvailable := prior.At(0)
		current, currentOK := placementdomain.AuthenticateFactCell(current, currentPresent, currentAvailable)
		if !currentOK {
			return placementdomain.BottomFact(), false
		}
		want, wantOK := PlacementForState(canonical.state)
		if !wantOK {
			return placementdomain.BottomFact(), false
		}
		if plan.widened() {
			return placementdomain.UnknownFact(), true
		}
		return placementdomain.RaiseClassChecked(current, want)
	})
}
