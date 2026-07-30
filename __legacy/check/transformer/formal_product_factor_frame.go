package transformer

import (
	"errors"
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalProductFactorFrameBinding is the physical lowering of one sealed
// ProductFactorSelection. It retains the exact selected coordinate ordinals;
// every omitted tuple ordinal is structural carry owned by the closed lift.
type formalProductFactorFrameBinding struct {
	selection     state.ProductFactorSelection
	ordinary      []formalProductOrdinaryFrameBinding
	coordinates   []formalProductCoordinateFrameBinding
	values        []formalFiberGroupMember
	valuesTop     formalFiberGroupMember
	valueGroup    formalFiberGroupDescriptor
	ordinals      []formalFiberOrdinal
	writeOrdinals []formalFiberOrdinal
}

type formalProductOrdinaryFrameBinding struct {
	lane  state.ProductLane
	group formalFiberGroupDescriptor
}

type formalProductCoordinateFrameBinding struct {
	group        formalFiberGroupDescriptor
	family       formalCoordinateFamilyFiberGroup
	slots        []state.CoordinateSlot
	positions    []int
	skeletonOnly bool
	overlay      state.CoordinateSkeletonOverlayPlan
}

func sealFormalProductFactorFrameBinding(
	domain state.ProductDomain,
	span formalFiberDescriptorSpan,
	selection state.ProductFactorSelection,
	valueSlot func(statekey.Value) (FormalSlot, bool),
	allowImplicitCoordinates bool,
	publication bool,
) (formalProductFactorFrameBinding, error) {
	if !domain.Valid() || !domain.OwnsProductFactorSelection(selection) || selection.CoordinateFactors().KeySpace() != span.keys {
		return formalProductFactorFrameBinding{}, errFormalComponentForeignOwner
	}
	groups := make(map[state.LaneOrdinal]formalFiberGroupDescriptor)
	var valuesGroup formalFiberGroupDescriptor
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			valuesGroup = group
		} else {
			groups[group.lane.Ordinal()] = group
		}
	}
	out := formalProductFactorFrameBinding{selection: selection, valueGroup: valuesGroup}
	for _, lane := range selection.OrdinaryLanes() {
		group, present := groups[lane.Ordinal()]
		if !present || group.kind != formalFiberGroupOrdinaryLane || group.lane != lane {
			return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected ordinary lane %q is outside the frozen product", lane.ID())
		}
		out.ordinary = append(out.ordinary, formalProductOrdinaryFrameBinding{lane: lane, group: group})
		out.ordinals = append(out.ordinals, group.members...)
		out.writeOrdinals = append(out.writeOrdinals, group.members...)
	}
	skeletonOnly := make(map[state.CoordinateFamily]struct{})
	for _, family := range selection.CoordinateSkeletonFamilies() {
		skeletonOnly[family] = struct{}{}
	}
	coordinateInventory := selection.CoordinateFactors()
	consumed := 0
	for _, lane := range selection.CoordinateLanes() {
		group, present := groups[lane.Ordinal()]
		if !present || group.kind != formalFiberGroupCoordinateLane || group.lane != lane {
			return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected coordinate lane %q is outside the frozen product", lane.ID())
		}
		if publication {
			out.ordinals = append(out.ordinals, group.members...)
		}
		for _, family := range (formalCoordinateLaneFiberGroup{descriptor: group}).families() {
			slots, err := coordinateInventory.FamilySlots(family.family)
			if err != nil {
				return formalProductFactorFrameBinding{}, err
			}
			_, wholeSkeleton := skeletonOnly[family.family]
			if len(slots) == 0 && !wholeSkeleton {
				continue
			}
			binding := formalProductCoordinateFrameBinding{
				group: group, family: family, slots: slots, skeletonOnly: wholeSkeleton,
			}
			out.ordinals = append(out.ordinals, family.skeleton)
			out.writeOrdinals = append(out.writeOrdinals, family.skeleton)
			if wholeSkeleton {
				if publication {
					out.ordinals = append(out.ordinals, family.scalars...)
					out.writeOrdinals = append(out.writeOrdinals, family.scalars...)
				}
			} else {
				binding.positions = make([]int, len(slots))
				binding.overlay, err = domain.SealCoordinateSkeletonOverlayPlan(slots)
				if err != nil {
					return formalProductFactorFrameBinding{}, err
				}
				for index, slot := range slots {
					position, exact := formalCoordinatePosition(domain, span, family, slot)
					if !exact {
						if !allowImplicitCoordinates {
							return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected coordinate is outside the frozen family")
						}
						binding.positions[index] = -1
						continue
					}
					binding.positions[index] = position
					out.ordinals = append(out.ordinals, family.scalars[position])
					out.writeOrdinals = append(out.writeOrdinals, family.scalars[position])
				}
			}
			consumed += len(slots)
			out.coordinates = append(out.coordinates, binding)
		}
	}
	if consumed != coordinateInventory.Len() {
		return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected coordinate inventory is outside the frozen product")
	}
	valueFactors := selection.ValueFactors()
	if len(valueFactors) != 0 || selection.ValuesTop() {
		if !valuesGroup.valid() || valueSlot == nil {
			return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected Values have no frozen binding")
		}
		values := formalValuesFiberGroup{descriptor: valuesGroup}
		top, present := values.top()
		if !present {
			return formalProductFactorFrameBinding{}, errFormalComponentMalformed
		}
		out.valuesTop = top
		topOrdinal, _ := top.address(valuesGroup)
		out.ordinals = append(out.ordinals, topOrdinal)
		if selection.ValuesTop() {
			out.writeOrdinals = append(out.writeOrdinals, topOrdinal)
		}
		out.values = make([]formalFiberGroupMember, len(valueFactors))
		for index, slot := range valueFactors {
			formalSlot, exact := valueSlot(slot)
			if !exact {
				return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected Values slot %d has no formal identity", slot)
			}
			member, exact := values.slot(formalSlot)
			if !exact {
				return formalProductFactorFrameBinding{}, fmt.Errorf("transformer: selected Values slot %d is outside the frozen carrier", slot)
			}
			out.values[index] = member
			ordinal, _ := member.address(valuesGroup)
			out.ordinals = append(out.ordinals, ordinal)
			out.writeOrdinals = append(out.writeOrdinals, ordinal)
		}
	}
	canonicalize := func(ordinals []formalFiberOrdinal) []formalFiberOrdinal {
		sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
		write := 0
		for _, ordinal := range ordinals {
			if write == 0 || ordinals[write-1] != ordinal {
				ordinals[write], write = ordinal, write+1
			}
		}
		return ordinals[:write]
	}
	out.ordinals = canonicalize(out.ordinals)
	out.writeOrdinals = canonicalize(out.writeOrdinals)
	return out, nil
}

