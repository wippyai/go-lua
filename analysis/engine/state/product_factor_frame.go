package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ProductFactorFrame is one exact selected view of a complete product
// carrier. Coordinate factors are family-native: a reduced physical lane is
// never presented as though it were a complete lane. Values are aligned with
// ProductFactorSelection.ValueFactors.
type ProductFactorFrame struct {
	authority   *productFactorSelectionSeal
	ordinary    []LaneFactor
	coordinates []CoordinateFamilyFactor
	values      []product.Value
	valuesTop   bool
}

// sealProductFactorFrame validates a component-produced frame against its
// exact selection. Public writers go through ProductFactorFrameTransaction,
// so callers cannot manufacture or reorder physical factors.
func (d ProductDomain) sealProductFactorFrame(
	selection ProductFactorSelection,
	ordinary []LaneFactor,
	coordinates []CoordinateFamilyFactor,
	values []product.Value,
	valuesTop bool,
) (ProductFactorFrame, error) {
	if err := d.validateFactorSelection(selection); err != nil || len(ordinary) != len(selection.ordinary) ||
		len(coordinates) != len(selection.coordinateFactors) || len(values) != len(selection.values) {
		return ProductFactorFrame{}, fmt.Errorf("%w: incomplete product factor frame", ErrIncompleteLaneFactors)
	}
	ownedOrdinary := append([]LaneFactor(nil), ordinary...)
	for index, factor := range ownedOrdinary {
		if factor.lane != selection.ordinary[index] {
			return ProductFactorFrame{}, fmt.Errorf("%w: ordinary frame factor %d", ErrInvalidLaneFactor, index)
		}
		if _, err := d.validateFactorFor(&d.factorLanes[factor.lane.ordinal], factor); err != nil {
			return ProductFactorFrame{}, err
		}
	}
	ownedCoordinates := append([]CoordinateFamilyFactor(nil), coordinates...)
	for index, factor := range ownedCoordinates {
		bucket := selection.coordinateFactors[index]
		if factor.Family() != bucket.family || factor.Skeleton().keys != selection.coordinates.keys {
			return ProductFactorFrame{}, fmt.Errorf("%w: coordinate frame factor %d", ErrInvalidLaneFactor, index)
		}
		if bucket.skeletonOnly {
			if len(factor.scalars) != 0 {
				return ProductFactorFrame{}, fmt.Errorf("%w: skeleton-only coordinate frame carries scalars", ErrInvalidLaneFactor)
			}
			if _, err := d.validateCoordinateSkeleton(factor.Skeleton()); err != nil {
				return ProductFactorFrame{}, err
			}
			continue
		}
		shape, err := d.SealCoordinateFamilyShape(factor.Skeleton(), bucket.slots)
		if err != nil {
			return ProductFactorFrame{}, err
		}
		equal, err := d.CoordinateSkeletonRepresentationEqual(shape.Skeleton(), factor.Skeleton())
		if err != nil || !equal {
			return ProductFactorFrame{}, fmt.Errorf("%w: coordinate frame skeleton %d exceeds selection", ErrInvalidLaneFactor, index)
		}
		for _, scalar := range factor.Scalars() {
			coordinate, _ := d.validateCoordinateFamily(bucket.family)
			if !coordinateSlotSelected(coordinate, bucket.slots, scalar.slot.key, selection.coordinates.keys) {
				return ProductFactorFrame{}, fmt.Errorf("%w: coordinate frame scalar %d exceeds selection", ErrInvalidLaneFactor, index)
			}
		}
	}
	ownedValues := append([]product.Value(nil), values...)
	for index, value := range ownedValues {
		if !product.BelongsToRegistry(d.reg, value) {
			return ProductFactorFrame{}, fmt.Errorf("%w: Values frame factor %d", ErrInvalidLaneFactor, index)
		}
	}
	if valuesTop && !selection.valuesTop {
		return ProductFactorFrame{}, fmt.Errorf("%w: Values Top is outside selection", ErrInvalidLaneFactor)
	}
	return ProductFactorFrame{
		authority: selection.authority, ordinary: ownedOrdinary, coordinates: ownedCoordinates,
		values: ownedValues, valuesTop: valuesTop,
	}, nil
}

