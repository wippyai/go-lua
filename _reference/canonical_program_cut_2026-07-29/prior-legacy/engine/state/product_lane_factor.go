package state

import (
	"errors"
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

var (
	// ErrInvalidProductLane reports a lane descriptor that does not belong to
	// the ProductDomain receiving the operation.
	ErrInvalidProductLane = errors.New("state: invalid product lane")
	// ErrInvalidLaneFactor reports an opaque component that does not belong to
	// the ProductDomain or lane receiving the operation.
	ErrInvalidLaneFactor = errors.New("state: invalid lane factor")
	// ErrIncompleteLaneFactors reports a Compose input that is not the exact,
	// registry-ordered factor inventory produced by Decompose.
	ErrIncompleteLaneFactors = errors.New("state: incomplete lane factor inventory")
)

// LaneOrdinal is a ProductDomain-local position in registry order. It is not
// a bit index and imposes no cardinality cap on the factor inventory.
type LaneOrdinal int

// ProductLane is an opaque, domain-owned descriptor for one enabled State
// lane. Descriptors are obtained from ProductDomain.LaneInventory.
type ProductLane struct {
	seal         *productDomainSeal
	ordinal      LaneOrdinal
	id           LaneID
	slotFactored bool
}

// ID returns the registered semantic lane name.
func (l ProductLane) ID() LaneID { return l.id }

// Ordinal returns the lane's stable position in this ProductDomain.
func (l ProductLane) Ordinal() LaneOrdinal { return l.ordinal }

// LaneFactor is one opaque component of the State product lattice. Its
// concrete representation remains owned by the lane registration; consumers
// cannot inspect, cast, or mutate it.
type LaneFactor struct {
	lane    ProductLane
	payload laneFactorPayload
}

// Lane returns the domain-owned descriptor for this component.
func (f LaneFactor) Lane() ProductLane { return f.lane }

// ProductFactorSelection is the singular exact physical vocabulary for one
// product transaction. Ordinary lanes are indivisible; coordinate-backed
// lanes are selected by an exact inventory; Values retain finite slots and an
// independent Top bit.
type ProductFactorSelection struct {
	seal               *productDomainSeal
	authority          *productFactorSelectionSeal
	ordinary           []ProductLane
	selected           []bool
	coordinates        CoordinateFactorInventory
	coordinateLanes    []ProductLane
	coordinateSelected []bool
	coordinateFactors  []productCoordinateFactorSelection
	coordinateGroups   []productCoordinateSelectionGroup
	values             []statekey.Value
	valuesTop          bool
}

type productFactorSelectionSeal struct{}

type productCoordinateSelectionGroup struct {
	lane         ProductLane
	first, count int
}

type productCoordinateFactorSelection struct {
	family       CoordinateFamily
	slots        []CoordinateSlot
	overlay      CoordinateSkeletonOverlayPlan
	skeletonOnly bool
}

type productLaneRuntime struct {
	lane               ProductLane
	ops                laneFactorOps
	fingerprint        func(*fingerprintWriter, State)
	valueDependencies  laneValueDependencyPolicy
	identitySupport    laneIdentitySupportPolicy
	numericConsistency laneNumericConsistencyPolicy
	rootAssignment     rootAssignmentLanePolicy
	dynamicRead        laneDynamicReadPolicy
	semanticLaws       []laneSemanticLaw
	formalRekey        laneFormalRekeyPolicy
	coordinates        []coordinateFamilyRuntime
}

// PatchLaneFactors returns base with exactly replacements installed. The
// replacement inventory is sparse, ownership-checked, and duplicate-free;
// no omitted lane is extracted or recomposed. This is the concrete handoff
// used by factor-native semantic programs after their transaction succeeds.
func (d ProductDomain) PatchLaneFactors(base State, replacements []LaneFactor) (State, error) {
	if !d.Valid() {
		return State{}, fmt.Errorf("%w: invalid product domain", ErrInvalidLaneFactor)
	}
	runtimes := make([]*productLaneRuntime, len(replacements))
	seen := make([]bool, len(d.factorLanes))
	for index, factor := range replacements {
		runtime, err := d.validateFactor(factor)
		if err != nil {
			return State{}, err
		}
		ordinal := int(runtime.lane.ordinal)
		if seen[ordinal] {
			return State{}, fmt.Errorf("%w: duplicate replacement factor", ErrIncompleteLaneFactors)
		}
		seen[ordinal] = true
		runtimes[index] = runtime
	}
	out := d.Normalize(base)
	for index, runtime := range runtimes {
		runtime.ops.install(&out, replacements[index].payload)
	}
	out.canonical = true
	return d.Normalize(out), nil
}

// LaneInventory returns every enabled lane in catalog registration order.
// The returned slice is caller-owned; its descriptors remain sealed to d.
func (d ProductDomain) LaneInventory() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, len(d.factorLanes))
	for i := range d.factorLanes {
		out[i] = d.factorLanes[i].lane
	}
	return out
}

