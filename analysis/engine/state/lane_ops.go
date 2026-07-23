package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

type laneKeySpaceMode uint8

const (
	laneKeySpaceInvalid laneKeySpaceMode = iota
	laneKeySpaceFree
	laneKeySpaceOwned
)

type laneFormalRekeyKind uint8

const (
	laneFormalRekeyInvalid laneFormalRekeyKind = iota
	laneFormalRekeyIndependent
	laneFormalRekeyStructural
)

// laneFormalRekeyPolicy certifies how one non-coordinate residual carrier
// crosses from concrete body keys into the typed formal keyspace.
type laneFormalRekeyPolicy struct{ kind laneFormalRekeyKind }

func formalRekeyIndependent() laneFormalRekeyPolicy {
	return laneFormalRekeyPolicy{kind: laneFormalRekeyIndependent}
}

func formalRekeyStructural() laneFormalRekeyPolicy {
	return laneFormalRekeyPolicy{kind: laneFormalRekeyStructural}
}

type laneValueDependencyKind uint8

const (
	laneValueDependenciesInvalid laneValueDependencyKind = iota
	laneValueDependenciesIndependent
	laneValueDependenciesEnumerated
)

// laneValueDependencyPolicy is an explicit sum type for cross-axis Values
// references. Every lane must declare either that it is independent of Values
// or provide the complete finite enumerator for the slots its residual facts
// reference. The invalid zero value deliberately fails catalog admission.
type laneValueDependencyPolicy struct {
	kind  laneValueDependencyKind
	visit func(State, *keyspace.KeySpace, func(statekey.ValueDependency))
}

func independentValueDependencies() laneValueDependencyPolicy {
	return laneValueDependencyPolicy{kind: laneValueDependenciesIndependent}
}

func enumeratedValueDependencies(visit func(State, *keyspace.KeySpace, func(statekey.ValueDependency))) laneValueDependencyPolicy {
	return laneValueDependencyPolicy{kind: laneValueDependenciesEnumerated, visit: visit}
}

type laneIdentitySupportKind uint8

const (
	laneIdentitiesInvalid laneIdentitySupportKind = iota
	laneIdentitiesIndependent
	laneIdentitiesEnumerated
)

// IdentityImageLaw is the exact unknown-image/pushforward law of one
// registered carrier. It is representation-level algebra, not a LaneID switch.
// The solver receives this sealed declaration from ProductDomain.
type IdentityImageLaw uint8

const (
	IdentityImageInvalid IdentityImageLaw = iota
	IdentityImageIndependent
	// IdentityImageEmbeddedValue substitutes the identity axis of embedded
	// product.Value leaves. Bottom eliminates the relation, Singleton renames,
	// and Top is represented by identity.Top in the same product.
	IdentityImageEmbeddedValue
	// IdentityImagePointwiseMap applies to identity-keyed may maps. Singleton
	// uses exact inverse fibers, Bottom eliminates the relation, and Top is the
	// registered whole-map Top sentinel.
	IdentityImagePointwiseMap
	// IdentityImageMustSet applies to definite identity-keyed proofs. Singleton
	// uses the universal inverse fiber, Bottom eliminates the relation, and Top
	// drops only the affected definite proof.
	IdentityImageMustSet
)

func (l IdentityImageLaw) valid() bool {
	return l >= IdentityImageIndependent && l <= IdentityImageMustSet
}

// laneIdentitySupportPolicy is the exhaustive allocation-identity ownership
// declaration for one State lane. Its invalid zero value makes catalog growth
// fail closed until a lane either proves independence or supplies its complete
// finite identity visitor.
type laneIdentitySupportPolicy struct {
	kind       laneIdentitySupportKind
	visit      func(*axis.Registry, laneFactorPayload, func(identity.Term) bool) bool
	visitState func(*axis.Registry, State, func(identity.Term) bool) bool
	image      IdentityImageLaw
}

// laneSemanticCapabilityID is the sealed operation vocabulary shared by every
// State axis.  New operations add one catalog entry; axes register an
// independent or participating law in semanticLaws rather than growing
// operation-specific fields on laneSpec.
type laneSemanticCapabilityID string

const (
	laneSemanticPathSubtreeMutation    laneSemanticCapabilityID = "path-subtree-mutation"
	laneSemanticPathDescendantMutation laneSemanticCapabilityID = "path-descendant-mutation"
	laneSemanticPathEqualityQuotient   laneSemanticCapabilityID = "path-equality-quotient"
	laneSemanticPathReplacement        laneSemanticCapabilityID = "path-replacement"
	laneSemanticGenericForBinding      laneSemanticCapabilityID = "generic-for-binding"
	laneSemanticEffectFactor           laneSemanticCapabilityID = "effect-factor"
	laneSemanticCallBoundary           laneSemanticCapabilityID = "call-boundary"
	// laneSemanticPathResolutionParticipant is an inventory-only capability.
	// Participating lanes contain facts consulted while resolving a member path;
	// the resolution operation itself remains owned by the caller and therefore
	// never dispatches a State or factor mutation through this law.
	laneSemanticPathResolutionParticipant laneSemanticCapabilityID = "path-resolution-participant"
)

var registeredLaneSemanticCapabilities = []laneSemanticCapabilityID{
	laneSemanticPathSubtreeMutation,
	laneSemanticPathDescendantMutation,
	laneSemanticPathEqualityQuotient,
	laneSemanticPathReplacement,
	laneSemanticPathResolutionParticipant,
	laneSemanticGenericForBinding,
	laneSemanticEffectFactor,
	laneSemanticCallBoundary,
}