// ProductFactorFrameTransaction is a selection-owned, State-free component
// result. Writes are addressed by sealed descriptors and rejected unless the
// selection owns that exact output factor.
type ProductFactorFrameTransaction struct {
	domain    ProductDomain
	selection ProductFactorSelection
	frame     ProductFactorFrame
	used      bool
}

// BeginProductFactorFrameTransaction opens an exact output frame for typed
// writes. Input/read frames are separate component authorities in factapply;
// outputBase is the carrier projection for outputs only and remains immutable.
func (d ProductDomain) BeginProductFactorFrameTransaction(
	outputs ProductFactorSelection,
	outputBase ProductFactorFrame,
) (*ProductFactorFrameTransaction, error) {
	sealed, err := d.sealProductFactorFrame(
		outputs, outputBase.ordinary, outputBase.coordinates, outputBase.values, outputBase.valuesTop,
	)
	if err != nil {
		return nil, err
	}
	return &ProductFactorFrameTransaction{domain: d, selection: outputs, frame: sealed}, nil
}

// BindProductFactorFrame seals one already-factored, State-free carrier view.
// It is the adapter boundary for decision-diagram and other transposed
// evaluators: physical factors stay in their family-native spelling and no
// sparse State is reconstructed merely to obtain a frame authority.
func (d ProductDomain) BindProductFactorFrame(
	selection ProductFactorSelection,
	ordinary []LaneFactor,
	coordinates []CoordinateFamilyFactor,
	values []product.Value,
	valuesTop bool,
) (ProductFactorFrame, error) {
	return d.sealProductFactorFrame(selection, ordinary, coordinates, values, valuesTop)
}

// WriteOrdinary replaces one selection-owned whole-lane factor.
func (t *ProductFactorFrameTransaction) WriteOrdinary(lane ProductLane, factor LaneFactor) error {
	if t == nil || t.used || lane.seal == nil || factor.lane != lane {
		return fmt.Errorf("%w: ordinary frame transaction write", ErrInvalidLaneFactor)
	}
	for index, selected := range t.selection.ordinary {
		if selected == lane {
			if _, err := t.domain.validateFactorFor(&t.domain.factorLanes[lane.ordinal], factor); err != nil {
				return err
			}
			t.frame.ordinary[index] = factor
			return nil
		}
	}
	return fmt.Errorf("%w: ordinary frame write is undeclared", ErrInvalidLaneFactor)
}

// WriteCoordinate replaces one selection-owned family-native factor.
func (t *ProductFactorFrameTransaction) WriteCoordinate(family CoordinateFamily, factor CoordinateFamilyFactor) error {
	if t == nil || t.used || factor.Family() != family {
		return fmt.Errorf("%w: coordinate frame transaction write", ErrInvalidLaneFactor)
	}
	for index, bucket := range t.selection.coordinateFactors {
		if bucket.family == family {
			t.frame.coordinates[index] = factor
			return nil
		}
	}
	return fmt.Errorf("%w: coordinate frame write is undeclared", ErrInvalidLaneFactor)
}

// OwnsProductFactorFrame reports whether frame was sealed by selection. It is
// the constant-time component handoff check; full structural validation is
// paid only when a frame is projected, written, or finished.
func (d ProductDomain) OwnsProductFactorFrame(selection ProductFactorSelection, frame ProductFactorFrame) bool {
	return d.validateFactorSelection(selection) == nil && frame.authority != nil && frame.authority == selection.authority
}

// WriteValue replaces one selection-owned finite Values factor.
func (t *ProductFactorFrameTransaction) WriteValue(slot statekey.Value, value product.Value) error {
	if t == nil || t.used || !product.BelongsToRegistry(t.domain.reg, value) {
		return fmt.Errorf("%w: Values frame transaction write", ErrInvalidLaneFactor)
	}
	position := sort.Search(len(t.selection.values), func(index int) bool { return t.selection.values[index] >= slot })
	if position >= len(t.selection.values) || t.selection.values[position] != slot {
		return fmt.Errorf("%w: Values frame write is undeclared", ErrInvalidLaneFactor)
	}
	t.frame.values[position] = value
	return nil
}

// WriteValuesTop replaces the lifted Values Top bit when explicitly owned.
func (t *ProductFactorFrameTransaction) WriteValuesTop(top bool) error {
	if t == nil || t.used || !t.selection.valuesTop {
		return fmt.Errorf("%w: Values Top frame write is undeclared", ErrInvalidLaneFactor)
	}
	t.frame.valuesTop = top
	return nil
}

