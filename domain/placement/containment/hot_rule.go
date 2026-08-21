package containment

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
)

const routeTagLowMask = uint64(^uint32(0))

// HotRule is the Link-lane recursive containment transport. It retains exact
// owner authorities plus immutable populated-mount prefixes; Heap remains the
// allocation-key directory. Child routes are discovered from each live Heap
// Value during selector execution.
type HotRule struct {
	implementation *placementowner.RuleImplementation[operand]
	owner          *placementowner.HotOwner
	heap           *heapowner.HotOwner
	// catalogue retains only mounted prefix geometry. Heap remains the sole
	// allocation-key directory; Placement does not copy one key per allocation.
	catalogue *containmentCatalogue
	parent    engine.Read[engine.OrderedCells[placement.Placement]]
	heapRead  engine.Read[engine.OrderedCells[heapdomain.Value]]
	routes    engine.Read[engine.Selection[uint64, engine.OrderedCells[placement.Placement]]]
}

type catalogueMount struct {
	issuer heapdomain.OccurrenceMount
	start  int
	end    int
}

type containmentCatalogue struct {
	mounts []catalogueMount
	count  int
}

func (catalogue *containmentCatalogue) mountAt(index int) (catalogueMount, int, bool) {
	if catalogue == nil || index < 0 || index >= catalogue.count {
		return catalogueMount{}, 0, false
	}
	mountIndex := sort.Search(len(catalogue.mounts), func(candidate int) bool {
		return catalogue.mounts[candidate].end > index
	})
	if mountIndex >= len(catalogue.mounts) {
		return catalogueMount{}, 0, false
	}
	mount := catalogue.mounts[mountIndex]
	if index < mount.start {
		return catalogueMount{}, 0, false
	}
	return mount, index - mount.start, true
}

func buildCatalogue(schema placement.Schema) (*containmentCatalogue, bool) {
	if !schema.Valid() {
		return nil, false
	}
	heapSchema := schema.Heap()
	mounts := make([]catalogueMount, 0, heapSchema.ArtifactMountCount())
	count := 0
	for mountIndex := 0; mountIndex < heapSchema.ArtifactMountCount(); mountIndex++ {
		mount, mountOK := heapSchema.ArtifactMountAt(mountIndex)
		if !mountOK {
			return nil, false
		}
		issuer, issuerOK := heapSchema.OccurrenceMountForModule(mount.Module())
		if !issuerOK {
			return nil, false
		}
		allocationCount := issuer.AllocationCount()
		if allocationCount < 0 || allocationCount > int(^uint(0)>>1)-count {
			return nil, false
		}
		if allocationCount != 0 {
			mounts = append(mounts, catalogueMount{issuer: issuer, start: count, end: count + allocationCount})
		}
		count += allocationCount
	}
	return &containmentCatalogue{mounts: mounts, count: count}, true
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
	catalogue, catalogueOK := buildCatalogue(schema)
	if !catalogueOK {
		return nil, false
	}
	rule := &HotRule{owner: owner, heap: heap, catalogue: catalogue}
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

// Count implements rule.LinkCatalog from the immutable mounted allocation
// prefixes sealed at BindHot. Their total width is Heap's canonical mounted
// Program-allocation denominator. Binding is O(mounts): individual keys remain
// Heap-owned and are authenticated lazily by IDAt.
func (rule *HotRule) Count() int {
	if rule == nil || rule.owner == nil || rule.heap == nil || rule.catalogue == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return 0
	}
	return rule.catalogue.count
}