type pathSubtreeMutationRequest struct {
	keys     *keyspace.KeySpace
	prefixes []pathdom.PathKey
	path     pathdom.PathKey
}

func (pathSubtreeMutationRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticPathSubtreeMutation
}

type laneSemanticRequest interface {
	semanticCapabilityID() laneSemanticCapabilityID
}

type pathDescendantMutationRequest struct {
	keys     *keyspace.KeySpace
	prefixes pathevidence.PathKeyDescendantInvalidationPrefixes
	path     pathdom.PathKey
}

type pathEqualityQuotientRequest struct {
	reg      *axis.Registry
	keys     *keyspace.KeySpace
	quotient pathevidence.EqualityQuotient
}

func (pathEqualityQuotientRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticPathEqualityQuotient
}

func (pathDescendantMutationRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticPathDescendantMutation
}

type laneSemanticLaw struct {
	id                laneSemanticCapabilityID
	participates      bool
	applyState        func(State, laneSemanticRequest) (State, bool, bool)
	applyFactor       func(laneFactorPayload, laneSemanticRequest) (laneFactorPayload, bool, bool)
	genericForBinding func(genericForBindingRequest) genericForLaneBinding
	pathReplacement   pathReplacementLaneBinding
	effectKinds       effectFactorKind
	effectObserve     func(laneFactorPayload, DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, bool)
}

type effectFactorKind uint8

const (
	effectFactorDynamicIndexMembership effectFactorKind = 1 << iota
	effectFactorDelta
)

type effectFactorRequest struct {
	kind    effectFactorKind
	dynamic DynamicIndexMembershipFactorPlan
	delta   EffectDeltaFactorPlan
}

type callBoundaryRequest struct{ reg *axis.Registry }

func (callBoundaryRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticCallBoundary
}

func callBoundaryIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticCallBoundary,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

func callBoundaryLane[T any](
	get func(State) T,
	set func(*State, T),
	apply func(T, *axis.Registry) (T, bool, bool),
) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticCallBoundary, participates: true,
		applyState: func(value State, raw laneSemanticRequest) (State, bool, bool) {
			request, ok := raw.(callBoundaryRequest)
			if !ok || request.reg == nil {
				return value, false, false
			}
			next, changed, valid := apply(get(value), request.reg)
			if !valid || !changed {
				return value, false, valid
			}
			set(&value, next)
			return value, true, true
		},
		applyFactor: func(value laneFactorPayload, raw laneSemanticRequest) (laneFactorPayload, bool, bool) {
			request, ok := raw.(callBoundaryRequest)
			if !ok || request.reg == nil {
				return value, false, false
			}
			next, changed, valid := apply(typedLaneFactorValue[T](value), request.reg)
			if !valid || !changed {
				return value, false, valid
			}
			return typedLaneFactorPayload[T]{value: next}, true, true
		},
	}
}

func (effectFactorRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticEffectFactor
}

func effectFactorIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticEffectFactor,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

func effectFactorLane[T any](
	kinds effectFactorKind,
	get func(State) T,
	set func(*State, T),
	apply func(T, effectFactorRequest) (T, bool, bool),
) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticEffectFactor, participates: true, effectKinds: kinds,
		applyState: func(value State, raw laneSemanticRequest) (State, bool, bool) {
			request, ok := raw.(effectFactorRequest)
			if !ok || request.kind&kinds == 0 {
				return value, false, false
			}
			next, changed, valid := apply(get(value), request)
			if !valid || !changed {
				return value, false, valid
			}
			set(&value, next)
			return value, true, true
		},
		applyFactor: func(value laneFactorPayload, raw laneSemanticRequest) (laneFactorPayload, bool, bool) {
			request, ok := raw.(effectFactorRequest)
			if !ok || request.kind&kinds == 0 {
				return value, false, false
			}
			next, changed, valid := apply(typedLaneFactorValue[T](value), request)
			if !valid || !changed {
				return value, false, valid
			}
			return typedLaneFactorPayload[T]{value: next}, true, true
		},
	}
}

func effectFactorLaneWithObserver[T any](
	kinds effectFactorKind,
	get func(State) T,
	set func(*State, T),
	apply func(T, effectFactorRequest) (T, bool, bool),
	observe func(T, DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, bool),
) laneSemanticLaw {
	law := effectFactorLane(kinds, get, set, apply)
	law.effectObserve = func(payload laneFactorPayload, query DynamicIndexMembershipEvidenceQuery) (DynamicIndexMembershipEvidence, bool) {
		return observe(typedLaneFactorValue[T](payload), query)
	}
	return law
}

// pathReplacementLaneBinding is the complete access/evaluator declaration for
// one axis in the atomic path-replacement endomorphism.  Its zero value is not
// a declaration: every admitted lane must register either independence or an
// exact read/write law under laneSemanticPathReplacement.
type pathReplacementLaneBinding struct {
	declared    bool
	pointRead   bool
	currentRead bool
	write       bool
	apply       func(ProductDomain, laneFactorPayload, laneFactorPayload, PathReplacementTransaction) (laneFactorPayload, bool, bool)
}

func pathReplacementIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathReplacement,
		pathReplacement: pathReplacementLaneBinding{declared: true,
			apply: func(_ ProductDomain, _ laneFactorPayload, current laneFactorPayload, _ PathReplacementTransaction) (laneFactorPayload, bool, bool) {
				return current, false, true
			},
		},
	}
}

