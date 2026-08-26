package suspension

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// EvidenceHotRule is the Link-lane producer for the independent suspension
// evidence Factor. It shares the sealed Program/Value route catalog with the
// Placement consumer, but its output is typed to the evidence
// Factor and never inspect Placement class.
type EvidenceHotRule struct {
	implementation *EvidenceRuleImplementation[operand]
	owner          *EvidenceOwner
	values         *valueowner.HotOwner
	catalog        *Catalog
	valueAnchor    engine.Read[engine.OrderedCells[valuedomain.Value]]
	valueRead      engine.Read[engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]]]
	evidenceRead   engine.Read[engine.Selection[routeTag, engine.OrderedCells[Evidence]]]
}

// BindEvidenceHot binds the independent evidence producer to one shared hot
// transaction. It shares the sealed Program/Value catalog with the class
// consumer, but never admits an owner from an equal-schema foreign binding.
func BindEvidenceHot(binding *engine.SchemaBinding, fragment *EvidenceSchemaFragment, owner *EvidenceOwner, values *valueowner.HotOwner, valueSchema *valuedomain.Schema, schema placementdomain.Schema) (*EvidenceHotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || !fragment.route || owner == nil || !owner.MatchesBinding(binding) ||
		values == nil || !values.MatchesBinding(binding) || valueSchema == nil ||
		!owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() || !valueSchema.Valid() || values.Schema() != valueSchema ||
		owner.Schema() != schema || !valueSchema.OwnsHeapSchema(schema.Heap()) {
		return nil, false
	}
	catalog, catalogOK := SealEvidenceCatalog(schema, valueSchema)
	if !catalogOK {
		return nil, false
	}
	rule := &EvidenceHotRule{owner: owner, values: values, catalog: catalog}
	implementation, implementationOK := BindSelectedEvidenceRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, engine.HotRuleSpec[Evidence, operand]{
		OperandContent:  func(candidate operand) (operand, [32]byte, bool) { return rule.operandContent(candidate) },
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[Evidence, operand]{})
	if !implementationOK || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	valueAnchor, anchorOK := AddSelectedEvidenceRuleDirectExactRead(implementation, fragment.valueAnchor, values.FactorRef(), rule.anchorCoordinate)
	if !anchorOK {
		return nil, false
	}
	rule.valueAnchor = valueAnchor
	valueRead, valueOK := AddSelectedEvidenceRuleDirectOperandRead[operand, valuedomain.Value, routeTag](implementation, fragment.valueRead, values.FactorRef(), rule.locateValues)
	if !valueOK {
		return nil, false
	}
	rule.valueRead = valueRead
	evidenceRead, evidenceOK := AddSelectedEvidenceRuleDirectOperandRead[operand, Evidence, routeTag](implementation, fragment.evidenceRead, owner.FactorRef(), rule.locateEvidence)
	if !evidenceOK {
		return nil, false
	}
	rule.evidenceRead = evidenceRead
	return rule, true
}

// anchorCoordinate is the structural exact predecessor for the selected Value
// read. Keep it in lockstep with HotRule: it must never stand in for the
// operand's authenticated source list.
func (rule *EvidenceHotRule) anchorCoordinate(candidate operand) (uint64, bool) {
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
	// The selected-read predecessor is structural, but it still needs an
	// authenticated coordinate. A zero coordinate is not a legal synthetic
	// anchor for an operand with no source join.
	return 0, false
}

func (rule *EvidenceHotRule) operandContent(candidate operand) (operand, [32]byte, bool) {
	if rule == nil || rule.owner == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return operand{}, [32]byte{}, false
	}
	canonical, ok := rule.catalog.operandForID(candidate.id)
	if !ok || canonical.id != candidate.id || canonical.state != candidate.state || canonical.key != candidate.key {
		return operand{}, [32]byte{}, false
	}
	return canonical, [32]byte(canonical.id), true
}

func (rule *EvidenceHotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.catalog == nil {
		return operand{}, false
	}
	return rule.catalog.operandForID(coords.Occurrence)
}

func (rule *EvidenceHotRule) Catalog() *Catalog {
	if rule == nil {
		return nil
	}
	return rule.catalog
}

