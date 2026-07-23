package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// CoordinateFamilyID is the registration-local semantic name of one
// independently guarded family inside a State lane. It is descriptive only;
// dispatch is through sealed ProductDomain ordinals.
type CoordinateFamilyID string

// CoordinateFamilyOrdinal is a ProductLane-local position in registration
// order. It is not a global enum and imposes no family-count cap.
type CoordinateFamilyOrdinal int

// CoordinateFamily is an opaque ProductDomain-owned descriptor. Consumers
// obtain it from ProductDomain.CoordinateFamilies and cannot forge ownership.
type CoordinateFamily struct {
	seal    *productDomainSeal
	lane    ProductLane
	ordinal CoordinateFamilyOrdinal
	id      CoordinateFamilyID
}

func (f CoordinateFamily) ID() CoordinateFamilyID           { return f.id }
func (f CoordinateFamily) Lane() ProductLane                { return f.lane }
func (f CoordinateFamily) Ordinal() CoordinateFamilyOrdinal { return f.ordinal }

type coordinateSkeletonPayload interface{ isCoordinateSkeletonPayload() }
type coordinateKeyPayload interface{ isCoordinateKeyPayload() }
type coordinateScalarPayload interface{ isCoordinateScalarPayload() }
type coordinateSkeletonOverlayPlanPayload interface{ isCoordinateSkeletonOverlayPlanPayload() }

type typedCoordinateSkeletonPayload[T any] struct{ value T }
type typedCoordinateKeyPayload[T any] struct{ value T }
type typedCoordinateScalarPayload[T any] struct{ value T }
type typedCoordinateSkeletonOverlayPlanPayload[T any] struct{ value T }

func (typedCoordinateSkeletonPayload[T]) isCoordinateSkeletonPayload()                       {}
func (typedCoordinateKeyPayload[T]) isCoordinateKeyPayload()                                 {}
func (typedCoordinateScalarPayload[T]) isCoordinateScalarPayload()                           {}
func (typedCoordinateSkeletonOverlayPlanPayload[T]) isCoordinateSkeletonOverlayPlanPayload() {}

// CoordinateFamilySkeleton is the value-less quotient of one registered
// family. It owns global family states such as map Top/Bottom; key scalars are
// carried only by CoordinateScalarFactor.
type CoordinateFamilySkeleton struct {
	family  CoordinateFamily
	keys    *keyspace.KeySpace
	payload coordinateSkeletonPayload
}

func (s CoordinateFamilySkeleton) Family() CoordinateFamily     { return s.family }
func (s CoordinateFamilySkeleton) KeySpace() *keyspace.KeySpace { return s.keys }

// CoordinateSlot is an immutable family/key coordinate with no scalar value.
type CoordinateSlot struct {
	family CoordinateFamily
	keys   *keyspace.KeySpace
	key    coordinateKeyPayload
}

func (s CoordinateSlot) Family() CoordinateFamily { return s.family }

// CoordinateScalarFactor binds one opaque scalar-lattice value to a sealed
// family/key slot. The scalar spelling remains owned by lane registration.
type CoordinateScalarFactor struct {
	slot    CoordinateSlot
	payload coordinateScalarPayload
}

// CoordinateSkeletonOverlayPlan is one family-owned immutable sparse patch
// plan. Selection membership and family-specific grouping are paid once at
// seal time; DD leaf execution borrows canonical scalars and performs only a
// linear merge.
type CoordinateSkeletonOverlayPlan struct {
	seal    *productDomainSeal
	family  CoordinateFamily
	keys    *keyspace.KeySpace
	payload coordinateSkeletonOverlayPlanPayload
	whole   bool
}

func (f CoordinateScalarFactor) Slot() CoordinateSlot { return f.slot }

// CoordinateScalarSupport is the complete relation between a family
// skeleton and one scalar coordinate. Forbidden coordinates are outside the
// selected fiber, Optional coordinates may be omitted when equal to their
// registered default, and Required coordinates must remain explicit even when
// their scalar equals an ordinary lattice default.
type CoordinateScalarSupport uint8

const (
	CoordinateScalarForbidden CoordinateScalarSupport = iota
	CoordinateScalarOptional
	CoordinateScalarRequired
)

func (s CoordinateScalarSupport) valid() bool { return s <= CoordinateScalarRequired }

type coordinateEntry struct {
	key    coordinateKeyPayload
	scalar coordinateScalarPayload
}

// coordinateFiberPayload is a family-owned, comparable structural preimage
// identity. Boundary quotienting uses it to prove that every source member of
// a destination fiber contributed before publishing a must fact.
type coordinateFiberPayload interface{ isCoordinateFiberPayload() }

type typedCoordinateFiberPayload[T comparable] struct{ value T }

func (typedCoordinateFiberPayload[T]) isCoordinateFiberPayload() {}

// coordinateBoundaryAdmissionLaw is the sparse quotient law for deciding
// when a rebased destination coordinate has a complete fragment.  It is
// deliberately independent of IdentityImageLaw: identity substitution says
// how allocation identities move inside a carrier, while this law says how
// omitted coordinates participate when structural source fibers coalesce.
type coordinateBoundaryAdmissionLaw uint8

const (
	coordinateBoundaryAdmissionInvalid coordinateBoundaryAdmissionLaw = iota
	// AnyPresent is the existential image of a sparse may carrier: one present
	// source fiber establishes the destination coordinate.
	coordinateBoundaryAdmissionAnyPresent
	// AllPreimages is the universal image of a sparse must carrier: omission is
	// Top, so every structural inverse fiber must be present before publication.
	coordinateBoundaryAdmissionAllPreimages
)

func (l coordinateBoundaryAdmissionLaw) valid() bool {
	return l == coordinateBoundaryAdmissionAnyPresent || l == coordinateBoundaryAdmissionAllPreimages
}

// coordinateFamilyBoundaryOps is the mandatory boundary transaction law for
// a factored family. Every family declares projection, quotient, destination
// ownership, application, and root semantics through this one descriptor.
type coordinateFamilyBoundaryOps struct {
	admission       coordinateBoundaryAdmissionLaw
	rootUse         BoundaryRootUse
	reachabilityKey func(*boundaryReachabilityProgramBuilder, coordinateKeyPayload)
	projectSkeleton func(*boundaryProjectContext, coordinateSkeletonPayload) (coordinateSkeletonPayload, bool)
	projectKey      func(*boundaryProjectContext, coordinateKeyPayload) (coordinateKeyPayload, bool, bool)
	projectScalar   func(*boundaryProjectContext, coordinateKeyPayload, coordinateScalarPayload) (coordinateScalarPayload, bool)
	rebaseSkeleton  func(*boundaryRebaseContext, coordinateSkeletonPayload) (coordinateSkeletonPayload, bool)
	rebaseKeys      func(*boundaryRebaseContext, coordinateKeyPayload) ([]coordinateKeyPayload, bool)
	rebaseScalar    func(*boundaryRebaseContext, coordinateKeyPayload, coordinateScalarPayload) (coordinateScalarPayload, bool)
	sourceFiber     func(coordinateKeyPayload) coordinateFiberPayload
	inverseFibers   func(*boundaryRebaseContext, coordinateKeyPayload) ([]coordinateFiberPayload, bool)
	// postEntries is the family's static post-incidence relation. It may depend
	// only on sealed aliases; skeleton support decides candidate admission.
	postEntries   func([][2]keyspace.Key) []coordinateEntry
	applySkeleton func(*boundaryApplyContext, coordinateSkeletonPayload, coordinateSkeletonPayload) (coordinateSkeletonPayload, bool)
	// affectedSelector is the one declarative destination-ownership law for
	// closure replacement. The sealed selector drives both runtime application
	// and static wake incidences; families cannot scan an inventory or State.
	affectedSelector  func(*boundaryAffectedSelectorBuilder, coordinateKeyPayload)
	applyScalar       func(coordinateKeyPayload, coordinateScalarPayload, coordinateScalarPayload, bool) (coordinateScalarPayload, bool)
	applyRootSkeleton func(*boundaryApplyContext, coordinateSkeletonPayload, bool) (coordinateSkeletonPayload, bool)
	rootSlot          func(*boundaryApplyContext, BoundaryFactorTarget) (coordinateKeyPayload, bool, bool)
	rootScalar        func(*boundaryApplyContext, coordinateKeyPayload, product.Value) (coordinateScalarPayload, bool)
}