// PathResolutionLanes returns the exact enabled lane inventory whose facts
// participate in member-path resolution. Participation is declared by each
// lane registration, so adding or removing a product axis cannot silently
// grow a second operation-owned LaneID table.
func (d ProductDomain) PathResolutionLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathResolutionParticipant)
		if declared && law.participates {
			out = append(out, runtime.lane)
		}
	}
	return out
}

// NonValuesLaneInventory returns the whole-lane carrier inventory used by the
// transposed evaluator. Values is intentionally absent because it is factored
// by exact slots through ValueLaneFactor rather than transported as one axis.
func (d ProductDomain) NonValuesLaneInventory() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, len(d.factorLanes))
	for index := range d.factorLanes {
		if !d.factorLanes[index].lane.slotFactored {
			out = append(out, d.factorLanes[index].lane)
		}
	}
	return out
}

// NonValuesLaneCount returns the dense non-Values carrier width without
// materializing an inventory slice. Hot factor transactions pair it with
// NonValuesLaneAt to validate caller-owned tuples directly against the frozen
// catalog.
func (d ProductDomain) NonValuesLaneCount() int {
	if !d.Valid() {
		return 0
	}
	count := len(d.factorLanes)
	if d.hasSlotFactor {
		count--
	}
	return count
}

// NonValuesLaneAt returns one dense non-Values descriptor in registry order.
// The bool is false for an invalid domain or out-of-range dense ordinal.
func (d ProductDomain) NonValuesLaneAt(index int) (ProductLane, bool) {
	if !d.Valid() || index < 0 || index >= d.NonValuesLaneCount() {
		return ProductLane{}, false
	}
	factorIndex := index
	if d.hasSlotFactor && factorIndex >= int(d.slotFactor) {
		factorIndex++
	}
	if factorIndex < 0 || factorIndex >= len(d.factorLanes) || d.factorLanes[factorIndex].lane.slotFactored {
		return ProductLane{}, false
	}
	return d.factorLanes[factorIndex].lane, true
}

// SlotFactoredCarrier returns the unique enabled lane whose coordinates are
// transported through ValueLaneFactor rather than as one opaque LaneFactor.
func (d ProductDomain) SlotFactoredCarrier() (ProductLane, bool) {
	if !d.Valid() || !d.hasSlotFactor || int(d.slotFactor) < 0 || int(d.slotFactor) >= len(d.factorLanes) {
		return ProductLane{}, false
	}
	return d.factorLanes[d.slotFactor].lane, true
}

// BoundaryClosureCompanion returns the optional unique enabled lane whose
// independently projected factor extends the shared boundary closure.
func (d ProductDomain) BoundaryClosureCompanion() (ProductLane, bool) {
	if !d.Valid() || !d.hasBoundaryClosureCompanion || int(d.boundaryClosureCompanion) < 0 || int(d.boundaryClosureCompanion) >= len(d.factorLanes) {
		return ProductLane{}, false
	}
	return d.factorLanes[d.boundaryClosureCompanion].lane, true
}