func pathReplacementLane[T any](pointRead, currentRead, write bool, apply func(ProductDomain, T, T, PathReplacementTransaction) (T, bool, bool)) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathReplacement, participates: pointRead || currentRead || write,
		pathReplacement: pathReplacementLaneBinding{
			declared: true, pointRead: pointRead, currentRead: currentRead, write: write,
			apply: func(domain ProductDomain, point, current laneFactorPayload, transaction PathReplacementTransaction) (laneFactorPayload, bool, bool) {
				next, changed, valid := apply(domain, typedLaneFactorValue[T](point), typedLaneFactorValue[T](current), transaction)
				if !valid || !changed {
					return current, false, valid
				}
				return typedLaneFactorPayload[T]{value: next}, true, true
			},
		},
	}
}

func findLaneSemanticLaw(laws []laneSemanticLaw, id laneSemanticCapabilityID) (laneSemanticLaw, bool) {
	for _, law := range laws {
		if law.id == id {
			return law, true
		}
	}
	return laneSemanticLaw{}, false
}

func pathDescendantMutationIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathDescendantMutation,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

func pathSubtreeMutationIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathSubtreeMutation,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

func pathEqualityQuotientIndependent() laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathEqualityQuotient,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

func pathEqualityQuotientLane[T any](
	get func(State) T,
	set func(*State, T),
	apply func(T, *axis.Registry, *keyspace.KeySpace, pathevidence.EqualityQuotient) (T, bool, bool),
) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathEqualityQuotient, participates: true,
		applyState: func(value State, raw laneSemanticRequest) (State, bool, bool) {
			request, ok := raw.(pathEqualityQuotientRequest)
			if !ok {
				return value, false, false
			}
			next, changed, valid := apply(get(value), request.reg, request.keys, request.quotient)
			if !valid || !changed {
				return value, false, valid
			}
			set(&value, next)
			return value, true, true
		},
		applyFactor: func(value laneFactorPayload, raw laneSemanticRequest) (laneFactorPayload, bool, bool) {
			request, ok := raw.(pathEqualityQuotientRequest)
			if !ok {
				return value, false, false
			}
			next, changed, valid := apply(typedLaneFactorValue[T](value), request.reg, request.keys, request.quotient)
			if !valid || !changed {
				return value, false, valid
			}
			return typedLaneFactorPayload[T]{value: next}, true, true
		},
	}
}

func pathSubtreeMutationLane[T any](
	get func(State) T,
	set func(*State, T),
	apply func(T, *keyspace.KeySpace, []pathdom.PathKey, pathdom.PathKey) (T, bool, bool),
) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathSubtreeMutation, participates: true,
		applyState: func(value State, raw laneSemanticRequest) (State, bool, bool) {
			request, ok := raw.(pathSubtreeMutationRequest)
			if !ok {
				return value, false, false
			}
			next, changed, valid := apply(get(value), request.keys, request.prefixes, request.path)
			if !valid || !changed {
				return value, false, valid
			}
			set(&value, next)
			return value, true, true
		},
		applyFactor: func(value laneFactorPayload, raw laneSemanticRequest) (laneFactorPayload, bool, bool) {
			request, ok := raw.(pathSubtreeMutationRequest)
			if !ok {
				return value, false, false
			}
			next, changed, valid := apply(typedLaneFactorValue[T](value), request.keys, request.prefixes, request.path)
			if !valid || !changed {
				return value, false, valid
			}
			return typedLaneFactorPayload[T]{value: next}, true, true
		},
	}
}

func pathResolutionIndependent() laneSemanticLaw {
	return pathResolutionLaw(false)
}

func pathResolutionParticipant() laneSemanticLaw {
	return pathResolutionLaw(true)
}

func pathResolutionLaw(participates bool) laneSemanticLaw {
	return laneSemanticLaw{
		id:           laneSemanticPathResolutionParticipant,
		participates: participates,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
	}
}

// genericForBindingRequest is the complete operation-dependent input to one
// lane's generic-for transaction law. Indexed value binding is the only
// generic-for shape whose residual factor footprint differs by operation.
type genericForBindingRequest struct {
	indexedValue bool
}

func (genericForBindingRequest) semanticCapabilityID() laneSemanticCapabilityID {
	return laneSemanticGenericForBinding
}

// genericForLaneBinding declares the exact factor transaction owned by one
// registered lane. A false triple is an explicit independence proof, not an
// omitted declaration.
type genericForLaneBinding struct {
	sourceRead  bool
	currentRead bool
	write       bool
	observe     genericForFactorObserve
	apply       genericForFactorApply
}

func genericForBindingIndependent() laneSemanticLaw {
	return genericForBindingLaw(false, func(genericForBindingRequest) genericForLaneBinding {
		return genericForLaneBinding{}
	})
}

func genericForBindingFixed(sourceRead, currentRead, write bool) laneSemanticLaw {
	return genericForBindingLaw(sourceRead || currentRead || write, func(genericForBindingRequest) genericForLaneBinding {
		return genericForLaneBinding{sourceRead: sourceRead, currentRead: currentRead, write: write}
	})
}

func genericForBindingLaw(participates bool, resolve func(genericForBindingRequest) genericForLaneBinding) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticGenericForBinding, participates: participates,
		applyState: func(value State, _ laneSemanticRequest) (State, bool, bool) {
			return value, false, true
		},
		applyFactor: func(value laneFactorPayload, _ laneSemanticRequest) (laneFactorPayload, bool, bool) {
			return value, false, true
		},
		genericForBinding: resolve,
	}
}

// genericForBindingWithFactors attaches the lane's factor-native part of the
// one generic-for transaction. Read/write roles and executable law are sealed
// together so a scheduler cannot select the right axes while invoking a
// different implementation.
func genericForBindingWithFactors(
	participates bool,
	resolve func(genericForBindingRequest) genericForLaneBinding,
) laneSemanticLaw {
	return genericForBindingLaw(participates, resolve)
}

