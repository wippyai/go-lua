package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// LaneCatalog is the registration boundary for State product-lattice axes.
// The State container folds registered lane operations and reachability
// transitions; it does not need a second hand-maintained list of semantic lanes.
type LaneCatalog struct {
	specs []laneSpec
}

func newLaneCatalog(specs []laneSpec) LaneCatalog {
	out := make([]laneSpec, len(specs))
	copy(out, specs)
	closureCompanion := -1
	for i := range out {
		if out[i].fingerprint == nil {
			panic(fmt.Sprintf("state: lane %q has no semantic fingerprint", out[i].id))
		}
		if out[i].boundary.project == nil || out[i].boundary.rebase == nil ||
			out[i].boundary.postRebase == nil || out[i].boundary.equal == nil {
			panic(fmt.Sprintf("state: lane %q has incomplete boundary transport", out[i].id))
		}
		switch out[i].valueDependencies.kind {
		case laneValueDependenciesIndependent:
			if out[i].valueDependencies.visit != nil {
				panic(fmt.Sprintf("state: Values-independent lane %q has a dependency enumerator", out[i].id))
			}
		case laneValueDependenciesEnumerated:
			if out[i].valueDependencies.visit == nil {
				panic(fmt.Sprintf("state: Values-dependent lane %q has no dependency enumerator", out[i].id))
			}
		default:
			panic(fmt.Sprintf("state: lane %q has no Values dependency declaration", out[i].id))
		}
		switch out[i].identitySupport.kind {
		case laneIdentitiesIndependent:
			if out[i].identitySupport.visit != nil || out[i].identitySupport.visitState != nil || out[i].identitySupport.image != IdentityImageIndependent {
				panic(fmt.Sprintf("state: identity-independent lane %q has an identity enumerator", out[i].id))
			}
		case laneIdentitiesEnumerated:
			if out[i].identitySupport.visit == nil || out[i].identitySupport.visitState == nil || !out[i].identitySupport.image.valid() || out[i].identitySupport.image == IdentityImageIndependent {
				panic(fmt.Sprintf("state: identity-bearing lane %q has no identity enumerator", out[i].id))
			}
		default:
			panic(fmt.Sprintf("state: lane %q has no identity-support declaration", out[i].id))
		}
		switch out[i].numericConsistency.kind {
		case laneNumericConsistencyIndependent:
			if out[i].numericConsistency.contribute != nil {
				panic(fmt.Sprintf("state: numeric-independent lane %q has a contributor", out[i].id))
			}
		case laneNumericConsistencyContributor:
			if out[i].numericConsistency.contribute == nil {
				panic(fmt.Sprintf("state: numeric lane %q has no contributor", out[i].id))
			}
		default:
			panic(fmt.Sprintf("state: lane %q has no numeric-consistency declaration", out[i].id))
		}
		seenSemantic := make(map[laneSemanticCapabilityID]struct{}, len(out[i].semanticLaws))
		for _, law := range out[i].semanticLaws {
			dedicatedPathReplacement := law.id == laneSemanticPathReplacement && law.pathReplacement.declared && law.pathReplacement.apply != nil
			if law.id == "" || dedicatedPathReplacement && (law.applyState != nil || law.applyFactor != nil) ||
				!dedicatedPathReplacement && (law.applyState == nil || law.applyFactor == nil) {
				panic(fmt.Sprintf("state: lane %q has an incomplete semantic capability law", out[i].id))
			}
			if law.id == laneSemanticGenericForBinding && law.genericForBinding == nil {
				panic(fmt.Sprintf("state: lane %q has an incomplete semantic capability %q", out[i].id, law.id))
			}
			if law.id != laneSemanticGenericForBinding && law.genericForBinding != nil {
				panic(fmt.Sprintf("state: lane %q attaches a generic-for binding to semantic capability %q", out[i].id, law.id))
			}
			if law.id == laneSemanticEffectFactor {
				if law.participates != (law.effectKinds != 0) || law.effectKinds&^(effectFactorDynamicIndexMembership|effectFactorDelta) != 0 {
					panic(fmt.Sprintf("state: lane %q has invalid effect-factor declaration", out[i].id))
				}
			} else if law.effectKinds != 0 {
				panic(fmt.Sprintf("state: lane %q attaches effect kinds to semantic capability %q", out[i].id, law.id))
			}
			if _, duplicate := seenSemantic[law.id]; duplicate {
				panic(fmt.Sprintf("state: lane %q duplicates semantic capability %q", out[i].id, law.id))
			}
			seenSemantic[law.id] = struct{}{}
		}
		for _, id := range registeredLaneSemanticCapabilities {
			if _, ok := seenSemantic[id]; !ok {
				panic(fmt.Sprintf("state: lane %q has no semantic capability %q", out[i].id, id))
			}
		}
		if len(seenSemantic) != len(registeredLaneSemanticCapabilities) {
			panic(fmt.Sprintf("state: lane %q registered an unknown semantic capability", out[i].id))
		}
		switch out[i].boundaryClosureCompanion.kind {
		case laneBoundaryClosureCompanionNone:
		case laneBoundaryClosureCompanionUnique:
			if closureCompanion >= 0 {
				panic(fmt.Sprintf("state: lanes %q and %q both declare the boundary closure companion", out[closureCompanion].id, out[i].id))
			}
			closureCompanion = i
		default:
			panic(fmt.Sprintf("state: lane %q has no boundary closure companion declaration", out[i].id))
		}
		rootAssignment := out[i].rootAssignment
		if !rootAssignment.declared || rootAssignment.applyState == nil || rootAssignment.applyFactor == nil ||
			rootAssignment.applyScalarState == nil || rootAssignment.applyScalarFactor == nil {
			panic(fmt.Sprintf("state: lane %q has no complete root-assignment law", out[i].id))
		}
		completionDependencies := rootAssignment.completionDependencies.bits
		if completionDependencies&^rootAssignmentCompletionAllDependencies != 0 ||
			(rootAssignment.completion && completionDependencies == 0) ||
			(!rootAssignment.completion && completionDependencies != 0) {
			panic(fmt.Sprintf("state: lane %q has invalid root-assignment completion dependencies", out[i].id))
		}
		if rootAssignment.dynamicSourceInput > rootAssignmentDynamicSourceInputMemberships {
			panic(fmt.Sprintf("state: lane %q has invalid dynamic-source input role", out[i].id))
		}
		switch out[i].keySpaceMode {
		case laneKeySpaceFree:
			if out[i].rekey != nil {
				panic(fmt.Sprintf("state: keyspace-free lane %q has a rekey operation", out[i].id))
			}
		case laneKeySpaceOwned:
			if out[i].rekey == nil {
				panic(fmt.Sprintf("state: keyspace-owned lane %q has no rekey operation", out[i].id))
			}
		default:
			panic(fmt.Sprintf("state: lane %q has no keyspace ownership declaration", out[i].id))
		}
		familyIDs := make(map[CoordinateFamilyID]struct{}, len(out[i].coordinateFamilies))
		for familyIndex, family := range out[i].coordinateFamilies {
			if family.id == "" {
				panic(fmt.Sprintf("state: lane %q coordinate family %d has no identity", out[i].id, familyIndex))
			}
			if _, duplicate := familyIDs[family.id]; duplicate {
				panic(fmt.Sprintf("state: lane %q has duplicate coordinate family %q", out[i].id, family.id))
			}
			familyIDs[family.id] = struct{}{}
			if family.build == nil {
				panic(fmt.Sprintf("state: lane %q coordinate family %q has no lattice builder", out[i].id, family.id))
			}
			if !coordinateBoundaryOpsComplete(family.boundary) {
				panic(fmt.Sprintf("state: lane %q coordinate family %q has incomplete boundary law", out[i].id, family.id))
			}
			if !family.identityImage.valid() {
				panic(fmt.Sprintf("state: lane %q coordinate family %q has no identity-image law", out[i].id, family.id))
			}
		}
		ordinary := !out[i].slotFactored && len(out[i].coordinateFamilies) == 0
		switch out[i].formalRekey.kind {
		case laneFormalRekeyIndependent, laneFormalRekeyStructural:
			if !ordinary {
				panic(fmt.Sprintf("state: non-ordinary lane %q declares ordinary formal rekey", out[i].id))
			}
		case laneFormalRekeyInvalid:
			if ordinary {
				panic(fmt.Sprintf("state: ordinary lane %q has no formal rekey declaration", out[i].id))
			}
		default:
			panic(fmt.Sprintf("state: lane %q has invalid formal rekey declaration", out[i].id))
		}
		out[i].bit = laneBit(i)
	}
	return LaneCatalog{specs: out}
}