func formalCoordinateSkeletonAt(view formalSparseLeafView, family formalCoordinateFamilyFiberGroup) (state.CoordinateFamilySkeleton, error) {
	leaf, present := view.leaf(family.skeleton)
	if !present {
		return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
	}
	if leaf == 0 {
		return view.authority.product.CoordinateSkeletonBottom(family.family, view.span.keys)
	}
	terminal, err := view.authority.terminal(leaf)
	if err != nil || terminal.kind != formalComponentCoordinateSkeleton || !coordinateFamilySame(terminal.skeleton.Family(), family.family) {
		if err != nil {
			return state.CoordinateFamilySkeleton{}, err
		}
		return state.CoordinateFamilySkeleton{}, errFormalComponentMalformed
	}
	return terminal.skeleton, nil
}

func formalCoordinateScalarsAt(
	view formalSparseLeafView,
	binding formalProductCoordinateFrameBinding,
	skeleton state.CoordinateFamilySkeleton,
	all bool,
) ([]state.CoordinateScalarFactor, error) {
	positions := binding.positions
	slots := binding.slots
	if all {
		positions = make([]int, len(binding.family.scalars))
		slots = make([]state.CoordinateSlot, len(binding.family.scalars))
		for index, ordinal := range binding.family.scalars {
			positions[index] = index
			slots[index] = view.span.forest.descriptors[view.span.first+int(ordinal)].coordinate
		}
	}
	out := make([]state.CoordinateScalarFactor, 0, len(positions))
	for index, position := range positions {
		if position < 0 {
			continue
		}
		if position < 0 || position >= len(binding.family.scalars) || index >= len(slots) {
			return nil, errFormalComponentMalformed
		}
		leaf, present := view.leaf(binding.family.scalars[position])
		if !present {
			return nil, errFormalComponentMalformed
		}
		if leaf == 0 {
			continue
		}
		terminal, err := view.authority.terminal(leaf)
		if err != nil || terminal.kind != formalComponentCoordinateScalar {
			if err != nil {
				return nil, err
			}
			return nil, errFormalComponentMalformed
		}
		equal, err := view.authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), slots[index])
		if err != nil || !equal {
			if err != nil {
				return nil, err
			}
			return nil, errFormalComponentMalformed
		}
		omitted, err := view.authority.product.CoordinateScalarIsOmitted(skeleton, terminal.scalar)
		if err != nil {
			return nil, err
		}
		if !omitted {
			out = append(out, terminal.scalar)
		}
	}
	return out, nil
}