type coordinateFamilySpec struct {
	id            CoordinateFamilyID
	build         func(*axis.Registry, DomainOptions) coordinateFamilyOps
	boundary      coordinateFamilyBoundaryOps
	dynamicRead   coordinateDynamicReadPolicy
	identityImage IdentityImageLaw
}

type coordinateFamilyOps struct {
	decompose func(laneFactorPayload, *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, error)
	replace   func(laneFactorPayload, *keyspace.KeySpace, coordinateSkeletonPayload, []coordinateEntry) (laneFactorPayload, error)
	// inventoryCompletion is the family's registered finite monotone
	// consequence relation over admitted keys. Every family registers the law
	// explicitly, including families with no consequences, so a new family
	// cannot silently bypass inventory completion. A consequence remains in the
	// owning family and cannot inspect runtime State or family names.
	inventoryCompletion coordinateInventoryCompletionLaw
	// requiredScalarKeys enumerates the complete finite fiber which the
	// skeleton requires to be explicit.  It is the inverse of scalarSupport
	// for Required coordinates and lets structural transactions prove totality
	// without inspecting an encoded lane value.
	requiredScalarKeys func(coordinateSkeletonPayload) []coordinateKeyPayload
	// sealSkeletonInventory applies the family's order-neutral, sound
	// fail-closed quotient after boundary routing. It may neither introduce
	// structure nor require a coordinate outside admitted or an explicit
	// conservative post witness returned with the skeleton. The law is
	// deliberately not expressed as <=: family skeleton orders differ.
	sealSkeletonInventory func(coordinateSkeletonPayload, []coordinateKeyPayload, *keyspace.KeySpace) (coordinateSkeletonPayload, []coordinateEntry, bool)
	// selectionSupport declares the complete structural support required to
	// retain an exact selected component. Nil means scalar selection has no
	// additional structural dependency beyond the selected keys themselves.
	// The selector validates this before applying the family's narrowing law so
	// a missing component cannot be silently reclassified as absent.
	selectionSupport func(coordinateSkeletonPayload, []coordinateKeyPayload) ([]coordinateKeyPayload, bool)
	// overlaySelectedSkeleton is the family-owned exact carrier law used when a
	// sparse factor image is patched back onto a complete carrier. For every
	// selected key, support and omission come from image; support for every
	// unselected key remains current. Family-global markers are recomputed from
	// the selected image plus unselected explicit support.
	sealSelectedSkeletonOverlay func(selected []coordinateKeyPayload, keys *keyspace.KeySpace) (coordinateSkeletonOverlayPlanPayload, bool)
	overlaySelectedSkeleton     func(plan coordinateSkeletonOverlayPlanPayload, current, image coordinateSkeletonPayload, currentScalars []CoordinateScalarFactor, keys *keyspace.KeySpace) (coordinateSkeletonPayload, bool)

	skeletonBottom   func() coordinateSkeletonPayload
	skeletonTop      func() coordinateSkeletonPayload
	skeletonEqual    func(coordinateSkeletonPayload, coordinateSkeletonPayload) bool
	skeletonLessOrEq func(coordinateSkeletonPayload, coordinateSkeletonPayload) bool
	skeletonJoin     func(coordinateSkeletonPayload, coordinateSkeletonPayload) coordinateSkeletonPayload
	skeletonMeet     func(coordinateSkeletonPayload, coordinateSkeletonPayload) coordinateSkeletonPayload
	skeletonWiden    func(coordinateSkeletonPayload, coordinateSkeletonPayload) coordinateSkeletonPayload
	skeletonNarrow   func(coordinateSkeletonPayload, coordinateSkeletonPayload) coordinateSkeletonPayload
	skeletonHash     func(coordinateSkeletonPayload) uint64
	// skeletonRepresentationEqual/Hash identify the canonical retained
	// factorization, which may be finer than the skeleton's semantic quotient.
	// Equal representations are substitutable for scalarSupport and therefore
	// safe to share as one guarded terminal. Families whose semantic equality
	// already has that property register the semantic pair directly.
	skeletonRepresentationEqual func(coordinateSkeletonPayload, coordinateSkeletonPayload) bool
	skeletonRepresentationHash  func(coordinateSkeletonPayload) uint64
	importSkeleton              func(coordinateSkeletonPayload, *keyspace.KeySpace, *keyspace.KeySpace) (coordinateSkeletonPayload, bool)

	keyValid               func(coordinateKeyPayload, *keyspace.KeySpace) bool
	keyEqual               func(coordinateKeyPayload, coordinateKeyPayload) bool
	keyLess                func(coordinateKeyPayload, coordinateKeyPayload, *keyspace.KeySpace) bool
	keyHash                func(coordinateKeyPayload, *keyspace.KeySpace) uint64
	importKey              func(coordinateKeyPayload, *keyspace.KeySpace, *keyspace.KeySpace) (coordinateKeyPayload, bool)
	visitValueDependencies func(coordinateKeyPayload, *keyspace.KeySpace, func(statekey.ValueDependency))

	defaultScalar func(coordinateSkeletonPayload, coordinateKeyPayload) (coordinateScalarPayload, error)
	scalarSupport func(coordinateSkeletonPayload, coordinateKeyPayload) CoordinateScalarSupport
	scalarValid   func(coordinateKeyPayload, coordinateScalarPayload) bool
	scalarEqual   func(coordinateScalarPayload, coordinateScalarPayload) bool
	// scalarRepresentationEqual is the canonical terminal collision law,
	// distinct from any storage-sharing fast path.
	scalarRepresentationEqual func(coordinateScalarPayload, coordinateScalarPayload) bool
	scalarLessOrEq            func(coordinateScalarPayload, coordinateScalarPayload) bool
	scalarJoin                func(coordinateScalarPayload, coordinateScalarPayload) coordinateScalarPayload
	scalarMeet                func(coordinateScalarPayload, coordinateScalarPayload) coordinateScalarPayload
	scalarWiden               func(coordinateScalarPayload, coordinateScalarPayload) coordinateScalarPayload
	scalarNarrow              func(coordinateScalarPayload, coordinateScalarPayload) coordinateScalarPayload
	scalarHash                func(coordinateScalarPayload) uint64
	importScalar              func(coordinateScalarPayload) (coordinateScalarPayload, bool)
	// formalRekey is the mandatory registration-owned conversion from concrete
	// lexical roots to full formal.Root identity. Families explicitly declare
	// either key independence or a complete structural mapper.
	formalRekey coordinateFormalRekeyPolicy
	// branchRelation is the mandatory registered scalar-mutation capability
	// used by prepared branch atoms.  It is either an explicit independence
	// proof or one closed coordinate operation; orchestration never switches on
	// lane/family identity.
	branchRelation coordinateBranchRelationOps
	// reachableDefault is the skeleton-independent omitted scalar for exact
	// keyed queries. Nil means family topology is required to derive default.
	reachableDefault func(coordinateKeyPayload) (coordinateScalarPayload, bool)

	// returnIdentity is the registered inter-axis protocol used by normal
	// return publication. Every family declares it explicitly; families which
	// do not participate install the exact no-op protocol. The solver never
	// dispatches on a lane or family name.
	returnIdentity coordinateReturnIdentityOps

	// pathEvidence declares whether this family uniquely owns the persistent
	// coordinate path-evidence carrier. Presence implications, branch proofs,
	// equality and subtree mutation all consume this one registered storage
	// capability; none may nominate a second owner.
	pathEvidence coordinatePathEvidenceOps

	// pathValues declares whether this family uniquely owns point-local path
	// value reads. This is an evaluator dependency role, not a lane name.
	pathValues coordinatePathValueOps

	// rootAssignment declares the representation-owned observations used by
	// caller-local N4 completion.  Every family declares this role explicitly;
	// exactly one enabled family may own fresh-empty container evidence.
	rootAssignment coordinateRootAssignmentOps

	// pathMutation is the registered destructive path-replacement law for
	// independently transposed coordinate families.
	pathMutation coordinatePathMutationOps

	// objectMutation is the registered factorwise object-graph mutation law.
	// Constructors replace one object graph while allocation templates join a
	// recursive graph contribution. Every family declares an exact participant
	// or exact no-op; orchestration never selects Heap/Placement by name.
	objectMutation coordinateObjectMutationOps
}