func pathDescendantMutationLane[T any](
	get func(State) T,
	set func(*State, T),
	apply func(T, *keyspace.KeySpace, pathevidence.PathKeyDescendantInvalidationPrefixes, pathdom.PathKey) (T, bool, bool),
) laneSemanticLaw {
	return laneSemanticLaw{
		id: laneSemanticPathDescendantMutation, participates: true,
		applyState: func(value State, raw laneSemanticRequest) (State, bool, bool) {
			request, ok := raw.(pathDescendantMutationRequest)
			if !ok {
				return value, false, false
			}
			next, changed, ok := apply(get(value), request.keys, request.prefixes, request.path)
			if !ok || !changed {
				return value, false, ok
			}
			set(&value, next)
			return value, true, true
		},
		applyFactor: func(value laneFactorPayload, raw laneSemanticRequest) (laneFactorPayload, bool, bool) {
			request, ok := raw.(pathDescendantMutationRequest)
			if !ok {
				return value, false, false
			}
			next, changed, ok := apply(typedLaneFactorValue[T](value), request.keys, request.prefixes, request.path)
			if !ok || !changed {
				return value, false, ok
			}
			return typedLaneFactorPayload[T]{value: next}, true, true
		},
	}
}

type laneBoundaryClosureCompanionKind uint8

const (
	laneBoundaryClosureCompanionInvalid laneBoundaryClosureCompanionKind = iota
	laneBoundaryClosureCompanionNone
	laneBoundaryClosureCompanionUnique
)

// laneBoundaryClosureCompanionPolicy declares whether a lane is the optional
// factor whose projected facts extend the common boundary closure. The zero
// value is deliberately invalid so catalog growth cannot silently omit this
// boundary-orchestration decision.
type laneBoundaryClosureCompanionPolicy struct {
	kind laneBoundaryClosureCompanionKind
}

func noBoundaryClosureCompanion() laneBoundaryClosureCompanionPolicy {
	return laneBoundaryClosureCompanionPolicy{kind: laneBoundaryClosureCompanionNone}
}

func uniqueBoundaryClosureCompanion() laneBoundaryClosureCompanionPolicy {
	return laneBoundaryClosureCompanionPolicy{kind: laneBoundaryClosureCompanionUnique}
}

func independentIdentitySupport() laneIdentitySupportPolicy {
	return laneIdentitySupportPolicy{kind: laneIdentitiesIndependent, image: IdentityImageIndependent}
}

func enumeratedIdentitySupport[T any](
	visit func(*axis.Registry, T, func(identity.Term) bool) bool,
	get func(State) T,
	image IdentityImageLaw,
) laneIdentitySupportPolicy {
	return laneIdentitySupportPolicy{
		kind: laneIdentitiesEnumerated,
		visit: func(reg *axis.Registry, payload laneFactorPayload, yield func(identity.Term) bool) bool {
			return visit(reg, typedLaneFactorValue[T](payload), yield)
		},
		visitState: func(reg *axis.Registry, state State, yield func(identity.Term) bool) bool {
			return visit(reg, get(state), yield)
		},
		image: image,
	}
}

// laneSpec is the registration unit for one State product-lattice axis.
// It names the axis, builds its lattice operations, and optionally marks the
// lane reachable when a write leaves lattice bottom.
type laneSpec struct {
	id            LaneID
	bit           laneBit
	slotFactored  bool
	build         func(*axis.Registry, DomainOptions) laneOps
	markReachable func(State) State
	fingerprint   func(*fingerprintWriter, State)
	keySpaceMode  laneKeySpaceMode
	rekey         func(State, *keyspace.KeySpace, *keyspace.KeySpace) (State, bool)
	formalRekey   laneFormalRekeyPolicy
	// valueDependencies explicitly declares whether this lane is independent of
	// Values or enumerates the point-local slots referenced by its residual
	// facts. Product factorization uses this registration to preserve cross-axis
	// correlation without selecting the whole Values lane.
	valueDependencies laneValueDependencyPolicy
	// identitySupport declares the complete allocation-identity support of the
	// lane independently from fingerprint spelling and keyspace serialization.
	identitySupport laneIdentitySupportPolicy
	// numericConsistency declares whether this lane contributes assertions to
	// the single product-level arithmetic satisfiability invariant.
	numericConsistency laneNumericConsistencyPolicy
	semanticLaws       []laneSemanticLaw
	// boundaryClosureCompanion explicitly declares the unique optional lane
	// whose projected factor contributes version-insensitive closure paths.
	boundaryClosureCompanion laneBoundaryClosureCompanionPolicy
	// rootAssignment is the complete N4 access and caller-local completion law
	// for this lane. Its invalid zero value makes catalog growth fail closed.
	// The semantic operation is registered beside the lane representation, so
	// concrete and guarded execution share one factor implementation.
	rootAssignment rootAssignmentLanePolicy
	// dynamicRead is the lane-owned query law for canonical dynamic reads.
	// Every lane explicitly declares independence or contributes one ordinary
	// factor observation. Coordinate families declare their own sparse demand
	// laws beside their transposed representation.
	dynamicRead laneDynamicReadPolicy
	// coordinateFamilies is the complete registered transposition surface for
	// lanes whose semantic carrier can be split into independently guarded
	// coordinate families. An empty inventory means the lane remains an atomic
	// product factor. Family operations are sealed into ProductDomain exactly
	// once; evaluators never dispatch on LaneID.
	coordinateFamilies []coordinateFamilySpec
	boundary           boundaryLaneOps
}