// Finish validates the complete typed result and consumes the transaction.
func (t *ProductFactorFrameTransaction) Finish() (ProductFactorFrame, error) {
	if t == nil || t.used {
		return ProductFactorFrame{}, fmt.Errorf("%w: product factor frame transaction consumed", ErrInvalidLaneFactor)
	}
	t.used = true
	if t.frame.valuesTop {
		top := product.Top()
		for index := range t.frame.values {
			t.frame.values[index] = top
		}
	}
	return t.domain.sealProductFactorFrame(
		t.selection, t.frame.ordinary, t.frame.coordinates, t.frame.values, t.frame.valuesTop,
	)
}

// OrdinaryFactors returns detached whole-lane factors in selection order.
func (f ProductFactorFrame) OrdinaryFactors() []LaneFactor {
	return append([]LaneFactor(nil), f.ordinary...)
}

// CoordinateFactors returns detached family-native factors in selection order.
func (f ProductFactorFrame) CoordinateFactors() []CoordinateFamilyFactor {
	return append([]CoordinateFamilyFactor(nil), f.coordinates...)
}

// Values returns detached finite Values factors in selection order.
func (f ProductFactorFrame) Values() []product.Value {
	return append([]product.Value(nil), f.values...)
}

// ValuesTop reports the selected lifted-map Top bit.
func (f ProductFactorFrame) ValuesTop() bool { return f.valuesTop }

// ProjectProductFactorFrame projects selection from a complete State. The
// returned coordinate factors retain their family skeleton and only the exact
// selected scalar inventory. No sparse State is constructed.
func (d ProductDomain) ProjectProductFactorFrame(value State, selection ProductFactorSelection) (ProductFactorFrame, error) {
	if err := d.validateFactorSelection(selection); err != nil {
		return ProductFactorFrame{}, err
	}
	value = d.Normalize(value)
	ordinary, err := d.DecomposeLanes(value, selection.ordinary)
	if err != nil {
		return ProductFactorFrame{}, err
	}
	frame := ProductFactorFrame{
		authority: selection.authority,
		ordinary:  ordinary,
		values:    make([]product.Value, len(selection.values)),
	}
	for index, slot := range selection.values {
		frame.values[index] = value.ReadValue(d.reg, slot)
	}
	_, valueFactor := DecomposeValueLane(d.lattice, value)
	if selection.valuesTop {
		frame.valuesTop = valueFactor.Top
	}
	for _, group := range selection.coordinateGroups {
		laneFactor, factorErr := d.DecomposeLane(value, group.lane)
		if factorErr != nil {
			return ProductFactorFrame{}, fmt.Errorf("%w: coordinate carrier projection", ErrInvalidLaneFactor)
		}
		for familyIndex := group.first; familyIndex < group.first+group.count; familyIndex++ {
			bucket := selection.coordinateFactors[familyIndex]
			skeleton, scalars, familyErr := d.DecomposeCoordinateFamily(laneFactor, bucket.family, selection.coordinates.keys)
			if familyErr != nil {
				return ProductFactorFrame{}, familyErr
			}
			if bucket.skeletonOnly {
				factor, sealErr := d.SealCoordinateFamilyFactor(skeleton, nil)
				if sealErr != nil {
					return ProductFactorFrame{}, sealErr
				}
				frame.coordinates = append(frame.coordinates, factor)
				continue
			}
			shape, familyErr := d.SealCoordinateFamilyShape(skeleton, bucket.slots)
			if familyErr != nil {
				return ProductFactorFrame{}, familyErr
			}
			selected := make([]CoordinateScalarFactor, 0, len(bucket.slots))
			coordinate, _ := d.validateCoordinateFamily(bucket.family)
			for scalarIndex, slotIndex := 0, 0; scalarIndex < len(scalars) && slotIndex < len(bucket.slots); {
				scalar, slot := scalars[scalarIndex], bucket.slots[slotIndex]
				switch {
				case coordinate.ops.keyEqual(scalar.slot.key, slot.key):
					selected = append(selected, scalar)
					scalarIndex++
					slotIndex++
				case coordinate.ops.keyLess(scalar.slot.key, slot.key, selection.coordinates.keys):
					scalarIndex++
				default:
					slotIndex++
				}
			}
			factor, familyErr := d.SealCoordinateFamilyFactor(shape.Skeleton(), selected)
			if familyErr != nil {
				return ProductFactorFrame{}, familyErr
			}
			frame.coordinates = append(frame.coordinates, factor)
		}
	}
	return d.sealProductFactorFrame(selection, frame.ordinary, frame.coordinates, frame.values, frame.valuesTop)
}

