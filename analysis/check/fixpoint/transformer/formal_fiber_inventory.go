package transformer

import (
	"fmt"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalFiberRole is the closed storage vocabulary of one formal relation
// tuple.  It is deliberately structural: semantic dispatch remains owned by
// the registered ProductDomain descriptor retained by each product fiber.
type formalFiberRole uint8

const (
	formalFiberInvalid formalFiberRole = iota
	formalFiberCare
	formalFiberMiddleValue
	formalFiberMiddlePath
	formalFiberOutcome
	formalFiberCallOutcome
	// Diagnostics is the one recursive call-boundary diagnostic lattice. It is
	// deliberately outside the registered product groups: a diagnostic-only
	// relation contribution updates one scalar fiber and never materializes the
	// 17-axis product.
	formalFiberDiagnostics
	formalFiberOrdinaryLane
	formalFiberCoordinate
	formalFiberGroundValueTop
	formalFiberGroundValue
)

type formalFiberCoordinateKind uint8

const (
	formalFiberCoordinateInvalid formalFiberCoordinateKind = iota
	formalFiberCoordinateFamilySkeleton
	formalFiberCoordinateFamilyScalar
)

// formalFiberDescriptor is one immutable typed leaf in the forest inventory.
// Only the fields selected by role are populated.  Opaque ProductDomain
// descriptors retain their registration-owned algebra; this package never
// switches on a lane or family name.
type formalFiberDescriptor struct {
	forest   *formalFiberInventory
	variable relationVar
	global   int
	role     formalFiberRole

	slot           FormalSlot
	outcome        boundaryOutcomeRef
	point          cfg.Point
	lane           state.ProductLane
	family         state.CoordinateFamily
	coordinate     state.CoordinateSlot
	coordinateKind formalFiberCoordinateKind
}

type formalFiberGroupKind uint8

const (
	formalFiberGroupInvalid formalFiberGroupKind = iota
	formalFiberGroupOrdinaryLane
	formalFiberGroupCoordinateLane
	formalFiberGroupValues
)

// formalCoordinateFamilyFiberGroup is the exact, explicitly addressed part
// of a whole coordinate-lane group owned by one registered family.  The
// scalar ordinals are not assumed to be adjacent to the skeleton or to one
// another; freeze validates the complete descriptor-to-group relation once.
type formalCoordinateFamilyFiberGroup struct {
	family           state.CoordinateFamily
	skeleton         formalFiberOrdinal
	skeletonPosition int
	scalars          []formalFiberOrdinal
	scalarPositions  []int
}

type formalValueSlotFiber struct {
	slot     FormalSlot
	ordinal  formalFiberOrdinal
	position int
}

// formalFiberGroupDescriptor is immutable relation-owned publication
// metadata. A group is the smallest semantic unit that may be updated: the
// complete Values ValueFactor, one complete ordinary LaneFactor, or every
// registered coordinate family of one ProductLane. There is exactly one group
// per ProductLane, in ProductDomain order. Callers receive typed capabilities
// below, never a writable raw ordinal list.
type formalFiberGroupDescriptor struct {
	forest   *formalFiberInventory
	variable relationVar
	global   int
	kind     formalFiberGroupKind
	lane     state.ProductLane
	members  []formalFiberOrdinal

	coordinateFamilies []formalCoordinateFamilyFiberGroup
	valueTop           formalFiberOrdinal
	valueTopPosition   int
	valueSlots         []formalValueSlotFiber
	valueSlotPosition  map[FormalSlot]int
	valueDomain        lattice.Lattice[state.ValueFactor[FormalSlot]]
}

// formalOrdinaryLaneFiberGroup is the complete carrier capability for one
// non-coordinate ProductLane. Its single physical member is an implementation
// detail; all semantic operations still use the registered LaneFactor laws.
type formalOrdinaryLaneFiberGroup struct{ descriptor formalFiberGroupDescriptor }

func (g formalOrdinaryLaneFiberGroup) valid() bool {
	return g.descriptor.valid() && g.descriptor.kind == formalFiberGroupOrdinaryLane && len(g.descriptor.members) == 1
}

func (g formalOrdinaryLaneFiberGroup) lane() (state.ProductLane, bool) {
	return g.descriptor.lane, g.valid()
}

func (g formalOrdinaryLaneFiberGroup) member() (formalFiberGroupMember, bool) {
	if !g.valid() {
		return formalFiberGroupMember{}, false
	}
	return g.descriptor.member(g.descriptor.members[0])
}

// formalFiberGroupMember is a group-owned point-update capability. Its
// ordinal is deliberately private: only the directory bridge in this package
// may lower a checked member to a physical address.
type formalFiberGroupMember struct {
	group    formalFiberGroupDescriptor
	ordinal  formalFiberOrdinal
	position int
}

// formalCallOutcomeFiber is the typed scalar capability for one lexical call
// point. Its ordinal remains private to the frozen body span; writers never
// scan inventory or address a raw fiber.
type formalCallOutcomeFiber struct {
	descriptor formalFiberDescriptor
	ordinal    formalFiberOrdinal
}

func (f formalCallOutcomeFiber) valid() bool {
	return f.descriptor.forest != nil && f.descriptor.variable != 0 && f.descriptor.role == formalFiberCallOutcome &&
		f.descriptor.point > 0 && f.ordinal >= 0
}

func (m formalFiberGroupMember) address(group formalFiberGroupDescriptor) (formalFiberOrdinal, bool) {
	if !group.valid() || !m.group.same(group) || m.position < 0 || m.position >= len(group.members) ||
		group.members[m.position] != m.ordinal {
		return 0, false
	}
	return m.ordinal, true
}

func (g formalFiberGroupDescriptor) valid() bool {
	return g.forest != nil && g.variable != 0 && g.global >= 0 && g.global < len(g.forest.groups) &&
		g.forest.groups[g.global].forest == g.forest && g.forest.groups[g.global].variable == g.variable
}

func (g formalFiberGroupDescriptor) same(other formalFiberGroupDescriptor) bool {
	return g.valid() && other.valid() && g.forest == other.forest && g.variable == other.variable && g.global == other.global
}

func (g formalFiberGroupDescriptor) member(ordinal formalFiberOrdinal) (formalFiberGroupMember, bool) {
	if !g.valid() {
		return formalFiberGroupMember{}, false
	}
	span, ok := g.forest.span(g.variable)
	if !ok || int(ordinal) < 0 || int(ordinal) >= span.count {
		return formalFiberGroupMember{}, false
	}
	membership := g.forest.groupMembership[span.first+int(ordinal)]
	if membership.group != g.global || membership.position < 0 || membership.position >= len(g.members) || g.members[membership.position] != ordinal {
		return formalFiberGroupMember{}, false
	}
	return formalFiberGroupMember{group: g, ordinal: ordinal, position: membership.position}, true
}

func (g formalFiberGroupDescriptor) ordinals() []formalFiberOrdinal {
	if !g.valid() {
		return nil
	}
	return append([]formalFiberOrdinal(nil), g.members...)
}

func cloneFormalFiberGroupDescriptor(group formalFiberGroupDescriptor) formalFiberGroupDescriptor {
	group.members = append([]formalFiberOrdinal(nil), group.members...)
	group.valueSlots = append([]formalValueSlotFiber(nil), group.valueSlots...)
	if group.valueSlotPosition != nil {
		positions := make(map[FormalSlot]int, len(group.valueSlotPosition))
		for slot, position := range group.valueSlotPosition {
			positions[slot] = position
		}
		group.valueSlotPosition = positions
	}
	group.coordinateFamilies = append([]formalCoordinateFamilyFiberGroup(nil), group.coordinateFamilies...)
	for index := range group.coordinateFamilies {
		group.coordinateFamilies[index].scalars = append([]formalFiberOrdinal(nil), group.coordinateFamilies[index].scalars...)
		group.coordinateFamilies[index].scalarPositions = append([]int(nil), group.coordinateFamilies[index].scalarPositions...)
	}
	return group
}

// formalValuesFiberGroup is the only capability permitted to publish a
// state.ValueFactor[FormalSlot]. Its exact lattice and full slot ownership
// live beside the physical group, so Values cannot silently degrade into
// independent per-slot updates.
type formalValuesFiberGroup struct{ descriptor formalFiberGroupDescriptor }

func (g formalValuesFiberGroup) valid() bool {
	return g.descriptor.valid() && g.descriptor.kind == formalFiberGroupValues
}

func (g formalValuesFiberGroup) lattice() (lattice.Lattice[state.ValueFactor[FormalSlot]], bool) {
	return g.descriptor.valueDomain, g.valid()
}

func (g formalValuesFiberGroup) top() (formalFiberGroupMember, bool) {
	if !g.valid() {
		return formalFiberGroupMember{}, false
	}
	member := formalFiberGroupMember{group: g.descriptor, ordinal: g.descriptor.valueTop, position: g.descriptor.valueTopPosition}
	_, ok := member.address(g.descriptor)
	return member, ok
}

func (g formalValuesFiberGroup) slot(slot FormalSlot) (formalFiberGroupMember, bool) {
	if !g.valid() {
		return formalFiberGroupMember{}, false
	}
	index, ok := g.descriptor.valueSlotPosition[slot]
	if !ok || index < 0 || index >= len(g.descriptor.valueSlots) {
		return formalFiberGroupMember{}, false
	}
	candidate := g.descriptor.valueSlots[index]
	member := formalFiberGroupMember{group: g.descriptor, ordinal: candidate.ordinal, position: candidate.position}
	_, ok = member.address(g.descriptor)
	return member, ok
}

func (g formalValuesFiberGroup) owns(factor state.ValueFactor[FormalSlot]) bool {
	if !g.valid() || (factor.Top && len(factor.Values) != 0) {
		return false
	}
	for slot := range factor.Values {
		if _, ok := g.slot(slot); !ok {
			return false
		}
	}
	return true
}

// formalCoordinateLaneFiberGroup is the only capability permitted to publish
// a factored coordinate ProductLane. All registered families of the lane are
// one group because ComposeCoordinateFamilies returns one LaneFactor.
type formalCoordinateLaneFiberGroup struct{ descriptor formalFiberGroupDescriptor }

func (g formalCoordinateLaneFiberGroup) valid() bool {
	return g.descriptor.valid() && g.descriptor.kind == formalFiberGroupCoordinateLane
}

func (g formalCoordinateLaneFiberGroup) lane() (state.ProductLane, bool) {
	return g.descriptor.lane, g.valid()
}

func (g formalCoordinateLaneFiberGroup) families() []formalCoordinateFamilyFiberGroup {
	if !g.valid() {
		return nil
	}
	out := make([]formalCoordinateFamilyFiberGroup, len(g.descriptor.coordinateFamilies))
	for index, family := range g.descriptor.coordinateFamilies {
		out[index] = formalCoordinateFamilyFiberGroup{
			family: family.family, skeleton: family.skeleton, skeletonPosition: family.skeletonPosition,
			scalars: append([]formalFiberOrdinal(nil), family.scalars...), scalarPositions: append([]int(nil), family.scalarPositions...),
		}
	}
	return out
}

func (g formalCoordinateLaneFiberGroup) member(ordinal formalFiberOrdinal) (formalFiberGroupMember, bool) {
	if !g.valid() {
		return formalFiberGroupMember{}, false
	}
	return g.descriptor.member(ordinal)
}

// formalFiberDescriptorSpan is one body's contiguous descriptor range and
// sole persistent-directory authority.  Directory ordinals are local to the
// span, so a foreign body cannot accidentally address a same-numbered fiber.
type formalFiberDescriptorSpan struct {
	forest      *formalFiberInventory
	variable    relationVar
	first       int
	count       int
	keys        *keyspace.KeySpace
	rekey       state.CoordinateFormalRootRekey
	valueRekey  state.ExactValueFactorRekey[FormalSlot, statekey.Value]
	liveValues  map[statekey.ValueDependency]FormalSlot
	coordinates state.CoordinateFactorInventory
	groupFirst  int
	groupCount  int
}

// formalFiberInventory is the one forest-owned product schema.  Descriptors
// are retained once; equation cells retain only roots in their body's arena.
// No solve-time query is accepted by this type, so DynamicReadQueryPlan cannot
// grow the fixed product universe.
type formalFiberInventory struct {
	program              *RelationProgram
	slots                *SlotSpace
	descriptors          []formalFiberDescriptor
	spans                []formalFiberDescriptorSpan
	groups               []formalFiberGroupDescriptor
	groupMembership      []formalFiberGroupMembership
	externalCalls        map[formalExternalCallSiteKey]formalPreparedExternalCallSite
	operatorFootprints   *formalOperatorCoordinateFootprints
	applySelectors       *formalApplyCoordinateSelectorCatalog
	applyCoordinateTrace map[formalFrameFootprintKey]formalApplyCoordinateStaticTrace
	// outcomeSourceIdentities retains the static identity-support solution in
	// Outcome-source order.  The N5 executor consumes this frozen plan before
	// result binding; it must not rediscover identities from coordinate facts.
	outcomeSourceIdentities map[formalRelationCell][]formalOutcomeSourceIdentityPlan
	// pathStoreOwnerIdentities is the occurrence-specific N4 owner authority
	// for descendant heap-member writes. It is frozen from cell-input identity
	// support before relation execution and is never rebuilt from leaf state.
	pathStoreOwnerIdentities map[formalRelationCell]formalPathStoreOwnerIdentityPlan
	operatorTopology         formalDefinitionResourceTopology
}

type formalFiberGroupMembership struct {
	group    int
	position int
}

func (i *formalFiberInventory) span(variable relationVar) (formalFiberDescriptorSpan, bool) {
	if i == nil || variable == 0 || int(variable) > len(i.spans) {
		return formalFiberDescriptorSpan{}, false
	}
	span := i.spans[variable-1]
	return span, span.forest == i && span.variable == variable && span.keys != nil && span.keys.Valid()
}

func (s formalFiberDescriptorSpan) descriptors() []formalFiberDescriptor {
	if s.forest == nil || s.variable == 0 || s.first < 0 || s.count < 0 || s.first+s.count > len(s.forest.descriptors) {
		return nil
	}
	return append([]formalFiberDescriptor(nil), s.forest.descriptors[s.first:s.first+s.count]...)
}

func (s formalFiberDescriptorSpan) ordinal(descriptor formalFiberDescriptor) (formalFiberOrdinal, bool) {
	if s.forest == nil || descriptor.forest != s.forest || descriptor.variable != s.variable ||
		descriptor.global < s.first || descriptor.global >= s.first+s.count {
		return 0, false
	}
	return formalFiberOrdinal(descriptor.global - s.first), true
}

func (s formalFiberDescriptorSpan) groupDescriptors() []formalFiberGroupDescriptor {
	if s.forest == nil || s.groupFirst < 0 || s.groupCount < 0 || s.groupFirst+s.groupCount > len(s.forest.groups) {
		return nil
	}
	out := make([]formalFiberGroupDescriptor, s.groupCount)
	for index, group := range s.forest.groups[s.groupFirst : s.groupFirst+s.groupCount] {
		out[index] = cloneFormalFiberGroupDescriptor(group)
	}
	return out
}

func (s formalFiberDescriptorSpan) valuesGroup() (formalValuesFiberGroup, bool) {
	for _, group := range s.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			return formalValuesFiberGroup{descriptor: group}, true
		}
	}
	return formalValuesFiberGroup{}, false
}

