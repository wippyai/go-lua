package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalClosedSelectedFactor is the exact non-Values carrier slice owned by
// one footprint-certified operator. Ordinary lanes have one indivisible member.
// Coordinate lanes retain only family skeletons and scalar coordinates named
// by the operator footprint; every other tuple member remains structural carry.
type formalClosedSelectedFactor struct {
	group    formalFiberGroupDescriptor
	ordinary bool
	families []formalClosedSelectedCoordinateFamily
	ordinals []formalFiberOrdinal
}

type formalClosedSelectedCoordinateFamily struct {
	family    formalCoordinateFamilyFiberGroup
	selected  bool
	slots     []state.CoordinateSlot
	positions []int
}

type formalClosedSelectedFactorOutput struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

func formalCoordinatePosition(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
	family formalCoordinateFamilyFiberGroup,
	slot state.CoordinateSlot,
) (int, bool) {
	for index, ordinal := range family.scalars {
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		equal, err := domain.CoordinateSlotEqual(descriptor.coordinate, slot)
		if err != nil {
			return 0, false
		}
		if equal {
			return index, true
		}
	}
	return 0, false
}

func sealFormalClosedSelectedFactor(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
	group formalFiberGroupDescriptor,
	footprint state.CoordinateFactorInventory,
) (formalClosedSelectedFactor, error) {
	if !domain.Valid() || !group.valid() || group.variable != span.variable ||
		!footprint.ValidFor(domain, span.keys) || group.kind == formalFiberGroupValues {
		return formalClosedSelectedFactor{}, errFormalComponentForeignOwner
	}
	plan := formalClosedSelectedFactor{group: group}
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		plan.ordinary = true
		plan.ordinals = append(plan.ordinals, group.members...)
	case formalFiberGroupCoordinateLane:
		consumed := 0
		for _, family := range (formalCoordinateLaneFiberGroup{descriptor: group}).families() {
			slots, err := footprint.FamilySlots(family.family)
			if err != nil {
				return formalClosedSelectedFactor{}, err
			}
			entry := formalClosedSelectedCoordinateFamily{family: family, selected: len(slots) != 0, slots: slots}
			if entry.selected {
				entry.positions = make([]int, len(slots))
				plan.ordinals = append(plan.ordinals, family.skeleton)
				for index, slot := range slots {
					position, exact := formalCoordinatePosition(domain, span, family, slot)
					if !exact || position < 0 || position >= len(family.scalars) {
						return formalClosedSelectedFactor{}, fmt.Errorf("coordinate footprint slot is outside the frozen family")
					}
					entry.positions[index] = position
					plan.ordinals = append(plan.ordinals, family.scalars[position])
				}
				consumed += len(slots)
			}
			plan.families = append(plan.families, entry)
		}
		laneSlots := 0
		for _, slot := range footprint.Slots() {
			if slot.Family().Lane() == group.lane {
				laneSlots++
			}
		}
		if consumed != laneSlots {
			return formalClosedSelectedFactor{}, fmt.Errorf("coordinate footprint is outside the lane family inventory")
		}
	default:
		return formalClosedSelectedFactor{}, errFormalComponentMalformed
	}
	sort.Slice(plan.ordinals, func(i, j int) bool { return plan.ordinals[i] < plan.ordinals[j] })
	for index := 1; index < len(plan.ordinals); index++ {
		if plan.ordinals[index-1] == plan.ordinals[index] {
			return formalClosedSelectedFactor{}, fmt.Errorf("coordinate footprint repeats a physical fiber")
		}
	}
	return plan, nil
}

