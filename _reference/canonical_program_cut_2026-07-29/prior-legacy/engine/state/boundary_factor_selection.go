package state

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// BoundaryFactorRoot is the scalar-free spelling of one boundary tuple
// coordinate. Slot and Path are independent, optional structural addresses:
// a root with neither is a value-only/rvalue coordinate whose scalar is still
// identified by its tuple ordinal and transported separately. Values are
// deliberately absent because guarded decision roots remain caller-owned.
type BoundaryFactorRoot struct {
	Slot statekey.Value
	Path keyspace.Key
}

// BoundaryFactorSelection is the sealed finite closure authority used by
// factorwise Project+Rebase. It cannot expose or mutate BoundaryClosure and
// therefore cannot publish a partial whole-State boundary artifact.
type BoundaryFactorSelection struct {
	seal             *boundaryFactorSelectionSeal
	keys             *keyspace.KeySpace
	closure          BoundaryClosure
	roots            []BoundaryFactorRoot
	coordinates      CoordinateFactorInventory
	exactCoordinates bool
}

type boundaryFactorSelectionSeal struct{ owned byte }

func (s BoundaryFactorSelection) valid() bool {
	return s.seal != nil && s.keys != nil && s.keys.Valid() && s.closure.slots != nil && s.closure.paths != nil &&
		s.closure.identities != nil && s.closure.heapSuffixes != nil && (!s.exactCoordinates || s.coordinates.keys == s.keys)
}

// SealBoundaryFactorSelection validates and canonicalizes the complete
// scalar-free root/identity inventory. selectedIdentities is the already
// computed finite guarded closure; allIdentities represents the exact map-Top
// case and is not a work shortcut or budget.
func SealBoundaryFactorSelection(
	keys *keyspace.KeySpace,
	roots []BoundaryFactorRoot,
	selectedIdentities []identity.Term,
	allIdentities bool,
) (BoundaryFactorSelection, error) {
	if keys == nil || !keys.Valid() {
		return BoundaryFactorSelection{}, fmt.Errorf("state: factorwise boundary selection requires a valid keyspace")
	}
	closure := emptyBoundaryClosure()
	closure.allIdentities = allIdentities
	// Root order is semantic: BoundaryTransport addresses this tuple by ordinal,
	// and repeated aliases such as f(x, x) must remain two distinct wires.
	ownedRoots := append([]BoundaryFactorRoot(nil), roots...)
	for index, root := range ownedRoots {
		// The tuple ordinal, not a structural address, owns the root scalar.
		// Consequently the empty Slot/Path pair is the canonical value-only
		// spelling and contributes nothing to the structural closure. This is
		// required for scalar call arguments and matches BoundaryArtifact.
		if root.Slot != 0 {
			if !validBoundaryRootSlot(root.Slot) {
				return BoundaryFactorSelection{}, fmt.Errorf("state: factorwise boundary root %d has malformed slot", index)
			}
			closure.slots[root.Slot] = struct{}{}
		}
		if root.Path.Kind != keyspace.KindInvalid {
			if keys.FormatReadOnly(root.Path) == "" {
				return BoundaryFactorSelection{}, fmt.Errorf("state: factorwise boundary root %d belongs to a foreign keyspace", index)
			}
			closure.paths[root.Path] = struct{}{}
		}
	}
	for index, term := range selectedIdentities {
		if !term.Valid() {
			return BoundaryFactorSelection{}, fmt.Errorf("state: factorwise boundary identity %d is empty", index)
		}
		closure.identities[term] = struct{}{}
	}
	return BoundaryFactorSelection{seal: &boundaryFactorSelectionSeal{}, keys: keys, closure: closure, roots: ownedRoots}, nil
}