func (a *formalTupleAlgebra) materializeFormalProductFactorFrame(
	view formalSparseLeafView,
	binding formalProductFactorFrameBinding,
) (state.ProductFactorFrame, error) {
	if a == nil || view.algebra != a || view.authority == nil || !view.authority.product.OwnsProductFactorSelection(binding.selection) {
		return state.ProductFactorFrame{}, errFormalComponentForeignOwner
	}
	ordinary := make([]state.LaneFactor, len(binding.ordinary))
	for index, selected := range binding.ordinary {
		factor, err := view.laneFactor(selected.group)
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
		ordinary[index] = factor
	}
	coordinates := make([]state.CoordinateFamilyFactor, len(binding.coordinates))
	for index, selected := range binding.coordinates {
		skeleton, err := formalCoordinateSkeletonAt(view, selected.family)
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
		if selected.skeletonOnly {
			coordinates[index], err = view.authority.product.SealCoordinateFamilyFactor(skeleton, nil)
		} else {
			shape, shapeErr := view.authority.product.SealCoordinateFamilyShape(skeleton, selected.slots)
			if errors.Is(shapeErr, state.ErrIncompleteLaneFactors) {
				bottom, projectErr := view.authority.product.CoordinateSkeletonBottom(selected.family.family, view.span.keys)
				if projectErr != nil {
					return state.ProductFactorFrame{}, projectErr
				}
				selectedSkeleton, projectErr := view.authority.product.OverlaySelectedCoordinateSkeleton(selected.overlay, bottom, skeleton, nil)
				if projectErr != nil {
					return state.ProductFactorFrame{}, projectErr
				}
				shape, shapeErr = view.authority.product.SealCoordinateFamilyShape(selectedSkeleton, selected.slots)
			}
			if shapeErr != nil {
				return state.ProductFactorFrame{}, shapeErr
			}
			scalars, scalarErr := formalCoordinateScalarsAt(view, selected, shape.Skeleton(), false)
			if scalarErr != nil {
				return state.ProductFactorFrame{}, scalarErr
			}
			coordinates[index], err = view.authority.product.SealCoordinateFamilyFactor(shape.Skeleton(), scalars)
		}
		if err != nil {
			return state.ProductFactorFrame{}, err
		}
	}
	values := make([]product.Value, len(binding.values))
	valuesTop := false
	if len(binding.values) != 0 || binding.selection.ValuesTop() {
		topOrdinal, exact := binding.valuesTop.address(binding.valueGroup)
		topLeaf, present := view.leaf(topOrdinal)
		if !exact || !present || topLeaf > 1 {
			return state.ProductFactorFrame{}, errFormalComponentMalformed
		}
		valuesTop = binding.selection.ValuesTop() && topLeaf == 1
		for index, member := range binding.values {
			value, valueExact := view.value(member, binding.valuesTop)
			if !valueExact {
				return state.ProductFactorFrame{}, errFormalComponentMalformed
			}
			values[index] = value
		}
	}
	return view.authority.product.BindProductFactorFrame(binding.selection, ordinary, coordinates, values, valuesTop)
}

func formalCoordinateScalarFromImage(domain state.ProductDomain, image state.CoordinateFamilyFactor, slot state.CoordinateSlot) (state.CoordinateScalarFactor, bool, error) {
	for _, scalar := range image.Scalars() {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), slot)
		if err != nil {
			return state.CoordinateScalarFactor{}, false, err
		}
		if equal {
			return scalar, true, nil
		}
	}
	return state.CoordinateScalarFactor{}, false, nil
}

func (a *formalTupleAlgebra) internFormalCoordinateSkeleton(
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	family state.CoordinateFamily,
	skeleton state.CoordinateFamilySkeleton,
) (decisionLeaf, error) {
	bottom, err := authority.product.CoordinateSkeletonBottom(family, span.keys)
	if err != nil {
		return 0, err
	}
	same, err := authority.product.CoordinateSkeletonRepresentationEqual(skeleton, bottom)
	if err != nil || same {
		return 0, err
	}
	return authority.internCoordinateSkeleton(skeleton)
}

