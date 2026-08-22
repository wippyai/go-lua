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
	implementation   *placementowner.RuleImplementation[operand]
	owner            *placementowner.HotOwner
	heap             *heapowner.HotOwner
	closure          operand
	placementSummary engine.Read[engine.OrderedCells[placement.Placement]]
	heapSummary      engine.Read[engine.OrderedCells[heapdomain.Value]]
	routes           engine.Read[engine.Selection[uint64, engine.OrderedCells[placement.Placement]]]
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

func validPlacement(value placement.Placement) bool {
	switch value {
	case placement.Bottom, placement.Stack, placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return true
	default:
		return false
	}
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
	implementation, bound := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Placement, operand]{
		OperandContent: rule.operandContent, OperandResolver: rule.resolveOperand, Fold: rule.fold,
	}, engine.HotCarrySpec[placement.Placement, operand]{}, nil)
	if !bound || implementation == nil {
		return nil, false
	}
	placementSummary, placementOK := placementowner.AddSelectedRuleDirectSummaryRead[operand, placement.Placement, engine.OrderedCells[placement.Placement]](
		implementation, fragment.placementSummary, owner.FactorRef(), owner.FoldSummaryRead())
	heapSummary, heapOK := placementowner.AddSelectedRuleDirectSummaryRead[operand, heapdomain.Value, engine.OrderedCells[heapdomain.Value]](
		implementation, fragment.heapSummary, heap.FactorRef(), heap.SummaryRead())
	routes, routesOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placement.Placement, uint64](implementation, fragment.routes, owner.FactorRef(), rule.locate)
	if !placementOK || !heapOK || !routesOK {
		return nil, false
	}
	rule.implementation, rule.placementSummary, rule.heapSummary, rule.routes = implementation, placementSummary, heapSummary, routes
	return rule, true
}

func (rule *HotRule) operandContent(candidate operand) (operand, [32]byte, bool) {
	if rule == nil || rule.owner == nil {
		return operand{}, [32]byte{}, false
	}
	return operandContentForSchema(rule.owner.Schema(), candidate)
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.owner == nil || rule.heap == nil || !coords.Mount.Available() || !coords.Point.Available() ||
		coords.Occurrence != rule.closure.id || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return operand{}, false
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

func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

func (rule *HotRule) accepts(candidate operand) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	_, _, ok := operandContentForSchema(rule.owner.Schema(), candidate)
	return ok
}

func summaryCell[T any](cells engine.OrderedCells[T], index int, bottom T, equal func(T, T) bool) (T, bool) {
	value, present, available := cells.At(index)
	return value, available && (present || equal(value, bottom))
}

func (rule *HotRule) validSummaryShapes(placements engine.OrderedCells[placement.Placement], heaps engine.OrderedCells[heapdomain.Value]) bool {
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

func (rule *HotRule) locate(context engine.SelectorContext, candidate operand) bool {
	if !rule.accepts(candidate) {
		return false
	}
	placements, placementsOK := engine.SelectorRead(context, rule.placementSummary)
	heaps, heapsOK := engine.SelectorRead(context, rule.heapSummary)
	if !placementsOK || !heapsOK || !rule.validSummaryShapes(placements, heaps) {
		return false
	}
	for parentIndex := 0; parentIndex < placements.Count(); parentIndex++ {
		parent, parentPresent, parentAvailable := placements.At(parentIndex)
		parent, parentOK := placement.AuthenticateFactorCell(parent, parentPresent, parentAvailable)
		parentKey, parentKeyOK := rule.owner.Schema().KeyAt(parentIndex)
		heapIndex, heapIndexOK := rule.heap.Schema().AllocationKeyIndex(parentKey)
		heapValue, heapOK := summaryCell(heaps, heapIndex, rule.heap.Schema().Bottom(), heapdomain.Equal)
		if !parentOK || !parentKeyOK || !heapIndexOK || !heapOK || !validPlacement(parent) || !heapValue.Valid() {
			return false
		}
		if heapdomain.Equal(heapValue, rule.heap.Schema().Bottom()) {
			continue
		}
		emit := func(child heapdomain.Key) bool {
			childIndex, childOK := rule.heap.Schema().AllocationKeyIndex(child)
			if !childOK || childIndex < 0 || childIndex >= placements.Count() || child.Kind() != heapdomain.RootAllocation {
				return false
			}
			tag, tagOK := routeTag(parentIndex, childIndex)
			return tagOK && rule.owner.SelectRouteSet(context, child, tag)
		}
		if heapdomain.Equal(heapValue, rule.heap.Schema().Top()) {
			if !rule.walkAllRoots(emit) {
				return false
			}
			continue
		}
		opaque, complete := rule.containmentEvidence(heapValue)
		if !complete {
			return false
		}
		if opaque {
			if !rule.walkAllRoots(emit) {
				return false
			}
			continue
		}
		if !rule.walkContainments(heapValue, emit) {
			return false
		}
	}
	return true
}

func (rule *HotRule) walkAllRoots(emit func(heapdomain.Key) bool) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || emit == nil {
		return false
	}
	schema := rule.owner.Schema()
	if !schema.Valid() || schema.Heap() != rule.heap.Schema() {
		return false
	}
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK || !key.Valid() {
			return false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		if !schema.Heap().OwnsKey(key) {
			return false
		}
		if !emit(key) {
			return false
		}
	}
	return true
}