// ExpandCoordinateClosure computes the least structural closure demanded by
// the registered coordinate keys. Coordinate scalars remain independently
// guarded; this transaction adds only routing capability, never a fact. A
// coupled must relation can therefore acquire destinations for every endpoint
// before Project→Rebase without assembling a State.
func (d ProductDomain) ExpandBoundaryFactorCoordinateClosure(selection BoundaryFactorSelection, slots []CoordinateSlot) (BoundaryFactorSelection, error) {
	if !d.Valid() || !selection.valid() {
		return BoundaryFactorSelection{}, fmt.Errorf("%w: coordinate boundary closure is unowned", ErrInvalidLaneFactor)
	}
	program, err := d.PrepareBoundaryCoordinateReachability(selection.keys, slots)
	if err != nil {
		return BoundaryFactorSelection{}, err
	}
	return program.Close(selection, nil)
}

// selectBoundaryFactorCoordinates computes the structural inverse image of an
// exact, already-selected coordinate inventory. Unlike demand-driven boundary
// expansion, every registered slot is a seed. The same family reachability
// declarations and least-fixed-point engine are used in both directions.
func (d ProductDomain) selectBoundaryFactorCoordinates(selection BoundaryFactorSelection, slots []CoordinateSlot) (BoundaryFactorSelection, error) {
	if !d.Valid() || !selection.valid() {
		return BoundaryFactorSelection{}, fmt.Errorf("%w: coordinate boundary selection is unowned", ErrInvalidLaneFactor)
	}
	program, err := d.PrepareBoundaryCoordinateReachability(selection.keys, slots)
	if err != nil {
		return BoundaryFactorSelection{}, err
	}
	return program.closeSelected(selection)
}

// PrepareBoundaryCoordinateReachability freezes the exact selected coordinate
// inventory into a declarative program. Artifacts retain this program; Apply
// executes Close only and never scans coordinate factors or family inventory.
func (d ProductDomain) PrepareBoundaryCoordinateReachability(keys *keyspace.KeySpace, slots []CoordinateSlot) (BoundaryReachabilityProgram, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return BoundaryReachabilityProgram{}, fmt.Errorf("%w: coordinate boundary reachability is unowned", ErrInvalidLaneFactor)
	}
	builder := newBoundaryReachabilityProgramBuilder(d.reg, keys)
	for index, slot := range slots {
		coordinate, err := d.validateCoordinateFamily(slot.family)
		if err != nil || d.validateCoordinateSlotFor(coordinate, slot, keys) != nil {
			return BoundaryReachabilityProgram{}, fmt.Errorf("%w: invalid coordinate reachability slot %d", ErrInvalidLaneFactor, index)
		}
		coordinate.boundary.reachabilityKey(builder, slot.key)
		builder.resetClause()
	}
	return builder.seal()
}

// WithIdentities binds the conditionally reachable identity fiber to an
// already-expanded structural selection. Structural closure is immutable and
// shared only by value; identity maps are copied before extension.
func (selection BoundaryFactorSelection) WithIdentities(selected []identity.Term, all bool) (BoundaryFactorSelection, error) {
	if !selection.valid() {
		return BoundaryFactorSelection{}, fmt.Errorf("%w: boundary identity selection is unowned", ErrInvalidLaneFactor)
	}
	closure := cloneBoundaryFactorClosure(selection.closure)
	closure.allIdentities = closure.allIdentities || all
	for index, term := range selected {
		if !term.Valid() {
			return BoundaryFactorSelection{}, fmt.Errorf("state: factorwise boundary identity %d is empty", index)
		}
		closure.identities[term] = struct{}{}
	}
	return BoundaryFactorSelection{
		seal: &boundaryFactorSelectionSeal{}, keys: selection.keys, closure: closure,
		roots:       append([]BoundaryFactorRoot(nil), selection.roots...),
		coordinates: selection.coordinates, exactCoordinates: selection.exactCoordinates,
	}, nil
}

func cloneBoundaryFactorClosure(source BoundaryClosure) BoundaryClosure {
	out := emptyBoundaryClosure()
	out.allIdentities = source.allIdentities
	for value := range source.slots {
		out.slots[value] = struct{}{}
	}
	for value := range source.paths {
		out.paths[value] = struct{}{}
	}
	for value := range source.identities {
		out.identities[value] = struct{}{}
	}
	for value := range source.heapSuffixes {
		out.heapSuffixes[value] = struct{}{}
	}
	return out
}
