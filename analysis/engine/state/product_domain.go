package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// productDomainSeal must have non-zero size: Go permits distinct zero-sized
// allocations to share an address, which would collapse cross-domain factor
// ownership checks.
type productDomainSeal struct{ owned byte }

// productDomainRuntime is constructed once by the registered lane catalog and
// immutable thereafter. ProductDomain is a small capability handle to this
// runtime; copying a domain must never copy the complete registered catalog.
type productDomainRuntime struct {
	reg                              *axis.Registry
	lattice                          lattice.Lattice[State]
	lanes                            LaneSet
	options                          DomainOptions
	factorLanes                      []productLaneRuntime
	slotFactor                       LaneOrdinal
	hasSlotFactor                    bool
	boundaryClosureCompanion         LaneOrdinal
	hasBoundaryClosureCompanion      bool
	pathEvidenceFamily               CoordinateFamily
	hasPathEvidenceFamily            bool
	pathValueFamily                  CoordinateFamily
	hasPathValueFamily               bool
	rootAssignmentFamily             CoordinateFamily
	hasRootAssignmentFamily          bool
	returnIdentityContainerFamily    CoordinateFamily
	hasReturnIdentityContainerFamily bool
	mask                             laneMask
	seal                             *productDomainSeal
}

// ProductDomain is the unforgeable capability produced by the registered
// State lane catalog. It proves that whole-State operations are exactly the
// componentwise product of the selected lanes and options.
type ProductDomain struct {
	*productDomainRuntime
}