// CoordinateReachableDefault returns the registered omitted scalar for an
// already-reachable family without observing its global topology root.
func (d ProductDomain) CoordinateReachableDefault(slot CoordinateSlot) (CoordinateScalarFactor, bool, error) {
	coordinate, err := d.validateCoordinateFamily(slot.family)
	if err != nil || d.validateCoordinateSlotFor(coordinate, slot, slot.keys) != nil {
		return CoordinateScalarFactor{}, false, ErrInvalidLaneFactor
	}
	if coordinate.ops.reachableDefault == nil {
		return CoordinateScalarFactor{}, false, nil
	}
	payload, ok := coordinate.ops.reachableDefault(slot.key)
	if !ok || payload == nil || !coordinate.ops.scalarValid(slot.key, payload) {
		return CoordinateScalarFactor{}, false, ErrInvalidLaneFactor
	}
	return CoordinateScalarFactor{slot: slot, payload: payload}, true, nil
}

// CoordinateReturnIdentityRole is one independently registered operation in
// the returned-identity closure. A family may own several roles; the sealed
// ProductDomain role set, rather than family order or a lane name, is the sole
// dispatch authority for both concrete and formal N5.
type CoordinateReturnIdentityRole uint8

const (
	CoordinateReturnIdentitySeed CoordinateReturnIdentityRole = 1 << iota
	CoordinateReturnIdentitySkeletonEdge
	CoordinateReturnIdentityScalarEdge
	CoordinateReturnIdentityContainer
	CoordinateReturnIdentityPublisher
)

const coordinateReturnIdentityAllRoles = CoordinateReturnIdentitySeed |
	CoordinateReturnIdentitySkeletonEdge |
	CoordinateReturnIdentityScalarEdge |
	CoordinateReturnIdentityContainer |
	CoordinateReturnIdentityPublisher

type coordinateReturnIdentityRoleBits uint8

func coordinateReturnIdentityRoles(roles ...CoordinateReturnIdentityRole) coordinateReturnIdentityRoleBits {
	var out CoordinateReturnIdentityRole
	for _, role := range roles {
		out |= role
	}
	return coordinateReturnIdentityRoleBits(out)
}

func (r coordinateReturnIdentityRoleBits) valid() bool {
	return CoordinateReturnIdentityRole(r)&^coordinateReturnIdentityAllRoles == 0
}

func (r coordinateReturnIdentityRoleBits) has(role CoordinateReturnIdentityRole) bool {
	return role != 0 && role&^coordinateReturnIdentityAllRoles == 0 && CoordinateReturnIdentityRole(r)&role == role
}

type coordinateReturnIdentityOps struct {
	roles               coordinateReturnIdentityRoleBits
	visitInventoryTerms func(coordinateKeyPayload, func(identity.Term) bool) bool
	visitTermKeys       func(identity.Term, func(coordinateKeyPayload) bool) bool
	imageInventoryKey   func(coordinateKeyPayload, *CoordinateIdentityTermImage, func(coordinateKeyPayload) bool) bool
	visitSeedRoots      func(coordinateSkeletonPayload, func(identity.Term) bool)
	visitSkeletonEdges  func(coordinateSkeletonPayload, func(identity.Term, identity.Term) bool)
	visitScalarEdges    func(coordinateKeyPayload, coordinateScalarPayload, func(identity.Term, identity.Term) bool)
	containerScalar     func(coordinateKeyPayload, coordinateScalarPayload) (identity.Term, product.Value, bool)
	visitContainerFacts func(coordinateSkeletonPayload, identity.Term, func(dynamicindex.Fact)) bool
	publicationKey      func(identity.Term) (coordinateKeyPayload, bool)
	publishScalar       func(coordinateKeyPayload, coordinateScalarPayload, placement.Value) (coordinateScalarPayload, bool)
}

