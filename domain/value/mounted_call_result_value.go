package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// mountedCallResultSlotKey is the exact mounted Program result coordinate.
// A Call can contribute more than one finite result slot, so the result
// ordinal is part of the key rather than being inferred from a CallResult
// shape.
type mountedCallResultSlotKey struct {
	module  identity.ContentID
	call    identity.ContentID
	ordinal uint32
}

// MountedCallResultSlot is Value's immutable projection of one admitted
// Program CallResultSlot. Fixed and direct slots reuse existing mounted Value
// coordinates. A finite tail consumed by a Cell receives a distinct logical
// coordinate here; the Cell remains a separate storage endpoint.
type MountedCallResultSlot struct {
	schema     *Schema
	key        mountedCallResultSlotKey
	slotID     identity.ContentID
	sourceKind programschema.CallResultSlotSourceKind
	value      identity.ContentID
	coordinate Coordinate
}

func (row MountedCallResultSlot) valid() bool {
	return row.schema != nil && row.schema.Valid() && row.key.module.Available() &&
		row.key.call.Available() && row.slotID.Available() && row.sourceKind.Valid() && row.value.Available() &&
		row.coordinate.schema == row.schema && row.coordinate.Valid()
}

// MountedCallResultSlotFor resolves one exact admitted mounted result slot.
// Unknown, structural, unresolved, and foreign rows fail closed.
func (schema *Schema) MountedCallResultSlotFor(module, call identity.ContentID, ordinal uint32) (MountedCallResultSlot, bool) {
	if schema == nil || !schema.Valid() || schema.mountedCallResultSlots == nil ||
		!module.Available() || !call.Available() {
		return MountedCallResultSlot{}, false
	}
	row, ok := schema.mountedCallResultSlots[mountedCallResultSlotKey{module: module, call: call, ordinal: ordinal}]
	return row, ok && schema.OwnsMountedCallResultSlot(row)
}

// OwnsMountedCallResultSlot is the exact Schema owner fence for a detached
// mounted CallResultSlot projection. Equal-content Value schemas cannot
// exchange rows.
func (schema *Schema) OwnsMountedCallResultSlot(row MountedCallResultSlot) bool {
	if schema == nil || row.schema != schema || !row.valid() || schema.mountedCallResultSlots == nil {
		return false
	}
	canonical, ok := schema.mountedCallResultSlots[row.key]
	return ok && canonical == row
}

// Module returns the exact mounted module identity for this row.
func (row MountedCallResultSlot) Module() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.module, true
}

// SlotID returns the existing Program CallResultSlot identity.
func (row MountedCallResultSlot) SlotID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.slotID, true
}

// CallID returns the authored Call identity for this mounted slot.
func (row MountedCallResultSlot) CallID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.call, true
}

// Ordinal returns the exact result ordinal relative to the parent CallResult.
func (row MountedCallResultSlot) Ordinal() (uint32, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.key.ordinal, true
}

// ResultOrdinal is a descriptive alias for Ordinal.
func (row MountedCallResultSlot) ResultOrdinal() (uint32, bool) { return row.Ordinal() }

// ValueID returns the existing semantic Value identity carried by this slot.
func (row MountedCallResultSlot) ValueID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.value, true
}

// Coordinate returns the existing Value coordinate receiving this result
// slot.
func (row MountedCallResultSlot) Coordinate() (Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, false
	}
	return row.coordinate, true
}

// sealMountedCallResultSlots projects every canonical Program CallResultSlot
// after the mounted semantic coordinate directory has been sealed. The cold
// CallResult/CallResultSlot families were already walked together by
// sealMountedCallResultGeometry; this pass performs only O(1) slot lookup and one
// semantic-coordinate join per finite slot.
func (builder *valueBuilder) sealMountedCallResultSlots() bool {
	if builder == nil || builder.Schema == nil || builder.mountedCallResultSlots == nil ||
		builder.mountedCallResultSlotOrder == nil || len(builder.Schema.mountedCallResultSlots) != 0 {
		return false
	}
	for _, key := range builder.mountedCallResultSlotOrder {
		slot, slotOK := builder.mountedCallResultSlots[key]
		if !slotOK || !slot.Available() || slot.CallID() != key.call || slot.Index() != key.ordinal {
			return false
		}
		valueID, valueOK := slot.ValueID()
		// Structural and lens consumers may have no ValueID. An absent value
		// is not a Value coordinate and is omitted rather than turned into a
		// synthetic atom or Coordinate.
		if !valueOK || !valueID.Available() {
			continue
		}
		coordinate, coordinateOK := builder.CoordinateForMountedSemantic(key.module, valueID)
		if slot.SourceKind() == programschema.CallResultSlotSourceValuesTail && slot.ConsumerKind() == programschema.CallResultSlotConsumerCell {
			// The result is not the destination Cell. Reserve one dense logical
			// coordinate per mounted finite tail slot so stale Cell state cannot
			// join the call summary before the explicit storage transfer.
			if coordinateOK || builder.Schema.coordinateCount == ^uint32(0) {
				return false
			}
			builder.Schema.coordinateCount++
			builder.Schema.syntheticCoordinateCount++
			coordinate = Coordinate{schema: builder.Schema, index: builder.Schema.coordinateCount}
			mountedKey := mountedCoordinateKey{module: key.module, value: valueID}
			if _, duplicate := builder.Schema.mountedCoordinates[mountedKey]; duplicate {
				return false
			}
			builder.Schema.mountedCoordinates[mountedKey] = coordinate.index
			coordinateOK = true
		} else if slot.SourceKind() == programschema.CallResultSlotSourceCallValue {
			// A direct scalar Call result carries Program's existing evaluation
			// span identity. Boundary owns the mount-qualified inverse from that
			// span to its Value row; Value then reuses the already-sealed
			// coordinate rather than fabricating a semantic atom.
			boundaryValue, boundaryOK := builder.sealBoundary().Values().ForMountedSpan(key.module, valueID)
			coordinate, coordinateOK = builder.coordinateForCold(boundaryValue)
			if !boundaryOK {
				coordinateOK = false
			}
		}
		if !coordinateOK {
			// A slot that claims an existing ValueID but cannot be rebound to
			// this mount's semantic directory is malformed canonical geometry.
			// Refuse the seal rather than admitting a synthetic coordinate.
			return false
		}
		row := MountedCallResultSlot{
			schema:     builder.Schema,
			key:        key,
			slotID:     slot.ID(),
			sourceKind: slot.SourceKind(),
			value:      valueID,
			coordinate: coordinate,
		}
		if !row.valid() {
			return false
		}
		if _, duplicate := builder.Schema.mountedCallResultSlots[key]; duplicate {
			return false
		}
		builder.Schema.mountedCallResultSlots[key] = row
	}
	return true
}