// PatchProductFactorFrame overlays an exact component result onto a complete
// carrier. Ordinary lanes replace whole factors. Coordinate scalars replace
// only selected slots while the family skeleton is supplied by the component;
// all unselected scalars and sibling families are physically carried. Values
// Top remains absorbing unless the selection explicitly owns that bit.
func (d ProductDomain) PatchProductFactorFrame(base State, selection ProductFactorSelection, frame ProductFactorFrame) (State, error) {
	if err := d.validateFactorSelection(selection); err != nil || frame.authority == nil || frame.authority != selection.authority ||
		len(frame.ordinary) != len(selection.ordinary) || len(frame.coordinates) != len(selection.coordinateFactors) ||
		len(frame.values) != len(selection.values) {
		return State{}, fmt.Errorf("%w: product factor frame authority", ErrInvalidLaneFactor)
	}
	for index, factor := range frame.ordinary {
		if factor.lane != selection.ordinary[index] {
			return State{}, fmt.Errorf("%w: ordinary frame factor %d", ErrInvalidLaneFactor, index)
		}
	}
	out, err := d.PatchLaneFactors(base, frame.ordinary)
	if err != nil {
		return State{}, err
	}
	coordinateReplacements := make([]LaneFactor, 0, len(selection.coordinateGroups))
	for _, group := range selection.coordinateGroups {
		currentCoordinateFactor, decomposeErr := d.DecomposeLane(out, group.lane)
		if decomposeErr != nil {
			return State{}, fmt.Errorf("%w: coordinate frame carrier", ErrInvalidLaneFactor)
		}
		for familyIndex := group.first; familyIndex < group.first+group.count; familyIndex++ {
			bucket, image := selection.coordinateFactors[familyIndex], frame.coordinates[familyIndex]
			if image.Family() != bucket.family {
				return State{}, fmt.Errorf("%w: coordinate frame factor %d", ErrInvalidLaneFactor, familyIndex)
			}
			if bucket.skeletonOnly {
				currentCoordinateFactor, err = d.ReconcileCoordinateFamily(currentCoordinateFactor, image.Skeleton(), nil)
			} else {
				currentCoordinateFactor, err = d.patchSelectedCoordinateFamily(currentCoordinateFactor, bucket.slots, bucket.overlay, image)
			}
			if err != nil {
				return State{}, err
			}
		}
		coordinateReplacements = append(coordinateReplacements, currentCoordinateFactor)
	}
	if len(coordinateReplacements) != 0 {
		out, err = d.PatchLaneFactors(out, coordinateReplacements)
		if err != nil {
			return State{}, err
		}
	}
	_, values := DecomposeValueLane(d.lattice, out)
	if selection.valuesTop {
		values.Top = frame.valuesTop
		if values.Top {
			values.Values = nil
		}
	}
	if !values.Top {
		if values.Values == nil && len(frame.values) != 0 {
			values.Values = make(map[statekey.Value]product.Value, len(frame.values))
		}
		bottom := product.Bottom(d.reg)
		for index, slot := range selection.values {
			if !product.BelongsToRegistry(d.reg, frame.values[index]) {
				return State{}, fmt.Errorf("%w: Values frame factor %d", ErrInvalidLaneFactor, index)
			}
			if product.Equal(d.reg, frame.values[index], bottom) {
				delete(values.Values, slot)
			} else {
				values.Values[slot] = frame.values[index]
			}
		}
	}
	out = RecomposeValueLane(d.reg, d.lattice, out, values)
	return d.Normalize(out), nil
}