func (rule *EvidenceHotRule) Implementation() (*EvidenceRuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

func (rule *EvidenceHotRule) locateValues(context engine.SelectorContext, candidate operand) bool {
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

func (rule *EvidenceHotRule) locateEvidence(context engine.SelectorContext, candidate operand) bool {
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
		return SelectRouteTyped(rule.owner, context, canonical.key, routeTag(uint64(index)+1))
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
		if !itemOK || !SelectRouteTyped(rule.owner, context, item.key, item.tag) {
			return false
		}
	}
	return true
}

func suspensionEvidenceForState(state lifecycle.SubjectLivenessState) (Evidence, bool) {
	switch state {
	case lifecycle.SubjectLivenessDiesBefore:
		return EvidenceProven, true
	case lifecycle.SubjectLivenessLive:
		return EvidenceRefuted, true
	case lifecycle.SubjectLivenessUnknown:
		return EvidenceUnknown, true
	default:
		return EvidenceMissing, false
	}
}

func (rule *EvidenceHotRule) fold(frame engine.Frame[Evidence, operand]) engine.RuleResult[Evidence] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.owner == nil || rule.values == nil || rule.catalog == nil || rule.catalog.values == nil {
		return engine.RuleResult[Evidence]{}
	}
	canonical, _, contentOK := rule.operandContent(candidate)
	if !contentOK {
		return engine.RuleResult[Evidence]{}
	}
	valueSelection, valuesOK := engine.ReadValue(frame, rule.valueRead)
	evidenceSelection, evidenceOK := engine.ReadValue(frame, rule.evidenceRead)
	if !valuesOK || !evidenceOK {
		return engine.RuleResult[Evidence]{}
	}
	count, countOK := engine.SelectionCount(frame, evidenceSelection)
	if !countOK {
		return engine.RuleResult[Evidence]{}
	}
	plan, planOK := rule.transferPlan(frame, valueSelection, canonical)
	if !planOK || count != plan.count() {
		return engine.RuleResult[Evidence]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, evidenceSelection)
	}
	return engine.Routed(frame, evidenceSelection, func(tag routeTag, prior engine.OrderedCells[Evidence]) (Evidence, bool) {
		if _, routeOK := routeAtTag(plan, tag); !routeOK || prior.Count() != 1 {
			return EvidenceMissing, false
		}
		current, present, available := prior.At(0)
		current, currentOK := authenticateEvidenceCell(current, present, available)
		if !currentOK {
			return EvidenceMissing, false
		}
		want, wantOK := suspensionEvidenceForState(canonical.state)
		if !wantOK {
			return EvidenceMissing, false
		}
		if plan.widened() {
			// The widening came from an authenticated opaque/Top Value fact;
			// this is the one non-error path that intentionally publishes top.
			want = EvidenceUnknown
		}
		return current.JoinChecked(want)
	})
}

func (rule *EvidenceHotRule) transferPlan(frame engine.Frame[Evidence, operand], valueSelection engine.Selection[routeTag, engine.OrderedCells[valuedomain.Value]], canonical operand) (routePlan, bool) {
	selectedCount, selectedOK := engine.SelectionCount(frame, valueSelection)
	if !selectedOK {
		return routePlan{}, false
	}
	if canonical.key.Kind() == heapdomain.RootAllocation {
		index, indexOK := rule.owner.Schema().Heap().AllocationKeyIndex(canonical.key)
		if !indexOK || index < 0 || selectedCount != 0 {
			return routePlan{}, false
		}
		var plan routePlan
		plan.class = routeExact
		return plan, plan.add(route{key: canonical.key, tag: routeTag(uint64(index) + 1)})
	}
	if selectedCount != len(canonical.sources) {
		return routePlan{}, false
	}
	var factsInline [sourceFactInlineWidth]sourceFact
	facts, factsOK := sourceFactBuffer(selectedCount, factsInline[:])
	if !factsOK {
		return routePlan{}, false
	}
	for index := 0; index < selectedCount; index++ {
		tag, cells, selectedOK := engine.SelectionAt(frame, valueSelection, index)
		if !selectedOK || index >= len(canonical.sources) || tag != canonical.sources[index].tag || cells.Count() != 1 {
			return routePlan{}, false
		}
		fact, present, available := cells.At(0)
		facts[index] = sourceFact{fact: fact, present: present, available: available}
	}
	return routePlanForFacts(rule.owner.Schema(), rule.values.Schema(), facts)
}