// ProductLane returns the enabled descriptor named id.
func (d ProductDomain) ProductLane(id LaneID) (ProductLane, bool) {
	if !d.Valid() {
		return ProductLane{}, false
	}
	for i := range d.factorLanes {
		if d.factorLanes[i].lane.id == id {
			return d.factorLanes[i].lane, true
		}
	}
	return ProductLane{}, false
}

// SealProductFactorSelection validates and freezes the exact physical factors
// one semantic component may read or write. Ordinary lanes are selected as a
// whole. Dependent coordinate lanes are selected only through inventory, and
// Values is selected by finite slot plus its independent lifted Top bit.
//
// A physical lane cannot be selected both ordinarily and by coordinate. That
// prohibition makes component composition unambiguous: structural carry of a
// coordinate lane is owned by ProductDomain, never reconstructed by a caller.
func (d ProductDomain) SealProductFactorSelection(
	lanes []ProductLane,
	coordinates CoordinateFactorInventory,
	values []statekey.Value,
	valuesTop bool,
	skeletonFamilies ...CoordinateFamily,
) (ProductFactorSelection, error) {
	keys := coordinates.KeySpace()
	if !d.Valid() || keys == nil || !keys.Valid() || !coordinates.ValidFor(d, keys) {
		return ProductFactorSelection{}, fmt.Errorf("%w: invalid product factor selection authority", ErrInvalidProductLane)
	}
	closedCoordinates, err := d.CloseCoordinateFactorInventory(keys, coordinates)
	if err != nil {
		return ProductFactorSelection{}, err
	}
	if closedCoordinates.set != coordinates.set {
		return ProductFactorSelection{}, fmt.Errorf("%w: coordinate factor selection is not dependency-closed", ErrInvalidProductLane)
	}
	selection := ProductFactorSelection{
		seal:               d.seal,
		authority:          new(productFactorSelectionSeal),
		selected:           make([]bool, len(d.factorLanes)),
		coordinates:        coordinates,
		coordinateSelected: make([]bool, len(d.factorLanes)),
		values:             append([]statekey.Value(nil), values...),
		valuesTop:          valuesTop,
	}
	for index, lane := range lanes {
		runtime, err := d.validateLane(lane)
		if err != nil || runtime.lane.slotFactored || len(runtime.coordinates) != 0 {
			return ProductFactorSelection{}, fmt.Errorf("%w: ordinary factor %d is foreign or coordinate-backed", ErrInvalidProductLane, index)
		}
		ordinal := int(runtime.lane.ordinal)
		if selection.selected[ordinal] {
			return ProductFactorSelection{}, fmt.Errorf("%w: duplicate ordinary factor %q", ErrInvalidProductLane, runtime.lane.id)
		}
		selection.selected[ordinal] = true
	}
	coordinateFactors := make([]productCoordinateFactorSelection, 0, len(coordinates.set.families)+len(skeletonFamilies))
	seenFamilies := make(map[CoordinateFamily]struct{}, len(coordinates.set.families)+len(skeletonFamilies))
	for _, bucket := range coordinates.set.families {
		overlay, err := d.SealCoordinateSkeletonOverlayPlan(bucket.slots)
		if err != nil {
			return ProductFactorSelection{}, err
		}
		coordinateFactors = append(coordinateFactors, productCoordinateFactorSelection{
			family: bucket.family, slots: bucket.slots, overlay: overlay,
		})
		seenFamilies[bucket.family] = struct{}{}
	}
	for index, family := range skeletonFamilies {
		coordinate, err := d.validateCoordinateFamily(family)
		if err != nil {
			return ProductFactorSelection{}, fmt.Errorf("%w: skeleton-only coordinate family %d", ErrInvalidProductLane, index)
		}
		if _, duplicate := seenFamilies[coordinate.family]; duplicate {
			return ProductFactorSelection{}, fmt.Errorf("%w: duplicate coordinate family %q", ErrInvalidProductLane, coordinate.family.id)
		}
		overlay, err := d.sealCoordinateSkeletonOverlayPlan(coordinate.family, keys, nil, true)
		if err != nil {
			return ProductFactorSelection{}, err
		}
		coordinateFactors = append(coordinateFactors, productCoordinateFactorSelection{
			family: coordinate.family, overlay: overlay, skeletonOnly: true,
		})
		seenFamilies[coordinate.family] = struct{}{}
	}
	sort.Slice(coordinateFactors, func(left, right int) bool {
		return coordinateFamilyLess(coordinateFactors[left].family, coordinateFactors[right].family)
	})
	selection.coordinateFactors = coordinateFactors
	for factorIndex, factor := range selection.coordinateFactors {
		ordinal := int(factor.family.lane.ordinal)
		if ordinal < 0 || ordinal >= len(d.factorLanes) || selection.selected[ordinal] {
			return ProductFactorSelection{}, fmt.Errorf("%w: coordinate factor overlaps ordinary factor", ErrInvalidProductLane)
		}
		if !selection.coordinateSelected[ordinal] {
			selection.coordinateSelected[ordinal] = true
		}
		if len(selection.coordinateGroups) == 0 || selection.coordinateGroups[len(selection.coordinateGroups)-1].lane != factor.family.lane {
			selection.coordinateGroups = append(selection.coordinateGroups, productCoordinateSelectionGroup{
				lane: factor.family.lane, first: factorIndex, count: 1,
			})
		} else {
			selection.coordinateGroups[len(selection.coordinateGroups)-1].count++
		}
	}
	for ordinal, selected := range selection.coordinateSelected {
		if selected {
			selection.coordinateLanes = append(selection.coordinateLanes, d.factorLanes[ordinal].lane)
		}
	}
	for ordinal, selected := range selection.selected {
		if selected {
			selection.ordinary = append(selection.ordinary, d.factorLanes[ordinal].lane)
		}
	}
	sort.Slice(selection.values, func(left, right int) bool {
		return selection.values[left] < selection.values[right]
	})
	for index, slot := range selection.values {
		if slot == 0 || index > 0 && selection.values[index-1] == slot {
			return ProductFactorSelection{}, fmt.Errorf("%w: zero or duplicate Values factor", ErrInvalidProductLane)
		}
	}
	return selection, nil
}

