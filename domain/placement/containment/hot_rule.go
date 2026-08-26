package containment

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

const routeTagLowMask = uint64(^uint32(0))

// HotRule is one artifact-independent mounted-point closure. Its singleton
// operand addresses complete local Placement and Heap summaries; exact child
// routes are discovered only while the staged selector is live.
type HotRule struct {
	implementation   *placementowner.RuleImplementation[Operand]
	owner            *placementowner.HotOwner
	heap             *heapowner.HotOwner
	closure          Operand
	placementSummary engine.Read[engine.OrderedCells[placement.Fact]]
	heapSummary      engine.Read[engine.OrderedCells[heapdomain.Value]]
	routes           engine.Read[engine.Selection[uint64, engine.OrderedCells[placement.Fact]]]
}

// routeTag is the exact semantic parent->child route. Both Factor owners use
// uint32 coordinates, so the pair occupies one lossless uint64 tag.
func routeTag(parent, child int) (uint64, bool) {
	if parent < 0 || child < 0 || uint64(parent) >= routeTagLowMask || uint64(child) >= routeTagLowMask {
		return 0, false
	}
	return (uint64(parent)+1)<<32 | uint64(child) + 1, true
}

func routeIndices(schema placement.Schema, tag uint64) (parent, child int, ok bool) {
	if !schema.Valid() || tag == 0 || tag>>32 == 0 || tag&routeTagLowMask == 0 {
		return 0, 0, false
	}
	parent64, child64 := (tag>>32)-1, (tag&routeTagLowMask)-1
	if parent64 >= uint64(schema.KeyCount()) || child64 >= uint64(schema.KeyCount()) {
		return 0, 0, false
	}
	childKey, childOK := schema.KeyAt(int(child64))
	if !childOK || childKey.Kind() != heapdomain.RootAllocation {
		return 0, 0, false
	}
	return int(parent64), int(child64), true
}

func validPlacement(value placement.Fact) bool {
	return value.Valid() && value.Class != placement.Bottom && value.RetainEscape != placement.EvidenceAbsent
}

func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, heap *heapowner.HotOwner, schema placement.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) ||
		heap == nil || !heap.MatchesBinding(binding) || !owner.Schema().Valid() || owner.Schema() != schema ||
		heap.Schema() != schema.Heap() || !fragment.semantic.Available() {
		return nil, false
	}
	closure, closureOK := operandForSchema(schema)
	if !closureOK {
		return nil, false
	}
	rule := &HotRule{owner: owner, heap: heap, closure: closure}
	implementation, bound := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Fact, Operand]{
		OperandContent: rule.operandContent, OperandResolver: rule.resolveOperand, Fold: rule.fold,
	}, engine.HotCarrySpec[placement.Fact, Operand]{}, nil)
	if !bound || implementation == nil {
		return nil, false
	}
	placementSummary, placementOK := placementowner.AddSelectedRuleDirectSummaryRead[Operand, placement.Fact, engine.OrderedCells[placement.Fact]](
		implementation, fragment.placementSummary, owner.FactorRef(), owner.FoldSummaryRead())
	heapSummary, heapOK := placementowner.AddSelectedRuleDirectSummaryRead[Operand, heapdomain.Value, engine.OrderedCells[heapdomain.Value]](
		implementation, fragment.heapSummary, heap.FactorRef(), heap.SummaryRead())
	routes, routesOK := placementowner.AddSelectedRuleDirectOperandRead[Operand, placement.Fact, uint64](implementation, fragment.routes, owner.FactorRef(), rule.locate)
	if !placementOK || !heapOK || !routesOK {
		return nil, false
	}
	rule.implementation, rule.placementSummary, rule.heapSummary, rule.routes = implementation, placementSummary, heapSummary, routes
	return rule, true
}

func (rule *HotRule) operandContent(candidate Operand) (Operand, [32]byte, bool) {
	if rule == nil || rule.owner == nil {
		return Operand{}, [32]byte{}, false
	}
	return operandContentForSchema(rule.owner.Schema(), candidate)
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (Operand, bool) {
	if rule == nil || rule.owner == nil || rule.heap == nil || !coords.Mount.Available() || !coords.Point.Available() ||
		coords.Occurrence != rule.closure.id || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return Operand{}, false
	}
	return rule.closure, true
}

