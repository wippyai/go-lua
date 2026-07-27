package state

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// BoundaryFactorPatch is the registry-dispatched Project→Rebase→Apply→Roots
// transaction for one complete registered product factor. It is built
// entirely from a sealed factor and an ordered root tuple: no source State is
// assembled and no coordinate family is discovered or dispatched here.
//
// Coordinate-factored lanes are not a special case. Their registered lane
// hooks own their finite coordinate inventory and execute the same transaction
// as every other non-Values factor.
type BoundaryFactorPatch struct {
	domain   ProductDomain
	keys     *keyspace.KeySpace
	closure  BoundaryClosure
	rootPlan boundaryRootPlan
	lane     *LaneFactor
	values   *ValueLaneFactor
}

// BoundaryFactorTransportPlan is the sealed structural part of a boundary
// transaction.  It is computed once from the ordered roots, allocation
// selection, and the independently projected EffectDeltas companion.  Product
// lanes are never aligned with one another: each lane is projected and rebased
// separately against this plan.
type BoundaryFactorTransportPlan struct {
	seal          *boundaryFactorTransportSeal
	domain        ProductDomain
	sourceDomain  ProductDomain
	keys          *keyspace.KeySpace
	closure       BoundaryClosure
	targets       []BoundaryFactorTarget
	sourceTargets [][]int
	aliases       [][2]keyspace.Key
	rootPaths     boundaryPathMap
	existentials  BoundaryExistentialNamespace
	projectCtx    boundaryProjectContext
	rebaseCtx     boundaryRebaseContext
	values        BoundaryValueFactorTransport[statekey.Value, statekey.Value]
}

// boundaryFactorTransportSeal is exact transaction identity. ProductDomain
// ownership alone is insufficient: two plans over the same domain may carry
// different root relations, keyspaces, allocation images, and closure laws.
// The byte is intentional: pointers to distinct zero-size allocations are not
// required to be distinct in Go, so an empty seal would not prove identity.
type boundaryFactorTransportSeal struct{ _ byte }

// BoundaryClosureCompanionProjection is the source-domain-owned projection
// of the optional lane that extends structural boundary closure. It retains no
// whole State and is sealed to the exact factor selection that produced it.
// Destination domains consume this artifact without accepting or resealing a
// foreign LaneFactor.
type BoundaryClosureCompanionProjection struct {
	selection *boundaryFactorSelectionSeal
	reg       *axis.Registry
	domain    ProductDomain
	present   bool
	companion LaneID
	payload   laneFactorPayload
}

// ProjectBoundaryClosureCompanion validates and projects the unique optional
// closure companion under its source ProductDomain.
func (d ProductDomain) ProjectBoundaryClosureCompanion(selection BoundaryFactorSelection, companion *LaneFactor) (BoundaryClosureCompanionProjection, error) {
	if !d.Valid() || !selection.valid() {
		return BoundaryClosureCompanionProjection{}, fmt.Errorf("%w: boundary companion projection is unowned", ErrInvalidLaneFactor)
	}
	out := BoundaryClosureCompanionProjection{selection: selection.seal, reg: d.reg, domain: d}
	lane, present := d.BoundaryClosureCompanion()
	if !present {
		if companion != nil {
			return BoundaryClosureCompanionProjection{}, fmt.Errorf("%w: unexpected boundary closure companion", ErrInvalidLaneFactor)
		}
		return out, nil
	}
	runtime, err := d.validateLane(lane)
	if err != nil || companion == nil {
		return BoundaryClosureCompanionProjection{}, fmt.Errorf("%w: boundary closure companion omitted", ErrInvalidLaneFactor)
	}
	if _, err := d.validateFactorFor(runtime, *companion); err != nil {
		return BoundaryClosureCompanionProjection{}, fmt.Errorf("%w: invalid boundary closure companion", ErrInvalidLaneFactor)
	}
	ctx := boundaryProjectContext{reg: d.reg, keys: selection.keys, closure: selection.closure}
	payload, ok := runtime.ops.boundaryProject(&ctx, companion.payload)
	if !ok {
		return BoundaryClosureCompanionProjection{}, fmt.Errorf("state: boundary closure companion projection failed")
	}
	out.present = true
	out.companion = runtime.lane.id
	out.payload = payload
	return out, nil
}