// OrdinaryLanes returns the whole-lane factors in ProductDomain order.
func (s ProductFactorSelection) OrdinaryLanes() []ProductLane {
	return append([]ProductLane(nil), s.ordinary...)
}

// CoordinateFactors returns the immutable exact dependent-coordinate
// inventory. The inventory itself retains ProductDomain and keyspace seals.
func (s ProductFactorSelection) CoordinateFactors() CoordinateFactorInventory {
	return s.coordinates
}

// CoordinateLanes returns the dependent physical carriers in ProductDomain
// order. Each factor extracted from one of these lanes must still be reduced
// through CoordinateFactors before semantic evaluation.
func (s ProductFactorSelection) CoordinateLanes() []ProductLane {
	return append([]ProductLane(nil), s.coordinateLanes...)
}

// CoordinateSkeletonFamilies returns the registered families whose complete
// structural skeleton, but no scalar coordinate, belongs to the selection.
func (s ProductFactorSelection) CoordinateSkeletonFamilies() []CoordinateFamily {
	out := make([]CoordinateFamily, 0)
	for _, factor := range s.coordinateFactors {
		if factor.skeletonOnly {
			out = append(out, factor.family)
		}
	}
	return out
}

// ValueFactors returns the detached, canonical finite Values inventory.
func (s ProductFactorSelection) ValueFactors() []statekey.Value {
	return append([]statekey.Value(nil), s.values...)
}

// ValuesTop reports whether the lifted Values Top bit belongs to the component.
func (s ProductFactorSelection) ValuesTop() bool { return s.valuesTop }

// OwnsProductFactorSelection reports exact ProductDomain ownership. The
// expensive slot/family validation was discharged at the sealing boundary.
func (d ProductDomain) OwnsProductFactorSelection(selection ProductFactorSelection) bool {
	return d.validateFactorSelection(selection) == nil
}

