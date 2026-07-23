package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// PlacementBoundaryPatch is the independently factored Placement fragment of
// one already projected and rebased BoundaryPatch.
type PlacementBoundaryPatch struct {
	domain   ProductDomain
	closure  BoundaryClosure
	skeleton PlacementSkeletonFactor
	values   map[identity.Term]PlacementFactor
	sources  map[identity.Term][]PlacementSlot
}

// PlacementBoundaryPlan owns closure replacement of the value-less skeleton
// and routes each output identity coordinate from exactly one operand.
type PlacementBoundaryPlan struct {
	patch      *PlacementBoundaryPatch
	skeleton   PlacementSkeletonFactor
	selections []PlacementBoundarySelection
}

// PlacementBoundarySelection routes one output identity coordinate.
type PlacementBoundarySelection struct {
	slot            PlacementSlot
	source          BoundaryFactorSource
	fragment        PlacementFactor
	fragmentSources []PlacementSlot
}

// PlacementFactors seals the Placement coordinate fragment carried by this
// boundary patch.
func (p BoundaryPatch) PlacementFactors() (PlacementBoundaryPatch, error) {
	if !p.valid() {
		return PlacementBoundaryPatch{}, fmt.Errorf("%w: invalid boundary patch", ErrInvalidLaneFactor)
	}
	runtime, ok := p.domain.runtimeForLaneID(LanePlacement)
	if !ok {
		return PlacementBoundaryPatch{}, fmt.Errorf("%w: product has no placement lane", ErrInvalidLaneFactor)
	}
	index := int(runtime.lane.ordinal)
	if index < 0 || index >= len(p.lanes) || p.lanes[index].lane != runtime.lane {
		return PlacementBoundaryPatch{}, fmt.Errorf("%w: boundary placement inventory drift", ErrInvalidLaneFactor)
	}
	skeleton, values, err := p.domain.DecomposePlacement(p.lanes[index].fragment)
	if err != nil {
		return PlacementBoundaryPatch{}, err
	}
	out := PlacementBoundaryPatch{
		domain: p.domain, closure: p.closure, skeleton: skeleton,
		values: make(map[identity.Term]PlacementFactor, len(values)),
	}
	for _, value := range values {
		out.values[value.id] = value
	}
	return out, nil
}

// Plan applies boundary replacement to structure only. destination names the
// finite destination inventory as slots; their scalar values remain separate.
func (p PlacementBoundaryPatch) Plan(destination PlacementSkeletonFactor, slots []PlacementSlot) (PlacementBoundaryPlan, error) {
	if !p.domain.Valid() || p.skeleton.seal == nil {
		return PlacementBoundaryPlan{}, fmt.Errorf("%w: invalid placement boundary patch", ErrInvalidLaneFactor)
	}
	if _, err := p.domain.validatePlacementSkeleton(destination); err != nil {
		return PlacementBoundaryPlan{}, err
	}
	if _, err := p.domain.validatePlacementSkeleton(p.skeleton); err != nil {
		return PlacementBoundaryPlan{}, err
	}
	if destination.top || p.skeleton.top {
		top := p.skeleton
		top.top = true
		return PlacementBoundaryPlan{patch: &p, skeleton: top}, nil
	}
	destinationIDs := make(map[identity.Term]struct{}, len(slots))
	for index, slot := range slots {
		if err := p.domain.validatePlacementSlot(slot); err != nil {
			return PlacementBoundaryPlan{}, fmt.Errorf("%w: destination placement slot %d: %v", ErrInvalidLaneFactor, index, err)
		}
		if _, duplicate := destinationIDs[slot.id]; duplicate {
			return PlacementBoundaryPlan{}, fmt.Errorf("%w: duplicate destination placement slot", ErrInvalidLaneFactor)
		}
		destinationIDs[slot.id] = struct{}{}
	}
	outputIDs := make(map[identity.Term]struct{}, len(destinationIDs)+len(p.values)+len(p.sources))
	for id := range destinationIDs {
		if !p.closure.ContainsIdentityTerm(id) {
			outputIDs[id] = struct{}{}
		}
	}
	for id := range p.values {
		outputIDs[id] = struct{}{}
	}
	for id := range p.sources {
		outputIDs[id] = struct{}{}
	}
	ids := make([]identity.Term, 0, len(outputIDs))
	for id := range outputIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
	plan := PlacementBoundaryPlan{patch: &p, skeleton: destination, selections: make([]PlacementBoundarySelection, 0, len(ids))}
	for _, id := range ids {
		selection := PlacementBoundarySelection{
			slot:   PlacementSlot{seal: p.domain.seal, lane: p.skeleton.lane, id: id},
			source: BoundaryFactorSourceDestination,
		}
		if p.closure.ContainsIdentityTerm(id) {
			fragment, hasFactor := p.values[id]
			sources := p.sources[id]
			if !hasFactor && len(sources) == 0 {
				return PlacementBoundaryPlan{}, fmt.Errorf("%w: transported placement %v is absent", ErrInvalidLaneFactor, id)
			}
			selection.source = BoundaryFactorSourceFragment
			selection.fragment, selection.fragmentSources = fragment, append([]PlacementSlot(nil), sources...)
		}
		plan.selections = append(plan.selections, selection)
	}
	return plan, nil
}

// Skeleton returns the exact output map-Top coordinate.
func (p PlacementBoundaryPlan) Skeleton() PlacementSkeletonFactor { return p.skeleton }

// Selections returns the identity-sorted scalar routing inventory.
func (p PlacementBoundaryPlan) Selections() []PlacementBoundarySelection {
	return append([]PlacementBoundarySelection(nil), p.selections...)
}

// Slot returns the output identity coordinate.
func (s PlacementBoundarySelection) Slot() PlacementSlot { return s.slot }

// Source reports which operand owns the output scalar.
func (s PlacementBoundarySelection) Source() BoundaryFactorSource { return s.source }

// Fragment returns the transported scalar when Source is Fragment.
func (s PlacementBoundarySelection) Fragment() (PlacementFactor, bool) {
	return s.fragment, s.source == BoundaryFactorSourceFragment && s.fragment.seal != nil
}

// FragmentSources returns the sorted original placement coordinates whose
// scalar values must be joined with placement.Join for this output identity.
func (s PlacementBoundarySelection) FragmentSources() []PlacementSlot {
	return append([]PlacementSlot(nil), s.fragmentSources...)
}

func (p PlacementBoundaryPatch) applyFactor(destination LaneFactor) (LaneFactor, error) {
	skeleton, values, err := p.domain.DecomposePlacement(destination)
	if err != nil {
		return LaneFactor{}, err
	}
	slots := make([]PlacementSlot, len(values))
	destinationValues := make(map[identity.Term]PlacementFactor, len(values))
	for index, value := range values {
		slots[index] = value.Slot()
		destinationValues[value.id] = value
	}
	plan, err := p.Plan(skeleton, slots)
	if err != nil {
		return LaneFactor{}, err
	}
	output := make([]PlacementFactor, 0, len(plan.selections))
	for _, selection := range plan.selections {
		switch selection.source {
		case BoundaryFactorSourceDestination:
			value, present := destinationValues[selection.slot.id]
			if !present {
				return LaneFactor{}, fmt.Errorf("%w: destination placement %v is absent", ErrInvalidLaneFactor, selection.slot.id)
			}
			output = append(output, value)
		case BoundaryFactorSourceFragment:
			output = append(output, selection.fragment)
		default:
			return LaneFactor{}, fmt.Errorf("%w: invalid placement source", ErrInvalidLaneFactor)
		}
	}
	return p.domain.ComposePlacement(plan.skeleton, output)
}