// DefaultLaneCatalog returns the standard set of State lanes.
func DefaultLaneCatalog() LaneCatalog {
	return defaultLaneCatalog
}

// LaneSet returns the ordered lane IDs in this catalog.
func (c LaneCatalog) LaneSet() LaneSet {
	out := make([]LaneID, 0, len(c.specs))
	for _, spec := range c.specs {
		out = append(out, spec.id)
	}
	return LaneSet{ids: out}
}

// Domain builds a State lattice with every lane in this catalog enabled.
func (c LaneCatalog) Domain(reg *axis.Registry) lattice.Lattice[State] {
	return c.ProductDomain(reg).Lattice()
}

func (c LaneCatalog) ProductDomain(reg *axis.Registry) ProductDomain {
	return newProductDomain(reg, c.LaneSet(), DomainOptions{}, domainFromLaneSpecs(reg, c.specs, c.specs), c.specs)
}

// DomainWithOptions builds a State lattice with every lane in this catalog
// enabled and per-solve options applied.
func (c LaneCatalog) DomainWithOptions(reg *axis.Registry, options DomainOptions) lattice.Lattice[State] {
	return c.ProductDomainWithOptions(reg, options).Lattice()
}

func (c LaneCatalog) ProductDomainWithOptions(reg *axis.Registry, options DomainOptions) ProductDomain {
	return newProductDomain(reg, c.LaneSet(), options, domainFromLaneSpecsWithOptions(reg, c.specs, c.specs, options), c.specs)
}