func noCoordinateReturnIdentity() coordinateReturnIdentityOps {
	return coordinateReturnIdentityOps{
		roles: 0,
		visitInventoryTerms: func(coordinateKeyPayload, func(identity.Term) bool) bool {
			return true
		},
		visitTermKeys: func(identity.Term, func(coordinateKeyPayload) bool) bool { return true },
		imageInventoryKey: func(key coordinateKeyPayload, _ *CoordinateIdentityTermImage, emit func(coordinateKeyPayload) bool) bool {
			return emit(key)
		},
		visitSeedRoots:     func(coordinateSkeletonPayload, func(identity.Term) bool) {},
		visitSkeletonEdges: func(coordinateSkeletonPayload, func(identity.Term, identity.Term) bool) {},
		visitScalarEdges:   func(coordinateKeyPayload, coordinateScalarPayload, func(identity.Term, identity.Term) bool) {},
		containerScalar: func(coordinateKeyPayload, coordinateScalarPayload) (identity.Term, product.Value, bool) {
			return identity.Term{}, product.Value{}, false
		},
		visitContainerFacts: func(coordinateSkeletonPayload, identity.Term, func(dynamicindex.Fact)) bool { return false },
		publicationKey:      func(identity.Term) (coordinateKeyPayload, bool) { return nil, false },
		publishScalar: func(coordinateKeyPayload, coordinateScalarPayload, placement.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	}
}

func coordinateReturnIdentityOpsComplete(ops coordinateReturnIdentityOps) bool {
	return ops.roles.valid() &&
		ops.visitInventoryTerms != nil && ops.visitTermKeys != nil && ops.imageInventoryKey != nil &&
		ops.visitSeedRoots != nil && ops.visitSkeletonEdges != nil && ops.visitScalarEdges != nil &&
		ops.containerScalar != nil && ops.visitContainerFacts != nil && ops.publicationKey != nil && ops.publishScalar != nil
}

type coordinateFamilyRuntime struct {
	family        CoordinateFamily
	ops           coordinateFamilyOps
	boundary      coordinateFamilyBoundaryOps
	dynamicRead   coordinateDynamicReadPolicy
	identityImage IdentityImageLaw
}

// boundaryTargetRequiredFibers is the registry-owned Horn antecedent for one
// projected coordinate target. A sparse may coordinate is established by any
// one present preimage; collisions still remain separate scalar wires and are
// joined by the boundary evaluator. A sparse must coordinate retains the
// family's complete inverse fiber, so must facts cannot escape before all
// structural preimages are present.
//
// present is the source fiber which produced this candidate target.  Keeping
// this law on the sealed family runtime makes admission independent of lane
// names and opaque key representations, and gives static footprint discovery
// and concrete boundary routing exactly one definition.
func (c *coordinateFamilyRuntime) boundaryTargetRequiredFibers(
	ctx *boundaryRebaseContext,
	destination coordinateKeyPayload,
	present coordinateFiberPayload,
) ([]coordinateFiberPayload, bool) {
	if !c.boundary.admission.valid() || ctx == nil || destination == nil || present == nil {
		return nil, false
	}
	if c.boundary.admission == coordinateBoundaryAdmissionAnyPresent {
		return []coordinateFiberPayload{present}, true
	}
	return c.boundary.inverseFibers(ctx, destination)
}

func coordinateFamilyOpsComplete(ops coordinateFamilyOps) bool {
	return ops.decompose != nil && ops.replace != nil && ops.inventoryCompletion.valid() &&
		ops.requiredScalarKeys != nil && ops.sealSkeletonInventory != nil && ops.sealSelectedSkeletonOverlay != nil && ops.overlaySelectedSkeleton != nil &&
		ops.skeletonBottom != nil && ops.skeletonTop != nil && ops.skeletonEqual != nil && ops.skeletonLessOrEq != nil &&
		ops.skeletonJoin != nil && ops.skeletonMeet != nil && ops.skeletonWiden != nil && ops.skeletonNarrow != nil && ops.importSkeleton != nil &&
		ops.skeletonHash != nil && ops.skeletonRepresentationEqual != nil && ops.skeletonRepresentationHash != nil &&
		ops.keyValid != nil && ops.keyEqual != nil && ops.keyLess != nil && ops.keyHash != nil && ops.importKey != nil && ops.visitValueDependencies != nil && ops.defaultScalar != nil && ops.scalarSupport != nil && ops.scalarValid != nil &&
		ops.scalarEqual != nil && ops.scalarRepresentationEqual != nil && ops.scalarLessOrEq != nil && ops.scalarJoin != nil && ops.scalarMeet != nil &&
		ops.scalarWiden != nil && ops.scalarNarrow != nil && ops.scalarHash != nil && ops.importScalar != nil &&
		coordinateFormalRekeyPolicyComplete(ops.formalRekey) &&
		coordinateBranchRelationOpsComplete(ops.branchRelation) &&
		coordinateReturnIdentityOpsComplete(ops.returnIdentity) &&
		coordinatePathEvidenceOpsComplete(ops.pathEvidence) &&
		coordinatePathValueOpsComplete(ops.pathValues) &&
		coordinateRootAssignmentOpsComplete(ops.rootAssignment) &&
		coordinatePathMutationOpsComplete(ops.pathMutation) &&
		coordinateObjectMutationOpsComplete(ops.objectMutation)
}

// OverlaySelectedCoordinateSkeleton returns the exact family skeleton formed
// by taking support/omission for selected slots from image and structurally
// carrying every unselected slot from current. The operation is dispatched
// exclusively through the sealed family registration; ProductDomain never
// switches on an axis or family identity.
func (d ProductDomain) SealCoordinateSkeletonOverlayPlan(selected []CoordinateSlot) (CoordinateSkeletonOverlayPlan, error) {
	if !d.Valid() || len(selected) == 0 || selected[0].keys == nil {
		return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: coordinate skeleton overlay plan", ErrInvalidLaneFactor)
	}
	return d.sealCoordinateSkeletonOverlayPlan(selected[0].family, selected[0].keys, selected, false)
}

func (d ProductDomain) sealCoordinateSkeletonOverlayPlan(
	family CoordinateFamily,
	keys *keyspace.KeySpace,
	selected []CoordinateSlot,
	whole bool,
) (CoordinateSkeletonOverlayPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || whole && len(selected) != 0 {
		return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: coordinate skeleton overlay plan", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return CoordinateSkeletonOverlayPlan{}, err
	}
	if whole {
		return CoordinateSkeletonOverlayPlan{seal: d.seal, family: coordinate.family, keys: keys, whole: true}, nil
	}
	if len(selected) == 0 {
		return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: empty scalar coordinate overlay", ErrInvalidLaneFactor)
	}
	keysPayload := make([]coordinateKeyPayload, len(selected))
	for index, slot := range selected {
		if d.validateCoordinateSlotFor(coordinate, slot, keys) != nil {
			return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: selected coordinate skeleton slot %d", ErrInvalidLaneFactor, index)
		}
		if index != 0 && !coordinate.ops.keyLess(selected[index-1].key, slot.key, keys) {
			return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: selected coordinate skeleton slots are not canonical", ErrInvalidLaneFactor)
		}
		keysPayload[index] = slot.key
	}
	payload, ok := coordinate.ops.sealSelectedSkeletonOverlay(keysPayload, keys)
	if !ok || payload == nil {
		return CoordinateSkeletonOverlayPlan{}, fmt.Errorf("%w: coordinate skeleton overlay plan sealing", ErrInvalidLaneFactor)
	}
	return CoordinateSkeletonOverlayPlan{seal: d.seal, family: coordinate.family, keys: keys, payload: payload}, nil
}

func (d ProductDomain) OverlaySelectedCoordinateSkeleton(
	plan CoordinateSkeletonOverlayPlan,
	current, image CoordinateFamilySkeleton,
	currentScalars []CoordinateScalarFactor,
) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(current, image)
	if err != nil || plan.seal != d.seal || plan.family != current.family || plan.keys != current.keys || !plan.whole && plan.payload == nil {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: coordinate skeleton overlay plan authority", ErrInvalidLaneFactor)
	}
	if plan.whole {
		return image, nil
	}
	payload, ok := coordinate.ops.overlaySelectedSkeleton(plan.payload, current.payload, image.payload, currentScalars, current.keys)
	if !ok || payload == nil {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: selected coordinate skeleton overlay", ErrInvalidLaneFactor)
	}
	keys := current.keys
	out := CoordinateFamilySkeleton{family: coordinate.family, keys: keys, payload: payload}
	if err := d.validateCoordinateSkeletonFor(coordinate, out, keys); err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	return out, nil
}

