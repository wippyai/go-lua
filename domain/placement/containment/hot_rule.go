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

// HotRule is the Link-lane recursive containment transport. It retains only
// exact owner authorities; child routes are discovered from each live Heap
// Value during selector execution.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	heap           *heapowner.HotOwner
	parent         engine.Read[engine.OrderedCells[placement.Placement]]
	heapRead       engine.Read[engine.OrderedCells[heapdomain.Value]]
	routes         engine.Read[engine.Selection[uint64, engine.OrderedCells[placement.Placement]]]
}

// routeTag encodes an existing dense Heap coordinate and a traversal-local
// edge ordinal. The coordinate portion keeps duplicate edges to one child as
// separate Selection evidence without a parent/child pair catalog. The low
// part is nonzero because zero is reserved for an absent tag.
func routeTag(index int, edgeOrdinal uint64) (uint64, bool) {
	if index < 0 || uint64(index) >= uint64(^uint32(0)) || edgeOrdinal == 0 || edgeOrdinal > routeTagLowMask {
		return 0, false
	}
	return (uint64(index)+1)<<32 | edgeOrdinal, true
}

func exactRouteTag(index int, edgeOrdinal uint64) (uint64, bool) {
	if edgeOrdinal == 0 || edgeOrdinal > routeTagLowMask>>1 {
		return 0, false
	}
	return routeTag(index, edgeOrdinal<<1)
}

func broadcastRouteTag(index, dense int) (uint64, bool) {
	if dense < 0 || uint64(dense) > (routeTagLowMask>>1) {
		return 0, false
	}
	return routeTag(index, (uint64(dense)+1)<<1|1)
}

func routeKey(schema placement.Schema, tag uint64) (heapdomain.Key, bool) {
	if !schema.Valid() || tag == 0 || tag&routeTagLowMask == 0 || tag>>32 == 0 {
		return heapdomain.Key{}, false
	}
	index := (tag >> 32) - 1
	if index >= uint64(schema.KeyCount()) {
		return heapdomain.Key{}, false
	}
	key, ok := schema.KeyAt(int(index))
	return key, ok && key.Kind() == heapdomain.RootAllocation
}

func oneOrderedCell[T any](cells engine.OrderedCells[T]) (value T, present, available bool) {
	if cells.Count() != 1 {
		return value, false, false
	}
	return cells.At(0)
}

func validPlacement(value placement.Placement) bool {
	switch value {
	case placement.Bottom, placement.Stack, placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return true
	default:
		return false
	}
}

// BindHot binds the exact Link-lane containment transport to Placement's
// output factor and Heap's parent-read factor. Every admissible parent root is
// represented once by the owner-issued Link denominator.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, owner *placementowner.HotOwner, heap *heapowner.HotOwner, schema placement.Schema) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || owner == nil || !owner.MatchesBinding(binding) || heap == nil || !heap.MatchesBinding(binding) ||
		!owner.Schema().Valid() || heap.Schema() != schema.Heap() || owner.Schema() != schema ||
		!fragment.semantic.Available() {
		return nil, false
	}
	rule := &HotRule{owner: owner, heap: heap}
	implementation, bound := placementowner.BindSelectedRouteRuleDirect(owner, fragment.slot, fragment.carry, fragment.write, owner.FactorRef(), engine.HotRuleSpec[placement.Placement, operand]{
		OperandContent: rule.operandContent,
		Fold:           rule.fold,
	}, engine.HotCarrySpec[placement.Placement, operand]{}, nil)
	if !bound || implementation == nil {
		return nil, false
	}
	project := func(candidate operand) (uint64, bool) {
		return operandCoordinateForSchema(schema, candidate)
	}
	parentRead, parentOK := placementowner.AddSelectedRuleDirectExactRead(implementation, fragment.parentPlacement, owner.FactorRef(), project)
	if !parentOK {
		return nil, false
	}
	heapRead, heapOK := placementowner.AddSelectedRuleDirectExactRead[operand, heapdomain.Value](implementation, fragment.parentHeap, heap.FactorRef(), project)
	if !heapOK {
		return nil, false
	}
	routes, routesOK := placementowner.AddSelectedRuleDirectOperandRead[operand, placement.Placement, uint64](implementation, fragment.routes, owner.FactorRef(), rule.locate)
	if !routesOK {
		return nil, false
	}
	rule.implementation, rule.parent, rule.heapRead, rule.routes = implementation, parentRead, heapRead, routes
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) operandContent(candidate operand) (operand, [32]byte, bool) {
	if rule == nil || rule.owner == nil {
		return operand{}, [32]byte{}, false
	}
	canonical, digest, ok := operandContentForSchema(rule.owner.Schema(), candidate)
	if !ok {
		return operand{}, [32]byte{}, false
	}
	return canonical, digest, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (operand, bool) {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() ||
		rule.owner.Schema().Heap() != rule.heap.Schema() || !coords.Occurrence.Available() {
		return operand{}, false
	}
	key, keyOK := rule.owner.Schema().Heap().KeyForID(coords.Occurrence)
	if !keyOK {
		return operand{}, false
	}
	return operandForSchema(rule.owner.Schema(), key)
}

// Count implements rule.LinkCatalog by enumerating Heap's canonical mounted
// Program allocation rows. No Link-local ID or operand directory is retained.
func (rule *HotRule) Count() int {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return 0
	}
	count := 0
	heapSchema := rule.owner.Schema().Heap()
	for mountIndex := 0; mountIndex < heapSchema.ArtifactMountCount(); mountIndex++ {
		mount, mountOK := heapSchema.ArtifactMountAt(mountIndex)
		if !mountOK {
			return 0
		}
		issuer, issuerOK := heapSchema.OccurrenceMountForModule(mount.Module())
		if !issuerOK {
			return 0
		}
		allocationCount := issuer.AllocationCount()
		if allocationCount < 0 || count > int(^uint(0)>>1)-allocationCount {
			return 0
		}
		count += allocationCount
	}
	return count
}

