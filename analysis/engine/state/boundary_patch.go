package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryPatch is the immutable, ProductDomain-owned transposition of one
// final projected and rebased BoundaryArtifact. Each application reads only
// one destination factor and the sealed artifact closure. Lane semantics stay
// owned by the same catalog hooks used by ApplyBoundary.
type BoundaryPatch struct {
	domain   ProductDomain
	keys     *keyspace.KeySpace
	closure  BoundaryClosure
	rootPlan boundaryRootPlan
	lanes    []boundaryPatchLane
	mask     laneMask
}

type boundaryPatchLane struct {
	lane     ProductLane
	fragment LaneFactor
	apply    func(*boundaryApplyContext, laneFactorPayload, laneFactorPayload) (laneFactorPayload, bool)
	roots    func(*boundaryApplyContext, laneFactorPayload, boundaryRootPlan) (laneFactorPayload, bool)
}

// boundaryRootPlan is the once-sealed root transaction. Slot and path roots
// are separated so only their owning factors scan them; every other factor
// receives the O(1) reachability consequence.
type boundaryRootPlan struct {
	slots                   BoundaryRoots
	paths                   BoundaryRoots
	establishesReachability bool
}

func sealBoundaryRootPlan(reg *axis.Registry, roots BoundaryRoots) boundaryRootPlan {
	plan := boundaryRootPlan{}
	bottom := product.Bottom(reg)
	for _, root := range roots {
		if root.Slot != 0 {
			plan.slots = append(plan.slots, root)
			if !product.Equal(reg, root.Value, bottom) {
				plan.establishesReachability = true
			}
		}
		if root.Path.Kind != keyspace.KindInvalid {
			plan.paths = append(plan.paths, root)
			plan.establishesReachability = true
		}
	}
	return plan
}

// SealBoundaryPatch binds a final boundary artifact to this product's exact
// registry, keyspace, lane inventory, and catalog hooks. Missing or reordered
// inventory fails before any factor can be published.
func (d ProductDomain) SealBoundaryPatch(keys *keyspace.KeySpace, artifact BoundaryArtifact) (BoundaryPatch, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || artifact.reg != d.reg || artifact.keys != keys {
		return BoundaryPatch{}, fmt.Errorf("state: boundary patch requires product registry and artifact keyspace authority")
	}
	if !artifact.world.canonical || artifact.world.laneMask != d.mask || len(d.factorLanes) != d.lanes.Len() {
		return BoundaryPatch{}, fmt.Errorf("state: boundary patch artifact lane inventory differs from product domain")
	}
	factors, err := d.Decompose(artifact.world)
	if err != nil {
		return BoundaryPatch{}, err
	}
	patch := BoundaryPatch{
		domain: d, keys: keys, closure: artifact.closure, mask: artifact.world.laneMask,
		lanes: make([]boundaryPatchLane, len(d.factorLanes)),
	}
	rootPlan := sealBoundaryRootPlan(d.reg, artifact.roots)
	patch.rootPlan = rootPlan
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		if runtime.ops.boundaryApply == nil || runtime.ops.boundaryRoots == nil || factors[index].lane != runtime.lane {
			return BoundaryPatch{}, fmt.Errorf("state: boundary patch lane %q has no exact catalog hook", runtime.lane.id)
		}
		patch.lanes[index] = boundaryPatchLane{
			lane: runtime.lane, fragment: factors[index],
			apply: runtime.ops.boundaryApply, roots: runtime.ops.boundaryRoots,
		}
	}
	return patch, nil
}

func (p BoundaryPatch) valid() bool {
	return p.domain.Valid() && p.keys != nil && p.keys.Valid() && len(p.lanes) == len(p.domain.factorLanes) &&
		p.mask == p.domain.mask
}