func (s formalFiberDescriptorSpan) callOutcomeFiber(point cfg.Point) (formalCallOutcomeFiber, bool) {
	if s.forest == nil || s.variable == 0 || point <= 0 {
		return formalCallOutcomeFiber{}, false
	}
	for _, descriptor := range s.descriptors() {
		if descriptor.role != formalFiberCallOutcome || descriptor.point != point {
			continue
		}
		ordinal, exact := s.ordinal(descriptor)
		fiber := formalCallOutcomeFiber{descriptor: descriptor, ordinal: ordinal}
		return fiber, exact && fiber.valid()
	}
	return formalCallOutcomeFiber{}, false
}

func (s formalFiberDescriptorSpan) coordinateLaneGroup(lane state.ProductLane) (formalCoordinateLaneFiberGroup, bool) {
	for _, group := range s.groupDescriptors() {
		if group.kind == formalFiberGroupCoordinateLane && group.lane == lane {
			return formalCoordinateLaneFiberGroup{descriptor: group}, true
		}
	}
	return formalCoordinateLaneFiberGroup{}, false
}

func (s formalFiberDescriptorSpan) ordinaryLaneGroup(lane state.ProductLane) (formalOrdinaryLaneFiberGroup, bool) {
	for _, group := range s.groupDescriptors() {
		if group.kind == formalFiberGroupOrdinaryLane && group.lane == lane {
			return formalOrdinaryLaneFiberGroup{descriptor: group}, true
		}
	}
	return formalOrdinaryLaneFiberGroup{}, false
}