// Decompose returns the exact registry-ordered component representation of
// value. Disabled lanes are normalized away before extraction.
func (d ProductDomain) Decompose(value State) ([]LaneFactor, error) {
	if !d.Valid() {
		return nil, fmt.Errorf("%w: invalid product domain", ErrInvalidLaneFactor)
	}
	value = d.Normalize(value)
	out := make([]LaneFactor, len(d.factorLanes))
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		out[i] = LaneFactor{lane: runtime.lane, payload: runtime.ops.extract(value)}
	}
	return out, nil
}

// DecomposeLanes extracts only the requested sealed descriptors, in caller
// order. It performs no scan over unrequested lanes; duplicates and foreign
// descriptors fail before a factor is returned.
func (d ProductDomain) DecomposeLanes(value State, lanes []ProductLane) ([]LaneFactor, error) {
	if !d.Valid() {
		return nil, fmt.Errorf("%w: invalid product domain", ErrInvalidLaneFactor)
	}
	runtimes := make([]*productLaneRuntime, len(lanes))
	seen := make([]bool, len(d.factorLanes))
	for index, lane := range lanes {
		runtime, err := d.validateLane(lane)
		if err != nil {
			return nil, err
		}
		ordinal := int(runtime.lane.ordinal)
		if seen[ordinal] {
			return nil, fmt.Errorf("%w: duplicate lane descriptor", ErrInvalidProductLane)
		}
		seen[ordinal] = true
		runtimes[index] = runtime
	}
	value = d.Normalize(value)
	out := make([]LaneFactor, len(runtimes))
	for index, runtime := range runtimes {
		out[index] = LaneFactor{lane: runtime.lane, payload: runtime.ops.extract(value)}
	}
	return out, nil
}

// DecomposeLane extracts one sealed factor without allocating an inventory.
func (d ProductDomain) DecomposeLane(value State, lane ProductLane) (LaneFactor, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return LaneFactor{}, err
	}
	value = d.Normalize(value)
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.extract(value)}, nil
}

// Compose is the inverse of Decompose. It accepts exactly one factor for every
// enabled lane, in registry order; omission, duplication, reordering, and
// cross-domain factors fail closed.
func (d ProductDomain) Compose(factors []LaneFactor) (State, error) {
	if !d.Valid() {
		return State{}, fmt.Errorf("%w: invalid product domain", ErrIncompleteLaneFactors)
	}
	if len(factors) != len(d.factorLanes) {
		return State{}, fmt.Errorf("%w: got %d factors, want %d", ErrIncompleteLaneFactors, len(factors), len(d.factorLanes))
	}
	out := d.lattice.Bottom()
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		if _, err := d.validateFactorFor(runtime, factors[i]); err != nil {
			return State{}, fmt.Errorf("%w: position %d: %v", ErrIncompleteLaneFactors, i, err)
		}
		runtime.ops.install(&out, factors[i].payload)
	}
	out.canonical = true
	return d.Normalize(out), nil
}

// ComposeSparse installs only the supplied sealed factors over product
// Bottom. Omitted lanes remain Bottom; input order is irrelevant, while
// duplicate or foreign factors fail without publishing a partial State.
func (d ProductDomain) ComposeSparse(factors []LaneFactor) (State, error) {
	if !d.Valid() {
		return State{}, fmt.Errorf("%w: invalid product domain", ErrIncompleteLaneFactors)
	}
	runtimes := make([]*productLaneRuntime, len(factors))
	seen := make([]bool, len(d.factorLanes))
	for index, factor := range factors {
		runtime, err := d.validateFactor(factor)
		if err != nil {
			return State{}, err
		}
		ordinal := int(runtime.lane.ordinal)
		if seen[ordinal] {
			return State{}, fmt.Errorf("%w: duplicate lane factor", ErrIncompleteLaneFactors)
		}
		seen[ordinal] = true
		runtimes[index] = runtime
	}
	out := d.lattice.Bottom()
	for index, runtime := range runtimes {
		runtime.ops.install(&out, factors[index].payload)
	}
	out.canonical = true
	return d.Normalize(out), nil
}