// boundaryLaneOps owns the boundary-reachability contribution of one State
// lane. Every registered lane supplies an expander, including lanes whose
// facts do not introduce paths or identities. That makes adding a lane an
// explicit boundary-design decision instead of silently omitting it from a
// central inventory.
type boundaryLaneOps struct {
	project    func(*boundaryProjectContext, State, *State) bool
	rebase     func(*boundaryRebaseContext, State, *State) bool
	postRebase func(*boundaryRebaseContext, [][2]keyspace.Key, *State) bool
	equal      func(*axis.Registry, State, State) bool
}

func postRebaseBoundaryNoop(*boundaryRebaseContext, [][2]keyspace.Key, *State) bool { return true }

// laneBit is a catalog ordinal, not a machine-word bit. Keeping the ordinal
// independent from laneMask removes the historical 63-axis limit.
type laneBit int

// laneMask is an immutable, comparable enabled-lane set. Catalogs through 64
// axes use only inline and allocate nothing. Larger catalogs encode successive
// 64-bit words in a canonical trimmed string, retaining value equality without
// a slice, pointer identity, or maximum lane count.
type laneMask struct {
	inline uint64
	// spill is empty for the unscoped State zero value, the constant one-byte
	// scope marker for inline masks, and marker+canonical words for overflow.
	spill string
}

const laneMaskScopeMarker = "\x01"

func scopedLaneMask(bits []laneBit) laneMask {
	mask := laneMask{spill: laneMaskScopeMarker}
	maxSpillWord := -1
	for _, bit := range bits {
		if bit < 0 {
			panic("state: negative lane ordinal")
		}
		if bit >= 64 {
			word := (int(bit) - 64) / 64
			if word > maxSpillWord {
				maxSpillWord = word
			}
		}
	}
	var spill []byte
	if maxSpillWord >= 0 {
		spill = make([]byte, 1+(maxSpillWord+1)*8)
		spill[0] = laneMaskScopeMarker[0]
	}
	for _, bit := range bits {
		ordinal := int(bit)
		if ordinal < 64 {
			mask.inline |= uint64(1) << ordinal
			continue
		}
		spillOrdinal := ordinal - 64
		wordOffset := 1 + (spillOrdinal/64)*8
		word := laneMaskSpillWord(spill, wordOffset)
		word |= uint64(1) << (spillOrdinal % 64)
		putLaneMaskSpillWord(spill, wordOffset, word)
	}
	if spill != nil {
		mask.spill = string(spill)
	}
	return mask
}

func (m laneMask) allows(bit laneBit) bool {
	if m.spill == "" {
		return true
	}
	ordinal := int(bit)
	if ordinal < 0 {
		return false
	}
	if ordinal < 64 {
		return m.inline&(uint64(1)<<ordinal) != 0
	}
	spillOrdinal := ordinal - 64
	wordOffset := 1 + (spillOrdinal/64)*8
	if wordOffset+8 > len(m.spill) {
		return false
	}
	return laneMaskSpillWordString(m.spill, wordOffset)&(uint64(1)<<(spillOrdinal%64)) != 0
}

func (m laneMask) forEach(visit func(laneBit) bool) {
	if visit == nil {
		return
	}
	for bit := 0; bit < 64; bit++ {
		if m.inline&(uint64(1)<<bit) != 0 && !visit(laneBit(bit)) {
			return
		}
	}
	for offset := 1; offset+8 <= len(m.spill); offset += 8 {
		word := laneMaskSpillWordString(m.spill, offset)
		for bit := 0; bit < 64; bit++ {
			if word&(uint64(1)<<bit) != 0 && !visit(laneBit(64+((offset-1)/8)*64+bit)) {
				return
			}
		}
	}
}

func (m laneMask) hash64() uint64 {
	// FNV-1a over the canonical value encoding. This is deliberately local:
	// lane masks are configuration identities, not semantic State digests.
	hash := uint64(14695981039346656037)
	add := func(value byte) {
		hash ^= uint64(value)
		hash *= 1099511628211
	}
	for shift := 0; shift < 64; shift += 8 {
		add(byte(m.inline >> shift))
	}
	for i := range m.spill {
		add(m.spill[i])
	}
	return hash
}

func laneMaskSpillWord(bytes []byte, offset int) uint64 {
	var word uint64
	for i := 0; i < 8; i++ {
		word |= uint64(bytes[offset+i]) << (8 * i)
	}
	return word
}

func laneMaskSpillWordString(bytes string, offset int) uint64 {
	var word uint64
	for i := 0; i < 8; i++ {
		word |= uint64(bytes[offset+i]) << (8 * i)
	}
	return word
}

func putLaneMaskSpillWord(bytes []byte, offset int, word uint64) {
	for i := 0; i < 8; i++ {
		bytes[offset+i] = byte(word >> (8 * i))
	}
}

type laneOps struct {
	bottom   func(*State)
	top      func(*State)
	equal    func(State, State) bool
	same     func(State, State) bool
	lessOrEq func(State, State) bool
	join     func(*State, State, State, bool)
	widen    func(*State, State, State, bool)
	narrow   func(*State, State, State)
	factor   laneFactorOps
}

// laneFactorPayload is the package-private erased carrier used by the
// catalog-owned per-lane product API. The concrete type never escapes the
// registration closure that created it: callers can only hold LaneFactor,
// and all lattice operations dispatch through laneFactorOps.
type laneFactorPayload interface {
	isLaneFactorPayload()
}

type typedLaneFactorPayload[T any] struct {
	value T
}

func (typedLaneFactorPayload[T]) isLaneFactorPayload() {}

