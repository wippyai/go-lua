package state

import "fmt"

// SelectCoordinateLaneFactor restricts one dependent coordinate lane to an
// already-sealed operator inventory. The family skeleton and scalar payloads
// are reduced together through the registered family law; an omitted scalar
// therefore cannot leave structural reachability behind. Ordinary lanes are
// deliberately outside this operation.
func (d ProductDomain) SelectCoordinateLaneFactor(
	factor LaneFactor,
	selector CoordinateFactorInventory,
) (LaneFactor, error) {
	if !d.Valid() || !selector.ValidFor(d, selector.KeySpace()) || factor.lane.seal != d.seal {
		return LaneFactor{}, fmt.Errorf("%w: coordinate lane selector is unowned", ErrInvalidLaneFactor)
	}
	families, err := d.CoordinateFamilies(factor.lane)
	if err != nil || len(families) == 0 {
		return LaneFactor{}, fmt.Errorf("%w: coordinate lane selector requires a dependent lane", ErrInvalidLaneFactor)
	}
	skeletons := make([]CoordinateFamilySkeleton, len(families))
	scalars := make([][]CoordinateScalarFactor, len(families))
	for familyIndex, family := range families {
		skeleton, current, decomposeErr := d.DecomposeCoordinateFamily(factor, family, selector.KeySpace())
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		// The selector is immutable after sealing, so internal operators can
		// consume its canonical bucket without allocating a detached public view.
		selectedSlots, slotsErr := selector.familySlots(family)
		if slotsErr != nil {
			return LaneFactor{}, slotsErr
		}
		coordinate, coordinateErr := d.validateCoordinateFamily(family)
		if coordinateErr != nil {
			return LaneFactor{}, coordinateErr
		}
		if err := d.requireCoordinateSelectionSupport(coordinate, skeleton, selectedSlots); err != nil {
			return LaneFactor{}, err
		}
		shape, shapeErr := d.SealCoordinateFamilyShape(skeleton, selectedSlots)
		if shapeErr != nil {
			return LaneFactor{}, shapeErr
		}
		skeletons[familyIndex] = shape.Skeleton()
		// Both vectors are already sealed in this family's canonical key order.
		// Intersect them once; per-scalar Inventory.Contains revalidated and
		// rescanned the whole selector, turning wide identity families into O(n²).
		for currentIndex, selectedIndex := 0, 0; currentIndex < len(current) && selectedIndex < len(selectedSlots); {
			left, right := current[currentIndex], selectedSlots[selectedIndex]
			switch {
			case coordinate.ops.keyEqual(left.slot.key, right.key):
				scalars[familyIndex] = append(scalars[familyIndex], left)
				currentIndex++
				selectedIndex++
			case coordinate.ops.keyLess(left.slot.key, right.key, selector.keys):
				currentIndex++
			default:
				selectedIndex++
			}
		}
	}
	return d.ComposeCoordinateFamilies(factor.lane, selector.KeySpace(), skeletons, scalars)
}

// requireCoordinateSelectionSupport prevents an exact coordinate selector
// from passing an incomplete structural component to a family's narrowing
// law. Families that carry dependent structure register their support law;
// the generic selector owns validation and never interprets family payloads.
func (d ProductDomain) requireCoordinateSelectionSupport(
	coordinate *coordinateFamilyRuntime,
	skeleton CoordinateFamilySkeleton,
	selected []CoordinateSlot,
) error {
	if coordinate == nil || coordinate.ops.selectionSupport == nil {
		return nil
	}
	keys := make([]coordinateKeyPayload, len(selected))
	for index, slot := range selected {
		if d.validateCoordinateSlotFor(coordinate, slot, skeleton.keys) != nil {
			return fmt.Errorf("%w: coordinate selection support slot %d", ErrInvalidLaneFactor, index)
		}
		keys[index] = slot.key
	}
	required, ok := coordinate.ops.selectionSupport(skeleton.payload, keys)
	if !ok {
		return fmt.Errorf("%w: coordinate selection support in family %q", ErrInvalidLaneFactor, coordinate.family.id)
	}
	for _, key := range required {
		if key == nil || !coordinate.ops.keyValid(key, skeleton.keys) {
			return fmt.Errorf("%w: coordinate selection support in family %q", ErrInvalidLaneFactor, coordinate.family.id)
		}
		found := false
		for _, slot := range selected {
			if coordinate.ops.keyEqual(slot.key, key) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: coordinate selection in family %q omits required structural support", ErrIncompleteLaneFactors, coordinate.family.id)
		}
	}
	return nil
}