func withSemanticSkeletonRepresentation(ops coordinateFamilyOps) coordinateFamilyOps {
	ops.skeletonRepresentationEqual = ops.skeletonEqual
	ops.skeletonRepresentationHash = ops.skeletonHash
	ops.scalarRepresentationEqual = ops.scalarEqual
	return ops
}

type coordinateInventoryCompletionKind uint8

const (
	coordinateInventoryCompletionInvalid coordinateInventoryCompletionKind = iota
	coordinateInventoryCompletionNone
	coordinateInventoryCompletionConsequences
)

// coordinateInventoryCompletionLaw makes key consequences an explicit family
// capability. emit must enumerate the finite immediate consequences of one
// admitted key. CloseCoordinateFactorInventory owns transitive completion and
// therefore applies every newly admitted key exactly once. kind preserves the
// zero-allocation exact-return path when every present family declares None;
// it is semantic registration data, never inferred from a family name.
type coordinateInventoryCompletionLaw struct {
	kind coordinateInventoryCompletionKind
	emit func(*keyspace.KeySpace, coordinateKeyPayload, func(coordinateKeyPayload) bool) bool
}

func (l coordinateInventoryCompletionLaw) valid() bool {
	return (l.kind == coordinateInventoryCompletionNone || l.kind == coordinateInventoryCompletionConsequences) && l.emit != nil
}

func noCoordinateInventoryCompletions() coordinateInventoryCompletionLaw {
	return coordinateInventoryCompletionLaw{
		kind: coordinateInventoryCompletionNone,
		emit: func(*keyspace.KeySpace, coordinateKeyPayload, func(coordinateKeyPayload) bool) bool {
			return true
		},
	}
}

func coordinateBoundaryOpsComplete(ops coordinateFamilyBoundaryOps) bool {
	return ops.admission.valid() && ops.rootUse.declared && ops.reachabilityKey != nil && ops.projectSkeleton != nil && ops.projectKey != nil && ops.projectScalar != nil && ops.rebaseSkeleton != nil && ops.rebaseKeys != nil &&
		ops.rebaseScalar != nil && ops.sourceFiber != nil && ops.inverseFibers != nil && ops.postEntries != nil &&
		ops.applySkeleton != nil && ops.affectedSelector != nil && ops.applyScalar != nil && ops.applyRootSkeleton != nil && ops.rootSlot != nil && ops.rootScalar != nil
}

func noCoordinatePostEntries([][2]keyspace.Key) []coordinateEntry { return nil }

func (ops coordinateFamilyBoundaryOps) sealAffectedSelector(keys *keyspace.KeySpace, key coordinateKeyPayload) (boundaryAffectedSelector, error) {
	if ops.affectedSelector == nil || keys == nil || !keys.Valid() || key == nil {
		return boundaryAffectedSelector{}, fmt.Errorf("%w: coordinate boundary affected selector is unowned", ErrInvalidLaneFactor)
	}
	builder := newBoundaryAffectedSelectorBuilder(keys)
	ops.affectedSelector(builder, key)
	return builder.seal()
}

// CoordinateBoundaryRootUse returns the exact registered root dependency of
// one coordinate family. It is the factorwise analogue of BoundaryRootUse.
func (d ProductDomain) CoordinateBoundaryRootUse(family CoordinateFamily) (BoundaryRootUse, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || !coordinate.boundary.rootUse.declared {
		if err == nil {
			err = ErrIncompleteLaneFactors
		}
		return BoundaryRootUse{}, err
	}
	return coordinate.boundary.rootUse, nil
}

// CoordinateFamilies returns the registered coordinate inventory for lane in
// stable family order. Atomic lanes return nil.
func (d ProductDomain) CoordinateFamilies(lane ProductLane) ([]CoordinateFamily, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return nil, err
	}
	out := make([]CoordinateFamily, len(runtime.coordinates))
	for index := range runtime.coordinates {
		out[index] = runtime.coordinates[index].family
	}
	return out, nil
}

// CoordinateIdentityImageLaw returns the registered exact substitution law
// for one transposed coordinate family without inspecting its opaque key or
// scalar payload.
func (d ProductDomain) CoordinateIdentityImageLaw(family CoordinateFamily) (IdentityImageLaw, error) {
	runtime, err := d.validateCoordinateFamily(family)
	if err != nil {
		return IdentityImageInvalid, err
	}
	return runtime.identityImage, nil
}

// CoordinateSkeletonBottom returns the registered family-bottom quotient.
func (d ProductDomain) CoordinateSkeletonBottom(family CoordinateFamily, keys *keyspace.KeySpace) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	payload, post, ok := coordinate.ops.sealSkeletonInventory(coordinate.ops.skeletonBottom(), nil, keys)
	if !ok || payload == nil || len(post) != 0 {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: coordinate skeleton Bottom", ErrInvalidLaneFactor)
	}
	return CoordinateFamilySkeleton{family: coordinate.family, keys: keys, payload: payload}, nil
}

// CoordinateSkeletonTop returns the registered family-top quotient.
func (d ProductDomain) CoordinateSkeletonTop(family CoordinateFamily, keys *keyspace.KeySpace) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	payload, post, ok := coordinate.ops.sealSkeletonInventory(coordinate.ops.skeletonTop(), nil, keys)
	if !ok || payload == nil || len(post) != 0 {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: coordinate skeleton Top", ErrInvalidLaneFactor)
	}
	return CoordinateFamilySkeleton{family: coordinate.family, keys: keys, payload: payload}, nil
}

// DecomposeCoordinateFamily extracts one family skeleton and its explicitly
// stored scalar coordinates in exact registered key order.
func (d ProductDomain) DecomposeCoordinateFamily(factor LaneFactor, family CoordinateFamily, keys *keyspace.KeySpace) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	runtime, coordinate, err := d.validateCoordinateFamilyFactor(factor, family)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}
	skeleton, entries, err := coordinate.ops.decompose(factor.payload, keys)
	if err != nil || skeleton == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: coordinate family %q decomposition", err, family.id)
	}
	out := make([]CoordinateScalarFactor, len(entries))
	for index, entry := range entries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: coordinate family %q entry %d", ErrInvalidLaneFactor, family.id, index)
		}
		if index != 0 && !coordinate.ops.keyLess(entries[index-1].key, entry.key, keys) {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: coordinate family %q is not in strict key order", ErrInvalidLaneFactor, family.id)
		}
		if support := coordinate.ops.scalarSupport(skeleton, entry.key); !support.valid() || support == CoordinateScalarForbidden {
			return CoordinateFamilySkeleton{}, nil, fmt.Errorf("%w: coordinate family %q entry %d is outside its skeleton fiber", ErrInvalidLaneFactor, family.id, index)
		}
		slot := CoordinateSlot{family: coordinate.family, keys: keys, key: entry.key}
		out[index] = CoordinateScalarFactor{slot: slot, payload: entry.scalar}
	}
	_ = runtime
	return CoordinateFamilySkeleton{family: coordinate.family, keys: keys, payload: skeleton}, out, nil
}