type laneFactorOps struct {
	bottom     func() laneFactorPayload
	top        func() laneFactorPayload
	extract    func(State) laneFactorPayload
	install    func(*State, laneFactorPayload)
	copy       func(*State, State)
	equalState func(State, State) bool
	equal      func(laneFactorPayload, laneFactorPayload) bool
	// canonicalEqual is the factor-terminal quotient law. Unlike same (the
	// O(1) persistent-representation fast path), it proves two independently
	// built canonical factor spellings denote the same internable terminal.
	canonicalEqual  func(laneFactorPayload, laneFactorPayload) bool
	same            func(laneFactorPayload, laneFactorPayload) bool
	lessOrEq        func(laneFactorPayload, laneFactorPayload) bool
	join            func(laneFactorPayload, laneFactorPayload) laneFactorPayload
	meet            func(laneFactorPayload, laneFactorPayload) laneFactorPayload
	widen           func(laneFactorPayload, laneFactorPayload) laneFactorPayload
	narrow          func(laneFactorPayload, laneFactorPayload) laneFactorPayload
	boundaryApply   func(*boundaryApplyContext, laneFactorPayload, laneFactorPayload) (laneFactorPayload, bool)
	boundaryRoots   func(*boundaryApplyContext, laneFactorPayload, boundaryRootPlan) (laneFactorPayload, bool)
	boundaryRootUse BoundaryRootUse
	// boundaryProject and boundaryRebase are the canonical per-factor boundary
	// laws.  Guarded execution invokes them without ever assembling a source
	// State; whole-State ProjectBoundary/Rebase dispatch through the same hooks.
	boundaryProject       func(*boundaryProjectContext, laneFactorPayload) (laneFactorPayload, bool)
	boundaryRebase        func(*boundaryRebaseContext, laneFactorPayload) (laneFactorPayload, bool)
	boundaryPostRebase    func(*boundaryRebaseContext, [][2]keyspace.Key, laneFactorPayload) (laneFactorPayload, bool)
	boundaryReachability  func(*axis.Registry, *keyspace.KeySpace, laneFactorPayload) (BoundaryReachabilityProgram, error)
	boundaryClosureExtend func(*keyspace.KeySpace, laneFactorPayload, boundaryPathMap, BoundaryClosure) (BoundaryClosure, bool)
}

type typedBoundaryFactorOps[T any] struct {
	apply         func(*boundaryApplyContext, T, T) (T, bool)
	roots         typedBoundaryRootOps[T]
	project       func(*boundaryProjectContext, T) (T, bool)
	rebase        func(*boundaryRebaseContext, T) (T, bool)
	postRebase    func(*boundaryRebaseContext, [][2]keyspace.Key, T) (T, bool)
	reachability  func(*boundaryReachabilityProgramBuilder, T)
	extendClosure func(*keyspace.KeySpace, T, boundaryPathMap, BoundaryClosure) (BoundaryClosure, bool)
}

// typedLaneFactorRepresentation is the mandatory representation contract for
// factor-terminal hash-consing. It is intentionally separate from boundary
// transport: terminal identity is a property of the lane carrier itself.
// equal is never inferred from lattice Equal because semantic quotients may
// admit multiple spellings which are not interchangeable for deterministic
// factoring.
type typedLaneFactorRepresentation[T any] struct {
	equal func(T, T) bool
}

type typedBoundaryRootOps[T any] struct {
	apply func(*boundaryApplyContext, T, boundaryRootPlan) (T, bool)
	use   BoundaryRootUse
}

func boundaryPostRebaseUnchanged[T any](_ *boundaryRebaseContext, _ [][2]keyspace.Key, lane T) (T, bool) {
	return lane, true
}

func boundaryRootsUnchanged[T any]() typedBoundaryRootOps[T] {
	return typedBoundaryRootOps[T]{
		use: boundaryRootUseNone(),
		apply: func(_ *boundaryApplyContext, lane T, _ boundaryRootPlan) (T, bool) {
			return lane, true
		},
	}
}

func boundaryRootsReachable[T any](reachable func(T) T) typedBoundaryRootOps[T] {
	return typedBoundaryRootOps[T]{
		use: boundaryRootUseReachability(),
		apply: func(_ *boundaryApplyContext, lane T, roots boundaryRootPlan) (T, bool) {
			if roots.establishesReachability {
				lane = reachable(lane)
			}
			return lane, true
		},
	}
}

func boundaryRootsSlotValues[T any](apply func(*boundaryApplyContext, T, boundaryRootPlan) (T, bool)) typedBoundaryRootOps[T] {
	return typedBoundaryRootOps[T]{apply: apply, use: boundaryRootUseSlotValues()}
}

func boundaryRootsPathValuesAndReachability[T any](apply func(*boundaryApplyContext, T, boundaryRootPlan) (T, bool)) typedBoundaryRootOps[T] {
	return typedBoundaryRootOps[T]{apply: apply, use: boundaryRootUsePathValuesAndReachability()}
}

func typedLaneFactorValue[T any](payload laneFactorPayload) T {
	typed, ok := payload.(typedLaneFactorPayload[T])
	if !ok {
		panic("state: internal lane factor payload mismatch")
	}
	return typed.value
}