// PatchFactors replaces the declared ordinary whole lanes in base.
func (d ProductDomain) PatchFactors(base, delta State, writes LaneSet) (State, error) {
	if err := d.validateTupleLaneSelection(writes); err != nil {
		return State{}, err
	}
	out := d.Normalize(base)
	delta = d.Normalize(delta)
	for index := range d.factorLanes {
		if writes.Has(d.factorLanes[index].lane.id) {
			d.factorLanes[index].ops.copy(&out, delta)
		}
	}
	out.canonical = true
	return d.Normalize(out), nil
}

func (d ProductDomain) validateTupleLaneSelection(lanes LaneSet) error {
	if !d.Valid() {
		return fmt.Errorf("%w: invalid product domain", ErrInvalidProductLane)
	}
	seen := make(map[LaneID]struct{}, lanes.Len())
	for _, id := range lanes.ids {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%w: duplicate lane %q", ErrInvalidProductLane, id)
		}
		seen[id] = struct{}{}
		if !d.lanes.Has(id) {
			return fmt.Errorf("%w: lane %q is not enabled", ErrInvalidProductLane, id)
		}
		runtime, ok := d.runtimeForLaneID(id)
		if !ok || runtime.lane.slotFactored {
			return fmt.Errorf("%w: lane %q requires slot-level projection", ErrInvalidProductLane, id)
		}
	}
	return nil
}

func (d ProductDomain) validateFactorSelection(selection ProductFactorSelection) error {
	if !d.Valid() || selection.seal == nil || selection.seal != d.seal || selection.authority == nil || selection.coordinates.KeySpace() == nil ||
		!selection.coordinates.ValidFor(d, selection.coordinates.KeySpace()) || len(selection.selected) != len(d.factorLanes) ||
		len(selection.coordinateSelected) != len(d.factorLanes) || len(selection.coordinateFactors) < len(selection.coordinates.set.families) ||
		len(selection.coordinateGroups) != len(selection.coordinateLanes) {
		return fmt.Errorf("%w: foreign factor selection", ErrInvalidProductLane)
	}
	return nil
}

// LaneBottom returns the bottom component for lane.
func (d ProductDomain) LaneBottom(lane ProductLane) (LaneFactor, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.bottom()}, nil
}

// LaneTop returns the top component for lane.
func (d ProductDomain) LaneTop(lane ProductLane) (LaneFactor, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.top()}, nil
}

// LaneEqual reports semantic equality of two components of the same lane.
func (d ProductDomain) LaneEqual(left, right LaneFactor) (bool, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil {
		return false, err
	}
	return runtime.ops.equal(left.payload, right.payload), nil
}

// LaneSame reports whether two components share the same persistent
// representation. False does not imply semantic inequality: lanes whose
// registered lattice has no Same predicate, and equal values held by distinct
// persistent representations, both report false.
func (d ProductDomain) LaneSame(left, right LaneFactor) (bool, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil {
		return false, err
	}
	return runtime.ops.same(left.payload, right.payload), nil
}

// LaneCanonicalRepresentationEqual is the collision proof for the canonical
// factor-terminal interner. It is deliberately separate from LaneSame: maps
// or immutable values built independently may be canonical-equal without
// sharing a persistent representation.
func (d ProductDomain) LaneCanonicalRepresentationEqual(left, right LaneFactor) (bool, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil || runtime.ops.canonicalEqual == nil {
		return false, err
	}
	return runtime.ops.canonicalEqual(left.payload, right.payload), nil
}

// LaneLessOrEq reports the component lattice order.
func (d ProductDomain) LaneLessOrEq(left, right LaneFactor) (bool, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil {
		return false, err
	}
	return runtime.ops.lessOrEq(left.payload, right.payload), nil
}

// LaneJoin returns the componentwise least upper bound.
func (d ProductDomain) LaneJoin(left, right LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.join(left.payload, right.payload)}, nil
}

// LaneMeet returns the componentwise greatest lower bound. Domains that do
// not define meet reject the operation instead of approximating it.
func (d ProductDomain) LaneMeet(left, right LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactorPair(left, right)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.meet(left.payload, right.payload)}, nil
}