func (a *formalTupleAlgebra) materializeFormalClosedSelectedFactor(
	view formalSparseLeafView,
	plan formalClosedSelectedFactor,
) (state.LaneFactor, error) {
	if a == nil || view.algebra != a || view.authority == nil || !plan.group.valid() ||
		plan.group.variable != view.variable {
		return state.LaneFactor{}, errFormalComponentForeignOwner
	}
	if plan.ordinary {
		return view.laneFactor(plan.group)
	}
	if plan.group.kind != formalFiberGroupCoordinateLane || len(plan.families) != len(plan.group.coordinateFamilies) {
		return state.LaneFactor{}, errFormalComponentMalformed
	}
	skeletons := make([]state.CoordinateFamilySkeleton, len(plan.families))
	scalars := make([][]state.CoordinateScalarFactor, len(plan.families))
	for familyIndex, family := range plan.families {
		var err error
		if !family.selected {
			skeletons[familyIndex], err = view.authority.product.CoordinateSkeletonBottom(family.family.family, view.span.keys)
			if err != nil {
				return state.LaneFactor{}, err
			}
			continue
		}
		leaf, present := view.leaf(family.family.skeleton)
		if !present {
			return state.LaneFactor{}, errFormalComponentMalformed
		}
		if leaf == 0 {
			skeletons[familyIndex], err = view.authority.product.CoordinateSkeletonBottom(family.family.family, view.span.keys)
		} else {
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton ||
				!coordinateFamilySame(terminal.skeleton.Family(), family.family.family) {
				if terminalErr != nil {
					return state.LaneFactor{}, terminalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			skeletons[familyIndex] = terminal.skeleton
		}
		if err != nil {
			return state.LaneFactor{}, err
		}
		for slotIndex, position := range family.positions {
			if position < 0 || position >= len(family.family.scalars) || slotIndex >= len(family.slots) {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			leaf, present := view.leaf(family.family.scalars[position])
			if !present {
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			if leaf == 0 {
				continue
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
				if terminalErr != nil {
					return state.LaneFactor{}, terminalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			equal, equalErr := view.authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), family.slots[slotIndex])
			if equalErr != nil || !equal {
				if equalErr != nil {
					return state.LaneFactor{}, equalErr
				}
				return state.LaneFactor{}, errFormalComponentMalformed
			}
			omitted, omitErr := view.authority.product.CoordinateScalarIsOmitted(skeletons[familyIndex], terminal.scalar)
			if omitErr != nil {
				return state.LaneFactor{}, omitErr
			}
			if !omitted {
				scalars[familyIndex] = append(scalars[familyIndex], terminal.scalar)
			}
		}
	}
	return view.authority.product.ComposeCoordinateFamilies(plan.group.lane, view.span.keys, skeletons, scalars)
}

func (a *formalTupleAlgebra) factorFormalClosedSelectedFactor(
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	plan formalClosedSelectedFactor,
	factor state.LaneFactor,
) ([]formalClosedSelectedFactorOutput, error) {
	if a == nil || authority == nil || !plan.group.valid() || plan.group.variable != span.variable ||
		factor.Lane() != plan.group.lane {
		return nil, errFormalComponentForeignOwner
	}
	if plan.ordinary {
		leaves, err := a.factorFormalSparseLane(authority, span, plan.group, factor)
		if err != nil || len(leaves) != len(plan.ordinals) {
			if err != nil {
				return nil, err
			}
			return nil, errFormalComponentMalformed
		}
		out := make([]formalClosedSelectedFactorOutput, len(leaves))
		for index := range leaves {
			out[index] = formalClosedSelectedFactorOutput{ordinal: plan.ordinals[index], leaf: leaves[index]}
		}
		return out, nil
	}
	byOrdinal := make(map[formalFiberOrdinal]decisionLeaf, len(plan.ordinals))
	for _, ordinal := range plan.ordinals {
		byOrdinal[ordinal] = 0
	}
	for _, family := range plan.families {
		skeleton, scalars, err := authority.product.DecomposeCoordinateFamily(factor, family.family.family, span.keys)
		if err != nil {
			return nil, err
		}
		bottom, err := authority.product.CoordinateSkeletonBottom(family.family.family, span.keys)
		if err != nil {
			return nil, err
		}
		sameBottom, err := authority.product.CoordinateSkeletonRepresentationEqual(skeleton, bottom)
		if err != nil {
			return nil, err
		}
		if !family.selected {
			if !sameBottom || len(scalars) != 0 {
				return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
			}
			continue
		}
		if !sameBottom {
			leaf, internErr := authority.internCoordinateSkeleton(skeleton)
			if internErr != nil {
				return nil, internErr
			}
			byOrdinal[family.family.skeleton] = leaf
		}
		scalarIndex := 0
		for slotIndex, slot := range family.slots {
			if scalarIndex >= len(scalars) {
				break
			}
			scalar := scalars[scalarIndex]
			equal, equalErr := authority.product.CoordinateSlotEqual(slot, scalar.Slot())
			if equalErr != nil {
				return nil, equalErr
			}
			if !equal {
				less, lessErr := authority.product.CoordinateSlotLess(scalar.Slot(), slot)
				if lessErr != nil {
					return nil, lessErr
				}
				if less {
					return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
				}
				continue
			}
			if slotIndex >= len(family.positions) || family.positions[slotIndex] < 0 || family.positions[slotIndex] >= len(family.family.scalars) {
				return nil, errFormalComponentMalformed
			}
			leaf, internErr := authority.internCoordinateScalar(scalar)
			if internErr != nil {
				return nil, internErr
			}
			byOrdinal[family.family.scalars[family.positions[slotIndex]]] = leaf
			scalarIndex++
		}
		if scalarIndex != len(scalars) {
			return nil, fmt.Errorf("transformer: formal selected factor escaped its coordinate footprint")
		}
	}
	out := make([]formalClosedSelectedFactorOutput, len(plan.ordinals))
	for index, ordinal := range plan.ordinals {
		out[index] = formalClosedSelectedFactorOutput{ordinal: ordinal, leaf: byOrdinal[ordinal]}
	}
	return out, nil
}