func (s formalFiberDescriptorSpan) groupForOrdinal(ordinal formalFiberOrdinal) (formalFiberGroupDescriptor, bool) {
	if int(ordinal) < 0 || int(ordinal) >= s.count {
		return formalFiberGroupDescriptor{}, false
	}
	membership := s.forest.groupMembership[s.first+int(ordinal)]
	if membership.group < s.groupFirst || membership.group >= s.groupFirst+s.groupCount {
		return formalFiberGroupDescriptor{}, false
	}
	group := cloneFormalFiberGroupDescriptor(s.forest.groups[membership.group])
	_, ok := group.member(ordinal)
	return group, ok
}

// validateFormalFiberProductGroups proves that retained formal groups are the
// exact dependent-product inventory of the body ProductDomain. This is rerun
// at consumer freeze boundaries so mutated, stale, or foreign metadata cannot
// be accepted merely because individual lane lookups still succeed.
func validateFormalFiberProductGroups(span formalFiberDescriptorSpan, domain state.ProductDomain) error {
	if span.forest == nil || !domain.Valid() {
		return fmt.Errorf("formal product group inventory is unowned")
	}
	groups := span.groupDescriptors()
	lanes := domain.LaneInventory()
	if len(groups) != len(lanes) {
		return fmt.Errorf("formal product group count %d differs from registered lane count %d", len(groups), len(lanes))
	}
	valuesLane, valuesEnabled := domain.SlotFactoredCarrier()
	for index, lane := range lanes {
		group := groups[index]
		if !group.valid() || group.lane != lane {
			return fmt.Errorf("formal product group %d differs from registered lane order", index)
		}
		if valuesEnabled && lane == valuesLane {
			if group.kind != formalFiberGroupValues {
				return fmt.Errorf("formal Values lane %q has wrong group kind", lane.ID())
			}
			continue
		}
		families, err := domain.CoordinateFamilies(lane)
		if err != nil {
			return err
		}
		if len(families) == 0 {
			if group.kind != formalFiberGroupOrdinaryLane || len(group.members) != 1 {
				return fmt.Errorf("formal ordinary lane %q has wrong group shape", lane.ID())
			}
			continue
		}
		if group.kind != formalFiberGroupCoordinateLane || len(group.coordinateFamilies) != len(families) {
			return fmt.Errorf("formal coordinate lane %q has wrong group shape", lane.ID())
		}
		for familyIndex, family := range families {
			if group.coordinateFamilies[familyIndex].family != family {
				return fmt.Errorf("formal coordinate lane %q differs from registered family order", lane.ID())
			}
		}
	}
	return nil
}