// IDAt projects one canonical Heap KeyID from mounted Program allocation
// rows in their owner-issued mount order. The key is reauthenticated against
// the exact Placement schema before its identity is returned.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() || index < 0 {
		return identity.ContentID{}, false
	}
	heapSchema := rule.owner.Schema().Heap()
	for mountIndex := 0; mountIndex < heapSchema.ArtifactMountCount(); mountIndex++ {
		mount, mountOK := heapSchema.ArtifactMountAt(mountIndex)
		if !mountOK {
			return identity.ContentID{}, false
		}
		issuer, issuerOK := heapSchema.OccurrenceMountForModule(mount.Module())
		if !issuerOK {
			return identity.ContentID{}, false
		}
		allocationCount := issuer.AllocationCount()
		if index >= allocationCount {
			index -= allocationCount
			continue
		}
		_, key, keyOK := issuer.AllocationAt(index)
		if !keyOK {
			return identity.ContentID{}, false
		}
		candidate, candidateOK := operandForSchema(rule.owner.Schema(), key)
		if !candidateOK {
			return identity.ContentID{}, false
		}
		return candidate.id, true
	}
	return identity.ContentID{}, false
}

// Implementation returns the pending owner-typed Rule issuer.
func (rule *HotRule) Implementation() (*placementowner.RuleImplementation[operand], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	return rule.implementation, true
}

// SealProgramRule publishes the exact sealed Link-lane Rule row.
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

func (rule *HotRule) selectAllRoots(context engine.SelectorContext) bool {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() {
		return false
	}
	schema := rule.owner.Schema().Heap()
	for dense := 0; dense < schema.KeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			return false
		}
		if key.Kind() != heapdomain.RootAllocation {
			continue
		}
		tag, tagOK := broadcastRouteTag(dense, dense)
		if !tagOK || !rule.owner.SelectRoute(context, key, tag) {
			return false
		}
	}
	return true
}

func (rule *HotRule) locate(context engine.SelectorContext, candidate operand) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.accepts(candidate) {
		return false
	}
	parentCells, parentOK := engine.SelectorRead(context, rule.parent)
	heapCells, heapOK := engine.SelectorRead(context, rule.heapRead)
	if !parentOK || !heapOK {
		return false
	}
	parent, parentPresent, parentAvailable := oneOrderedCell(parentCells)
	heapValue, heapPresent, heapAvailable := oneOrderedCell(heapCells)
	if !parentAvailable || !heapAvailable {
		return false
	}
	parentKey := candidate.key
	parentIndex, parentIndexOK := rule.owner.Schema().Heap().KeyIndex(parentKey)
	if !parentIndexOK || parentIndex < 0 {
		return false
	}
	if !heapPresent {
		return true
	}
	if rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	if parentPresent && !validPlacement(parent) {
		return false
	}
	if !heapValue.Valid() || heapValue.IsTop() {
		return rule.selectAllRoots(context)
	}
	uncertainty := rule.containmentUncertainty(heapValue)
	if uncertainty.identityUnknown {
		return rule.selectAllRoots(context)
	}

	var edgeOrdinal uint64
	routeFailed := false
	complete := rule.owner.Schema().Heap().VisitContainments(heapValue, func(observation heapdomain.ContainmentVisit) bool {
		edgeOrdinal++
		if !observation.Valid() {
			return false
		}
		switch observation.Kind() {
		case heapdomain.ContainmentNone:
			return true
		case heapdomain.ContainmentUnknown:
			return false
		case heapdomain.ContainmentExact:
			reference, referenceOK := observation.Reference()
			childKey, _, childOK := reference.Key()
			if !referenceOK || !childOK || !rule.owner.Schema().Heap().OwnsKey(childKey) {
				return false
			}
			if childKey.Kind() != heapdomain.RootAllocation {
				return true
			}
			childIndex, childIndexOK := rule.owner.Schema().Heap().KeyIndex(childKey)
			tag, tagOK := exactRouteTag(childIndex, edgeOrdinal)
			if !childIndexOK || childIndex < 0 || !tagOK {
				return false
			}
			if !rule.owner.SelectRoute(context, childKey, tag) {
				routeFailed = true
				return false
			}
			return true
		default:
			return false
		}
	})
	if routeFailed {
		return false
	}
	if !complete {
		return rule.selectAllRoots(context)
	}
	return true
}