// ComposeCoordinateFamilies is the exact inverse of per-family decomposition.
// It requires one skeleton for every registered family and rejects missing,
// duplicated, reordered, foreign, or non-canonical scalar coordinates.
func (d ProductDomain) ComposeCoordinateFamilies(lane ProductLane, keys *keyspace.KeySpace, skeletons []CoordinateFamilySkeleton, factors [][]CoordinateScalarFactor) (LaneFactor, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return LaneFactor{}, err
	}
	if len(skeletons) != len(runtime.coordinates) || len(factors) != len(runtime.coordinates) {
		return LaneFactor{}, fmt.Errorf("%w: incomplete coordinate family inventory", ErrIncompleteLaneFactors)
	}
	payload := runtime.ops.bottom()
	for index := range runtime.coordinates {
		coordinate := &runtime.coordinates[index]
		skeleton := skeletons[index]
		if err := d.validateCoordinateSkeletonFor(coordinate, skeleton, keys); err != nil {
			return LaneFactor{}, err
		}
		entries := make([]coordinateEntry, len(factors[index]))
		for entryIndex, factor := range factors[index] {
			if err := d.validateCoordinateFactorFor(coordinate, factor, keys); err != nil {
				return LaneFactor{}, err
			}
			if entryIndex != 0 && !coordinate.ops.keyLess(entries[entryIndex-1].key, factor.slot.key, keys) {
				return LaneFactor{}, fmt.Errorf("%w: coordinate family %q is not in strict key order", ErrInvalidLaneFactor, coordinate.family.id)
			}
			if support := coordinate.ops.scalarSupport(skeleton.payload, factor.slot.key); !support.valid() || support == CoordinateScalarForbidden {
				return LaneFactor{}, fmt.Errorf("%w: coordinate family %q entry %d is outside its skeleton fiber", ErrInvalidLaneFactor, coordinate.family.id, entryIndex)
			}
			entries[entryIndex] = coordinateEntry{key: factor.slot.key, scalar: factor.payload}
		}
		payload, err = coordinate.ops.replace(payload, keys, skeleton.payload, entries)
		if err != nil {
			return LaneFactor{}, fmt.Errorf("%w: coordinate family %q composition: %v", ErrInvalidLaneFactor, coordinate.family.id, err)
		}
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// CoordinateDefault returns the exact omitted-coordinate scalar for slot under
// skeleton. Defaults remain explicit opaque terminals, so a must-map's absent
// key cannot be confused with an explicitly stored element Top.
func (d ProductDomain) CoordinateDefault(skeleton CoordinateFamilySkeleton, slot CoordinateSlot) (CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	if err := d.validateCoordinateSlotFor(coordinate, slot, skeleton.keys); err != nil {
		return CoordinateScalarFactor{}, err
	}
	payload, err := coordinate.ops.defaultScalar(skeleton.payload, slot.key)
	if err != nil || payload == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return CoordinateScalarFactor{}, err
	}
	return CoordinateScalarFactor{slot: slot, payload: payload}, nil
}

// CoordinateSlotEqual reports exact semantic slot equality across the
// registered product inventory. Distinct registered families own disjoint
// coordinate namespaces; only slots in the same family invoke its key law.
func (d ProductDomain) CoordinateSlotEqual(left, right CoordinateSlot) (bool, error) {
	leftCoordinate, leftErr := d.validateCoordinateFamily(left.family)
	rightCoordinate, rightErr := d.validateCoordinateFamily(right.family)
	if leftErr != nil || rightErr != nil ||
		d.validateCoordinateSlotFor(leftCoordinate, left, left.keys) != nil ||
		d.validateCoordinateSlotFor(rightCoordinate, right, right.keys) != nil {
		return false, fmt.Errorf("%w: incompatible coordinate slots", ErrInvalidLaneFactor)
	}
	if left.family != right.family {
		return false, nil
	}
	if left.keys != right.keys {
		return false, fmt.Errorf("%w: incompatible coordinate slots", ErrInvalidLaneFactor)
	}
	return leftCoordinate.ops.keyEqual(left.key, right.key), nil
}

// CoordinateSlotLess is the registered total order used for deterministic
// product inventories. Registration ordinals order distinct families; the
// family-owned key order applies only within one family and keyspace.
func (d ProductDomain) CoordinateSlotLess(left, right CoordinateSlot) (bool, error) {
	leftCoordinate, leftErr := d.validateCoordinateFamily(left.family)
	rightCoordinate, rightErr := d.validateCoordinateFamily(right.family)
	if leftErr != nil || rightErr != nil ||
		d.validateCoordinateSlotFor(leftCoordinate, left, left.keys) != nil ||
		d.validateCoordinateSlotFor(rightCoordinate, right, right.keys) != nil {
		return false, fmt.Errorf("%w: incompatible coordinate slots", ErrInvalidLaneFactor)
	}
	if left.family != right.family {
		if left.family.lane.ordinal != right.family.lane.ordinal {
			return left.family.lane.ordinal < right.family.lane.ordinal, nil
		}
		return left.family.ordinal < right.family.ordinal, nil
	}
	if left.keys != right.keys {
		return false, fmt.Errorf("%w: incompatible coordinate slots", ErrInvalidLaneFactor)
	}
	return leftCoordinate.ops.keyLess(left.key, right.key, left.keys), nil
}

// CoordinateSlotHash is the family-owned structural hash of one coordinate
// key. Scalar interning must include it: many independent coordinates carry
// the same small scalar value, and payload-only hashing would collapse them
// into a linear equality bucket.
func (d ProductDomain) CoordinateSlotHash(slot CoordinateSlot) (uint64, error) {
	coordinate, err := d.validateCoordinateFamily(slot.family)
	if err != nil || slot.key == nil || !coordinate.ops.keyValid(slot.key, slot.keys) {
		return 0, fmt.Errorf("%w: invalid coordinate slot", ErrInvalidLaneFactor)
	}
	return coordinate.ops.keyHash(slot.key, slot.keys), nil
}

// OwnsCoordinateScalarFactor reports whether factor is a complete scalar of
// this product and the exact keyspace. This is the constant-space ownership
// predicate for already-sealed factors; callers must not manufacture a
// singleton CoordinateFactorInventory merely to prove the same fact.
func (d ProductDomain) OwnsCoordinateScalarFactor(keys *keyspace.KeySpace, factor CoordinateScalarFactor) bool {
	coordinate, err := d.validateCoordinateFamily(factor.slot.family)
	return err == nil && keys != nil && keys.Valid() &&
		d.validateCoordinateFactorFor(coordinate, factor, keys) == nil
}

// VisitCoordinateValueDependencies enumerates the finite concrete or formal
// Values roots whose identity is encoded by one registered coordinate key. The
// policy is family-owned, so a new coordinate family cannot silently bypass
// guarded component alignment.
func (d ProductDomain) VisitCoordinateValueDependencies(slot CoordinateSlot, visit func(statekey.ValueDependency)) error {
	coordinate, err := d.validateCoordinateFamily(slot.family)
	if err != nil || slot.key == nil || visit == nil || !coordinate.ops.keyValid(slot.key, slot.keys) {
		return fmt.Errorf("%w: invalid coordinate dependency visitation", ErrInvalidLaneFactor)
	}
	coordinate.ops.visitValueDependencies(slot.key, slot.keys, visit)
	return nil
}

func (d ProductDomain) CoordinateSkeletonJoin(left, right CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(left, right)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	left.payload = coordinate.ops.skeletonJoin(left.payload, right.payload)
	return left, nil
}

func (d ProductDomain) CoordinateSkeletonWiden(previous, next CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(previous, next)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	previous.payload = coordinate.ops.skeletonWiden(previous.payload, next.payload)
	return previous, nil
}