// freezeFormalFiberInventory freezes the complete physical product topology
// after term/Middle schemas and static coordinate producers have sealed, and
// before formal-region WTO execution begins.  Dynamic reads remain ValueTerm
// syntax; only their already-admitted formal roots and static producer
// coordinates participate here.
func freezeFormalFiberInventory(program *RelationProgram) (*formalFiberInventory, error) {
	if program == nil || len(program.bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal fiber inventory has no frozen forest")
	}
	slots, err := freezeSlotSpace(program)
	if err != nil {
		return nil, err
	}
	return freezeFormalFiberInventoryWithSlots(program, slots)
}

func freezeFormalFiberInventoryWithSlots(program *RelationProgram, slots *SlotSpace) (*formalFiberInventory, error) {
	if program == nil || len(program.bodies) == 0 || slots == nil {
		return nil, fmt.Errorf("transformer: formal fiber inventory has no frozen forest")
	}
	if program.formalRegion == nil {
		region, regionErr := freezeFormalRelationRegionInventory(program)
		if regionErr != nil {
			return nil, regionErr
		}
		program.formalRegion = region
	}
	topology, err := freezeFormalDefinitionResourceTopology(program)
	if err != nil {
		return nil, err
	}
	inventory := &formalFiberInventory{
		program: program, slots: slots, spans: make([]formalFiberDescriptorSpan, len(program.bodies)),
		externalCalls:            make(map[formalExternalCallSiteKey]formalPreparedExternalCallSite),
		outcomeSourceIdentities:  make(map[formalRelationCell][]formalOutcomeSourceIdentityPlan),
		pathStoreOwnerIdentities: make(map[formalRelationCell]formalPathStoreOwnerIdentityPlan),
		operatorTopology:         topology,
	}
	if formalApplyTraceConfigured() {
		inventory.applyCoordinateTrace = make(map[formalFrameFootprintKey]formalApplyCoordinateStaticTrace)
	}
	formalKeys := make([]*keyspace.KeySpace, len(program.bodies))
	rekeys := make([]state.CoordinateFormalRootRekey, len(program.bodies))
	coordinates := make([]state.CoordinateFactorInventory, len(program.bodies))
	// Root vocabularies and lexical producer seeds are sealed first. The single
	// dependency worklist below then closes local operator footprints and call
	// images together, independent of lexical body order.
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		body := &program.bodies[bodyIndex]
		keys, rekey, rekeyErr := freezeFormalCoordinateRootRekey(body, slots)
		if rekeyErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal coordinate roots for relation %d: %w", variable, rekeyErr)
		}
		formalKeys[bodyIndex], rekeys[bodyIndex] = keys, rekey
		if prepareErr := freezeFormalExternalCallSites(program, inventory, body, variable, keys, rekey); prepareErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal ExternalCall sites for relation %d: %w", variable, prepareErr)
		}
		local, coordinateErr := freezeFormalBodyCoordinateSeed(body, variable, keys, rekey, inventory.externalCallCoordinateSlots(variable))
		if coordinateErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal coordinates for relation %d: %w", variable, coordinateErr)
		}
		coordinates[bodyIndex] = local
	}
	if err := freezeFormalCoordinateDependencyClosure(program, program.formalRegion, inventory, formalKeys, rekeys, coordinates); err != nil {
		return nil, err
	}
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		body := &program.bodies[bodyIndex]
		keys, rekey, bodyCoordinates := formalKeys[bodyIndex], rekeys[bodyIndex], coordinates[bodyIndex]
		first := len(inventory.descriptors)
		bodyDescriptors, freezeErr := freezeFormalBodyFiberDescriptors(inventory, slots, body, variable, keys, rekey, bodyCoordinates)
		if freezeErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal fibers for relation %d: %w", variable, freezeErr)
		}
		for index := range bodyDescriptors {
			bodyDescriptors[index].forest = inventory
			bodyDescriptors[index].variable = variable
			bodyDescriptors[index].global = first + index
			if index != 0 && bodyDescriptors[index-1].role > bodyDescriptors[index].role {
				return nil, fmt.Errorf("transformer: formal fiber roles are not canonical")
			}
		}
		inventory.descriptors = append(inventory.descriptors, bodyDescriptors...)
		for range bodyDescriptors {
			inventory.groupMembership = append(inventory.groupMembership, formalFiberGroupMembership{group: -1, position: -1})
		}
		groupFirst := len(inventory.groups)
		bodyGroups, groupErr := freezeFormalBodyFiberGroups(inventory, program.registry, body, variable, bodyDescriptors)
		if groupErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal fiber groups for relation %d: %w", variable, groupErr)
		}
		for index := range bodyGroups {
			bodyGroups[index].forest = inventory
			bodyGroups[index].variable = variable
			bodyGroups[index].global = groupFirst + index
		}
		inventory.groups = append(inventory.groups, bodyGroups...)
		for groupIndex := range bodyGroups {
			group := &inventory.groups[groupFirst+groupIndex]
			for position, ordinal := range group.members {
				membership := &inventory.groupMembership[first+int(ordinal)]
				if membership.group >= 0 {
					return nil, fmt.Errorf("transformer: formal fiber ordinal %d has duplicate dense group ownership", ordinal)
				}
				membership.group = group.global
				membership.position = position
			}
		}
		liveValues, liveValuesErr := freezeFormalLiveValueSlots(slots, body, keys, rekey, bodyGroups)
		if liveValuesErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal live Values inverse: %w", liveValuesErr)
		}
		valueBindings := make([]state.ExactValueSlotBinding[FormalSlot, statekey.Value], 0)
		concreteValueSlots := make(map[statekey.Value]FormalSlot)
		for _, group := range bodyGroups {
			if group.kind != formalFiberGroupValues {
				continue
			}
			for _, member := range group.valueSlots {
				root, exact := member.slot.relationRoot()
				if !exact {
					return nil, fmt.Errorf("transformer: formal Values slot has no structural root")
				}
				var concrete statekey.Value
				switch root.Kind {
				case RootMiddle:
					concrete, _ = body.rootValueSlot(root)
				case RootResult:
					concrete = statekey.ReturnSlot(int(root.Index))
				case RootHeapTemplate:
					// Heap templates are owner-local allocation existentials, not
					// concrete State value cells. A non-Bottom value at publication
					// must therefore fail structurally rather than be dropped.
				}
				if concrete != 0 {
					if prior, duplicate := concreteValueSlots[concrete]; duplicate && prior != member.slot {
						return nil, fmt.Errorf("transformer: formal Values publication inverse is non-injective")
					}
					valueBindings = append(valueBindings, state.ExactValueSlotBinding[FormalSlot, statekey.Value]{Source: member.slot, Target: concrete})
					concreteValueSlots[concrete] = member.slot
				}
			}
		}
		valueRekey, valueRekeyErr := state.SealExactValueFactorRekey(body.productDomain, valueBindings)
		if valueRekeyErr != nil {
			return nil, fmt.Errorf("transformer: freeze formal Values publication inverse: %w", valueRekeyErr)
		}
		inventory.spans[bodyIndex] = formalFiberDescriptorSpan{
			forest: inventory, variable: variable, first: first, count: len(bodyDescriptors), keys: keys, rekey: rekey,
			valueRekey: valueRekey, liveValues: liveValues, coordinates: bodyCoordinates, groupFirst: groupFirst, groupCount: len(bodyGroups),
		}
		if err := validateFormalFiberProductGroups(inventory.spans[bodyIndex], body.productDomain); err != nil {
			return nil, err
		}
	}
	return inventory, nil
}