// ApplyLane applies the artifact closure to one opaque non-Values component.
// Values uses ApplyValues because its exact representation is slot-factored.
func (p BoundaryPatch) ApplyLane(destination LaneFactor) (LaneFactor, error) {
	if !p.valid() {
		return LaneFactor{}, fmt.Errorf("%w: invalid boundary patch", ErrInvalidLaneFactor)
	}
	runtime, err := p.domain.validateFactor(destination)
	if err != nil {
		return LaneFactor{}, err
	}
	if runtime.lane.slotFactored {
		return LaneFactor{}, fmt.Errorf("%w: Values requires boundary ValueLaneFactor", ErrInvalidLaneFactor)
	}
	if runtime.lane.id == LaneHeapTableIdentity || runtime.lane.id == LanePlacement {
		return LaneFactor{}, fmt.Errorf("%w: identity-indexed lane %q requires a factored boundary patch", ErrInvalidLaneFactor, runtime.lane.id)
	}
	index := int(runtime.lane.ordinal)
	if index < 0 || index >= len(p.lanes) {
		return LaneFactor{}, fmt.Errorf("%w: boundary patch lane ordinal", ErrInvalidLaneFactor)
	}
	sealed := p.lanes[index]
	if sealed.lane != runtime.lane || sealed.apply == nil {
		return LaneFactor{}, fmt.Errorf("%w: boundary patch lane inventory drift", ErrInvalidLaneFactor)
	}
	ctx := boundaryApplyContext{reg: p.domain.reg, keys: p.keys, closure: p.closure}
	payload, ok := sealed.apply(&ctx, destination.payload, sealed.fragment.payload)
	if !ok {
		return LaneFactor{}, fmt.Errorf("state: boundary patch apply failed in lane %q", runtime.lane.id)
	}
	payload, ok = sealed.roots(&ctx, payload, p.rootPlan)
	if !ok {
		return LaneFactor{}, fmt.Errorf("state: boundary patch root apply failed in lane %q", runtime.lane.id)
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// ApplyValues applies the Values closure replacement and then materializes the
// artifact roots, exactly matching ApplyBoundary ordering.
func (p BoundaryPatch) ApplyValues(destination ValueLaneFactor) (ValueLaneFactor, error) {
	if !p.valid() || !p.domain.ValuesEnabled() {
		return ValueLaneFactor{}, fmt.Errorf("%w: invalid Values boundary patch", ErrInvalidLaneFactor)
	}
	if !p.domain.hasSlotFactor || int(p.domain.slotFactor) >= len(p.domain.factorLanes) {
		return ValueLaneFactor{}, fmt.Errorf("%w: Values boundary lane is absent", ErrInvalidLaneFactor)
	}
	runtime := &p.domain.factorLanes[p.domain.slotFactor]
	index := int(runtime.lane.ordinal)
	if index < 0 || index >= len(p.lanes) || p.lanes[index].lane != runtime.lane || p.lanes[index].apply == nil {
		return ValueLaneFactor{}, fmt.Errorf("%w: Values boundary lane inventory drift", ErrInvalidLaneFactor)
	}
	ctx := boundaryApplyContext{reg: p.domain.reg, keys: p.keys, closure: p.closure}
	destinationPayload := typedLaneFactorPayload[valueLane]{value: valueLaneFromFactor(p.domain.reg, destination)}
	payload, ok := p.lanes[index].apply(&ctx, destinationPayload, p.lanes[index].fragment.payload)
	if !ok {
		return ValueLaneFactor{}, fmt.Errorf("state: boundary patch apply failed in lane %q", LaneValues)
	}
	payload, ok = p.lanes[index].roots(&ctx, payload, p.rootPlan)
	if !ok {
		return ValueLaneFactor{}, fmt.Errorf("state: boundary patch root apply failed in lane %q", LaneValues)
	}
	return valueLaneToFactor(typedLaneFactorValue[valueLane](payload)), nil
}

// Apply executes the same sealed per-factor transaction used by ApplyLane and
// ApplyValues over a complete State. It is the sole whole-product executor.
func (p BoundaryPatch) Apply(destination State) (State, error) {
	if !p.valid() || destination.laneMask != p.mask {
		return State{}, fmt.Errorf("state: destination and boundary patch lane inventories differ")
	}
	ctx := boundaryApplyContext{reg: p.domain.reg, keys: p.keys, closure: p.closure}
	out := destination
	for index := range p.lanes {
		sealed := p.lanes[index]
		runtime := &p.domain.factorLanes[index]
		payload, ok := sealed.apply(&ctx, runtime.ops.extract(destination), sealed.fragment.payload)
		if !ok {
			return State{}, fmt.Errorf("state: boundary patch apply failed in lane %q", runtime.lane.id)
		}
		payload, ok = sealed.roots(&ctx, payload, p.rootPlan)
		if !ok {
			return State{}, fmt.Errorf("state: boundary patch root apply failed in lane %q", runtime.lane.id)
		}
		runtime.ops.install(&out, payload)
	}
	out.canonical = true
	return p.domain.Normalize(out), nil
}

func (d ProductDomain) runtimeForLaneID(id LaneID) (*productLaneRuntime, bool) {
	for index := range d.factorLanes {
		if d.factorLanes[index].lane.id == id {
			return &d.factorLanes[index], true
		}
	}
	return nil, false
}