func newProductDomain(reg *axis.Registry, lanes LaneSet, options DomainOptions, domain lattice.Lattice[State], specs []laneSpec) ProductDomain {
	out := ProductDomain{
		productDomainRuntime: &productDomainRuntime{
			reg: reg, lattice: domain, lanes: NewLaneSet(lanes.IDs()...),
			options: DomainOptions{WidenThresholds: append([]int64(nil), options.WidenThresholds...)},
			seal:    &productDomainSeal{},
		},
	}
	bits := make([]laneBit, 0, len(specs))
	hasReturnIdentityClosure := false
	for _, spec := range specs {
		if !spec.dynamicRead.declared {
			panic("state: lane " + string(spec.id) + " has no dynamic-read query law")
		}
		if (spec.dynamicRead.demand != nil) != (spec.dynamicRead.project != nil && spec.dynamicRead.observe != nil) {
			panic("state: lane " + string(spec.id) + " has incomplete dynamic-read projection law")
		}
		bits = append(bits, spec.bit)
		ordinal := LaneOrdinal(len(out.factorLanes))
		if spec.slotFactored {
			if out.hasSlotFactor {
				panic("state: product has more than one slot-factored lane")
			}
			out.slotFactor, out.hasSlotFactor = ordinal, true
		}
		if spec.boundaryClosureCompanion.kind == laneBoundaryClosureCompanionUnique {
			if out.hasBoundaryClosureCompanion {
				panic("state: product has more than one boundary closure companion")
			}
			out.boundaryClosureCompanion, out.hasBoundaryClosureCompanion = ordinal, true
		}
		ops := spec.build(reg, options).factor
		if ops.meet == nil {
			panic("state: lane " + string(spec.id) + " has no exact factor meet")
		}
		if ops.canonicalEqual == nil {
			panic("state: lane " + string(spec.id) + " has no canonical factor representation law")
		}
		if ops.boundaryApply == nil || ops.boundaryRoots == nil || !ops.boundaryRootUse.declared || ops.boundaryProject == nil ||
			ops.boundaryRebase == nil || ops.boundaryPostRebase == nil {
			panic("state: lane " + string(spec.id) + " has incomplete factor boundary transport")
		}
		if ops.boundaryReachability == nil {
			panic("state: lane " + string(spec.id) + " has no typed boundary reachability program")
		}
		if spec.boundaryClosureCompanion.kind == laneBoundaryClosureCompanionUnique && ops.boundaryClosureExtend == nil {
			panic("state: boundary closure companion " + string(spec.id) + " has no factor extension law")
		}
		lane := ProductLane{
			seal:         out.seal,
			ordinal:      ordinal,
			id:           spec.id,
			slotFactored: spec.slotFactored,
		}
		coordinates := make([]coordinateFamilyRuntime, len(spec.coordinateFamilies))
		for familyIndex, familySpec := range spec.coordinateFamilies {
			if !familySpec.dynamicRead.declared {
				panic("state: lane " + string(spec.id) + " coordinate family " + string(familySpec.id) + " has no dynamic-read query law")
			}
			familyOps := familySpec.build(reg, options)
			if !coordinateFamilyOpsComplete(familyOps) {
				panic("state: lane " + string(spec.id) + " coordinate family " + string(familySpec.id) + " has incomplete lattice operations")
			}
			family := CoordinateFamily{seal: out.seal, lane: lane, ordinal: CoordinateFamilyOrdinal(familyIndex), id: familySpec.id}
			roles := familyOps.returnIdentity.roles
			if roles.has(CoordinateReturnIdentitySeed) || roles.has(CoordinateReturnIdentitySkeletonEdge) || roles.has(CoordinateReturnIdentityScalarEdge) {
				hasReturnIdentityClosure = true
			}
			if roles.has(CoordinateReturnIdentityContainer) {
				if out.hasReturnIdentityContainerFamily {
					panic("state: product has more than one return-identity container family")
				}
				out.returnIdentityContainerFamily, out.hasReturnIdentityContainerFamily = family, true
			}
			if familyOps.pathEvidence.kind == coordinatePathEvidenceUnique {
				if out.hasPathEvidenceFamily {
					panic("state: product has more than one path-evidence coordinate family")
				}
				out.pathEvidenceFamily, out.hasPathEvidenceFamily = family, true
			}
			if familyOps.pathValues.kind == coordinatePathValueUnique {
				if out.hasPathValueFamily {
					panic("state: product has more than one path-value coordinate family")
				}
				out.pathValueFamily, out.hasPathValueFamily = family, true
			}
			if familyOps.rootAssignment.kind == coordinateRootAssignmentUnique {
				if out.hasRootAssignmentFamily {
					panic("state: product has more than one root-assignment coordinate family")
				}
				out.rootAssignmentFamily, out.hasRootAssignmentFamily = family, true
			}
			coordinates[familyIndex] = coordinateFamilyRuntime{
				family: family,
				ops:    familyOps, boundary: familySpec.boundary, dynamicRead: familySpec.dynamicRead,
				identityImage: familySpec.identityImage,
			}
		}
		out.factorLanes = append(out.factorLanes, productLaneRuntime{
			lane:               lane,
			ops:                ops,
			fingerprint:        spec.fingerprint,
			valueDependencies:  spec.valueDependencies,
			identitySupport:    spec.identitySupport,
			numericConsistency: spec.numericConsistency,
			rootAssignment:     spec.rootAssignment,
			dynamicRead:        spec.dynamicRead,
			semanticLaws:       append([]laneSemanticLaw(nil), spec.semanticLaws...),
			formalRekey:        spec.formalRekey,
			coordinates:        coordinates,
		})
	}
	if hasReturnIdentityClosure != out.hasReturnIdentityContainerFamily {
		if hasReturnIdentityClosure {
			panic("state: return-identity closure has no unique container family")
		}
		panic("state: return-identity container family has no seed or edge producer")
	}
	out.mask = scopedLaneMask(bits)
	// Whole-State Meet is the concrete wrapper of the already-admitted factor
	// law. It contains no lane semantics: every component dispatches through
	// its registered opaque meet, exactly like the formal/guarded carrier.
	// Installing it only after the inventory is sealed makes the aggregate
	// operation automatically complete when axes are added or removed.
	out.lattice.Meet = func(left, right State) State {
		left, right = out.Normalize(left), out.Normalize(right)
		result := out.lattice.Bottom()
		for index := range out.factorLanes {
			runtime := out.factorLanes[index]
			factor := runtime.ops.meet(runtime.ops.extract(left), runtime.ops.extract(right))
			runtime.ops.install(&result, factor)
		}
		result.canonical = true
		result.numericConsistency = numericConsistencyUnknown
		return out.Normalize(result)
	}
	return out
}