// LaneWiden applies the registered lane widening.
func (d ProductDomain) LaneWiden(previous, next LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactorPair(previous, next)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.widen(previous.payload, next.payload)}, nil
}

// LaneNarrow applies the registered lane narrowing, or keeps previous when
// the lane has no narrowing operator, matching the whole-State domain.
func (d ProductDomain) LaneNarrow(previous, next LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactorPair(previous, next)
	if err != nil {
		return LaneFactor{}, err
	}
	return LaneFactor{lane: runtime.lane, payload: runtime.ops.narrow(previous.payload, next.payload)}, nil
}

// LaneFingerprint returns the deterministic semantic digest of one component.
// d owns the registry and lane selection; config supplies only the contextual
// fingerprint policy and keyspace. A conflicting registry or lane request is
// rejected instead of silently hashing a different product.
func (d ProductDomain) LaneFingerprint(config FingerprintConfig, factor LaneFactor) (uint64, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return 0, err
	}
	if config.Registry != nil && config.Registry != d.reg {
		return 0, fmt.Errorf("%w: fingerprint registry does not own product domain", ErrInvalidLaneFactor)
	}
	if config.Lanes != nil && (len(config.Lanes) != 1 || config.Lanes[0] != runtime.lane.id) {
		return 0, fmt.Errorf("%w: fingerprint lane selection does not name %q", ErrInvalidLaneFactor, runtime.lane.id)
	}
	return d.fingerprintLane(config, runtime, factor)
}

func (d ProductDomain) fingerprintLane(config FingerprintConfig, runtime *productLaneRuntime, factor LaneFactor) (uint64, error) {
	if config.Context != nil {
		if err := config.Context.Err(); err != nil {
			return 0, err
		}
	}
	if runtime.fingerprint == nil {
		return 0, fmt.Errorf("%w: lane %q", ErrFingerprintCoverage, runtime.lane.id)
	}
	config.Registry = d.reg
	config.Lanes = nil
	writer := newFingerprintWriter(config)
	writer.string("schema", "go-lua.state-lane-factor/v1")
	writer.string("lane", string(runtime.lane.id))
	component := d.lattice.Bottom()
	runtime.ops.install(&component, factor.payload)
	runtime.fingerprint(writer, component)
	if err := writer.err(); err != nil {
		return 0, err
	}
	return writer.sum64(), nil
}

// VisitLaneValueDependencies enumerates the exact concrete or formal Values
// roots referenced by one component. The dependency policy is catalog-owned,
// so adding a State lane cannot bypass factor alignment by omitting a central
// type switch.
func (d ProductDomain) VisitLaneValueDependencies(factor LaneFactor, keys *keyspace.KeySpace, visit func(statekey.ValueDependency)) error {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return err
	}
	if keys == nil || visit == nil {
		return fmt.Errorf("%w: Values dependency visitation requires keyspace and visitor", ErrInvalidLaneFactor)
	}
	switch runtime.valueDependencies.kind {
	case laneValueDependenciesIndependent:
		return nil
	case laneValueDependenciesEnumerated:
		component := d.lattice.Bottom()
		runtime.ops.install(&component, factor.payload)
		runtime.valueDependencies.visit(component, keys, visit)
		return nil
	default:
		return fmt.Errorf("%w: lane %q has no Values dependency policy", ErrInvalidLaneFactor, runtime.lane.id)
	}
}

// VisitLaneIdentities enumerates every exact identity stored by one opaque
// lane factor. The visitor is catalog-owned: lane growth cannot silently omit
// allocation templates from boundary quotienting.
func (d ProductDomain) VisitLaneIdentities(factor LaneFactor, visit func(identity.ID)) error {
	if visit == nil {
		return fmt.Errorf("%w: identity visitation requires visitor", ErrInvalidLaneFactor)
	}
	return d.visitLaneIdentities(factor, func(term identity.Term) bool {
		id, concrete := term.Concrete()
		if concrete {
			visit(id)
		}
		return true
	})
}