// IDAt locates one canonical mounted allocation through the prefix catalogue,
// then asks Heap for the key and reauthenticates it against the exact Placement
// schema before returning its identity.
func (rule *HotRule) IDAt(index int) (identity.ContentID, bool) {
	if rule == nil || rule.owner == nil || rule.heap == nil || rule.catalogue == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() || index < 0 || index >= rule.catalogue.count {
		return identity.ContentID{}, false
	}
	mount, local, mountOK := rule.catalogue.mountAt(index)
	if !mountOK {
		return identity.ContentID{}, false
	}
	_, key, keyOK := mount.issuer.AllocationAt(local)
	if !keyOK {
		return identity.ContentID{}, false
	}
	candidate, candidateOK := operandForSchema(rule.owner.Schema(), key)
	return candidate.id, candidateOK
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
	if !parentAvailable || !heapAvailable || !parentPresent || !heapPresent {
		return false
	}
	if rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	if !validPlacement(parent) || !heapValue.Valid() {
		return false
	}
	// A Top Heap relation is an authenticated semantic fact. It carries no
	// finite containment route, so it is the one class-level witness that may
	// widen this rule to every allocation root.
	if heapdomain.Equal(heapValue, rule.heap.Schema().Top()) {
		return rule.selectAllRoots(context)
	}

	// VisitContainments walks sealed arrays owned by an immutable Heap Schema.
	// First authenticate the complete walk and classify an opaque edge; only an
	// authenticated opaque edge may widen. Malformed, foreign, or interrupted
	// walks refuse instead of being compensated with all roots.
	opaque, complete := rule.containmentEvidence(heapValue)
	if !complete {
		return false
	}
	if opaque {
		return rule.selectAllRoots(context)
	}
	if !rule.walkContainments(heapValue, nil) {
		return false
	}
	if !rule.walkContainments(heapValue, func(key heapdomain.Key, tag uint64) bool {
		return rule.owner.SelectRoute(context, key, tag)
	}) {
		// A failed second pass may have emitted an exact prefix. Never append a
		// broadcast fallback to that partial Selection; fail closed instead.
		return false
	}
	return true
}

// containmentEvidence authenticates every containment observation and reports
// whether the value contains an owner-issued opaque edge. The two outcomes are
// deliberately separate: opaque is a semantic widening witness, whereas a
// failed walk is missing or malformed evidence and must refuse.
func (rule *HotRule) containmentEvidence(value heapdomain.Value) (opaque, complete bool) {
	if rule == nil || rule.owner == nil || !rule.owner.Schema().Valid() || !value.Valid() {
		return false, false
	}
	heapSchema := rule.owner.Schema().Heap()
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
			childKey, _, childOK := reference.Key()
			if !referenceOK || !childOK || !heapSchema.OwnsKey(childKey) {
				return false
			}
			return true
		default:
			return false
		}
	})
	return opaque, complete
}

func (rule *HotRule) walkContainments(value heapdomain.Value, emit func(heapdomain.Key, uint64) bool) bool {
	if rule == nil || rule.owner == nil || !value.Valid() || value.IsTop() {
		return false
	}
	heapSchema := rule.owner.Schema().Heap()
	var edgeOrdinal uint64
	return heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
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
			if !referenceOK || !childOK || !heapSchema.OwnsKey(childKey) {
				return false
			}
			if childKey.Kind() != heapdomain.RootAllocation {
				return true
			}
			childIndex, childIndexOK := heapSchema.KeyIndex(childKey)
			tag, tagOK := exactRouteTag(childIndex, edgeOrdinal)
			if !childIndexOK || childIndex < 0 || !tagOK {
				return false
			}
			return emit == nil || emit(childKey, tag)
		default:
			return false
		}
	})
}

func (rule *HotRule) accepts(candidate operand) bool {
	if rule == nil || rule.owner == nil || rule.heap == nil || !rule.owner.Schema().Valid() || !rule.heap.Schema().Valid() || rule.owner.Schema().Heap() != rule.heap.Schema() {
		return false
	}
	canonical, _, ok := operandContentForSchema(rule.owner.Schema(), candidate)
	return ok && canonical == candidate
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
	if !parentAvailable || !heapAvailable || !parentPresent || !heapPresent {
		return engine.RuleResult[placement.Placement]{}
	}
	if !validPlacement(parent) || !heapValue.Valid() {
		return engine.RuleResult[placement.Placement]{}
	}
	parentPlacement := parent
	count, countOK := engine.SelectionCount(frame, routes)
	if !countOK {
		return engine.RuleResult[placement.Placement]{}
	}
	if count == 0 {
		return engine.NoSelection(frame, routes)
	}
	// Top is the authenticated class-uncertainty witness. Opaque containment
	// widens identity routes but does not erase the known parent class.
	if heapdomain.Equal(heapValue, rule.heap.Schema().Top()) {
		parentPlacement = placement.Unknown
	}
	return engine.Routed(frame, routes, func(tag uint64, cells engine.OrderedCells[placement.Placement]) (placement.Placement, bool) {
		if _, keyOK := routeKey(rule.owner.Schema(), tag); !keyOK || cells.Count() != 1 {
			return placement.Bottom, false
		}
		_, present, available := cells.At(0)
		if !available || !present {
			return placement.Bottom, false
		}
		return parentPlacement, true
	})
}
