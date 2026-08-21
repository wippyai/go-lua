package store

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// HotRule is the mounted declarative Program-storage Placement consumer.
// Value issues the exact storage transfer receipt; Placement owns all route
// writes and its allocation coordinates.
type HotRule struct {
	implementation *placementowner.RuleImplementation[valuedomain.StorageTransfer]
	owner          *placementowner.HotOwner
	values         *valueowner.HotOwner
	valueRead      engine.Read[engine.OrderedCells[valuedomain.Value]]
	placementRead  engine.Read[engine.Selection[uint64, engine.OrderedCells[placement.Placement]]]
}

// BindHot binds the exact Value source read and selected Placement route lane.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, values *valueowner.HotOwner) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || values == nil || !values.MatchesBinding(binding) || !owner.Schema().Valid() || values.Schema() == nil || !values.Schema().Valid() ||
		!values.Schema().OwnsHeapSchema(owner.Schema().Heap()) {
		return nil, false
	}
	rule := &HotRule{owner: owner, values: values}
	implementation, ok := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Placement, valuedomain.StorageTransfer]{
		OperandContent: func(candidate valuedomain.StorageTransfer) (valuedomain.StorageTransfer, [32]byte, bool) {
			return hotStorageTransferContent(values.Schema(), candidate)
		},
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[placement.Placement, valuedomain.StorageTransfer]{}, nil)
	if !ok || implementation == nil {
		return nil, false
	}
	rule.implementation = implementation
	valueRead, valueReadOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.valueRead, values.FactorRef(), func(candidate valuedomain.StorageTransfer) (uint64, bool) {
		return storageTransferSourceCoordinate(values.Schema(), candidate)
	})
	if !valueReadOK {
		return nil, false
	}
	rule.valueRead = valueRead
	placementRead, placementReadOK := placementowner.AddSelectedRuleDirectOperandRead[valuedomain.StorageTransfer, placement.Placement, uint64](implementation, fragment.placementRead, owner.FactorRef(), rule.locate)
	if !placementReadOK {
		return nil, false
	}
	rule.placementRead = placementRead
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (valuedomain.StorageTransfer, bool) {
	if rule == nil || rule.values == nil || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return valuedomain.StorageTransfer{}, false
	}
	transfer, ok := rule.values.Schema().StorageTransferForArtifactOccurrence(coords.Mount, coords.Occurrence)
	return transfer, ok && rule.values.Schema().OwnsStorageTransfer(transfer)
}

func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[valuedomain.StorageTransfer], bool) {
	if rule == nil || rule.implementation == nil || rule.owner == nil {
		return nil, false
	}
	_, ok := placementowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	return rule.implementation, ok
}

func hotStorageTransferContent(schema *valuedomain.Schema, transfer valuedomain.StorageTransfer) (valuedomain.StorageTransfer, [32]byte, bool) {
	id, ok := transfer.ID()
	if schema == nil || !schema.OwnsStorageTransfer(transfer) || !ok || [32]byte(id) == ([32]byte{}) {
		return valuedomain.StorageTransfer{}, [32]byte{}, false
	}
	return transfer, [32]byte(id), true
}

func storageTransferSourceCoordinate(schema *valuedomain.Schema, transfer valuedomain.StorageTransfer) (uint64, bool) {
	if schema == nil || !schema.OwnsStorageTransfer(transfer) {
		return 0, false
	}
	from, _, ok := transfer.Endpoints()
	index, indexOK := schema.CoordinateIndex(from)
	return uint64(index), ok && indexOK
}

func (rule *HotRule) locate(context engine.SelectorContext, transfer valuedomain.StorageTransfer) bool {
	if rule == nil || rule.owner == nil || rule.values == nil {
		return false
	}
	canonical, _, contentOK := hotStorageTransferContent(rule.values.Schema(), transfer)
	if !contentOK || !canonical.Persistent() {
		return true
	}
	cells, readOK := engine.SelectorRead(context, rule.valueRead)
	if !readOK || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available || !present {
		return true
	}
	plan, planOK := Plan(rule.owner.Schema(), rule.values.Schema(), fact)
	if !planOK || plan.Bottom() {
		return false
	}
	for index := 0; index < plan.RouteCount(); index++ {
		route, routeOK := plan.RouteAt(index)
		if !routeOK || !placementowner.SelectRouteTyped(rule.owner, context, route.Key, route.Tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) fold(frame engine.Frame[placement.Placement, valuedomain.StorageTransfer]) engine.RuleResult[placement.Placement] {
	transfer, operandOK := engine.Operand(frame)
	if !operandOK || rule == nil || rule.owner == nil || rule.values == nil {
		return engine.RuleResult[placement.Placement]{}
	}
	canonical, _, contentOK := hotStorageTransferContent(rule.values.Schema(), transfer)
	if !contentOK {
		return engine.RuleResult[placement.Placement]{}
	}
	cells, valueReadOK := engine.ReadValue(frame, rule.valueRead)
	selection, selectionOK := engine.ReadValue(frame, rule.placementRead)
	if !valueReadOK || !selectionOK || cells.Count() != 1 {
		return engine.RuleResult[placement.Placement]{}
	}
	fact, present, available := cells.At(0)
	if !available {
		return engine.RuleResult[placement.Placement]{}
	}
	count, countOK := engine.SelectionCount(frame, selection)
	if !countOK {
		return engine.RuleResult[placement.Placement]{}
	}
	if !present || fact.IsBottom() {
		if count != 0 {
			return engine.RuleResult[placement.Placement]{}
		}
		// This Rule writes through a selected route. An absent source therefore
		// has an authenticated empty route set, not an unrouted candidate
		// omission. Returning NoCandidate would violate the routed-output shape
		// at settlement even though both reads completed successfully.
		return engine.NoSelection(frame, selection)
	}
	plan, planOK := Plan(rule.owner.Schema(), rule.values.Schema(), fact)
	if !planOK || count != plan.RouteCount() {
		return engine.RuleResult[placement.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, selection)
	}
	lifetime, lifetimeOK := canonical.Lifetime()
	if !lifetimeOK {
		return engine.RuleResult[placement.Placement]{}
	}
	return engine.Routed(frame, selection, func(tag uint64, prior engine.OrderedCells[placement.Placement]) (placement.Placement, bool) {
		if _, routeOK := plan.routeAtTag(tag); !routeOK || prior.Count() != 1 {
			return placement.Bottom, false
		}
		current, _, currentAvailable := prior.At(0)
		if !currentAvailable {
			return placement.Bottom, false
		}
		return Apply(current, FromProgram(lifetime)), true
	})
}