func (a *formalTupleAlgebra) factorFormalProductFactorFrame(
	view formalSparseLeafView,
	binding formalProductFactorFrameBinding,
	frame state.ProductFactorFrame,
) ([]formalClosedFactorLeafWrite, error) {
	if a == nil || view.algebra != a || view.authority == nil || !view.authority.product.OwnsProductFactorFrame(binding.selection, frame) {
		return nil, errFormalComponentForeignOwner
	}
	var out []formalClosedFactorLeafWrite
	ordinary := frame.OrdinaryFactors()
	if len(ordinary) != len(binding.ordinary) {
		return nil, errFormalComponentMalformed
	}
	for index, selected := range binding.ordinary {
		leaves, err := a.factorFormalSparseLane(view.authority, view.span, selected.group, ordinary[index])
		if err != nil || len(leaves) != len(selected.group.members) {
			if err == nil {
				err = errFormalComponentMalformed
			}
			return nil, err
		}
		for memberIndex, ordinal := range selected.group.members {
			out = append(out, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaves[memberIndex]})
		}
	}
	coordinates := frame.CoordinateFactors()
	if len(coordinates) != len(binding.coordinates) {
		return nil, errFormalComponentMalformed
	}
	coordinateWriteStart := len(out)
	for index, selected := range binding.coordinates {
		image := coordinates[index]
		currentSkeleton, err := formalCoordinateSkeletonAt(view, selected.family)
		if err != nil {
			return nil, err
		}
		if selected.skeletonOnly {
			currentScalars, scalarErr := formalCoordinateScalarsAt(view, selected, currentSkeleton, true)
			if scalarErr != nil {
				return nil, scalarErr
			}
			current, sealErr := view.authority.product.SealCoordinateFamilyFactor(currentSkeleton, currentScalars)
			if sealErr != nil {
				return nil, sealErr
			}
			reconciled, reconcileErr := view.authority.product.ReconcileCoordinateFamilyFactor(current, image.Skeleton(), nil)
			if reconcileErr != nil {
				return nil, reconcileErr
			}
			skeletonLeaf, internErr := a.internFormalCoordinateSkeleton(view.authority, view.span, selected.family.family, reconciled.Skeleton())
			if internErr != nil {
				return nil, internErr
			}
			out = append(out, formalClosedFactorLeafWrite{ordinal: selected.family.skeleton, leaf: skeletonLeaf})
			scalarIndex := 0
			reconciledScalars := reconciled.Scalars()
			for _, ordinal := range selected.family.scalars {
				leaf := decisionLeaf(0)
				if scalarIndex < len(reconciledScalars) {
					descriptor := view.span.forest.descriptors[view.span.first+int(ordinal)]
					equal, equalErr := view.authority.product.CoordinateSlotEqual(descriptor.coordinate, reconciledScalars[scalarIndex].Slot())
					if equalErr != nil {
						return nil, equalErr
					}
					if equal {
						leaf, equalErr = view.authority.internCoordinateScalar(reconciledScalars[scalarIndex])
						if equalErr != nil {
							return nil, equalErr
						}
						scalarIndex++
					}
				}
				out = append(out, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaf})
			}
			if scalarIndex != len(reconciledScalars) {
				return nil, errFormalComponentMalformed
			}
			continue
		}
		currentScalars, scalarErr := formalCoordinateScalarsAt(view, selected, currentSkeleton, false)
		if scalarErr != nil {
			return nil, scalarErr
		}
		overlaid, overlayErr := view.authority.product.OverlaySelectedCoordinateSkeleton(
			selected.overlay, currentSkeleton, image.Skeleton(), currentScalars,
		)
		if overlayErr != nil {
			return nil, overlayErr
		}
		skeletonLeaf, internErr := a.internFormalCoordinateSkeleton(view.authority, view.span, selected.family.family, overlaid)
		if internErr != nil {
			return nil, internErr
		}
		out = append(out, formalClosedFactorLeafWrite{ordinal: selected.family.skeleton, leaf: skeletonLeaf})
		for slotIndex, slot := range selected.slots {
			leaf := decisionLeaf(0)
			support, supportErr := view.authority.product.CoordinateScalarSupport(overlaid, slot)
			if supportErr != nil {
				return nil, supportErr
			}
			if support != state.CoordinateScalarForbidden {
				scalar, explicit, scalarErr := formalCoordinateScalarFromImage(view.authority.product, image, slot)
				if scalarErr != nil {
					return nil, scalarErr
				}
				if !explicit {
					scalar, scalarErr = view.authority.product.CoordinateDefault(image.Skeleton(), slot)
					if scalarErr != nil {
						return nil, scalarErr
					}
				}
				omitted, omitErr := view.authority.product.CoordinateScalarIsOmitted(overlaid, scalar)
				if omitErr != nil {
					return nil, omitErr
				}
				if !omitted {
					leaf, omitErr = view.authority.internCoordinateScalar(scalar)
					if omitErr != nil {
						return nil, omitErr
					}
				}
			}
			position := selected.positions[slotIndex]
			if position < 0 {
				if leaf != 0 {
					return nil, fmt.Errorf("transformer: RootAssignment produced an explicit coordinate outside the frozen carrier")
				}
				continue
			}
			out = append(out, formalClosedFactorLeafWrite{ordinal: selected.family.scalars[position], leaf: leaf})
		}
	}
	coordinateWrites := make(map[formalFiberOrdinal]int, len(out)-coordinateWriteStart)
	for index := coordinateWriteStart; index < len(out); index++ {
		coordinateWrites[out[index].ordinal] = index
	}
	seenCoordinateLanes := make(map[state.LaneOrdinal]struct{})
	for _, selected := range binding.coordinates {
		group := selected.group
		if _, seen := seenCoordinateLanes[group.lane.Ordinal()]; seen {
			continue
		}
		seenCoordinateLanes[group.lane.Ordinal()] = struct{}{}
		factor, factorErr := view.laneFactor(group)
		if factorErr != nil {
			return nil, factorErr
		}
		for imageIndex, candidate := range binding.coordinates {
			if candidate.group.lane != group.lane {
				continue
			}
			image := coordinates[imageIndex]
			if candidate.skeletonOnly {
				factor, factorErr = view.authority.product.ReconcileCoordinateFamily(factor, image.Skeleton(), nil)
			} else {
				factor, factorErr = view.authority.product.PatchSelectedCoordinateFamilyLaneFactor(factor, candidate.slots, image)
			}
			if factorErr != nil {
				return nil, factorErr
			}
		}
		fullLeaves, factorErr := a.factorFormalSparseLane(view.authority, view.span, group, factor)
		if factorErr != nil || len(fullLeaves) != len(group.members) {
			if factorErr == nil {
				factorErr = errFormalComponentMalformed
			}
			return nil, factorErr
		}
		for memberIndex, ordinal := range group.members {
			if writeIndex, selectedWrite := coordinateWrites[ordinal]; selectedWrite {
				// Coordinate factors can be rebuilt through a selected-family
				// overlay, but that reconstruction may choose an equivalent sparse
				// leaf spelling. The lane registry owns the one canonical spelling
				// for every coordinate, so publish its leaf rather than the
				// provisional selected-family leaf.
				out[writeIndex].leaf = fullLeaves[memberIndex]
				continue
			}
			prior, present := view.leaf(ordinal)
			if !present || prior != fullLeaves[memberIndex] {
				return nil, fmt.Errorf("transformer: coordinate publication would rewrite an unselected root")
			}
		}
	}
	values := frame.Values()
	if len(values) != len(binding.values) {
		return nil, errFormalComponentMalformed
	}
	if binding.selection.ValuesTop() {
		ordinal, exact := binding.valuesTop.address(binding.valueGroup)
		if !exact {
			return nil, errFormalComponentMalformed
		}
		leaf := decisionLeaf(0)
		if frame.ValuesTop() {
			leaf = 1
		}
		out = append(out, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaf})
	}
	bottom := product.Bottom(view.authority.product.Registry())
	for index, value := range values {
		ordinal, exact := binding.values[index].address(binding.valueGroup)
		if !exact {
			return nil, errFormalComponentMalformed
		}
		leaf := decisionLeaf(0)
		if !frame.ValuesTop() && !product.Equal(view.authority.product.Registry(), value, bottom) {
			var err error
			leaf, err = view.authority.internGroundValue(value)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaf})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ordinal < out[j].ordinal })
	for index := 1; index < len(out); index++ {
		if out[index-1].ordinal == out[index].ordinal {
			return nil, errFormalComponentMalformed
		}
	}
	return out, nil
}