// freezeFormalLiveValueSlots seals the one body-owned inverse from every
// concrete or formal Values dependency spelling to its evolving tuple slot.
// Structural Input roots remain durable path identities, while resolver and
// point-local reads share the corresponding Middle register.
func freezeFormalLiveValueSlots(
	slots *SlotSpace,
	body *relationProgramBody,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	groups []formalFiberGroupDescriptor,
) (map[statekey.ValueDependency]FormalSlot, error) {
	if slots == nil || body == nil || body.relation.arena == nil || body.keys == nil || !body.keys.Valid() ||
		formalKeys == nil || !formalKeys.Valid() || !body.productDomain.OwnsCoordinateFormalRootRekey(rekey) {
		return nil, fmt.Errorf("live Values inverse is unowned")
	}
	bound := make(map[statekey.ValueDependency]FormalSlot)
	bind := func(dependency statekey.ValueDependency, slot FormalSlot) error {
		if !dependency.Valid() || !slot.Valid() || slot.Body() != body.body {
			return fmt.Errorf("live Values inverse contains an invalid binding")
		}
		if prior, duplicate := bound[dependency]; duplicate && prior != slot {
			return fmt.Errorf("one Values dependency names two live slots")
		}
		bound[dependency] = slot
		return nil
	}
	for _, group := range groups {
		if group.kind != formalFiberGroupValues {
			continue
		}
		for _, member := range group.valueSlots {
			root, exact := member.slot.Root()
			if !exact {
				return nil, fmt.Errorf("live Values slot has no formal root")
			}
			relationRoot, exact := member.slot.relationRoot()
			if !exact {
				return nil, fmt.Errorf("live Values slot has no relation root")
			}
			// Input slots are immutable boundary syntax, not point-local Values.
			// Their structural dependency is rebound to Middle below.
			if relationRoot.Kind < RootParam || relationRoot.Kind > RootAmbient {
				if err := bind(statekey.FormalDependency(root), member.slot); err != nil {
					return nil, err
				}
			}
			var concrete statekey.Value
			switch relationRoot.Kind {
			case RootMiddle:
				concrete, _ = body.rootValueSlot(relationRoot)
			case RootResult:
				concrete = statekey.ReturnSlot(int(relationRoot.Index))
			}
			if concrete != 0 {
				if err := bind(statekey.ConcreteDependency(concrete), member.slot); err != nil {
					return nil, err
				}
			}
		}
	}
	for index, register := range body.relation.arena.middle.registers {
		if register.kind != relationMiddleRegisterSymbol {
			continue
		}
		lexical, present := body.keys.LookupResolverKey(register.symbol, 0, nil)
		if !present {
			return nil, fmt.Errorf("Middle symbol %d has no lexical structural root", index)
		}
		formalKey, err := body.productDomain.RekeyStructuralKeyFormal(rekey, lexical)
		if err != nil {
			return nil, fmt.Errorf("Middle symbol %d structural root: %w", index, err)
		}
		root, exact := formalKeys.DescribeFormalRoot(formalKey)
		middle, middleExact := slots.Slot(body.body, Root{Kind: RootMiddle, Index: uint32(index)})
		if !exact || !middleExact {
			return nil, fmt.Errorf("Middle symbol %d has no exact structural Values binding", index)
		}
		if err := bind(statekey.FormalDependency(root), middle); err != nil {
			return nil, err
		}
	}
	return bound, nil
}

