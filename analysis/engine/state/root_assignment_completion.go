package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// RootAssignmentAccess is the complete registered State footprint of the N4
// root-assignment transaction. PointEntry is the immutable source snapshot;
// Current is the sequential state consumed and updated after earlier effects
// at the same point.
type RootAssignmentAccess struct {
	PointEntry    LaneSet
	Current       LaneSet
	CurrentWrites LaneSet
}

// RootAssignmentFactorPlan is the ProductDomain-sealed component topology for
// one N4 transaction. It is derived only from lane registrations: adding,
// removing, or reordering an axis cannot require a second operation-owned
// inventory. Values remains present as its registered ProductLane here even
// though execution projects it by exact slots rather than as an opaque lane.
type RootAssignmentFactorPlan struct {
	seal          *productDomainSeal
	pointEntry    []ProductLane
	current       []ProductLane
	currentWrites []ProductLane
}

// SealRootAssignmentFactorPlan freezes the registered N4 read/write topology
// for this exact product. The returned descriptors are the sole lane-level
// authority used when the resolved transaction is lowered into factor blocks.
func (d ProductDomain) SealRootAssignmentFactorPlan() (RootAssignmentFactorPlan, error) {
	if !d.Valid() {
		return RootAssignmentFactorPlan{}, fmt.Errorf("state: invalid root-assignment factor plan")
	}
	plan := RootAssignmentFactorPlan{seal: d.seal}
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		coordinateScalar := false
		for familyIndex := range runtime.coordinates {
			coordinate := &runtime.coordinates[familyIndex]
			coordinateScalar = coordinateScalar || coordinate.ops.rootAssignment.scalarTransfer.kind == coordinateScalarTransferParticipant
		}
		if runtime.rootAssignment.pointRead || coordinateScalar {
			plan.pointEntry = append(plan.pointEntry, runtime.lane)
		}
		if runtime.rootAssignment.currentRead || coordinateScalar {
			plan.current = append(plan.current, runtime.lane)
		}
		if runtime.rootAssignment.currentWrite || coordinateScalar {
			plan.currentWrites = append(plan.currentWrites, runtime.lane)
		}
	}
	return plan, nil
}

func cloneRootAssignmentProductLanes(lanes []ProductLane) []ProductLane {
	out := make([]ProductLane, len(lanes))
	copy(out, lanes)
	return out
}

// PointEntryLanes returns the immutable point-snapshot factors observed by N4.
func (p RootAssignmentFactorPlan) PointEntryLanes() []ProductLane {
	return cloneRootAssignmentProductLanes(p.pointEntry)
}

// CurrentLanes returns the sequential factors observed after earlier effects.
func (p RootAssignmentFactorPlan) CurrentLanes() []ProductLane {
	return cloneRootAssignmentProductLanes(p.current)
}

// CurrentWriteLanes returns the disjoint registered factor write inventory.
func (p RootAssignmentFactorPlan) CurrentWriteLanes() []ProductLane {
	return cloneRootAssignmentProductLanes(p.currentWrites)
}

// Owns reports whether this plan was sealed by d.
func (d ProductDomain) OwnsRootAssignmentFactorPlan(p RootAssignmentFactorPlan) bool {
	return d.Valid() && p.seal != nil && p.seal == d.seal
}

// RootAssignmentAccess returns the catalog-registered N4 footprint. No
// semantic axis inventory exists outside the lane registrations.
func (c LaneCatalog) RootAssignmentAccess() RootAssignmentAccess {
	point := make([]LaneID, 0, len(c.specs))
	current := make([]LaneID, 0, len(c.specs))
	writes := make([]LaneID, 0, len(c.specs))
	for _, spec := range c.specs {
		if spec.rootAssignment.pointRead {
			point = append(point, spec.id)
		}
		if spec.rootAssignment.currentRead {
			current = append(current, spec.id)
		}
		if spec.rootAssignment.currentWrite {
			writes = append(writes, spec.id)
		}
	}
	return RootAssignmentAccess{
		PointEntry: NewLaneSet(point...), Current: NewLaneSet(current...), CurrentWrites: NewLaneSet(writes...),
	}
}

