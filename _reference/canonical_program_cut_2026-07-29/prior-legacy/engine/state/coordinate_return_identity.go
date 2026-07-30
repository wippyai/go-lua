package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

// CoordinateReturnIdentityRoleSet is the ProductDomain-sealed N5 role
// descriptor for one family. It is immutable and cannot be transferred across
// domains. Consumers dispatch only through Has; family order is not semantic.
type CoordinateReturnIdentityRoleSet struct {
	seal   *productDomainSeal
	family CoordinateFamily
	bits   coordinateReturnIdentityRoleBits
}

func (r CoordinateReturnIdentityRoleSet) Family() CoordinateFamily { return r.family }

func (r CoordinateReturnIdentityRoleSet) Has(role CoordinateReturnIdentityRole) bool {
	return r.seal != nil && r.family.seal == r.seal && r.bits.has(role)
}

// CoordinateReturnIdentityRoles returns the sole sealed role authority for a
// registered family.
func (d ProductDomain) CoordinateReturnIdentityRoles(family CoordinateFamily) (CoordinateReturnIdentityRoleSet, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return CoordinateReturnIdentityRoleSet{}, err
	}
	return CoordinateReturnIdentityRoleSet{seal: d.seal, family: family, bits: coordinate.ops.returnIdentity.roles}, nil
}

// ReturnIdentityContainerFamily returns the unique registered N5 container
// owner. ProductDomain sealing guarantees that a closure with seed or edge
// producers has exactly one owner and that a closure without them has none.
func (d ProductDomain) ReturnIdentityContainerFamily() (CoordinateFamily, bool) {
	if !d.Valid() || !d.hasReturnIdentityContainerFamily {
		return CoordinateFamily{}, false
	}
	return d.returnIdentityContainerFamily, true
}

// CoordinateReturnIdentityObservation is one root-local N5 fact. The guard is
// intentionally absent: concrete callers use true and formal callers attach
// the decision of the terminal from which this observation was obtained.
type CoordinateReturnIdentityObservation struct {
	role   CoordinateReturnIdentityRole
	root   identity.Term
	target identity.Term
	value  product.Value
}

func (o CoordinateReturnIdentityObservation) Role() CoordinateReturnIdentityRole { return o.role }
func (o CoordinateReturnIdentityObservation) Root() identity.Term                { return o.root }
func (o CoordinateReturnIdentityObservation) Target() identity.Term              { return o.target }
func (o CoordinateReturnIdentityObservation) Value() product.Value               { return o.value }

// VisitCoordinateReturnIdentitySkeletonObservations emits the exact rootwise
// seed and skeleton-edge observations owned by one guarded skeleton terminal.
func (d ProductDomain) VisitCoordinateReturnIdentitySkeletonObservations(
	skeleton CoordinateFamilySkeleton,
	visit func(CoordinateReturnIdentityObservation) bool,
) error {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return err
	}
	if visit == nil {
		return fmt.Errorf("%w: nil return-identity observation visitor", ErrInvalidLaneFactor)
	}
	roles := coordinate.ops.returnIdentity.roles
	if roles.has(CoordinateReturnIdentitySeed) {
		stopped := false
		coordinate.ops.returnIdentity.visitSeedRoots(skeleton.payload, func(root identity.Term) bool {
			if stopped || !root.Valid() {
				return !stopped
			}
			stopped = !visit(CoordinateReturnIdentityObservation{role: CoordinateReturnIdentitySeed, root: root})
			return !stopped
		})
		if stopped {
			return nil
		}
	}
	if roles.has(CoordinateReturnIdentitySkeletonEdge) {
		coordinate.ops.returnIdentity.visitSkeletonEdges(skeleton.payload, func(from, to identity.Term) bool {
			return !from.Valid() || !to.Valid() || visit(CoordinateReturnIdentityObservation{
				role: CoordinateReturnIdentitySkeletonEdge, root: from, target: to,
			})
		})
	}
	return nil
}

