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
	schema *Schema
	key    mountedCallResultSlotKey
	slotID identity.ContentID
	// content is the owner-issued identity of this mounted row. A Program
	// CallResultSlot identity is reusable across mounts of one program, so
	// the mount qualifies it here once instead of every consuming rule
	// re-qualifying it with a hash of its own.
	content    identity.ContentID
	sourceKind programschema.CallResultSlotSourceKind
	value      identity.ContentID
	coordinate Coordinate
	// directory is the one-based dense candidate address of a result-zero
	// row. Later result ordinals carry none: they belong to another output
	// geometry and no rule folds them through this directory.
	directory uint32
}

func (row MountedCallResultSlot) valid() bool {
	return row.schema != nil && row.schema.Valid() && row.key.module.Available() &&
		row.key.call.Available() && row.slotID.Available() && row.content.Available() && row.sourceKind.Valid() && row.value.Available() &&
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

// ID returns the owner-issued identity of this mounted result slot. It is
// the operand identity a consuming rule declares; a rule does not derive one.
func (row MountedCallResultSlot) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
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
			// join the call summary before the explicit storage transfer. The
			// slot carries Program's derived portable identity for that result,
			// so the reserved coordinate is named by it in the detached identity
			// range like every other coordinate.
			if coordinateOK || builder.Schema.coordinateCount == ^uint32(0) {
				return false
			}
			mountedKey := mountedCoordinateKey{module: key.module, value: valueID}
			_, mountedDuplicate := builder.Schema.mountedCoordinates[mountedKey]
			_, identityDuplicate := builder.Schema.coordinates[valueID]
			if mountedDuplicate || identityDuplicate {
				return false
			}
			builder.Schema.coordinateCount++
			coordinate = Coordinate{schema: builder.Schema, index: builder.Schema.coordinateCount}
			builder.Schema.mountedCoordinates[mountedKey] = coordinate.index
			builder.Schema.coordinates[valueID] = coordinateRow{coordinate: coordinate.index}
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
			content:    computationContent(builder.linkID, "val-callresultslot!", key.module, key.call, uint64(key.ordinal)),
			sourceKind: slot.SourceKind(),
			value:      valueID,
			coordinate: coordinate,
		}
		if key.ordinal == 0 {
			if uint64(len(builder.Schema.mountedCallResultSlotDirectory)) >= uint64(^uint32(0)) {
				return false
			}
			row.directory = uint32(len(builder.Schema.mountedCallResultSlotDirectory)) + 1
		}
		if !row.valid() {
			return false
		}
		if _, duplicate := builder.Schema.mountedCallResultSlots[key]; duplicate {
			return false
		}
		builder.Schema.mountedCallResultSlots[key] = row
		if row.directory != 0 {
			builder.Schema.mountedCallResultSlotDirectory = append(builder.Schema.mountedCallResultSlotDirectory, row)
		}
	}
	return true
}

// MountedCallResultSlotForMountedOccurrence resolves the owner-issued
// result-zero slot of one mounted Program call occurrence. It is the candidate
// resolver of the result-zero directory: a call that seals no valued first
// result has no row here, which is the same set the result-slot requirement
// issues a placement for.
func (schema *Schema) MountedCallResultSlotForMountedOccurrence(module, occurrence identity.ContentID) (MountedCallResultSlot, bool) {
	row, ok := schema.MountedCallResultSlotFor(module, occurrence, 0)
	return row, ok && row.directory != 0
}

// MountedCallResultSlotOrdinal returns the dense candidate address of one
// owner-issued result-zero slot.
func (schema *Schema) MountedCallResultSlotOrdinal(row MountedCallResultSlot) (uint32, bool) {
	if schema == nil || !schema.OwnsMountedCallResultSlot(row) || row.directory == 0 {
		return 0, false
	}
	return row.directory - 1, true
}

// MountedCallResultSlotAt redeems one dense result-zero candidate. The order is
// the sealed publication order of the slot geometry, and a later result ordinal
// is never reachable through it.
func (schema *Schema) MountedCallResultSlotAt(index int) (MountedCallResultSlot, bool) {
	if schema == nil || index < 0 || index >= len(schema.mountedCallResultSlotDirectory) {
		return MountedCallResultSlot{}, false
	}
	row := schema.mountedCallResultSlotDirectory[index]
	if !schema.OwnsMountedCallResultSlot(row) || row.directory != uint32(index)+1 {
		return MountedCallResultSlot{}, false
	}
	return row, true
}