// RootAssignmentAccess returns the enabled product's registered N4 footprint.
func (d ProductDomain) RootAssignmentAccess() RootAssignmentAccess {
	if !d.Valid() {
		return RootAssignmentAccess{}
	}
	point := make([]LaneID, 0, len(d.factorLanes))
	current := make([]LaneID, 0, len(d.factorLanes))
	writes := make([]LaneID, 0, len(d.factorLanes))
	for laneIndex := range d.factorLanes {
		runtime := &d.factorLanes[laneIndex]
		if runtime.rootAssignment.pointRead {
			point = append(point, runtime.lane.id)
		}
		if runtime.rootAssignment.currentRead {
			current = append(current, runtime.lane.id)
		}
		if runtime.rootAssignment.currentWrite {
			writes = append(writes, runtime.lane.id)
		}
	}
	return RootAssignmentAccess{
		PointEntry: NewLaneSet(point...), Current: NewLaneSet(current...), CurrentWrites: NewLaneSet(writes...),
	}
}

type rootAssignmentLanePolicy struct {
	declared                 bool
	pointRead                bool
	currentRead              bool
	currentWrite             bool
	completion               bool
	completionDependencies   RootAssignmentCompletionDependencies
	scalar                   bool
	dynamicSource            bool
	dynamicSourceInput       rootAssignmentDynamicSourceInputRole
	applyState               func(*State, RootAssignmentCompletion) bool
	applyFactor              func(laneFactorPayload, RootAssignmentCompletion) (laneFactorPayload, bool)
	applyScalarState         func(*State, State, RootAssignmentScalarTransfer) bool
	applyScalarFactor        func(laneFactorPayload, laneFactorPayload, RootAssignmentScalarTransfer) (laneFactorPayload, bool)
	applyDynamicSourceState  func(*State, RootAssignmentDynamicSourceTransaction) bool
	applyDynamicSourceFactor func(laneFactorPayload, RootAssignmentDynamicSourceTransaction) (laneFactorPayload, bool)
}

type rootAssignmentCompletionDependencyBits uint8

const (
	rootAssignmentCompletionSourceValue rootAssignmentCompletionDependencyBits = 1 << iota
	rootAssignmentCompletionFreshEmptyPredicates
	rootAssignmentCompletionAllDependencies = rootAssignmentCompletionSourceValue | rootAssignmentCompletionFreshEmptyPredicates
)

// RootAssignmentCompletionDependencies is the exact upstream evidence needed
// to derive one registered completion lane. Its representation is opaque so
// dependency ownership remains with the lane registration.
type RootAssignmentCompletionDependencies struct {
	bits rootAssignmentCompletionDependencyBits
}

// SourceValue reports whether completion of the lane needs the resolved
// source value.
func (d RootAssignmentCompletionDependencies) SourceValue() bool {
	return d.bits&rootAssignmentCompletionSourceValue != 0
}

// FreshEmptyPredicates reports whether completion of the lane needs the
// per-path fresh-empty predicates.
func (d RootAssignmentCompletionDependencies) FreshEmptyPredicates() bool {
	return d.bits&rootAssignmentCompletionFreshEmptyPredicates != 0
}

func rootAssignmentCompletionSourceValueDependencies() RootAssignmentCompletionDependencies {
	return RootAssignmentCompletionDependencies{bits: rootAssignmentCompletionSourceValue}
}

func rootAssignmentCompletionFreshEmptyDependencies() RootAssignmentCompletionDependencies {
	return RootAssignmentCompletionDependencies{bits: rootAssignmentCompletionFreshEmptyPredicates}
}

// rootAssignmentUnchanged declares an axis that participates in the ordinary
// N4 access footprint but has no caller-local post-boundary derivation.
func rootAssignmentUnchanged(pointRead, currentRead, currentWrite bool) rootAssignmentLanePolicy {
	return rootAssignmentLanePolicy{
		declared: true, pointRead: pointRead, currentRead: currentRead, currentWrite: currentWrite,
		applyState: func(*State, RootAssignmentCompletion) bool { return false },
		applyFactor: func(lane laneFactorPayload, _ RootAssignmentCompletion) (laneFactorPayload, bool) {
			return lane, false
		},
		applyScalarState: func(*State, State, RootAssignmentScalarTransfer) bool { return false },
		applyScalarFactor: func(_ laneFactorPayload, current laneFactorPayload, _ RootAssignmentScalarTransfer) (laneFactorPayload, bool) {
			return current, false
		},
		applyDynamicSourceState: func(*State, RootAssignmentDynamicSourceTransaction) bool { return false },
		applyDynamicSourceFactor: func(current laneFactorPayload, _ RootAssignmentDynamicSourceTransaction) (laneFactorPayload, bool) {
			return current, false
		},
	}
}

