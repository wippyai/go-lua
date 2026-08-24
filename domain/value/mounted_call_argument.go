package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// mountedCallArgumentKey is the exact mounted Program call actual coordinate.
// A Call contributes one row per actual: the receiver for a method-form call,
// then each declared argument, so the actual ordinal is part of the key
// rather than being inferred from an argument's own Program identity.
type mountedCallArgumentKey struct {
	module identity.ContentID
	call   identity.ContentID
	actual uint32
}

// mountedCallArgumentOccurrenceKey is the mount-qualified inverse from a
// row's own owner-issued content identity to its dense candidate ordinal.
type mountedCallArgumentOccurrenceKey struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

// MountedCallArgument is Value's immutable projection of one admitted mounted
// Program Call actual: the receiver first for a method-form call, then each
// declared argument in order, matching Pack's fixed endpoint list. Every row
// carries the actual's existing semantic Value identity and the Coordinate
// CoordinateForMountedSemantic resolves for it.
type MountedCallArgument struct {
	schema *Schema
	key    mountedCallArgumentKey
	// content is the owner-issued identity of this mounted row. A Program Call
	// identity is reusable across mounts of one program, so the mount
	// qualifies it here once instead of every consuming rule re-qualifying it
	// with a hash of its own.
	content    identity.ContentID
	value      identity.ContentID
	coordinate Coordinate
}

func (row MountedCallArgument) valid() bool {
	return row.schema != nil && row.schema.Valid() && row.key.module.Available() &&
		row.key.call.Available() && row.content.Available() && row.value.Available() &&
		row.coordinate.schema == row.schema && row.coordinate.Valid()
}

// OwnsMountedCallArgument is the exact Schema owner fence for a detached
// mounted Call actual projection. Equal-content Value schemas cannot exchange
// rows.
func (schema *Schema) OwnsMountedCallArgument(row MountedCallArgument) bool {
	if schema == nil || row.schema != schema || !row.valid() || schema.mountedCallArguments == nil {
		return false
	}
	canonical, ok := schema.mountedCallArguments[row.key]
	return ok && canonical == row
}

// MountedCallArgumentFor resolves one exact admitted mounted call actual by
// its declared coordinate. Unknown, structural, unresolved, and foreign rows
// fail closed.
func (schema *Schema) MountedCallArgumentFor(module, call identity.ContentID, actual uint32) (MountedCallArgument, bool) {
	if schema == nil || !schema.Valid() || schema.mountedCallArguments == nil || !module.Available() || !call.Available() {
		return MountedCallArgument{}, false
	}
	row, ok := schema.mountedCallArguments[mountedCallArgumentKey{module: module, call: call, actual: actual}]
	return row, ok && schema.OwnsMountedCallArgument(row)
}

// MountedCallArgumentCount is the dense, mount-major census of owner-issued
// mounted call actuals.
func (schema *Schema) MountedCallArgumentCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.mountedCallArgumentOrder)
}

// MountedCallArgumentAt returns one dense mounted call actual. Order is
// sealed mount order, then call order, then per-call actual order.
func (schema *Schema) MountedCallArgumentAt(index int) (MountedCallArgument, bool) {
	if schema == nil || index < 0 || index >= len(schema.mountedCallArgumentOrder) {
		return MountedCallArgument{}, false
	}
	row, ok := schema.mountedCallArguments[schema.mountedCallArgumentOrder[index]]
	return row, ok && schema.OwnsMountedCallArgument(row)
}

// MountedCallArgumentOrdinal is the exact inverse of MountedCallArgumentAt
// over this Schema.
func (schema *Schema) MountedCallArgumentOrdinal(row MountedCallArgument) (uint32, bool) {
	if schema == nil || !schema.OwnsMountedCallArgument(row) {
		return 0, false
	}
	for index, key := range schema.mountedCallArgumentOrder {
		if key == row.key {
			return uint32(index), true
		}
	}
	return 0, false
}

// MountedCallArgumentForMountedOccurrence is the mount-qualified candidate
// resolver: occurrence is the row's own owner-issued content identity, the
// exact inverse of the identity MountedCallArgument.ID returns.
func (schema *Schema) MountedCallArgumentForMountedOccurrence(module, occurrence identity.ContentID) (MountedCallArgument, bool) {
	if schema == nil || schema.mountedCallArgumentOccurrences == nil || !module.Available() || !occurrence.Available() {
		return MountedCallArgument{}, false
	}
	ordinal, ok := schema.mountedCallArgumentOccurrences[mountedCallArgumentOccurrenceKey{module: module, occurrence: occurrence}]
	if !ok {
		return MountedCallArgument{}, false
	}
	return schema.MountedCallArgumentAt(int(ordinal))
}

// ID returns the owner-issued identity of this mounted call actual. It is
// the operand identity a consuming rule declares; a rule does not derive one.
func (row MountedCallArgument) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

// Module returns the exact mounted module identity for this row.
func (row MountedCallArgument) Module() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.module, true
}