func (d ProductDomain) Valid() bool {
	return d.productDomainRuntime != nil && d.seal != nil && d.reg != nil && len(d.factorLanes) == d.lanes.Len() &&
		d.lattice.Bottom != nil && d.lattice.Equal != nil && d.lattice.LessOrEq != nil &&
		d.lattice.Join != nil && d.lattice.Widen != nil
}

func (d ProductDomain) Registry() *axis.Registry {
	if d.productDomainRuntime == nil {
		return nil
	}
	return d.reg
}
func (d ProductDomain) Lattice() lattice.Lattice[State] {
	if d.productDomainRuntime == nil {
		return lattice.Lattice[State]{}
	}
	return d.lattice
}
func (d ProductDomain) Lanes() LaneSet {
	if d.productDomainRuntime == nil {
		return LaneSet{}
	}
	return NewLaneSet(d.lanes.IDs()...)
}
func (d ProductDomain) Options() DomainOptions {
	if d.productDomainRuntime == nil {
		return DomainOptions{}
	}
	return DomainOptions{WidenThresholds: append([]int64(nil), d.options.WidenThresholds...)}
}
func (d ProductDomain) ValuesEnabled() bool { return d.Valid() && d.hasSlotFactor }

// Normalize returns value in this product's exact lane scope. Canonical values
// already owned by the same scope are immutable fixed points and incur no
// whole-product join; foreign or provisional spellings take the defensive
// normalization path.
func (d ProductDomain) Normalize(value State) State {
	if d.Valid() && value.canonical && value.laneMask == d.mask && value.numericConsistency == numericConsistencyCertified {
		return value
	}
	if !value.canonical || value.laneMask != d.mask {
		value = NormalizeForDomain(d.lattice, value)
	}
	return d.certifyNumericConsistency(value)
}

// VisitValueDependencies enumerates the exact finite Values roots referenced
// by enabled residual lanes in value. Roots are either concrete State cells or
// typed formal relation coordinates. It is the registered cross-axis seam used
// by product factorization; adding an axis never requires editing the solver.
func (d ProductDomain) VisitValueDependencies(value State, keys *keyspace.KeySpace, visit func(key.ValueDependency)) {
	if !d.Valid() || keys == nil || visit == nil {
		return
	}
	for i := range d.factorLanes {
		dependencies := d.factorLanes[i].valueDependencies
		if dependencies.kind == laneValueDependenciesEnumerated {
			dependencies.visit(value, keys, visit)
		}
	}
}

func (d ProductDomain) ValueBottom() product.Value { return product.Bottom(d.reg) }
func (d ProductDomain) ValueTop() product.Value {
	if !d.ValuesEnabled() {
		return product.Bottom(d.reg)
	}
	return product.Top()
}
func (d ProductDomain) ValueJoin(left, right product.Value) product.Value {
	if !d.ValuesEnabled() {
		return product.Bottom(d.reg)
	}
	return product.Join(d.reg, left, right)
}
func (d ProductDomain) ValueWiden(left, right product.Value) product.Value {
	if !d.ValuesEnabled() {
		return product.Bottom(d.reg)
	}
	return product.Widen(d.reg, left, right)
}
func (d ProductDomain) ValueMeet(left, right product.Value) product.Value {
	if !d.ValuesEnabled() {
		return product.Bottom(d.reg)
	}
	return product.Meet(d.reg, left, right)
}
func (d ProductDomain) ValueLessOrEq(left, right product.Value) bool {
	return !d.ValuesEnabled() || product.Domain(d.reg).LessOrEq(left, right)
}