func freezeFormalBodyFiberDescriptors(inventory *formalFiberInventory, slots *SlotSpace, body *relationProgramBody, variable relationVar, formalKeys *keyspace.KeySpace, rekey state.CoordinateFormalRootRekey, coordinates state.CoordinateFactorInventory) ([]formalFiberDescriptor, error) {
	if inventory == nil || slots == nil || body == nil || body.variable != variable ||
		body.body == ([32]byte{}) || body.relation.code == nil || !body.relation.code.sealed ||
		body.relation.arena == nil || !body.relation.arena.middle.sealed ||
		body.keys == nil || !body.keys.Valid() || !body.productDomain.Valid() {
		return nil, fmt.Errorf("formal body fiber schema is unsealed")
	}
	code := body.relation.code
	out := []formalFiberDescriptor{{role: formalFiberCare}}

	// Symbolic bindings are terms, not ground abstract values. Middle roots
	// receive stable slot descriptors in vocabulary order. Results are Output
	// roots in the same Values carrier: canonical N5 writes them before the
	// terminal occurrence is published, and Apply reads that stabilized tuple.
	for ordinal := uint32(0); uint64(ordinal) < body.relation.arena.middle.count(); ordinal++ {
		slot, ok := slots.Slot(body.body, Root{Kind: RootMiddle, Index: ordinal})
		if !ok {
			return nil, fmt.Errorf("Middle value slot %d is outside frozen schema", ordinal)
		}
		out = append(out, formalFiberDescriptor{role: formalFiberMiddleValue, slot: slot})
	}
	for ordinal, register := range body.relation.arena.middle.registers {
		if register.kind != relationMiddleRegisterSymbol {
			continue
		}
		slot, ok := slots.Slot(body.body, Root{Kind: RootMiddle, Index: uint32(ordinal)})
		if !ok {
			return nil, fmt.Errorf("Middle path slot %d is outside frozen schema", ordinal)
		}
		out = append(out, formalFiberDescriptor{role: formalFiberMiddlePath, slot: slot})
	}
	for outcome := boundaryOutcomeRef(1); int(outcome) < len(code.outcomes); outcome++ {
		out = append(out, formalFiberDescriptor{role: formalFiberOutcome, outcome: outcome})
	}
	callPoints := make(map[cfg.Point]struct{})
	for _, node := range code.nodes {
		for _, step := range node.steps {
			// Internal Apply outcomes are detached from input-certified
			// observation witnesses after WTO and therefore consume no semantic
			// tuple coordinate. External providers still publish their outcome as
			// part of the atomic provider transaction.
			if step.kind == boundaryStepExternalCall && step.point > 0 {
				callPoints[step.point] = struct{}{}
			}
		}
	}
	orderedCallPoints := make([]cfg.Point, 0, len(callPoints))
	for point := range callPoints {
		orderedCallPoints = append(orderedCallPoints, point)
	}
	sort.Slice(orderedCallPoints, func(i, j int) bool { return orderedCallPoints[i] < orderedCallPoints[j] })
	for _, point := range orderedCallPoints {
		out = append(out, formalFiberDescriptor{role: formalFiberCallOutcome, point: point})
	}
	// Reachability remains the Care fiber. Every reachable lexical tuple owns
	// exactly one diagnostic value, whose physical-zero interpretation is the
	// certified sequencing identity rather than the unknown zero value.
	out = append(out, formalFiberDescriptor{role: formalFiberDiagnostics})

	if !coordinates.ValidFor(body.productDomain, formalKeys) {
		return nil, fmt.Errorf("formal body coordinate inventory is unowned")
	}
	nonValues := body.productDomain.NonValuesLaneInventory()
	for _, lane := range nonValues {
		families, familyErr := body.productDomain.CoordinateFamilies(lane)
		if familyErr != nil {
			return nil, familyErr
		}
		if len(families) == 0 {
			out = append(out, formalFiberDescriptor{role: formalFiberOrdinaryLane, lane: lane})
		}
	}
	for _, lane := range nonValues {
		families, familyErr := body.productDomain.CoordinateFamilies(lane)
		if familyErr != nil {
			return nil, familyErr
		}
		for _, family := range families {
			familySlots, slotsErr := coordinates.FamilySlots(family)
			if slotsErr != nil {
				return nil, slotsErr
			}
			out = append(out, formalFiberDescriptor{role: formalFiberCoordinate, lane: lane, family: family, coordinateKind: formalFiberCoordinateFamilySkeleton})
			for _, coordinate := range familySlots {
				out = append(out, formalFiberDescriptor{role: formalFiberCoordinate, lane: lane, family: family, coordinate: coordinate, coordinateKind: formalFiberCoordinateFamilyScalar})
			}
		}
	}

	if _, valuesEnabled := body.productDomain.SlotFactoredCarrier(); valuesEnabled {
		// Values is a slot-factored registered lane. Its finite formal topology
		// is every admitted IN/MID/OUT root in canonical vocabulary/root order; the
		// symbolic binding leaves above remain distinct relation syntax.
		out = append(out, formalFiberDescriptor{role: formalFiberGroundValueTop})
		for _, kind := range []RootKind{RootParam, RootCapture, RootGlobal, RootAmbient, RootMiddle, RootHeapTemplate, RootResult} {
			for ordinal := uint32(0); ordinal < formalBodyRootCount(body, kind); ordinal++ {
				slot, ok := slots.Slot(body.body, Root{Kind: kind, Index: ordinal})
				if !ok {
					return nil, fmt.Errorf("ground value slot %d/%d is outside frozen schema", kind, ordinal)
				}
				out = append(out, formalFiberDescriptor{role: formalFiberGroundValue, slot: slot})
			}
		}
	}
	return out, nil
}