func (rule *HotRule) containmentEvidence(value heapdomain.Value) (opaque, complete bool) {
	if rule == nil || rule.heap == nil || !value.Valid() || value.IsTop() {
		return false, false
	}
	heapSchema := rule.heap.Schema()
	complete = heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if !observation.Valid() {
			return false
		}
		switch observation.Kind() {
		case heapdomain.ContainmentNone:
			return true
		case heapdomain.ContainmentUnknown:
			opaque = true
			return true
		case heapdomain.ContainmentExact:
			reference, referenceOK := observation.Reference()
			child, _, childOK := reference.Key()
			return referenceOK && childOK && heapSchema.OwnsKey(child)
		default:
			return false
		}
	})
	return opaque, complete
}

func (rule *HotRule) walkContainments(value heapdomain.Value, emit func(heapdomain.Key) bool) bool {
	if rule == nil || rule.heap == nil || !value.Valid() || value.IsTop() || emit == nil {
		return false
	}
	heapSchema := rule.heap.Schema()
	return heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if !observation.Valid() {
			return false
		}
		switch observation.Kind() {
		case heapdomain.ContainmentNone:
			return true
		case heapdomain.ContainmentExact:
			reference, referenceOK := observation.Reference()
			child, _, childOK := reference.Key()
			if !referenceOK || !childOK || !heapSchema.OwnsKey(child) {
				return false
			}
			return child.Kind() != heapdomain.RootAllocation || emit(child)
		default:
			return false
		}
	})
}

// routePlacement keeps the placement-policy coordinate separate from the
// child-identity coordinate. An authenticated Heap Top or opaque containment
// edge can widen which child route is selected, but it cannot erase the
// parent's known Placement. Unknown is returned only when that parent policy
// is itself Unknown (or when the input evidence is malformed and refused).
func routePlacement(parent placement.Placement, child heapdomain.Value, schema heapdomain.Schema) (placement.Placement, bool) {
	if !schema.Valid() || !validPlacement(parent) || !heapdomain.Equal(child, child) || placement.Equal(parent, placement.Bottom) || heapdomain.Equal(child, schema.Bottom()) {
		return placement.Bottom, false
	}
	return parent, true
}

func (rule *HotRule) fold(frame engine.Frame[placement.Placement, operand]) engine.RuleResult[placement.Placement] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || !rule.accepts(candidate) {
		return engine.RuleResult[placement.Placement]{}
	}
	placements, placementsOK := engine.ReadValue(frame, rule.placementSummary)
	heaps, heapsOK := engine.ReadValue(frame, rule.heapSummary)
	routes, routesOK := engine.ReadValue(frame, rule.routes)
	if !placementsOK || !heapsOK || !routesOK || !rule.validSummaryShapes(placements, heaps) {
		return engine.RuleResult[placement.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, routes)
	if !countOK {
		return engine.RuleResult[placement.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, routes)
	}
	return engine.Routed(frame, routes, func(tag uint64, cells engine.OrderedCells[placement.Placement]) (placement.Placement, bool) {
		parentIndex, _, tagOK := routeIndices(rule.owner.Schema(), tag)
		if !tagOK || cells.Count() != 1 {
			return placement.Bottom, false
		}
		current, present, available := cells.At(0)
		if _, currentOK := placement.AuthenticateFactorCell(current, present, available); !currentOK {
			return placement.Bottom, false
		}
		parent, parentPresent, parentAvailable := placements.At(parentIndex)
		parent, parentOK := placement.AuthenticateFactorCell(parent, parentPresent, parentAvailable)
		heapValue, heapOK := summaryCell(heaps, parentIndex, rule.heap.Schema().Bottom(), heapdomain.Equal)
		if !parentOK || !heapOK {
			return placement.Bottom, false
		}
		return routePlacement(parent, heapValue, rule.heap.Schema())
	})
}