// rootAssignmentCompletionLane declares one axis whose representation owns a
// caller-local N4 completion law. The same typed apply function backs both
// whole-State concrete execution and opaque factor execution.
func rootAssignmentCompletionLane[T any](
	dependencies RootAssignmentCompletionDependencies,
	pointRead, currentRead, currentWrite bool,
	get func(State) T,
	set func(*State, T),
	apply func(T, RootAssignmentCompletion) (T, bool),
) rootAssignmentLanePolicy {
	return rootAssignmentLanePolicy{
		declared: true, pointRead: pointRead, currentRead: currentRead, currentWrite: currentWrite,
		completion: true, completionDependencies: dependencies,
		applyState: func(out *State, completion RootAssignmentCompletion) bool {
			next, changed := apply(get(*out), completion)
			if changed {
				set(out, next)
			}
			return changed
		},
		applyFactor: func(payload laneFactorPayload, completion RootAssignmentCompletion) (laneFactorPayload, bool) {
			next, changed := apply(typedLaneFactorValue[T](payload), completion)
			if !changed {
				return payload, false
			}
			return typedLaneFactorPayload[T]{value: next}, true
		},
		applyScalarState: func(*State, State, RootAssignmentScalarTransfer) bool { return false },
		applyScalarFactor: func(_ laneFactorPayload, current laneFactorPayload, _ RootAssignmentScalarTransfer) (laneFactorPayload, bool) {
			return current, false
		},
		applyDynamicSourceState: func(*State, RootAssignmentDynamicSourceTransaction) bool { return false },
		applyDynamicSourceFactor: func(current laneFactorPayload, _ RootAssignmentDynamicSourceTransaction) (laneFactorPayload, bool) {
			return current, false
		},
	}
}

// withRootAssignmentScalarLaw installs one representation-owned scalar N4
// sidecar law. The same typed function is used by concrete and factor
// execution, preventing the two adapters from acquiring separate semantics.
func withRootAssignmentScalarLaw[T any](
	policy rootAssignmentLanePolicy,
	get func(State) T,
	set func(*State, T),
	apply func(T, T, RootAssignmentScalarTransfer) (T, bool),
) rootAssignmentLanePolicy {
	policy.scalar = true
	policy.applyScalarState = func(out *State, point State, transfer RootAssignmentScalarTransfer) bool {
		next, changed := apply(get(point), get(*out), transfer)
		if changed {
			set(out, next)
		}
		return changed
	}
	policy.applyScalarFactor = func(point, current laneFactorPayload, transfer RootAssignmentScalarTransfer) (laneFactorPayload, bool) {
		next, changed := apply(typedLaneFactorValue[T](point), typedLaneFactorValue[T](current), transfer)
		if !changed {
			return current, false
		}
		return typedLaneFactorPayload[T]{value: next}, true
	}
	return policy
}

// RootAssignmentLenFloor is one present coordinate in the sparse positive
// length-floor lattice. Its zero value is absence. Key and value are private
// so callers cannot represent a half-coordinate.
type RootAssignmentLenFloor struct {
	key     keyspace.Key
	floor   int64
	present bool
}

// NewRootAssignmentLenFloor constructs one atomic keyed length fact.
func NewRootAssignmentLenFloor(key keyspace.Key, floor int64) (RootAssignmentLenFloor, error) {
	if key.Kind == keyspace.KindInvalid || floor <= 0 {
		return RootAssignmentLenFloor{}, fmt.Errorf("state: invalid root-assignment length coordinate")
	}
	return RootAssignmentLenFloor{key: key, floor: floor, present: true}, nil
}

// RootAssignmentCompletionConfig is the caller-local evidence derived after a
// call boundary has already transported the result to its receiver. It never
// contains transported facts: those remain owned by boundary transport.
type RootAssignmentCompletionConfig struct {
	LenFloor       RootAssignmentLenFloor
	KeyMemberships []KeyMembership
}

// RootAssignmentCompletion is an immutable, validated caller-local N4 delta.
// Its representation contains no State and is safe to retain in a guarded
// transfer plan.
type RootAssignmentCompletion struct {
	lenFloorKey    keyspace.Key
	lenFloor       int64
	keyMemberships []KeyMembership
	sealed         bool
}