func freezeFormalBodyFiberGroups(inventory *formalFiberInventory, registry *axis.Registry, body *relationProgramBody, variable relationVar, descriptors []formalFiberDescriptor) ([]formalFiberGroupDescriptor, error) {
	if registry == nil || inventory == nil || body == nil || body.variable != variable || !body.productDomain.Valid() {
		return nil, fmt.Errorf("formal fiber group schema is unowned")
	}
	owners := make([]int, len(descriptors))
	for index := range owners {
		owners[index] = -1
	}
	out := make([]formalFiberGroupDescriptor, 0)
	claim := func(group int, ordinals []formalFiberOrdinal) error {
		seen := make(map[formalFiberOrdinal]struct{}, len(ordinals))
		for _, ordinal := range ordinals {
			if int(ordinal) < 0 || int(ordinal) >= len(descriptors) {
				return fmt.Errorf("formal fiber group contains an out-of-range ordinal")
			}
			if _, duplicate := seen[ordinal]; duplicate || owners[ordinal] >= 0 {
				return fmt.Errorf("formal fiber ordinal %d belongs to multiple groups", ordinal)
			}
			seen[ordinal] = struct{}{}
			owners[ordinal] = group
		}
		return nil
	}

	lanes := body.productDomain.LaneInventory()
	valuesLane, valuesEnabled := body.productDomain.SlotFactoredCarrier()
	for _, lane := range lanes {
		if valuesEnabled && lane == valuesLane {
			group := formalFiberGroupDescriptor{
				kind: formalFiberGroupValues, lane: lane, valueTop: -1, valueTopPosition: -1,
				valueSlotPosition: make(map[FormalSlot]int), valueDomain: state.ValueFactorLattice[FormalSlot](registry),
			}
			for ordinal, descriptor := range descriptors {
				switch descriptor.role {
				case formalFiberGroundValueTop:
					if group.valueTop >= 0 {
						return nil, fmt.Errorf("Values group has multiple Top fibers")
					}
					group.valueTop = formalFiberOrdinal(ordinal)
					group.valueTopPosition = len(group.members)
					group.members = append(group.members, formalFiberOrdinal(ordinal))
				case formalFiberGroundValue:
					if !descriptor.slot.Valid() {
						return nil, fmt.Errorf("Values group contains an invalid FormalSlot")
					}
					if _, duplicate := group.valueSlotPosition[descriptor.slot]; duplicate {
						return nil, fmt.Errorf("Values group contains a duplicate FormalSlot")
					}
					group.valueSlotPosition[descriptor.slot] = len(group.valueSlots)
					group.valueSlots = append(group.valueSlots, formalValueSlotFiber{slot: descriptor.slot, ordinal: formalFiberOrdinal(ordinal), position: len(group.members)})
					group.members = append(group.members, formalFiberOrdinal(ordinal))
				}
			}
			if group.valueTop < 0 {
				return nil, fmt.Errorf("Values group has no Top fiber")
			}
			if err := claim(len(out), group.members); err != nil {
				return nil, err
			}
			out = append(out, group)
			continue
		}
		families, err := body.productDomain.CoordinateFamilies(lane)
		if err != nil {
			return nil, err
		}
		if len(families) == 0 {
			group, err := freezeFormalOrdinaryLaneFiberGroup(lane, descriptors)
			if err != nil {
				return nil, err
			}
			if err := claim(len(out), group.members); err != nil {
				return nil, err
			}
			out = append(out, group)
			continue
		}
		group, err := freezeFormalCoordinateLaneFiberGroup(lane, families, descriptors)
		if err != nil {
			return nil, err
		}
		if err := claim(len(out), group.members); err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	if len(out) != len(lanes) {
		return nil, fmt.Errorf("formal product group count %d differs from registered lane count %d", len(out), len(lanes))
	}

	for ordinal, descriptor := range descriptors {
		shouldBeGrouped := descriptor.role == formalFiberOrdinaryLane || descriptor.role == formalFiberCoordinate || descriptor.role == formalFiberGroundValueTop || descriptor.role == formalFiberGroundValue
		if shouldBeGrouped != (owners[ordinal] >= 0) {
			return nil, fmt.Errorf("formal fiber ordinal %d has incomplete group ownership", ordinal)
		}
	}
	return out, nil
}

func freezeFormalOrdinaryLaneFiberGroup(lane state.ProductLane, descriptors []formalFiberDescriptor) (formalFiberGroupDescriptor, error) {
	group := formalFiberGroupDescriptor{kind: formalFiberGroupOrdinaryLane, lane: lane}
	for ordinal, descriptor := range descriptors {
		if descriptor.role != formalFiberOrdinaryLane || descriptor.lane != lane {
			continue
		}
		if len(group.members) != 0 {
			return formalFiberGroupDescriptor{}, fmt.Errorf("ordinary lane %q has multiple fibers", lane.ID())
		}
		group.members = append(group.members, formalFiberOrdinal(ordinal))
	}
	if len(group.members) != 1 {
		return formalFiberGroupDescriptor{}, fmt.Errorf("ordinary lane %q has no exact one-member fiber", lane.ID())
	}
	return group, nil
}

func freezeFormalCoordinateLaneFiberGroup(lane state.ProductLane, families []state.CoordinateFamily, descriptors []formalFiberDescriptor) (formalFiberGroupDescriptor, error) {
	group := formalFiberGroupDescriptor{kind: formalFiberGroupCoordinateLane, lane: lane}
	for _, family := range families {
		familyGroup := formalCoordinateFamilyFiberGroup{family: family, skeleton: -1}
		for ordinal, descriptor := range descriptors {
			if descriptor.role != formalFiberCoordinate || descriptor.lane != lane || descriptor.family != family {
				continue
			}
			switch descriptor.coordinateKind {
			case formalFiberCoordinateFamilySkeleton:
				if familyGroup.skeleton >= 0 {
					return formalFiberGroupDescriptor{}, fmt.Errorf("coordinate family %q has multiple skeleton fibers", family.ID())
				}
				familyGroup.skeleton = formalFiberOrdinal(ordinal)
			case formalFiberCoordinateFamilyScalar:
				familyGroup.scalars = append(familyGroup.scalars, formalFiberOrdinal(ordinal))
			default:
				return formalFiberGroupDescriptor{}, fmt.Errorf("coordinate family %q has invalid fiber kind", family.ID())
			}
		}
		if familyGroup.skeleton < 0 {
			return formalFiberGroupDescriptor{}, fmt.Errorf("registered coordinate family %q has no skeleton fiber", family.ID())
		}
		familyGroup.skeletonPosition = len(group.members)
		for range familyGroup.scalars {
			familyGroup.scalarPositions = append(familyGroup.scalarPositions, len(group.members)+1+len(familyGroup.scalarPositions))
		}
		group.coordinateFamilies = append(group.coordinateFamilies, familyGroup)
		group.members = append(group.members, familyGroup.skeleton)
		group.members = append(group.members, familyGroup.scalars...)
	}
	return group, nil
}

func formalBodyRootCount(body *relationProgramBody, kind RootKind) uint32 {
	if body == nil {
		return 0
	}
	switch kind {
	case RootMiddle:
		return uint32(body.relation.arena.middle.count())
	default:
		return body.relation.code.shape.count(kind)
	}
}

// freezeFormalBodyCoordinateInventory forms one body-wide set from static
// producer inventories.  Point-relative reachability remains useful to branch
// execution, but the persistent tuple schema must be able to represent every
// coordinate any frozen point can publish.  No DynamicReadQueryPlan is read.
func freezeFormalBodyCoordinateSeed(body *relationProgramBody, variable relationVar, formalKeys *keyspace.KeySpace, rekey state.CoordinateFormalRootRekey, extra []state.CoordinateSlot) (state.CoordinateFactorInventory, error) {
	if body == nil || body.variable != variable || body.keys == nil || !body.keys.Valid() || !body.productDomain.Valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal coordinate inventory is unowned")
	}
	empty, err := body.productDomain.SealCoordinateFactorInventory(body.keys, nil)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	var concrete state.CoordinateFactorInventory
	var pointwise relationCoordinateFactorInventory
	if body.pathSemantics != nil && body.pathSemantics.Valid() {
		var freezeErr error
		pointwise, freezeErr = freezeRelationCoordinateFactorInventory(body)
		if freezeErr != nil {
			return state.CoordinateFactorInventory{}, freezeErr
		}
		inputs := make([]state.CoordinateFactorInventory, 1, len(pointwise.producers)+1)
		inputs[0] = pointwise.empty
		for _, producer := range pointwise.producers {
			inputs = append(inputs, producer.inventory)
		}
		seed, unionErr := body.pathSemantics.UnionCoordinateFactorInventories(body.productDomain, inputs...)
		if unionErr != nil {
			return state.CoordinateFactorInventory{}, unionErr
		}
		concrete, err = body.pathSemantics.CloseCoordinateFactorInventory(body.productDomain, seed)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
	} else {
		inputs := []state.CoordinateFactorInventory{empty}
		for index := 0; index < body.initialStatePlan.Len(); index++ {
			_, prepared, present := body.initialStatePlan.Seed(index)
			if !present {
				return state.CoordinateFactorInventory{}, fmt.Errorf("initial-state coordinate seed %d is absent", index)
			}
			initial, initialErr := body.productDomain.CoordinateFactorInventoryFromPreparedState(body.keys, prepared)
			if initialErr != nil {
				return state.CoordinateFactorInventory{}, initialErr
			}
			inputs = append(inputs, initial)
		}
		seed, unionErr := body.productDomain.UnionCoordinateFactorInventories(body.keys, inputs...)
		if unionErr != nil {
			return state.CoordinateFactorInventory{}, unionErr
		}
		concrete, err = body.productDomain.CloseCoordinateFactorInventory(body.keys, seed)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
	}
	if formalKeys == nil || !formalKeys.Valid() {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal coordinate inventory has no keyspace")
	}
	formalInventory, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	mapped := make([]state.CoordinateSlot, 0, formalInventory.Len()+len(extra))
	mapped = append(mapped, formalInventory.Slots()...)
	mapped = append(mapped, extra...)
	seed, err := body.productDomain.SealCoordinateFactorInventory(formalKeys, mapped)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	formalInventory, err = body.productDomain.CloseCoordinateFactorInventory(formalKeys, seed)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}

	return formalInventory, nil
}

