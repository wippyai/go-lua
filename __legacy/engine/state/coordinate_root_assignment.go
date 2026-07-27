package state

import (
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type coordinateRootAssignmentKind uint8

const (
	coordinateRootAssignmentInvalid coordinateRootAssignmentKind = iota
	coordinateRootAssignmentNone
	coordinateRootAssignmentUnique
)

// coordinateRootAssignmentOps is the operation-owned coordinate protocol for
// N4 completion. Representation-specific evidence stays inside its family;
// the transformer observes only the mathematical predicate it needs.
type coordinateRootAssignmentOps struct {
	kind                   coordinateRootAssignmentKind
	scalarTransfer         coordinateScalarTransferOps
	freshEmpty             func(coordinateSkeletonPayload, identity.ID) bool
	completionDependencies RootAssignmentCompletionDependencies
	completionTarget       func(keyspace.Key) (coordinateKeyPayload, bool)
	completionSlot         func(RootAssignmentCompletion) (coordinateKeyPayload, bool)
	applyCompletion        func(coordinateSkeletonPayload, coordinateKeyPayload, coordinateScalarPayload, RootAssignmentCompletion) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
	applyCompletionLane    func(laneFactorPayload, RootAssignmentCompletion) (laneFactorPayload, bool)
	lenFloorValue          func(coordinateScalarPayload) (int64, bool)
}

type coordinateScalarTransferKind uint8

const (
	coordinateScalarTransferInvalid coordinateScalarTransferKind = iota
	coordinateScalarTransferIndependent
	coordinateScalarTransferParticipant
)

// coordinateScalarTransferDemand is the exact N4 coordinate footprint owned
// by one family. Source is read from point-entry only when hasSource is true;
// target is always read and replaced in current state.
type coordinateScalarTransferDemand struct {
	target    coordinateKeyPayload
	source    coordinateKeyPayload
	hasSource bool
}

// coordinateScalarTransferOps keeps keyed scalar transfer representation in
// its coordinate family. Concrete and guarded execution both consume the same
// finite demand and the same one-coordinate apply law.
type coordinateScalarTransferOps struct {
	kind coordinateScalarTransferKind
	// demand plans a finite, canonical target set from static skeleton
	// topology. It cannot inspect any scalar value.
	demand func([]coordinateKeyPayload, RootAssignmentScalarTransfer) ([]coordinateScalarTransferDemand, bool)
	apply  func(coordinateSkeletonPayload, coordinateScalarPayload, coordinateScalarPayload, bool, RootAssignmentScalarTransfer) (coordinateSkeletonPayload, coordinateScalarPayload, bool)
}

func coordinateScalarTransferIndependentOps() coordinateScalarTransferOps {
	return coordinateScalarTransferOps{kind: coordinateScalarTransferIndependent}
}

func noCoordinateRootAssignment() coordinateRootAssignmentOps {
	return coordinateRootAssignmentOps{kind: coordinateRootAssignmentNone, scalarTransfer: coordinateScalarTransferIndependentOps()}
}

func uniqueCoordinateRootAssignment(freshEmpty func(coordinateSkeletonPayload, identity.ID) bool) coordinateRootAssignmentOps {
	return coordinateRootAssignmentOps{kind: coordinateRootAssignmentUnique, freshEmpty: freshEmpty, scalarTransfer: coordinateScalarTransferIndependentOps()}
}

func coordinateRootAssignmentOpsComplete(ops coordinateRootAssignmentOps) bool {
	completion := ops.completionTarget != nil || ops.completionSlot != nil || ops.applyCompletion != nil || ops.applyCompletionLane != nil || ops.completionDependencies.bits != 0
	if completion && (ops.completionTarget == nil || ops.completionSlot == nil || ops.applyCompletion == nil || ops.applyCompletionLane == nil || ops.completionDependencies.bits == 0) {
		return false
	}
	scalarComplete := false
	switch ops.scalarTransfer.kind {
	case coordinateScalarTransferIndependent:
		scalarComplete = ops.scalarTransfer.demand == nil && ops.scalarTransfer.apply == nil
	case coordinateScalarTransferParticipant:
		scalarComplete = ops.scalarTransfer.demand != nil && ops.scalarTransfer.apply != nil
	}
	if !scalarComplete {
		return false
	}
	switch ops.kind {
	case coordinateRootAssignmentNone:
		return ops.freshEmpty == nil
	case coordinateRootAssignmentUnique:
		return ops.freshEmpty != nil
	default:
		return false
	}
}

// RootAssignmentScalarCoordinateDemand is the exact family-owned sparse
// footprint for one scalar transfer. Omitted source means the transfer is
// constant or clearing and has no point-entry coordinate dependency.
type RootAssignmentScalarCoordinateDemand struct {
	target    CoordinateSlot
	source    CoordinateSlot
	hasSource bool
}

func (d RootAssignmentScalarCoordinateDemand) Target() CoordinateSlot { return d.target }
func (d RootAssignmentScalarCoordinateDemand) PointSource() (CoordinateSlot, bool) {
	return d.source, d.hasSource
}

func (d ProductDomain) RootAssignmentScalarCoordinateFamilies() []CoordinateFamily {
	if !d.Valid() {
		return nil
	}
	var out []CoordinateFamily
	for _, lane := range d.factorLanes {
		for _, coordinate := range lane.coordinates {
			if coordinate.ops.rootAssignment.scalarTransfer.kind == coordinateScalarTransferParticipant {
				out = append(out, coordinate.family)
			}
		}
	}
	return out
}

func (d ProductDomain) RootAssignmentScalarCoordinateDemands(transaction RootAssignmentScalarFactorTransaction, family CoordinateFamily, keys *keyspace.KeySpace, inventory []CoordinateSlot) ([]RootAssignmentScalarCoordinateDemand, error) {
	if !d.Valid() || transaction.seal != d.seal || keys == nil || !keys.Valid() {
		return nil, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || coordinate.ops.rootAssignment.scalarTransfer.kind != coordinateScalarTransferParticipant {
		return nil, ErrInvalidLaneFactor
	}
	keysOnly := make([]coordinateKeyPayload, len(inventory))
	for i, slot := range inventory {
		if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
			return nil, err
		}
		if i > 0 {
			less, err := d.CoordinateSlotLess(inventory[i-1], slot)
			if err != nil || !less {
				return nil, ErrInvalidLaneFactor
			}
		}
		keysOnly[i] = slot.key
	}
	demands, present := coordinate.ops.rootAssignment.scalarTransfer.demand(keysOnly, transaction.transfer)
	if !present {
		return nil, nil
	}
	out := make([]RootAssignmentScalarCoordinateDemand, len(demands))
	for i, demand := range demands {
		if demand.target == nil || !coordinate.ops.keyValid(demand.target, keys) || demand.hasSource && (demand.source == nil || !coordinate.ops.keyValid(demand.source, keys)) {
			return nil, ErrInvalidLaneFactor
		}
		if i > 0 && !coordinate.ops.keyLess(demands[i-1].target, demand.target, keys) {
			return nil, ErrInvalidLaneFactor
		}
		out[i] = RootAssignmentScalarCoordinateDemand{target: CoordinateSlot{family: family, keys: keys, key: demand.target}, hasSource: demand.hasSource}
		if demand.hasSource {
			out[i].source = CoordinateSlot{family: family, keys: keys, key: demand.source}
		}
	}
	return out, nil
}

// ApplyRootAssignmentScalarCoordinate applies one exact family transfer. The
// point source is required exactly when declared by demand. Both target and
// source may be family defaults supplied by CoordinateDefault.
func (d ProductDomain) ApplyRootAssignmentScalarCoordinate(transaction RootAssignmentScalarFactorTransaction, currentSkeleton CoordinateFamilySkeleton, currentTarget CoordinateScalarFactor, pointSource CoordinateScalarFactor, hasPointSource bool) (CoordinateFamilySkeleton, CoordinateScalarFactor, error) {
	if !d.Valid() || transaction.seal != d.seal {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateSkeleton(currentSkeleton)
	if err != nil || coordinate.ops.rootAssignment.scalarTransfer.kind != coordinateScalarTransferParticipant || d.validateCoordinateFactorFor(coordinate, currentTarget, currentSkeleton.keys) != nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	var pointScalarPayload coordinateScalarPayload
	if hasPointSource {
		if d.validateCoordinateFactorFor(coordinate, pointSource, currentSkeleton.keys) != nil {
			return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
		}
		pointScalarPayload = pointSource.payload
	}
	nextSkeleton, nextScalar, ok := coordinate.ops.rootAssignment.scalarTransfer.apply(currentSkeleton.payload, currentTarget.payload, pointScalarPayload, hasPointSource, transaction.transfer)
	if !ok || nextSkeleton == nil || nextScalar == nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	currentSkeleton.payload, currentTarget.payload = nextSkeleton, nextScalar
	return currentSkeleton, currentTarget, nil
}

// RootAssignmentCompletionCoordinateTargetSlots freezes the complete family
// coordinate inventory that an N4 target may update before valuation.
func (d ProductDomain) RootAssignmentCompletionCoordinateTargetSlots(keys *keyspace.KeySpace, target keyspace.Key) ([]CoordinateSlot, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || target.Kind == keyspace.KindInvalid {
		return nil, ErrInvalidLaneFactor
	}
	out := make([]CoordinateSlot, 0, 1)
	for _, lane := range d.factorLanes {
		for _, coordinate := range lane.coordinates {
			if coordinate.ops.rootAssignment.completionTarget == nil {
				continue
			}
			key, present := coordinate.ops.rootAssignment.completionTarget(target)
			if !present {
				continue
			}
			if !coordinate.ops.keyValid(key, keys) {
				return nil, ErrInvalidLaneFactor
			}
			out = append(out, CoordinateSlot{family: coordinate.family, keys: keys, key: key})
		}
	}
	return out, nil
}

// RootAssignmentCompletionCoordinateFamilies returns the registered family
// participants in caller-local N4 completion. Atomic completion lanes are not
// included and no LaneID or family name participates in discovery.
func (d ProductDomain) RootAssignmentCompletionCoordinateFamilies() []CoordinateFamily {
	if !d.Valid() {
		return nil
	}
	out := make([]CoordinateFamily, 0, 1)
	for _, lane := range d.factorLanes {
		for _, family := range lane.coordinates {
			if family.ops.rootAssignment.completionSlot != nil {
				out = append(out, family.family)
			}
		}
	}
	return out
}

func (d ProductDomain) RootAssignmentCompletionCoordinateDependencies(family CoordinateFamily) (RootAssignmentCompletionDependencies, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || coordinate.ops.rootAssignment.completionSlot == nil {
		return RootAssignmentCompletionDependencies{}, ErrInvalidLaneFactor
	}
	return coordinate.ops.rootAssignment.completionDependencies, nil
}

// RootAssignmentCompletionCoordinateSlot returns the exact sparse coordinate
// written by transaction in family. Absence means that this valuation has no
// publication for the family.
func (d ProductDomain) RootAssignmentCompletionCoordinateSlot(transaction RootAssignmentFactorTransaction, family CoordinateFamily, keys *keyspace.KeySpace) (CoordinateSlot, bool, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.completion.valid() || keys == nil || !keys.Valid() {
		return CoordinateSlot{}, false, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || coordinate.ops.rootAssignment.completionSlot == nil {
		return CoordinateSlot{}, false, ErrInvalidLaneFactor
	}
	key, present := coordinate.ops.rootAssignment.completionSlot(transaction.completion)
	if !present {
		return CoordinateSlot{}, false, nil
	}
	if !coordinate.ops.keyValid(key, keys) {
		return CoordinateSlot{}, false, ErrInvalidLaneFactor
	}
	return CoordinateSlot{family: family, keys: keys, key: key}, true, nil
}

// ApplyRootAssignmentCompletionCoordinate applies one family-owned N4 write
// to an exact skeleton/scalar pair. The current scalar may be the family
// default; the returned skeleton owns any newly created key topology.
func (d ProductDomain) ApplyRootAssignmentCompletionCoordinate(transaction RootAssignmentFactorTransaction, skeleton CoordinateFamilySkeleton, current CoordinateScalarFactor) (CoordinateFamilySkeleton, CoordinateScalarFactor, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.completion.valid() {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || coordinate.ops.rootAssignment.applyCompletion == nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	if err := d.validateCoordinateFactorFor(coordinate, current, skeleton.keys); err != nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, err
	}
	if current.slot.family != skeleton.family {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	nextSkeleton, nextScalar, ok := coordinate.ops.rootAssignment.applyCompletion(skeleton.payload, current.slot.key, current.payload, transaction.completion)
	if !ok || nextSkeleton == nil || nextScalar == nil {
		return CoordinateFamilySkeleton{}, CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	skeleton.payload, current.payload = nextSkeleton, nextScalar
	return skeleton, current, nil
}

// RootAssignmentCoordinateFamily returns the unique enabled family that owns
// caller-local N4 coordinate evidence.
func (d ProductDomain) RootAssignmentCoordinateFamily() (CoordinateFamily, bool) {
	if !d.Valid() || !d.hasRootAssignmentFamily {
		return CoordinateFamily{}, false
	}
	if _, err := d.validateCoordinateFamily(d.rootAssignmentFamily); err != nil {
		return CoordinateFamily{}, false
	}
	return d.rootAssignmentFamily, true
}

// CoordinateRootAssignmentFreshEmpty decides whether id denotes an exactly
// known fresh empty container in the registered N4 family skeleton.
func (d ProductDomain) CoordinateRootAssignmentFreshEmpty(skeleton CoordinateFamilySkeleton, id identity.ID) (bool, error) {
	owner, ok := d.RootAssignmentCoordinateFamily()
	if !ok || skeleton.family != owner {
		return false, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return false, err
	}
	return coordinate.ops.rootAssignment.freshEmpty(skeleton.payload, id), nil
}

// RootAssignmentFreshEmptyState is the concrete adapter over the same
// coordinate-family predicate used by guarded N4 execution. It contains no
// second heap/object rule.
func (d ProductDomain) RootAssignmentFreshEmptyState(keys *keyspace.KeySpace, current State, slot statekey.Value) (bool, error) {
	family, ok := d.RootAssignmentCoordinateFamily()
	if !ok || keys == nil || !keys.Valid() || slot == 0 {
		return false, ErrInvalidLaneFactor
	}
	value := current.ReadValue(d.reg, slot)
	id, exact := product.Get(d.reg, value, identity.Key).ID()
	if !exact || id == (identity.ID{}) {
		return false, nil
	}
	factors, err := d.DecomposeLanes(current, []ProductLane{family.Lane()})
	if err != nil || len(factors) != 1 {
		return false, err
	}
	skeleton, _, err := d.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		return false, err
	}
	return d.CoordinateRootAssignmentFreshEmpty(skeleton, id)
}