func (d ProductDomain) patchSelectedCoordinateFamily(current LaneFactor, selected []CoordinateSlot, overlay CoordinateSkeletonOverlayPlan, image CoordinateFamilyFactor) (LaneFactor, error) {
	family := image.Family()
	currentSkeleton, currentScalars, err := d.DecomposeCoordinateFamily(current, family, image.Skeleton().keys)
	if err != nil {
		return LaneFactor{}, err
	}
	overlaidSkeleton, err := d.OverlaySelectedCoordinateSkeleton(overlay, currentSkeleton, image.Skeleton(), currentScalars)
	if err != nil {
		return LaneFactor{}, err
	}
	imageScalars := image.scalars
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil {
		return LaneFactor{}, err
	}
	for _, scalar := range imageScalars {
		if !coordinateSlotSelected(coordinate, selected, scalar.slot.key, image.Skeleton().keys) {
			return LaneFactor{}, fmt.Errorf("%w: coordinate image writes outside selection", ErrInvalidLaneFactor)
		}
	}
	updates := make([]CoordinateScalarFactor, 0, len(selected))
	for _, slot := range selected {
		outputSupport, supportErr := d.CoordinateScalarSupport(overlaidSkeleton, slot)
		if supportErr != nil {
			return LaneFactor{}, supportErr
		}
		if outputSupport == CoordinateScalarForbidden {
			continue
		}
		scalar, explicit := coordinateScalarAt(coordinate, imageScalars, slot.key, image.Skeleton().keys)
		if !explicit {
			scalar, err = d.CoordinateDefault(image.Skeleton(), slot)
			if err != nil {
				return LaneFactor{}, err
			}
		}
		omitted, omittedErr := d.CoordinateScalarIsOmitted(overlaidSkeleton, scalar)
		if omittedErr != nil {
			return LaneFactor{}, omittedErr
		}
		if !omitted {
			updates = append(updates, scalar)
		}
	}
	merged := make([]CoordinateScalarFactor, 0, len(currentScalars)+len(updates))
	currentIndex, selectedIndex, updateIndex := 0, 0, 0
	advanceCurrent := func() {
		for currentIndex < len(currentScalars) {
			key := currentScalars[currentIndex].slot.key
			for selectedIndex < len(selected) && coordinate.ops.keyLess(selected[selectedIndex].key, key, image.Skeleton().keys) {
				selectedIndex++
			}
			if selectedIndex >= len(selected) || !coordinate.ops.keyEqual(selected[selectedIndex].key, key) {
				return
			}
			currentIndex++
		}
	}
	advanceCurrent()
	for currentIndex < len(currentScalars) || updateIndex < len(updates) {
		switch {
		case currentIndex == len(currentScalars):
			merged = append(merged, updates[updateIndex:]...)
			updateIndex = len(updates)
		case updateIndex == len(updates):
			merged = append(merged, currentScalars[currentIndex])
			currentIndex++
			advanceCurrent()
		case coordinate.ops.keyLess(currentScalars[currentIndex].slot.key, updates[updateIndex].slot.key, image.Skeleton().keys):
			merged = append(merged, currentScalars[currentIndex])
			currentIndex++
			advanceCurrent()
		default:
			merged = append(merged, updates[updateIndex])
			updateIndex++
		}
	}
	return d.ReplaceCoordinateFamily(current, overlaidSkeleton, merged)
}

// PatchSelectedCoordinateFamilyLaneFactor applies one finite exact-selection image to an
// already-factored physical lane. It is the State-free spelling of the same
// registered overlay used by PatchProductFactorFrame.
func (d ProductDomain) PatchSelectedCoordinateFamilyLaneFactor(
	current LaneFactor,
	selected []CoordinateSlot,
	image CoordinateFamilyFactor,
) (LaneFactor, error) {
	overlay, err := d.SealCoordinateSkeletonOverlayPlan(selected)
	if err != nil {
		return LaneFactor{}, err
	}
	return d.patchSelectedCoordinateFamily(current, selected, overlay, image)
}

func coordinateScalarAt(runtime *coordinateFamilyRuntime, scalars []CoordinateScalarFactor, key coordinateKeyPayload, keys *keyspace.KeySpace) (CoordinateScalarFactor, bool) {
	position := sort.Search(len(scalars), func(index int) bool {
		return !runtime.ops.keyLess(scalars[index].slot.key, key, keys)
	})
	if position < len(scalars) && runtime.ops.keyEqual(scalars[position].slot.key, key) {
		return scalars[position], true
	}
	return CoordinateScalarFactor{}, false
}

func coordinateSlotSelected(runtime *coordinateFamilyRuntime, slots []CoordinateSlot, key coordinateKeyPayload, keys *keyspace.KeySpace) bool {
	position := sort.Search(len(slots), func(index int) bool {
		return !runtime.ops.keyLess(slots[index].key, key, keys)
	})
	return position < len(slots) && runtime.ops.keyEqual(slots[position].key, key)
}