// CallID returns the authored Call identity for this mounted actual.
func (row MountedCallArgument) CallID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.key.call, true
}

// ActualIndex returns the exact actual ordinal relative to the parent Call,
// receiver first for a method-form call, then each declared argument.
func (row MountedCallArgument) ActualIndex() (uint32, bool) {
	if !row.valid() {
		return 0, false
	}
	return row.key.actual, true
}

// ActualTag is the owner-issued selection tag for this actual: the one-based
// form of the ordinal. A selection reserves zero for "no member", so the tag a
// consumer selects a member under is published here rather than each rule
// minting its own one-based convention beside the ordinal.
func (row MountedCallArgument) ActualTag() (uint64, bool) {
	if !row.valid() {
		return 0, false
	}
	return uint64(row.key.actual) + 1, true
}

// ValueID returns the existing semantic Value identity carried by this
// actual.
func (row MountedCallArgument) ValueID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.value, true
}

// Coordinate returns the existing Value coordinate this actual resolves to.
func (row MountedCallArgument) Coordinate() (Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, false
	}
	return row.coordinate, true
}

// sealMountedCallArguments projects every admitted mounted Program Call
// actual after the mounted semantic coordinate directory has been sealed:
// receiver first for a method-form call, then each declared argument in
// order, the same order Pack's fixed endpoint list uses. A declared actual
// that cannot be rebound to this mount's semantic coordinate directory is
// malformed canonical geometry; the seal refuses rather than omitting the
// row.
func (builder *valueBuilder) sealMountedCallArguments() bool {
	if builder == nil || builder.Schema == nil || builder.sealProject() == nil || builder.artifacts == nil ||
		builder.mountedCallArguments == nil || builder.mountedCallArgumentOccurrences == nil ||
		len(builder.Schema.mountedCallArguments) != 0 {
		return false
	}
	mounts := builder.sealProject().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		module, moduleOK := builder.sealProject().ModuleKey(shard)
		mount := builder.artifacts[module]
		if !shardOK || !moduleOK || !module.Available() || !mount.Available() || mount.ModuleKey != module {
			return false
		}
		program := mount.Program.Program
		if !program.Available() {
			return false
		}
		callCount, callsOK := program.CallCount()
		if !callsOK {
			return false
		}
		for callIndex := 0; callIndex < callCount; callIndex++ {
			call, callOK := program.CallAt(callIndex)
			if !callOK || !call.Available() {
				return false
			}
			// The parent row's span opens before this call's first actual and
			// closes after its last, so a call with no actuals still publishes
			// the empty list it has rather than having no row at all.
			first := len(builder.Schema.mountedCallArgumentOrder)
			actual := uint32(0)
			if call.Form() == programschema.CallFormMethod {
				receiverID, receiverOK := call.ReceiverID()
				if !receiverOK || !receiverID.Available() {
					return false
				}
				if !builder.addMountedCallArgument(module, call.ID(), actual, receiverID) {
					return false
				}
				actual++
			}
			for argumentIndex := 0; argumentIndex < call.ArgumentCount(); argumentIndex++ {
				argument, argumentOK := program.CallArgumentFor(callIndex, argumentIndex)
				if !argumentOK || !argument.Available() || argument.CallID() != call.ID() || int(argument.Index()) != argumentIndex || !argument.ValueID().Available() {
					return false
				}
				if !builder.addMountedCallArgument(module, call.ID(), actual, argument.ValueID()) {
					return false
				}
				actual++
			}
			if !builder.addMountedCallActuals(module, call.ID(), first, actual) {
				return false
			}
		}
	}
	return true
}

// addMountedCallArgument admits one dense mounted call actual row. It fails
// closed when the declared actual cannot be rebound to an existing Value
// coordinate in this mount's sealed semantic directory.
func (builder *valueBuilder) addMountedCallArgument(module, call identity.ContentID, actual uint32, valueID identity.ContentID) bool {
	coordinate, coordinateOK := builder.CoordinateForMountedSemantic(module, valueID)
	if !coordinateOK {
		return false
	}
	key := mountedCallArgumentKey{module: module, call: call, actual: actual}
	if _, duplicate := builder.Schema.mountedCallArguments[key]; duplicate {
		return false
	}
	row := MountedCallArgument{
		schema:     builder.Schema,
		key:        key,
		content:    computationContent(builder.linkID, "val-callargument!", module, call, uint64(actual)),
		value:      valueID,
		coordinate: coordinate,
	}
	if !row.valid() {
		return false
	}
	occurrenceKey := mountedCallArgumentOccurrenceKey{module: module, occurrence: row.content}
	if _, duplicate := builder.Schema.mountedCallArgumentOccurrences[occurrenceKey]; duplicate {
		return false
	}
	builder.Schema.mountedCallArgumentOccurrences[occurrenceKey] = uint32(len(builder.Schema.mountedCallArgumentOrder))
	builder.Schema.mountedCallArgumentOrder = append(builder.Schema.mountedCallArgumentOrder, key)
	builder.Schema.mountedCallArguments[key] = row
	return true
}