func (d ProductDomain) CoordinateSkeletonMeet(left, right CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(left, right)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	left.payload = coordinate.ops.skeletonMeet(left.payload, right.payload)
	return left, nil
}

func (d ProductDomain) CoordinateSkeletonNarrow(previous, next CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(previous, next)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	previous.payload = coordinate.ops.skeletonNarrow(previous.payload, next.payload)
	return previous, nil
}

func (d ProductDomain) CoordinateSkeletonEqual(left, right CoordinateFamilySkeleton) (bool, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(left, right)
	if err != nil {
		return false, err
	}
	return coordinate.ops.skeletonEqual(left.payload, right.payload), nil
}

func (d ProductDomain) CoordinateSkeletonLessOrEq(left, right CoordinateFamilySkeleton) (bool, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(left, right)
	if err != nil {
		return false, err
	}
	return coordinate.ops.skeletonLessOrEq(left.payload, right.payload), nil
}

func (d ProductDomain) CoordinateSkeletonHash(value CoordinateFamilySkeleton) (uint64, error) {
	coordinate, err := d.validateCoordinateSkeleton(value)
	if err != nil {
		return 0, err
	}
	return coordinate.ops.skeletonHash(value.payload), nil
}

// CoordinateSkeletonRepresentationEqual reports whether two skeletons are the
// same canonical retained factorization. This is deliberately stronger than
// semantic skeleton equality for families whose transient topology aligns
// scalar fibers: only representation-equal skeletons are substitutable for
// scalarSupport and may share a guarded terminal.
func (d ProductDomain) CoordinateSkeletonRepresentationEqual(left, right CoordinateFamilySkeleton) (bool, error) {
	coordinate, err := d.validateCoordinateSkeletonPair(left, right)
	if err != nil {
		return false, err
	}
	return coordinate.ops.skeletonRepresentationEqual(left.payload, right.payload), nil
}

// CoordinateSkeletonRepresentationHash is congruent with
// CoordinateSkeletonRepresentationEqual and keys guarded skeleton interning.
func (d ProductDomain) CoordinateSkeletonRepresentationHash(value CoordinateFamilySkeleton) (uint64, error) {
	coordinate, err := d.validateCoordinateSkeleton(value)
	if err != nil {
		return 0, err
	}
	return coordinate.ops.skeletonRepresentationHash(value.payload), nil
}

func (d ProductDomain) coordinateScalarBinary(left, right CoordinateScalarFactor, op func(coordinateFamilyOps, coordinateScalarPayload, coordinateScalarPayload) coordinateScalarPayload) (CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateFactorPair(left, right)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	left.payload = op(coordinate.ops, left.payload, right.payload)
	return left, nil
}

func (d ProductDomain) CoordinateScalarJoin(left, right CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	return d.coordinateScalarBinary(left, right, func(ops coordinateFamilyOps, a, b coordinateScalarPayload) coordinateScalarPayload {
		return ops.scalarJoin(a, b)
	})
}

func (d ProductDomain) CoordinateScalarMeet(left, right CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	return d.coordinateScalarBinary(left, right, func(ops coordinateFamilyOps, a, b coordinateScalarPayload) coordinateScalarPayload {
		return ops.scalarMeet(a, b)
	})
}

func (d ProductDomain) CoordinateScalarWiden(left, right CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	return d.coordinateScalarBinary(left, right, func(ops coordinateFamilyOps, a, b coordinateScalarPayload) coordinateScalarPayload {
		return ops.scalarWiden(a, b)
	})
}

func (d ProductDomain) CoordinateScalarNarrow(left, right CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	return d.coordinateScalarBinary(left, right, func(ops coordinateFamilyOps, a, b coordinateScalarPayload) coordinateScalarPayload {
		return ops.scalarNarrow(a, b)
	})
}

func (d ProductDomain) CoordinateScalarEqual(left, right CoordinateScalarFactor) (bool, error) {
	coordinate, err := d.validateCoordinateFactorPair(left, right)
	if err != nil {
		return false, err
	}
	return coordinate.ops.scalarEqual(left.payload, right.payload), nil
}

// CoordinateScalarRepresentationEqual is the canonical scalar-terminal
// collision proof registered by the owning coordinate family.
func (d ProductDomain) CoordinateScalarRepresentationEqual(left, right CoordinateScalarFactor) (bool, error) {
	coordinate, err := d.validateCoordinateFactorPair(left, right)
	if err != nil || coordinate.ops.scalarRepresentationEqual == nil {
		return false, err
	}
	return coordinate.ops.scalarRepresentationEqual(left.payload, right.payload), nil
}

func (d ProductDomain) CoordinateScalarLessOrEq(left, right CoordinateScalarFactor) (bool, error) {
	coordinate, err := d.validateCoordinateFactorPair(left, right)
	if err != nil {
		return false, err
	}
	return coordinate.ops.scalarLessOrEq(left.payload, right.payload), nil
}

func (d ProductDomain) CoordinateScalarHash(value CoordinateScalarFactor) (uint64, error) {
	coordinate, err := d.validateCoordinateFamily(value.slot.family)
	if err != nil || value.slot.key == nil || !coordinate.ops.keyValid(value.slot.key, value.slot.keys) ||
		value.payload == nil || !coordinate.ops.scalarValid(value.slot.key, value.payload) {
		return 0, fmt.Errorf("%w: invalid coordinate scalar", ErrInvalidLaneFactor)
	}
	// A coordinate scalar is keyed by (slot, payload), not payload alone. Must
	// sets and small finite axes commonly give thousands of independent slots
	// the same Present/Absent scalar; payload-only hashing degenerates their
	// interner into one linear CoordinateSlotEqual bucket.
	hash := uint64(1469598103934665603)
	hash = (hash ^ coordinate.ops.keyHash(value.slot.key, value.slot.keys)) * 1099511628211
	hash = (hash ^ coordinate.ops.scalarHash(value.payload)) * 1099511628211
	return hash, nil
}

// CoordinateScalarIsOmitted reports whether value has no explicit coordinate
// under skeleton. Forbidden coordinates are omitted unconditionally, Required
// coordinates never are, and Optional coordinates are omitted exactly when
// their scalar equals the family-owned default.
func (d ProductDomain) CoordinateScalarIsOmitted(skeleton CoordinateFamilySkeleton, value CoordinateScalarFactor) (bool, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || d.validateCoordinateFactorFor(coordinate, value, skeleton.keys) != nil || value.slot.family != skeleton.family {
		return false, fmt.Errorf("%w: incompatible coordinate scalar omission", ErrInvalidLaneFactor)
	}
	support := coordinate.ops.scalarSupport(skeleton.payload, value.slot.key)
	if !support.valid() {
		return false, fmt.Errorf("%w: invalid coordinate scalar support", ErrInvalidLaneFactor)
	}
	if support == CoordinateScalarForbidden {
		return true, nil
	}
	if support == CoordinateScalarRequired {
		return false, nil
	}
	defaultValue, err := d.CoordinateDefault(skeleton, value.slot)
	if err != nil {
		return false, err
	}
	return d.CoordinateScalarEqual(defaultValue, value)
}