// DomainWithLaneSet builds a State lattice from an exact ordered lane
// selection against this catalog.
func (c LaneCatalog) DomainWithLaneSet(reg *axis.Registry, lanes LaneSet) lattice.Lattice[State] {
	domain, err := c.TryDomainWithLaneSet(reg, lanes)
	if err != nil {
		panic(err)
	}
	return domain
}

// TryDomainWithLaneSet builds a State lattice from an exact ordered lane
// selection against this catalog, returning configuration errors instead of
// panicking.
func (c LaneCatalog) TryDomainWithLaneSet(reg *axis.Registry, lanes LaneSet) (lattice.Lattice[State], error) {
	specs, err := c.selectSpecs(lanes)
	if err != nil {
		return lattice.Lattice[State]{}, err
	}
	return domainFromLaneSpecs(reg, specs, c.specs), nil
}

// TryDomainWithLaneSetAndOptions builds a selected-lane domain with per-solve
// options applied, returning configuration errors instead of panicking.
func (c LaneCatalog) TryDomainWithLaneSetAndOptions(reg *axis.Registry, lanes LaneSet, options DomainOptions) (lattice.Lattice[State], error) {
	specs, err := c.selectSpecs(lanes)
	if err != nil {
		return lattice.Lattice[State]{}, err
	}
	return domainFromLaneSpecsWithOptions(reg, specs, c.specs, options), nil
}

func (c LaneCatalog) TryProductDomainWithLaneSet(reg *axis.Registry, lanes LaneSet) (ProductDomain, error) {
	specs, err := c.selectSpecs(lanes)
	if err != nil {
		return ProductDomain{}, err
	}
	return newProductDomain(reg, lanes, DomainOptions{}, domainFromLaneSpecs(reg, specs, c.specs), specs), nil
}

func (c LaneCatalog) TryProductDomainWithLaneSetAndOptions(reg *axis.Registry, lanes LaneSet, options DomainOptions) (ProductDomain, error) {
	specs, err := c.selectSpecs(lanes)
	if err != nil {
		return ProductDomain{}, err
	}
	return newProductDomain(reg, lanes, options, domainFromLaneSpecsWithOptions(reg, specs, c.specs, options), specs), nil
}

// ValidateLaneSet checks that every selected lane exists in this catalog and
// that no lane is selected more than once.
func (c LaneCatalog) ValidateLaneSet(lanes LaneSet) error {
	_, err := c.selectSpecs(lanes)
	return err
}

func (c LaneCatalog) mustLaneBit(id LaneID) laneBit {
	for _, spec := range c.specs {
		if spec.id == id {
			return spec.bit
		}
	}
	panic(fmt.Sprintf("state: unknown lane %q", id))
}

type reachableLaneOp struct {
	bit           laneBit
	markReachable func(State) State
}

func (c LaneCatalog) reachableOps() []reachableLaneOp {
	out := make([]reachableLaneOp, 0, len(c.specs))
	for _, spec := range c.specs {
		if spec.markReachable != nil {
			out = append(out, reachableLaneOp{
				bit:           spec.bit,
				markReachable: spec.markReachable,
			})
		}
	}
	return out
}

func (c LaneCatalog) selectSpecs(lanes LaneSet) ([]laneSpec, error) {
	known := make(map[LaneID]struct{}, len(c.specs))
	for _, spec := range c.specs {
		known[spec.id] = struct{}{}
	}
	seen := make(map[LaneID]struct{}, lanes.Len())
	for _, id := range lanes.ids {
		if _, ok := known[id]; !ok {
			return nil, fmt.Errorf("state: unknown lane %q", id)
		}
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("state: duplicate lane %q", id)
		}
		seen[id] = struct{}{}
	}
	out := make([]laneSpec, 0, lanes.Len())
	for _, spec := range c.specs {
		if _, ok := seen[spec.id]; ok {
			out = append(out, spec)
		}
	}
	return out, nil
}