// BoundaryFactorTarget is one scalar-free destination root and its source
// inverse fiber. Source order remains the caller's semantic tuple order.
type BoundaryFactorTarget struct {
	Slot    statekey.Value
	Path    keyspace.Key
	Sources []int
}

func (p BoundaryFactorTransportPlan) RootTargets() []BoundaryFactorTarget {
	out := make([]BoundaryFactorTarget, len(p.targets))
	for index, target := range p.targets {
		out[index] = target
		out[index].Sources = append([]int(nil), target.Sources...)
	}
	return out
}

type BoundaryRootContribution struct {
	Target int
	Value  product.Value
}

type BoundaryValueContribution struct {
	Slot  statekey.Value
	Value product.Value
}

func (p BoundaryFactorTransportPlan) DestinationValueSlots() []statekey.Value {
	out := make([]statekey.Value, 0, len(p.closure.slots))
	for slot := range p.closure.slots {
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// RebaseValueSlot transports one finite Values coordinate independently.
func (p BoundaryFactorTransportPlan) RebaseValueSlot(slot statekey.Value, value product.Value) ([]BoundaryValueContribution, error) {
	if slot == 0 {
		return nil, fmt.Errorf("%w: invalid Values boundary source coordinate", ErrInvalidLaneFactor)
	}
	contributions, err := p.values.RebaseSlot(slot, value)
	if err != nil {
		return nil, err
	}
	out := make([]BoundaryValueContribution, len(contributions))
	for index, contribution := range contributions {
		out[index] = BoundaryValueContribution{Slot: contribution.Slot, Value: contribution.Value}
	}
	return out, nil
}

// RebaseRootSource transports one root scalar independently. The caller joins
// contributions only with the same destination Target.
func (p BoundaryFactorTransportPlan) RebaseRootSource(source int, value product.Value) ([]BoundaryRootContribution, error) {
	if source < 0 || source >= len(p.sourceTargets) || !product.BelongsToRegistry(p.domain.reg, value) {
		return nil, fmt.Errorf("%w: invalid boundary root source scalar", ErrInvalidLaneFactor)
	}
	value = product.ProjectBoundary(p.domain.reg, value)
	rebased, ok := rebaseBoundaryValue(&p.rebaseCtx, value)
	if !ok {
		return nil, fmt.Errorf("state: boundary root source scalar rebase failed")
	}
	out := make([]BoundaryRootContribution, len(p.sourceTargets[source]))
	for index, target := range p.sourceTargets[source] {
		out[index] = BoundaryRootContribution{Target: target, Value: rebased}
	}
	return out, nil
}

// PrepareBoundaryFactorTransportPlan seals the common structural boundary
// transaction. roots retain BoundaryFactorSelection's exact ordinal order.
// companion is produced and validated by the source ProductDomain. The
// destination consumes only its opaque structural projection, never a foreign
// LaneFactor.
func (d ProductDomain) PrepareBoundaryFactorTransportPlan(
	transport *BoundaryTransport,
	selection BoundaryFactorSelection,
	companion BoundaryClosureCompanionProjection,
) (BoundaryFactorTransportPlan, error) {
	if !d.Valid() || transport == nil || !selection.valid() ||
		companion.selection != selection.seal || companion.reg != d.reg || !companion.domain.Valid() ||
		transport.authority == nil || transport.toKeys == nil || !transport.toKeys.Valid() ||
		!boundaryAllocationAuthorityCovers(selection.closure, transport.authority) {
		return BoundaryFactorTransportPlan{}, fmt.Errorf("%w: ordinary factor boundary transport is unowned", ErrInvalidLaneFactor)
	}
	projectRoots := make(BoundaryRoots, len(selection.roots))
	for index := range selection.roots {
		projectRoots[index] = BoundaryRoot{
			Slot: selection.roots[index].Slot, Path: selection.roots[index].Path,
			Value: product.Bottom(d.reg),
		}
	}
	artifact := BoundaryArtifact{reg: d.reg, keys: selection.keys, closure: selection.closure, roots: projectRoots}
	plan, err := transport.planFor(artifact)
	if err != nil {
		return BoundaryFactorTransportPlan{}, err
	}

	destinationCompanion, expectsCompanion := d.BoundaryClosureCompanion()
	if expectsCompanion != companion.present {
		return BoundaryFactorTransportPlan{}, fmt.Errorf("%w: boundary closure companion configuration mismatch", ErrInvalidLaneFactor)
	}

	closure := plan.baseClosure
	if companion.present {
		runtime, err := d.validateLane(destinationCompanion)
		if err != nil || runtime.lane.id != companion.companion || runtime.ops.boundaryClosureExtend == nil || companion.payload == nil {
			return BoundaryFactorTransportPlan{}, fmt.Errorf("%w: boundary closure companion role mismatch", ErrInvalidLaneFactor)
		}
		var extended bool
		closure, extended = runtime.ops.boundaryClosureExtend(transport.toKeys, companion.payload, plan.rootPaths, closure)
		if !extended {
			return BoundaryFactorTransportPlan{}, fmt.Errorf("state: boundary closure companion extension failed")
		}
	}
	projectCtx := boundaryProjectContext{reg: d.reg, keys: selection.keys, closure: selection.closure}
	rebaseCtx := boundaryRebaseContext{
		reg: d.reg, fromKeys: selection.keys, toKeys: transport.toKeys,
		roots: plan.effectiveRoots, slots: plan.rootSlots, allocations: transport.authority,
		quotient: plan.quotient, fromClosure: selection.closure, toClosure: closure,
	}
	targets := make([]BoundaryFactorTarget, plan.rootCount)
	targetSet := make([]bool, plan.rootCount)
	targetSource := make([]int, plan.rootCount)
	sourceTargets := make([][]int, len(selection.roots))
	for _, binding := range transport.roots {
		if targetSet[binding.ToRoot] {
			if targets[binding.ToRoot].Slot != binding.ToSlot || targets[binding.ToRoot].Path != binding.To {
				return BoundaryFactorTransportPlan{}, fmt.Errorf(
					"state: boundary factor root schema collision at target %d: source %d has slot=%d path=%#v; source %d has slot=%d path=%#v",
					binding.ToRoot, targetSource[binding.ToRoot], targets[binding.ToRoot].Slot, targets[binding.ToRoot].Path,
					binding.FromRoot, binding.ToSlot, binding.To,
				)
			}
		} else {
			targets[binding.ToRoot] = BoundaryFactorTarget{Slot: binding.ToSlot, Path: binding.To}
			targetSet[binding.ToRoot] = true
			targetSource[binding.ToRoot] = binding.FromRoot
		}
		targets[binding.ToRoot].Sources = append(targets[binding.ToRoot].Sources, binding.FromRoot)
		sourceTargets[binding.FromRoot] = append(sourceTargets[binding.FromRoot], binding.ToRoot)
	}
	for _, set := range targetSet {
		if !set {
			return BoundaryFactorTransportPlan{}, fmt.Errorf("state: boundary factor destination has no scalar owner")
		}
	}
	valueRelation, err := sealConcreteBoundaryValueSlotRelation(projectCtx, rebaseCtx)
	if err != nil {
		return BoundaryFactorTransportPlan{}, err
	}
	result := BoundaryFactorTransportPlan{
		seal:   new(boundaryFactorTransportSeal),
		domain: d, sourceDomain: companion.domain, keys: transport.toKeys, closure: closure, targets: targets, sourceTargets: sourceTargets,
		aliases: append([][2]keyspace.Key(nil), plan.aliases...), rootPaths: append(boundaryPathMap(nil), plan.rootPaths...), existentials: transport.existentials,
		projectCtx: projectCtx, rebaseCtx: rebaseCtx,
	}
	result.values, err = PrepareBoundaryValueFactorTransport(result, valueRelation)
	if err != nil {
		return BoundaryFactorTransportPlan{}, err
	}
	return result, nil
}

// PrepareLane is the scalar-free projection used by guarded factor
// transposition. Coordinate-factored lanes require the complete root tuple and
// therefore enter through PrepareFactor.
func (p BoundaryFactorTransportPlan) PrepareLane(factor LaneFactor, establishesReachability bool) (BoundaryFactorPatch, error) {
	runtime, err := p.sourceDomain.validateFactor(factor)
	if err != nil || runtime.lane.slotFactored || len(runtime.coordinates) != 0 {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: scalar-free boundary factor requires an atomic lane", ErrInvalidLaneFactor)
	}
	return p.PrepareFactor(factor, nil, establishesReachability)
}

// PrepareFactor is the complete ground-factor boundary transaction for an
// ordinary lane. Coordinate-factored lanes execute only through the canonical
// coordinate-family lift; accepting them here would recreate a parallel
// whole-lane interpretation of the same registered family laws.
func (p BoundaryFactorTransportPlan) PrepareFactor(factor LaneFactor, roots []product.Value, establishesReachability bool) (BoundaryFactorPatch, error) {
	sourceRuntime, err := p.sourceDomain.validateFactor(factor)
	if err != nil || sourceRuntime.lane.slotFactored || len(sourceRuntime.coordinates) != 0 {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: invalid source lane factor", ErrInvalidLaneFactor)
	}
	targetLane, present := p.domain.ProductLane(sourceRuntime.lane.id)
	if !present {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: destination omits factor lane %q", ErrInvalidLaneFactor, sourceRuntime.lane.id)
	}
	targetRuntime, err := p.domain.validateLane(targetLane)
	if err != nil || targetRuntime.lane.slotFactored {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: invalid destination lane factor", ErrInvalidLaneFactor)
	}
	// The selection was closed by the sealed reachability ProgramSet before the
	// structural plan was built. Apply never scans a factor to discover a wider
	// closure; doing so here would recreate per-leaf semantic work and make the
	// plan's quotient depend on lane execution order.
	projectCtx, rebaseCtx, closure := p.projectCtx, p.rebaseCtx, p.closure
	payload, ok := sourceRuntime.ops.boundaryProject(&projectCtx, factor.payload)
	if !ok {
		return BoundaryFactorPatch{}, fmt.Errorf("state: boundary factor projection failed in lane %q", sourceRuntime.lane.id)
	}
	payload, ok = sourceRuntime.ops.boundaryRebase(&rebaseCtx, payload)
	if !ok {
		return BoundaryFactorPatch{}, fmt.Errorf("state: boundary factor rebase failed in lane %q", sourceRuntime.lane.id)
	}
	payload, ok = sourceRuntime.ops.boundaryPostRebase(&rebaseCtx, p.aliases, payload)
	if !ok {
		return BoundaryFactorPatch{}, fmt.Errorf("state: boundary factor post-rebase failed in lane %q", sourceRuntime.lane.id)
	}
	lane := LaneFactor{lane: targetRuntime.lane, payload: payload}
	rootUse := targetRuntime.ops.boundaryRootUse
	if !rootUse.declared {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: factor %q omitted its boundary root-use law", ErrIncompleteLaneFactors, targetRuntime.lane.id)
	}
	if sourceRuntime.ops.boundaryRootUse != rootUse {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: factor %q root-use law differs across the boundary", ErrIncompleteLaneFactors, targetRuntime.lane.id)
	}
	if len(roots) != 0 && len(roots) != len(p.targets) {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: boundary root value width", ErrIncompleteLaneFactors)
	}
	if len(p.targets) != 0 && len(roots) == 0 && (rootUse.slotValues || rootUse.pathValues) {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: factor %q requires the complete boundary root tuple", ErrIncompleteLaneFactors, targetRuntime.lane.id)
	}
	rootPlan := boundaryRootPlan{establishesReachability: establishesReachability}
	for index, target := range p.targets {
		if len(roots) == 0 {
			break
		}
		root := BoundaryRoot{Slot: target.Slot, Path: target.Path, Value: roots[index]}
		if rootUse.slotValues && root.Slot != 0 {
			rootPlan.slots = append(rootPlan.slots, root)
		}
		if rootUse.pathValues && root.Path.Kind != keyspace.KindInvalid {
			rootPlan.paths = append(rootPlan.paths, root)
		}
	}
	if rootUse.reachability {
		rootPlan.establishesReachability = establishesReachability
	}
	return BoundaryFactorPatch{domain: p.domain, keys: p.keys, closure: closure, rootPlan: rootPlan, lane: &lane}, nil
}

// PrepareValues projects and rebases the Values factor independently.
func (p BoundaryFactorTransportPlan) PrepareValues(values ValueLaneFactor) (BoundaryFactorPatch, error) {
	lane, ok := p.domain.SlotFactoredCarrier()
	if !ok {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: product has no Values lane", ErrInvalidLaneFactor)
	}
	_, err := p.domain.validateLane(lane)
	if err != nil {
		return BoundaryFactorPatch{}, fmt.Errorf("%w: invalid Values lane descriptor", ErrInvalidLaneFactor)
	}
	value, err := p.values.Apply(values)
	if err != nil {
		return BoundaryFactorPatch{}, err
	}
	return BoundaryFactorPatch{domain: p.domain, keys: p.keys, closure: p.closure, values: &value}, nil
}

type BoundaryValueRoot struct {
	Slot  statekey.Value
	Value product.Value
}

// ValueRoot evaluates one already-rebased destination root through the
// registered Values root law. Distinct target ordinals remain distinct writes;
// callers apply them in ordinal order, preserving canonical last-write.
func (p BoundaryFactorTransportPlan) ValueRoot(target int, value product.Value) (BoundaryValueRoot, error) {
	if target < 0 || target >= len(p.targets) || p.targets[target].Slot == 0 || !product.BelongsToRegistry(p.domain.reg, value) {
		return BoundaryValueRoot{}, fmt.Errorf("%w: invalid Values boundary root", ErrInvalidLaneFactor)
	}
	lane, ok := p.domain.SlotFactoredCarrier()
	if !ok {
		return BoundaryValueRoot{}, fmt.Errorf("%w: product has no Values lane", ErrInvalidLaneFactor)
	}
	runtime, err := p.domain.validateLane(lane)
	if err != nil {
		return BoundaryValueRoot{}, fmt.Errorf("%w: invalid Values lane descriptor", ErrInvalidLaneFactor)
	}
	var payload laneFactorPayload = typedLaneFactorPayload[valueLane]{value: valueLane{}}
	plan := sealBoundaryRootPlan(p.domain.reg, BoundaryRoots{{Slot: p.targets[target].Slot, Value: value}})
	payload, ok = runtime.ops.boundaryRoots(&boundaryApplyContext{reg: p.domain.reg, keys: p.keys, closure: p.closure}, payload, plan)
	if !ok {
		return BoundaryValueRoot{}, fmt.Errorf("state: unary Values root application failed")
	}
	values := typedLaneFactorValue[valueLane](payload)
	return BoundaryValueRoot{Slot: p.targets[target].Slot, Value: values.read(p.domain.reg, p.targets[target].Slot)}, nil
}

func withBoundaryEffectFactorCompanions(toKeys *keyspace.KeySpace, source effectDeltaLane, explicit boundaryPathMap, base BoundaryClosure) BoundaryClosure {
	if source.top {
		return base
	}
	return withBoundaryEffectStructuralTargets(toKeys, explicit, base, func(visit func(keyspace.Key)) {
		for effectKey := range source.values {
			visit(effectKey.Target)
		}
	})
}

// ApplyLane applies one complete transported fragment to the destination
// factor through its registered factor law.
func (p BoundaryFactorPatch) ApplyLane(destination LaneFactor) (LaneFactor, error) {
	if !p.domain.Valid() {
		return LaneFactor{}, fmt.Errorf("%w: invalid boundary factor patch", ErrInvalidLaneFactor)
	}
	runtime, err := p.domain.validateFactor(destination)
	if err != nil || runtime.lane.slotFactored {
		return LaneFactor{}, fmt.Errorf("%w: invalid destination factor lane", ErrInvalidLaneFactor)
	}
	if p.lane == nil || p.lane.lane.ordinal != runtime.lane.ordinal {
		return LaneFactor{}, fmt.Errorf("%w: boundary factor patch omits lane %q", ErrInvalidLaneFactor, runtime.lane.id)
	}
	ctx := boundaryApplyContext{reg: p.domain.reg, keys: p.rootPlanKeySpace(), closure: p.closure}
	payload, ok := runtime.ops.boundaryApply(&ctx, destination.payload, p.lane.payload)
	if !ok {
		return LaneFactor{}, fmt.Errorf("state: boundary factor apply failed in lane %q", runtime.lane.id)
	}
	payload, ok = runtime.ops.boundaryRoots(&ctx, payload, p.rootPlan)
	if !ok {
		return LaneFactor{}, fmt.Errorf("state: boundary factor roots failed in lane %q", runtime.lane.id)
	}
	// Canonical operand reuse is part of the factor transaction: registered
	// laws may construct an equal empty/map spelling while applying an exact
	// no-op fragment. Preserve the destination representative so repeated Apply
	// cannot grow terminal hash-conses or make convergence spelling-sensitive.
	if runtime.ops.same(destination.payload, payload) {
		return destination, nil
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// ApplyValues applies closure replacement and the rebased root tuple.
func (p BoundaryFactorPatch) ApplyValues(destination ValueLaneFactor) (ValueLaneFactor, error) {
	lane, ok := p.domain.SlotFactoredCarrier()
	if !ok || p.values == nil {
		return ValueLaneFactor{}, fmt.Errorf("%w: product has no Values lane", ErrInvalidLaneFactor)
	}
	runtime, err := p.domain.validateLane(lane)
	if err != nil {
		return ValueLaneFactor{}, fmt.Errorf("%w: invalid Values lane descriptor", ErrInvalidLaneFactor)
	}
	ctx := boundaryApplyContext{reg: p.domain.reg, keys: p.rootPlanKeySpace(), closure: p.closure}
	destinationPayload := typedLaneFactorPayload[valueLane]{value: valueLaneFromFactor(p.domain.reg, destination)}
	fragmentPayload := typedLaneFactorPayload[valueLane]{value: valueLaneFromFactor(p.domain.reg, *p.values)}
	payload, applied := runtime.ops.boundaryApply(&ctx, destinationPayload, fragmentPayload)
	if !applied {
		return ValueLaneFactor{}, fmt.Errorf("state: Values factor apply failed")
	}
	return valueLaneToFactor(typedLaneFactorValue[valueLane](payload)), nil
}

func (p BoundaryFactorPatch) rootPlanKeySpace() *keyspace.KeySpace {
	// Every path in rootPlan/closure is already owned by the transport target;
	// keep the authority explicitly on the patch rather than reverse-engineering
	// it from a possibly empty root tuple.
	return p.keys
}