func stateLaneWithBoundary[T any](
	domain lattice.Lattice[T],
	get func(State) T,
	set func(*State, T),
	representation typedLaneFactorRepresentation[T],
	boundary typedBoundaryFactorOps[T],
) laneOps {
	if representation.equal == nil {
		panic("state: lane has no canonical factor representation law")
	}
	factor := laneFactorOps{
		bottom: func() laneFactorPayload {
			return typedLaneFactorPayload[T]{value: domain.Bottom()}
		},
		top: func() laneFactorPayload {
			return typedLaneFactorPayload[T]{value: domain.Top()}
		},
		extract: func(source State) laneFactorPayload {
			return typedLaneFactorPayload[T]{value: get(source)}
		},
		install: func(destination *State, payload laneFactorPayload) {
			set(destination, typedLaneFactorValue[T](payload))
		},
		copy: func(destination *State, source State) {
			set(destination, get(source))
		},
		equalState: func(left, right State) bool {
			return domain.Equal(get(left), get(right))
		},
		equal: func(left, right laneFactorPayload) bool {
			return domain.Equal(typedLaneFactorValue[T](left), typedLaneFactorValue[T](right))
		},
		canonicalEqual: func(left, right laneFactorPayload) bool {
			return representation.equal(typedLaneFactorValue[T](left), typedLaneFactorValue[T](right))
		},
		same: func(left, right laneFactorPayload) bool {
			return domain.Same != nil && domain.Same(
				typedLaneFactorValue[T](left),
				typedLaneFactorValue[T](right),
			)
		},
		lessOrEq: func(left, right laneFactorPayload) bool {
			return domain.LessOrEq(typedLaneFactorValue[T](left), typedLaneFactorValue[T](right))
		},
		join: func(left, right laneFactorPayload) laneFactorPayload {
			leftValue := typedLaneFactorValue[T](left)
			rightValue := typedLaneFactorValue[T](right)
			switch {
			case domain.Equal(leftValue, rightValue), domain.LessOrEq(rightValue, leftValue):
				return left
			case domain.LessOrEq(leftValue, rightValue):
				return right
			default:
				return typedLaneFactorPayload[T]{value: domain.Join(leftValue, rightValue)}
			}
		},
		widen: func(previous, next laneFactorPayload) laneFactorPayload {
			previousValue := typedLaneFactorValue[T](previous)
			nextValue := typedLaneFactorValue[T](next)
			if domain.Equal(previousValue, nextValue) || domain.LessOrEq(nextValue, previousValue) {
				return previous
			}
			return typedLaneFactorPayload[T]{value: domain.Widen(previousValue, nextValue)}
		},
		narrow: func(previous, next laneFactorPayload) laneFactorPayload {
			if domain.Narrow == nil {
				return previous
			}
			return typedLaneFactorPayload[T]{value: domain.Narrow(
				typedLaneFactorValue[T](previous),
				typedLaneFactorValue[T](next),
			)}
		},
	}
	if domain.Meet != nil {
		factor.meet = func(left, right laneFactorPayload) laneFactorPayload {
			leftValue := typedLaneFactorValue[T](left)
			rightValue := typedLaneFactorValue[T](right)
			if domain.Equal(leftValue, rightValue) || domain.LessOrEq(leftValue, rightValue) {
				return left
			}
			if domain.LessOrEq(rightValue, leftValue) {
				return right
			}
			return typedLaneFactorPayload[T]{value: domain.Meet(leftValue, rightValue)}
		}
	}
	if boundary.apply != nil {
		factor.boundaryApply = func(ctx *boundaryApplyContext, destination, fragment laneFactorPayload) (laneFactorPayload, bool) {
			out, ok := boundary.apply(ctx, typedLaneFactorValue[T](destination), typedLaneFactorValue[T](fragment))
			return typedLaneFactorPayload[T]{value: out}, ok
		}
	}
	if boundary.roots.apply != nil {
		factor.boundaryRootUse = boundary.roots.use
		factor.boundaryRoots = func(ctx *boundaryApplyContext, destination laneFactorPayload, roots boundaryRootPlan) (laneFactorPayload, bool) {
			out, ok := boundary.roots.apply(ctx, typedLaneFactorValue[T](destination), roots)
			return typedLaneFactorPayload[T]{value: out}, ok
		}
	}
	if boundary.project != nil {
		factor.boundaryProject = func(ctx *boundaryProjectContext, source laneFactorPayload) (laneFactorPayload, bool) {
			out, ok := boundary.project(ctx, typedLaneFactorValue[T](source))
			return typedLaneFactorPayload[T]{value: out}, ok
		}
	}
	if boundary.rebase != nil {
		factor.boundaryRebase = func(ctx *boundaryRebaseContext, source laneFactorPayload) (laneFactorPayload, bool) {
			out, ok := boundary.rebase(ctx, typedLaneFactorValue[T](source))
			return typedLaneFactorPayload[T]{value: out}, ok
		}
	}
	if boundary.postRebase != nil {
		factor.boundaryPostRebase = func(ctx *boundaryRebaseContext, aliases [][2]keyspace.Key, source laneFactorPayload) (laneFactorPayload, bool) {
			out, ok := boundary.postRebase(ctx, aliases, typedLaneFactorValue[T](source))
			return typedLaneFactorPayload[T]{value: out}, ok
		}
	}
	if boundary.reachability != nil {
		factor.boundaryReachability = func(reg *axis.Registry, keys *keyspace.KeySpace, source laneFactorPayload) (BoundaryReachabilityProgram, error) {
			builder := newBoundaryReachabilityProgramBuilder(reg, keys)
			boundary.reachability(builder, typedLaneFactorValue[T](source))
			return builder.seal()
		}
	}
	if boundary.extendClosure != nil {
		factor.boundaryClosureExtend = func(keys *keyspace.KeySpace, source laneFactorPayload, roots boundaryPathMap, base BoundaryClosure) (BoundaryClosure, bool) {
			return boundary.extendClosure(keys, typedLaneFactorValue[T](source), roots, base)
		}
	}
	return laneOps{
		bottom: func(out *State) {
			set(out, domain.Bottom())
		},
		top: func(out *State) {
			set(out, domain.Top())
		},
		equal: func(a, b State) bool {
			return domain.Equal(get(a), get(b))
		},
		same: func(a, b State) bool {
			return domain.Same != nil && domain.Same(get(a), get(b))
		},
		lessOrEq: func(a, b State) bool {
			return domain.LessOrEq(get(a), get(b))
		},
		join: func(out *State, a, b State, reuseInputs bool) {
			av := get(a)
			bv := get(b)
			if reuseInputs {
				switch {
				case domain.Equal(av, bv):
					set(out, av)
					return
				case domain.LessOrEq(av, bv):
					set(out, bv)
					return
				case domain.LessOrEq(bv, av):
					set(out, av)
					return
				}
			}
			set(out, domain.Join(av, bv))
		},
		widen: func(out *State, prev, next State, reuseInputs bool) {
			pv := get(prev)
			nv := get(next)
			if reuseInputs {
				if domain.Equal(pv, nv) || domain.LessOrEq(nv, pv) {
					set(out, pv)
					return
				}
			}
			set(out, domain.Widen(pv, nv))
		},
		narrow: func(out *State, prev, next State) {
			if domain.Narrow == nil {
				set(out, get(prev))
				return
			}
			set(out, domain.Narrow(get(prev), get(next)))
		},
		factor: factor,
	}
}