// CoordinateScalarSupport reports the registered topology/fiber relation for
// slot under skeleton. ProductDomain, family, keyspace, and key provenance are
// validated before the family law is invoked.
func (d ProductDomain) CoordinateScalarSupport(skeleton CoordinateFamilySkeleton, slot CoordinateSlot) (CoordinateScalarSupport, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || slot.family != skeleton.family || slot.keys != skeleton.keys || slot.key == nil ||
		d.validateCoordinateSlotFor(coordinate, slot, skeleton.keys) != nil {
		return CoordinateScalarForbidden, fmt.Errorf("%w: incompatible coordinate scalar support", ErrInvalidLaneFactor)
	}
	support := coordinate.ops.scalarSupport(skeleton.payload, slot.key)
	if !support.valid() {
		return CoordinateScalarForbidden, fmt.Errorf("%w: invalid coordinate scalar support", ErrInvalidLaneFactor)
	}
	return support, nil
}

// ImportCoordinateSlot structurally re-seals a coordinate key. Keyspace-owned
// families import through their registration hook; free families validate and
// preserve the key spelling.
func (d ProductDomain) ImportCoordinateSlot(source CoordinateSlot, keys *keyspace.KeySpace) (CoordinateSlot, error) {
	coordinate, err := d.coordinateRuntimeForImport(source.family)
	if err != nil {
		return CoordinateSlot{}, err
	}
	key, ok := coordinate.ops.importKey(source.key, source.keys, keys)
	if !ok || key == nil || !coordinate.ops.keyValid(key, keys) {
		return CoordinateSlot{}, fmt.Errorf("%w: coordinate key import", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: coordinate.family, keys: keys, key: key}, nil
}

func (d ProductDomain) ImportCoordinateSkeleton(source CoordinateFamilySkeleton, keys *keyspace.KeySpace) (CoordinateFamilySkeleton, error) {
	coordinate, err := d.coordinateRuntimeForImport(source.family)
	if err != nil {
		return CoordinateFamilySkeleton{}, err
	}
	payload, ok := coordinate.ops.importSkeleton(source.payload, source.keys, keys)
	if !ok || payload == nil {
		return CoordinateFamilySkeleton{}, fmt.Errorf("%w: coordinate skeleton import", ErrInvalidLaneFactor)
	}
	return CoordinateFamilySkeleton{family: coordinate.family, keys: keys, payload: payload}, nil
}

func (d ProductDomain) ImportCoordinateScalar(source CoordinateScalarFactor, keys *keyspace.KeySpace) (CoordinateScalarFactor, error) {
	slot, err := d.ImportCoordinateSlot(source.slot, keys)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	coordinate, _ := d.validateCoordinateFamily(slot.family)
	payload, ok := coordinate.ops.importScalar(source.payload)
	if !ok || payload == nil {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: coordinate scalar import", ErrInvalidLaneFactor)
	}
	return CoordinateScalarFactor{slot: slot, payload: payload}, nil
}

func (d ProductDomain) validateCoordinateFamily(family CoordinateFamily) (*coordinateFamilyRuntime, error) {
	if !d.Valid() || family.seal == nil || family.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign coordinate family", ErrInvalidProductLane)
	}
	runtime, err := d.validateLane(family.lane)
	if err != nil || int(family.ordinal) < 0 || int(family.ordinal) >= len(runtime.coordinates) {
		return nil, fmt.Errorf("%w: invalid coordinate family ordinal", ErrInvalidProductLane)
	}
	coordinate := &runtime.coordinates[family.ordinal]
	if coordinate.family != family {
		return nil, fmt.Errorf("%w: coordinate family inventory drift", ErrInvalidProductLane)
	}
	return coordinate, nil
}

func (d ProductDomain) coordinateRuntimeForImport(source CoordinateFamily) (*coordinateFamilyRuntime, error) {
	if source.id == "" || source.lane.id == "" {
		return nil, fmt.Errorf("%w: empty coordinate family", ErrInvalidProductLane)
	}
	lane, ok := d.ProductLane(source.lane.id)
	if !ok {
		return nil, fmt.Errorf("%w: coordinate lane is disabled", ErrInvalidProductLane)
	}
	runtime, _ := d.validateLane(lane)
	for index := range runtime.coordinates {
		if runtime.coordinates[index].family.id == source.id {
			return &runtime.coordinates[index], nil
		}
	}
	return nil, fmt.Errorf("%w: coordinate family is unregistered", ErrInvalidProductLane)
}

func (d ProductDomain) validateCoordinateFamilyFactor(factor LaneFactor, family CoordinateFamily) (*productLaneRuntime, *coordinateFamilyRuntime, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return nil, nil, err
	}
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || coordinate.family.lane != runtime.lane {
		return nil, nil, fmt.Errorf("%w: coordinate family does not own lane factor", ErrInvalidLaneFactor)
	}
	return runtime, coordinate, nil
}

func (d ProductDomain) validateCoordinateSkeleton(skeleton CoordinateFamilySkeleton) (*coordinateFamilyRuntime, error) {
	coordinate, err := d.validateCoordinateFamily(skeleton.family)
	if err != nil || skeleton.payload == nil {
		return nil, fmt.Errorf("%w: invalid coordinate skeleton", ErrInvalidLaneFactor)
	}
	return coordinate, nil
}

func (d ProductDomain) validateCoordinateSkeletonFor(coordinate *coordinateFamilyRuntime, skeleton CoordinateFamilySkeleton, keys *keyspace.KeySpace) error {
	if coordinate == nil || skeleton.family != coordinate.family || skeleton.keys != keys || skeleton.payload == nil {
		return fmt.Errorf("%w: foreign coordinate skeleton", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateCoordinateSkeletonPair(left, right CoordinateFamilySkeleton) (*coordinateFamilyRuntime, error) {
	coordinate, err := d.validateCoordinateSkeleton(left)
	if err != nil || left.family != right.family || left.keys != right.keys || right.payload == nil {
		return nil, fmt.Errorf("%w: incompatible coordinate skeletons", ErrInvalidLaneFactor)
	}
	return coordinate, nil
}

func (d ProductDomain) validateCoordinateSlotFor(coordinate *coordinateFamilyRuntime, slot CoordinateSlot, keys *keyspace.KeySpace) error {
	if coordinate == nil || slot.family != coordinate.family || slot.keys != keys || slot.key == nil || !coordinate.ops.keyValid(slot.key, keys) {
		return fmt.Errorf("%w: invalid coordinate slot", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateCoordinateFactorFor(coordinate *coordinateFamilyRuntime, factor CoordinateScalarFactor, keys *keyspace.KeySpace) error {
	if err := d.validateCoordinateSlotFor(coordinate, factor.slot, keys); err != nil || factor.payload == nil || !coordinate.ops.scalarValid(factor.slot.key, factor.payload) {
		return fmt.Errorf("%w: invalid coordinate scalar", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateCoordinateFactorPair(left, right CoordinateScalarFactor) (*coordinateFamilyRuntime, error) {
	coordinate, err := d.validateCoordinateFamily(left.slot.family)
	if err != nil || left.slot.family != right.slot.family || left.slot.keys != right.slot.keys ||
		left.slot.key == nil || right.slot.key == nil || !coordinate.ops.keyEqual(left.slot.key, right.slot.key) ||
		left.payload == nil || right.payload == nil {
		return nil, fmt.Errorf("%w: incompatible coordinate scalars", ErrInvalidLaneFactor)
	}
	return coordinate, nil
}