func (rule *HotRule) Count() int {
	if rule == nil || !rule.accepts(rule.closure) {
		return 0
	}
	return 1
}

func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if index != 0 || rule == nil || rule.Count() != 1 {
		return identity.ContentID{}, false
	}
	return rule.closure.id, true
}

func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[Operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

func (rule *HotRule) accepts(candidate Operand) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	_, _, ok := operandContentForSchema(rule.owner.Schema(), candidate)
	return ok
}

func (rule *HotRule) validSummaryShapes(placements engine.OrderedCells[placement.Fact], heaps engine.OrderedCells[heapdomain.Value]) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil {
		return false
	}
	placementSchema, heapSchema := rule.owner.Schema(), rule.heap.Schema()
	if placements.Count() != placementSchema.KeyCount() || heaps.Count() != heapSchema.AllocationKeyCount() {
		return false
	}
	for index := 0; index < placements.Count(); index++ {
		placementKey, placementOK := placementSchema.KeyAt(index)
		heapKey, heapOK := heapSchema.AllocationKeyAt(index)
		if !placementOK || !heapOK || placementKey != heapKey {
			return false
		}
	}
	return true
}

func (rule *HotRule) locate(context engine.SelectorContext, candidate Operand) bool {
	if !rule.accepts(candidate) {
		return false
	}
	placements, placementsOK := engine.SelectorRead(context, rule.placementSummary)
	heaps, heapsOK := engine.SelectorRead(context, rule.heapSummary)
	if !placementsOK || !heapsOK || !rule.validSummaryShapes(placements, heaps) {
		return false
	}
	plan, planOK := DeriveContainmentRoutes(rule.owner.Schema(), rule.heap.Schema(), placements.Count(), placements.At, heaps.At)
	if !planOK {
		return false
	}
	for index := 0; index < plan.RouteCount(); index++ {
		item, itemOK := plan.RouteAt(index)
		child, _ := item.Coordinates()
		if !itemOK || !rule.owner.SelectRouteSet(context, child, item.Predicate()) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placement.Fact, Operand]) engine.RuleResult[placement.Fact] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || !rule.accepts(candidate) {
		return engine.RuleResult[placement.Fact]{}
	}
	placements, placementsOK := engine.ReadValue(frame, rule.placementSummary)
	heaps, heapsOK := engine.ReadValue(frame, rule.heapSummary)
	routes, routesOK := engine.ReadValue(frame, rule.routes)
	if !placementsOK || !heapsOK || !routesOK || !rule.validSummaryShapes(placements, heaps) {
		return engine.RuleResult[placement.Fact]{}
	}
	count, countOK := engine.SelectionCount(frame, routes)
	if !countOK {
		return engine.RuleResult[placement.Fact]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, routes)
	}
	return engine.Routed(frame, routes, func(tag uint64, cells engine.OrderedCells[placement.Fact]) (placement.Fact, bool) {
		parentIndex, _, tagOK := routeIndices(rule.owner.Schema(), tag)
		if !tagOK || cells.Count() != 1 {
			return placement.BottomFact(), false
		}
		current, present, available := cells.At(0)
		current, currentOK := placement.AuthenticateFactCell(current, present, available)
		if !currentOK {
			return placement.BottomFact(), false
		}
		parent, parentPresent, parentAvailable := placements.At(parentIndex)
		parent, parentOK := placement.AuthenticateFactCell(parent, parentPresent, parentAvailable)
		heapValue, heapOK := summaryCell(heaps.At, parentIndex, rule.heap.Schema().Bottom(), heapdomain.Equal)
		if !parentOK || !heapOK {
			return placement.BottomFact(), false
		}
		// The judgment is the declared fold's own: the placement policy a
		// contained child takes from its parent. The schema this cell was read
		// out of is already established by the operand admission and the
		// vector shapes, so what remains is the three cells themselves.
		return containmentValue(current, parent, heapValue)
	})
}