func domainFromLaneSpecs(reg *axis.Registry, specs []laneSpec, universe []laneSpec) lattice.Lattice[State] {
	return domainFromLaneSpecsWithOptions(reg, specs, universe, DomainOptions{})
}

func domainFromLaneSpecsWithOptions(reg *axis.Registry, specs []laneSpec, universe []laneSpec, options DomainOptions) lattice.Lattice[State] {
	lanes := make([]laneOps, 0, len(specs))
	bits := make([]laneBit, 0, len(specs))
	for _, spec := range specs {
		lanes = append(lanes, spec.build(reg, options))
		bits = append(bits, spec.bit)
	}
	bottomLanes := make([]laneOps, 0, len(universe))
	if sameLaneSpecs(specs, universe) {
		bottomLanes = lanes
	} else {
		for _, spec := range universe {
			bottomLanes = append(bottomLanes, spec.build(reg, options))
		}
	}
	scope := scopedLaneMask(bits)
	bottom := State{}
	for _, lane := range bottomLanes {
		lane.bottom(&bottom)
	}
	bottom.laneMask = scope
	bottom.canonical = true
	bottom.numericConsistency = numericConsistencyCertified
	return lattice.Lattice[State]{
		Bottom: func() State {
			return bottom
		},
		Top: func() State {
			out := bottom
			for _, lane := range lanes {
				lane.top(&out)
			}
			out.canonical = true
			out.numericConsistency = numericConsistencyCertified
			return out
		},
		Equal: func(a, b State) bool {
			for _, lane := range lanes {
				if !lane.equal(a, b) {
					return false
				}
			}
			return true
		},
		LessOrEq: func(a, b State) bool {
			for _, lane := range lanes {
				if !lane.lessOrEq(a, b) {
					return false
				}
			}
			return true
		},
		Join: func(a, b State) State {
			reuseInputs := a.canonical && b.canonical
			if reuseInputs {
				switch {
				case lanesEqual(lanes, a, b) && stateHasLaneMask(a, scope):
					return certifyNumericConsistencyForLattice(a, bottom, lanes, specs)
				case lanesLessOrEq(lanes, a, b) && stateHasLaneMask(b, scope):
					return certifyNumericConsistencyForLattice(b, bottom, lanes, specs)
				case lanesLessOrEq(lanes, b, a) && stateHasLaneMask(a, scope):
					return certifyNumericConsistencyForLattice(a, bottom, lanes, specs)
				}
			}
			out := bottom
			for _, lane := range lanes {
				lane.join(&out, a, b, reuseInputs)
			}
			out.canonical = true
			if a.numericConsistency == numericConsistencyCertified && b.numericConsistency == numericConsistencyCertified {
				// Join weakens every must-fact lane, so satisfiability is preserved.
				out.numericConsistency = numericConsistencyCertified
			} else {
				out.numericConsistency = numericConsistencyUnknown
			}
			return certifyNumericConsistencyForLattice(out, bottom, lanes, specs)
		},
		Widen: func(prev, next State) State {
			out := bottom
			reuseInputs := prev.canonical && next.canonical
			for _, lane := range lanes {
				lane.widen(&out, prev, next, reuseInputs)
			}
			out.canonical = true
			if prev.numericConsistency == numericConsistencyCertified && next.numericConsistency == numericConsistencyCertified {
				out.numericConsistency = numericConsistencyCertified
			} else {
				out.numericConsistency = numericConsistencyUnknown
			}
			return certifyNumericConsistencyForLattice(out, bottom, lanes, specs)
		},
		Narrow: func(prev, next State) State {
			out := bottom
			for _, lane := range lanes {
				lane.narrow(&out, prev, next)
			}
			out.canonical = true
			return certifyNumericConsistencyForLattice(out, bottom, lanes, specs)
		},
	}
}

func stateHasLaneMask(s State, mask laneMask) bool {
	return s.laneMask == mask
}

func lanesEqual(lanes []laneOps, a, b State) bool {
	for _, lane := range lanes {
		if !lane.equal(a, b) {
			return false
		}
	}
	return true
}

func lanesLessOrEq(lanes []laneOps, a, b State) bool {
	for _, lane := range lanes {
		if lane.same(a, b) {
			continue
		}
		if !lane.lessOrEq(a, b) {
			return false
		}
	}
	return true
}

func sameLaneSpecs(a, b []laneSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].id != b[i].id || a[i].bit != b[i].bit {
			return false
		}
	}
	return true
}