func freezeFormalCoordinateRootRekey(body *relationProgramBody, slots *SlotSpace) (*keyspace.KeySpace, state.CoordinateFormalRootRekey, error) {
	if body == nil || slots == nil || body.keys == nil || !body.keys.Valid() || body.relation.arena == nil {
		return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("formal coordinate root schema is unowned")
	}
	type sourceMode struct {
		root     keyspace.Key
		resolver bool
	}
	bindings := make([]state.CoordinateFormalRootBinding, 0, len(body.roots.roots)+2*len(body.relation.arena.middle.registers))
	seen := make(map[sourceMode]formal.Root, cap(bindings))
	structural := make(map[keyspace.Key]struct{}, len(body.roots.roots))
	addBinding := func(binding state.CoordinateFormalRootBinding) error {
		concrete, ok := body.keys.StructuralRoot(binding.Source)
		if !ok {
			return fmt.Errorf("formal coordinate binding has no concrete root")
		}
		mode := sourceMode{root: concrete, resolver: binding.ResolverVersions}
		if prior, exists := seen[mode]; exists {
			if prior != binding.Target {
				return fmt.Errorf("one concrete root mode names two formal roots")
			}
			return nil
		}
		seen[mode] = binding.Target
		bindings = append(bindings, binding)
		if !binding.ResolverVersions {
			structural[concrete] = struct{}{}
		}
		return nil
	}
	var maxInputOrdinal uint64
	for _, carrier := range body.roots.roots {
		slot, ok := slots.Slot(body.body, carrier.root)
		root, rootOK := slot.Root()
		if !ok || !rootOK || carrier.path.Kind == keyspace.KindInvalid {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("input root has no exact formal path binding")
		}
		if root.Vocabulary() != formal.Input {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("input root has the wrong formal vocabulary")
		}
		if root.Ordinal() > maxInputOrdinal {
			maxInputOrdinal = root.Ordinal()
		}
		if err := addBinding(state.CoordinateFormalRootBinding{Source: carrier.path, Target: root}); err != nil {
			return nil, state.CoordinateFormalRootRekey{}, err
		}
	}
	for index, register := range body.relation.arena.middle.registers {
		root, ok := body.relation.arena.middle.formalRoot(body.body, Root{Kind: RootMiddle, Index: uint32(index)})
		if !ok {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("Middle register %d has no formal root", index)
		}
		var source keyspace.Key
		switch register.kind {
		case relationMiddleRegisterSymbol:
			source = body.keys.FromPath(pathdom.NewPath(register.symbol, ""))
			concrete, exact := body.keys.StructuralRoot(source)
			if !exact {
				return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("Middle symbol register %d has no structural root", index)
			}
			if _, present := structural[concrete]; !present {
				if register.formalOrdinal == 0 || maxInputOrdinal > math.MaxUint64-register.formalOrdinal {
					return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("Middle symbol register %d has no durable structural ordinal", index)
				}
				input := formal.NewRoot(body.body, maxInputOrdinal+register.formalOrdinal, formal.Input)
				if err := addBinding(state.CoordinateFormalRootBinding{Source: source, Target: input}); err != nil {
					return nil, state.CoordinateFormalRootRekey{}, err
				}
			}
			if err := addBinding(state.CoordinateFormalRootBinding{Source: source, Target: root, ResolverVersions: true}); err != nil {
				return nil, state.CoordinateFormalRootRekey{}, err
			}
			continue
		case relationMiddleRegisterCallResult:
			_, source, _ = frameCallResultCarrier(body.keys, body.body, register.point, register.ordinal)
		case relationMiddleRegisterExpression:
			continue
		default:
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("Middle register %d has invalid root kind", index)
		}
		if source.Kind == keyspace.KindInvalid {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("Middle register %d has no concrete structural root", index)
		}
		if err := addBinding(state.CoordinateFormalRootBinding{Source: source, Target: root}); err != nil {
			return nil, state.CoordinateFormalRootRekey{}, err
		}
	}
	// Final result roots are a distinct Output vocabulary. They must be in the
	// same closed rekey plan as Values so every coordinate family observes the
	// N5 write under exactly the ret[n] spelling used by concrete semantics.
	for ordinal := uint32(0); ordinal < body.relation.Shape().Results; ordinal++ {
		slot, ok := slots.Slot(body.body, Root{Kind: RootResult, Index: ordinal})
		root, rootOK := slot.Root()
		if !ok || !rootOK {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("result root %d has no exact formal Output binding", ordinal)
		}
		source := body.keys.FromPath(pathdom.Path{Root: fmt.Sprintf("ret[%d]", ordinal)})
		if source.Kind == keyspace.KindInvalid {
			return nil, state.CoordinateFormalRootRekey{}, fmt.Errorf("result root %d has no concrete ret slot", ordinal)
		}
		if err := addBinding(state.CoordinateFormalRootBinding{Source: source, Target: root}); err != nil {
			return nil, state.CoordinateFormalRootRekey{}, err
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Source != bindings[j].Source {
			return body.keys.Less(bindings[i].Source, bindings[j].Source)
		}
		if bindings[i].ResolverVersions != bindings[j].ResolverVersions {
			return !bindings[i].ResolverVersions
		}
		return bindings[i].Target.Less(bindings[j].Target)
	})
	formalKeys := keyspace.New()
	plan, err := body.productDomain.SealCoordinateFormalRootRekey(body.body, body.keys, formalKeys, bindings)
	if err != nil {
		return nil, state.CoordinateFormalRootRekey{}, err
	}
	return formalKeys, plan, nil
}