func (rule *HotRule) accepts(candidate operand) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	canonical, _, ok := operandContentForSchema(rule.owner.Schema(), candidate)
	return ok && canonical == candidate
}

type containmentUncertainty struct {
	identityUnknown bool
	classUnknown    bool
}

func (rule *HotRule) containmentUncertainty(value heapdomain.Value) containmentUncertainty {
	uncertainty := containmentUncertainty{}
	if rule == nil || rule.owner == nil || !value.Valid() {
		uncertainty.identityUnknown = true
		uncertainty.classUnknown = true
		return uncertainty
	}
	if value.IsTop() {
		uncertainty.identityUnknown = true
		uncertainty.classUnknown = true
		return uncertainty
	}
	complete := rule.owner.Schema().Heap().VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
		if !observation.Valid() {
			uncertainty.identityUnknown = true
			return false
		}
		if observation.Kind() == heapdomain.ContainmentUnknown {
			uncertainty.identityUnknown = true
			return false
		}
		if observation.Kind() == heapdomain.ContainmentExact {
			reference, ok := observation.Reference()
			if !ok {
				uncertainty.identityUnknown = true
				return false
			}
			child, _, keyOK := reference.Key()
			if !keyOK || !rule.owner.Schema().Heap().OwnsKey(child) {
				uncertainty.identityUnknown = true
				return false
			}
		}
		return true
	})
	if !complete {
		uncertainty.identityUnknown = true
	}
	return uncertainty
}

func (rule *HotRule) fold(frame engine.Frame[placement.Placement, operand]) engine.RuleResult[placement.Placement] {
	candidate, operandOK := engine.Operand(frame)
	if !operandOK || !rule.accepts(candidate) {
		return engine.RuleResult[placement.Placement]{}
	}
	parentCells, parentOK := engine.ReadValue(frame, rule.parent)
	heapCells, heapOK := engine.ReadValue(frame, rule.heapRead)
	routes, routesOK := engine.ReadValue(frame, rule.routes)
	if !parentOK || !heapOK || !routesOK {
		return engine.RuleResult[placement.Placement]{}
	}
	parent, parentPresent, parentAvailable := oneOrderedCell(parentCells)
	heapValue, heapPresent, heapAvailable := oneOrderedCell(heapCells)
	if !parentAvailable || !heapAvailable || !heapPresent {
		count, countOK := engine.SelectionCount(frame, routes)
		if countOK && count == 0 {
			return engine.NoSelection(frame, routes)
		}
		return engine.RuleResult[placement.Placement]{}
	}
	if parentPresent && !validPlacement(parent) {
		return engine.RuleResult[placement.Placement]{}
	}
	parentPlacement := placement.Bottom
	if parentPresent {
		parentPlacement = parent
	}
	count, countOK := engine.SelectionCount(frame, routes)
	if !countOK {
		return engine.RuleResult[placement.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, routes)
	}
	uncertainty := rule.containmentUncertainty(heapValue)
	if uncertainty.classUnknown {
		parentPlacement = placement.Unknown
	}
	return engine.Routed(frame, routes, func(tag uint64, cells engine.OrderedCells[placement.Placement]) (placement.Placement, bool) {
		if _, keyOK := routeKey(rule.owner.Schema(), tag); !keyOK || cells.Count() != 1 {
			return placement.Bottom, false
		}
		_, _, available := cells.At(0)
		if !available {
			return placement.Bottom, false
		}
		return parentPlacement, true
	})
}