// SealRootAssignmentCompletion validates and copies one caller-local N4 delta.
func SealRootAssignmentCompletion(config RootAssignmentCompletionConfig) (RootAssignmentCompletion, error) {
	if config.LenFloor.present && (config.LenFloor.key.Kind == keyspace.KindInvalid || config.LenFloor.floor <= 0) {
		return RootAssignmentCompletion{}, fmt.Errorf("state: invalid root-assignment length coordinate")
	}
	if !config.LenFloor.present && (config.LenFloor.key.Kind != keyspace.KindInvalid || config.LenFloor.floor != 0) {
		return RootAssignmentCompletion{}, fmt.Errorf("state: unsealed root-assignment length coordinate")
	}
	memberships := append([]KeyMembership(nil), config.KeyMemberships...)
	for index, membership := range memberships {
		if !membership.valid() {
			return RootAssignmentCompletion{}, fmt.Errorf("state: invalid root-assignment key membership %d", index)
		}
	}
	return RootAssignmentCompletion{
		lenFloorKey: config.LenFloor.key, lenFloor: config.LenFloor.floor,
		keyMemberships: memberships, sealed: true,
	}, nil
}

func (c RootAssignmentCompletion) valid() bool { return c.sealed }

func applyRootAssignmentLenFloor(lane lenFloorLane, completion RootAssignmentCompletion) (lenFloorLane, bool) {
	if completion.lenFloor <= 0 {
		return lane, false
	}
	return lane.write(completion.lenFloorKey, completion.lenFloor)
}

func applyRootAssignmentKeyMemberships(lane keyMembershipLane, completion RootAssignmentCompletion) (keyMembershipLane, bool) {
	changed := false
	for _, membership := range completion.keyMemberships {
		var added bool
		lane, added = lane.add(membership)
		changed = changed || added
	}
	return lane, changed
}

// RootAssignmentFactorTransaction binds one completion to a ProductDomain.
// The seal prevents factors or transactions from crossing domain instances.
type RootAssignmentFactorTransaction struct {
	seal       *productDomainSeal
	completion RootAssignmentCompletion
}

// SealRootAssignmentCompletion binds completion to this exact product.
func (d ProductDomain) SealRootAssignmentCompletion(completion RootAssignmentCompletion) (RootAssignmentFactorTransaction, error) {
	if !d.Valid() || !completion.valid() {
		return RootAssignmentFactorTransaction{}, fmt.Errorf("state: invalid root-assignment factor transaction")
	}
	return RootAssignmentFactorTransaction{seal: d.seal, completion: completion}, nil
}

// ApplyRootAssignmentCompletion applies the same sealed per-factor laws used
// by guarded execution to a concrete product value. ProductDomain—not the
// default catalog—is the semantic authority, so custom and reordered catalogs
// retain their own registration and ordinal ownership.
func (d ProductDomain) ApplyRootAssignmentCompletion(transaction RootAssignmentFactorTransaction, current State) (State, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.completion.valid() {
		return State{}, fmt.Errorf("state: foreign root-assignment factor transaction")
	}
	out := d.Normalize(current)
	for index := range d.factorLanes {
		d.factorLanes[index].rootAssignment.applyState(&out, transaction.completion)
		for _, family := range d.factorLanes[index].coordinates {
			if family.ops.rootAssignment.applyCompletionLane == nil {
				continue
			}
			payload := d.factorLanes[index].ops.extract(out)
			next, changed := family.ops.rootAssignment.applyCompletionLane(payload, transaction.completion)
			if changed {
				d.factorLanes[index].ops.install(&out, next)
			}
		}
	}
	return out, nil
}

// RootAssignmentCompletionLanes returns the registered factors that may be
// changed by a caller-local completion, in catalog order.
func (d ProductDomain) RootAssignmentCompletionLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 2)
	for index := range d.factorLanes {
		if d.factorLanes[index].rootAssignment.completion {
			out = append(out, d.factorLanes[index].lane)
		}
	}
	return out
}

// RootAssignmentCompletionDependencies returns the exact upstream evidence
// declared by lane's completion registration. Lanes without a completion law
// return an empty descriptor.
func (d ProductDomain) RootAssignmentCompletionDependencies(lane ProductLane) (RootAssignmentCompletionDependencies, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return RootAssignmentCompletionDependencies{}, err
	}
	return runtime.rootAssignment.completionDependencies, nil
}

// ApplyRootAssignmentCompletionFactor applies one registered N4 completion
// law without composing a State. Unaffected factors retain operand identity.
func (d ProductDomain) ApplyRootAssignmentCompletionFactor(transaction RootAssignmentFactorTransaction, current LaneFactor) (LaneFactor, error) {
	if !d.Valid() || transaction.seal != d.seal || !transaction.completion.valid() {
		return LaneFactor{}, fmt.Errorf("state: foreign root-assignment factor transaction")
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	next, changed := runtime.rootAssignment.applyFactor(current.payload, transaction.completion)
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}