// VisitLaneIdentityTerms enumerates the complete typed identity support of one
// factor. Formal relation construction uses this method; concrete consumers
// may use VisitLaneIdentities, which filters to materialized IDs.
func (d ProductDomain) VisitLaneIdentityTerms(factor LaneFactor, visit func(identity.Term)) error {
	if visit == nil {
		return fmt.Errorf("%w: identity-term visitation requires visitor", ErrInvalidLaneFactor)
	}
	return d.visitLaneIdentities(factor, func(term identity.Term) bool {
		visit(term)
		return true
	})
}

// LaneIdentityImageLaw returns the exact registered pushforward/inverse-fiber
// behavior for lane. Dispatch is through the sealed domain descriptor, never
// through LaneID or factor payload inspection.
func (d ProductDomain) LaneIdentityImageLaw(lane ProductLane) (IdentityImageLaw, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return IdentityImageInvalid, err
	}
	return runtime.identitySupport.image, nil
}

func (d ProductDomain) visitLaneIdentities(factor LaneFactor, visit func(identity.Term) bool) error {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return err
	}
	if visit == nil {
		return fmt.Errorf("%w: identity visitation requires visitor", ErrInvalidLaneFactor)
	}
	switch runtime.identitySupport.kind {
	case laneIdentitiesIndependent:
		return nil
	case laneIdentitiesEnumerated:
		runtime.identitySupport.visit(d.reg, factor.payload, visit)
		return nil
	default:
		return fmt.Errorf("%w: lane %q has no identity-support policy", ErrInvalidLaneFactor, runtime.lane.id)
	}
}

// LaneContainsAllocationTemplate reports whether a factor contains any exact
// lexical allocation template. Unknown/top identity values are not templates;
// they retain their ordinary lattice meaning.
func (d ProductDomain) LaneContainsAllocationTemplate(factor LaneFactor) (bool, error) {
	contains := false
	err := d.visitLaneIdentities(factor, func(term identity.Term) bool {
		_, contains = term.Allocation()
		return !contains
	})
	return contains, err
}

func (d ProductDomain) validateLane(lane ProductLane) (*productLaneRuntime, error) {
	if !d.Valid() || lane.seal == nil || lane.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign product domain", ErrInvalidProductLane)
	}
	ordinal := int(lane.ordinal)
	if ordinal < 0 || ordinal >= len(d.factorLanes) {
		return nil, fmt.Errorf("%w: ordinal %d", ErrInvalidProductLane, ordinal)
	}
	runtime := &d.factorLanes[ordinal]
	if runtime.lane.id != lane.id {
		return nil, fmt.Errorf("%w: ordinal %d names %q, not %q", ErrInvalidProductLane, ordinal, runtime.lane.id, lane.id)
	}
	return runtime, nil
}

func (d ProductDomain) validateFactor(factor LaneFactor) (*productLaneRuntime, error) {
	runtime, err := d.validateLane(factor.lane)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLaneFactor, err)
	}
	if factor.payload == nil {
		return nil, fmt.Errorf("%w: lane %q has no payload", ErrInvalidLaneFactor, runtime.lane.id)
	}
	return runtime, nil
}

func (d ProductDomain) validateFactorFor(runtime *productLaneRuntime, factor LaneFactor) (*productLaneRuntime, error) {
	if runtime == nil {
		return nil, fmt.Errorf("%w: missing expected lane runtime", ErrInvalidLaneFactor)
	}
	actual, err := d.validateFactor(factor)
	if err != nil {
		return nil, err
	}
	if actual.lane.ordinal != runtime.lane.ordinal {
		return nil, fmt.Errorf("%w: got lane %q, want %q", ErrInvalidLaneFactor, actual.lane.id, runtime.lane.id)
	}
	return actual, nil
}

func (d ProductDomain) validateFactorPair(left, right LaneFactor) (*productLaneRuntime, error) {
	runtime, err := d.validateFactor(left)
	if err != nil {
		return nil, err
	}
	if _, err := d.validateFactorFor(runtime, right); err != nil {
		return nil, err
	}
	return runtime, nil
}