// VisitCoordinateReturnIdentityScalarObservations emits the exact rootwise
// scalar-edge and container observations owned by one guarded scalar terminal.
func (d ProductDomain) VisitCoordinateReturnIdentityScalarObservations(
	scalar CoordinateScalarFactor,
	visit func(CoordinateReturnIdentityObservation) bool,
) error {
	coordinate, err := d.validateCoordinateFamily(scalar.slot.family)
	if err != nil {
		return err
	}
	if err := d.validateCoordinateFactorFor(coordinate, scalar, scalar.slot.keys); err != nil {
		return err
	}
	if visit == nil {
		return fmt.Errorf("%w: nil return-identity observation visitor", ErrInvalidLaneFactor)
	}
	roles := coordinate.ops.returnIdentity.roles
	if roles.has(CoordinateReturnIdentityScalarEdge) {
		stopped := false
		coordinate.ops.returnIdentity.visitScalarEdges(scalar.slot.key, scalar.payload, func(from, to identity.Term) bool {
			if stopped || !from.Valid() || !to.Valid() {
				return !stopped
			}
			stopped = !visit(CoordinateReturnIdentityObservation{
				role: CoordinateReturnIdentityScalarEdge, root: from, target: to,
			})
			return !stopped
		})
		if stopped {
			return nil
		}
	}
	if roles.has(CoordinateReturnIdentityContainer) {
		root, value, handled := coordinate.ops.returnIdentity.containerScalar(scalar.slot.key, scalar.payload)
		if handled && root.Valid() {
			visit(CoordinateReturnIdentityObservation{role: CoordinateReturnIdentityContainer, root: root, value: value})
		}
	}
	return nil
}

// VisitCoordinateReturnContainerFacts visits the unique container owner's
// skeleton facts for one exact concrete, formal, or allocation root.
func (d ProductDomain) VisitCoordinateReturnContainerFacts(skeleton CoordinateFamilySkeleton, root identity.Term, visit func(dynamicindex.Fact)) (bool, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil {
		return false, err
	}
	if !coordinate.ops.returnIdentity.roles.has(CoordinateReturnIdentityContainer) {
		return false, nil
	}
	if !root.Valid() || visit == nil {
		return false, fmt.Errorf("%w: invalid return-container observation", ErrInvalidLaneFactor)
	}
	return coordinate.ops.returnIdentity.visitContainerFacts(skeleton.payload, root, visit), nil
}

// CoordinateReturnIdentityTermSlot locates the publisher-owned scalar updated
// for an exact concrete, formal, or allocation identity term.
func (d ProductDomain) CoordinateReturnIdentityTermSlot(family CoordinateFamily, keys *keyspace.KeySpace, root identity.Term) (slot CoordinateSlot, handled bool, err error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return CoordinateSlot{}, false, err
	}
	if !coordinate.ops.returnIdentity.roles.has(CoordinateReturnIdentityPublisher) {
		return CoordinateSlot{}, false, nil
	}
	key, handled := coordinate.ops.returnIdentity.publicationKey(root)
	if !handled {
		return CoordinateSlot{}, false, nil
	}
	if key == nil || !coordinate.ops.keyValid(key, keys) {
		return CoordinateSlot{}, false, fmt.Errorf("%w: invalid return-identity coordinate", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: family, keys: keys, key: key}, true, nil
}

// CoordinateReturnIdentitySlot is the concrete convenience boundary.
func (d ProductDomain) CoordinateReturnIdentitySlot(family CoordinateFamily, keys *keyspace.KeySpace, id identity.ID) (slot CoordinateSlot, handled bool, err error) {
	return d.CoordinateReturnIdentityTermSlot(family, keys, identity.ConcreteTerm(id))
}

// PublishCoordinateReturnIdentity applies the family-owned scalar publication
// law. The input slot is preserved exactly.
func (d ProductDomain) PublishCoordinateReturnIdentity(scalar CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	return d.PublishCoordinateIdentityPlacement(scalar, placement.OwnedHeap)
}

// PublishCoordinateIdentityPlacement applies the family-owned placement
// publication law for an arbitrary semantic placement target.
func (d ProductDomain) PublishCoordinateIdentityPlacement(scalar CoordinateScalarFactor, target placement.Value) (CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateFamily(scalar.slot.family)
	if err != nil {
		return CoordinateScalarFactor{}, err
	}
	if err := d.validateCoordinateFactorFor(coordinate, scalar, scalar.slot.keys); err != nil {
		return CoordinateScalarFactor{}, err
	}
	if !coordinate.ops.returnIdentity.roles.has(CoordinateReturnIdentityPublisher) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: family does not publish returned identities", ErrInvalidLaneFactor)
	}
	payload, handled := coordinate.ops.returnIdentity.publishScalar(scalar.slot.key, scalar.payload, target)
	if !handled || payload == nil || !coordinate.ops.scalarValid(scalar.slot.key, payload) {
		return CoordinateScalarFactor{}, fmt.Errorf("%w: family does not publish returned identities", ErrInvalidLaneFactor)
	}
	return CoordinateScalarFactor{slot: scalar.slot, payload: payload}, nil
}
