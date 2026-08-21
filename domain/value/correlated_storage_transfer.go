package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// storageTransferKind is the closed Program occurrence family whose fixed
// scalar storage relation belongs to Value. Pack-owned open-tail selection is
// intentionally absent.
type storageTransferKind uint8

const (
	storageTransferInvalid storageTransferKind = iota
	storageTransferRead
	storageTransferBind
	storageTransferWrite
)

func (kind storageTransferKind) valid() bool {
	return kind >= storageTransferRead && kind <= storageTransferWrite
}

// positional reports whether the family fans one authored statement out over a
// value list, so the target's list position is part of the relation identity.
// A read names one cell and takes no list position. A bind and an assignment
// write each take the member of their statement's value list at their own
// position, and Program issues one occurrence per position in both families.
func (kind storageTransferKind) positional() bool {
	return kind == storageTransferBind || kind == storageTransferWrite
}

func storageTransferKindForArtifact(kind programschema.OccurrenceKind) (storageTransferKind, bool) {
	switch kind {
	case programschema.OccurrenceStorageRead:
		return storageTransferRead, true
	case programschema.OccurrenceStorageBindTransfer:
		return storageTransferBind, true
	case programschema.OccurrenceStorageWrite:
		return storageTransferWrite, true
	default:
		return storageTransferInvalid, false
	}
}

// StorageTransferRef is the exact mounted inverse identity of one Value-owned
// fixed storage relation. The tuple names an existing Program occurrence; it
// never names a Link scalar row or allocation ordinal.
type StorageTransferRef struct {
	linkID     identity.ContentID
	mount      identity.ContentID
	occurrence identity.ContentID
	kind       storageTransferKind
	position   uint32
}

// StorageTransfer is one sealed directed relation from an existing Value
// coordinate to another. Its dense ordinal is private to its issuing Schema.
type StorageTransfer struct {
	schema *Schema
	index  uint32
}

type storageTransferRow struct {
	ref      StorageTransferRef
	id       identity.ContentID
	from     Coordinate
	to       Coordinate
	lifetime lifecycle.StorageLifetime
	ordinal  uint32
}

// storageTransferOccurrenceKey joins a reusable Program occurrence identity
// to the exact Link mount that issued its Value operand. The mount qualifier
// is mandatory: one reusable Program may appear in several Link modules.
type storageTransferOccurrenceKey struct {
	mount      identity.ContentID
	occurrence identity.ContentID
}

// storageLifetimeProof is a construction-only exact Program Cell directory.
// A missing key is not an Unknown lifetime: it is an endpoint that was never
// admitted to the mounted Program's Cell family. globals is a second, cold
// refinement directory built once from Host's exact inverse relation.
type storageLifetimeProof struct {
	cells  map[identity.ContentID]lifecycle.StorageLifetime
	global map[identity.ContentID]struct{}
}

type storageOccurrenceRecord struct {
	id     identity.ContentID
	body   identity.ContentID
	values identity.ContentID
	width  uint32
}

type storageValueMemberKey struct {
	values   identity.ContentID
	position uint32
}

type storageValueMemberRecord struct {
	// id is the canonical ValuesMember-family row identity. The occurrence's
	// second input is a separate semantic subject identity and is deliberately
	// not used to join storage transfer operands.
	id       identity.ContentID
	body     identity.ContentID
	values   identity.ContentID
	position uint32
}

type storageBindCellKey struct {
	bind     identity.ContentID
	position uint32
}

// storageOccurrenceIndex is built once per mounted Program. It keeps the
// transfer scan O(T+C+G), rather than rediscovering Values, parent rows, and
// global authority for every transfer occurrence. bindCells is keyed by the
// parent occurrence and dense target position so transfer validation never
// scans the parent occurrence family.
type storageOccurrenceIndex struct {
	values      map[identity.ContentID]storageOccurrenceRecord
	members     map[storageValueMemberKey]storageValueMemberRecord
	tailValues  map[storageValueMemberKey]storageTailValueRecord
	binds       map[identity.ContentID]storageOccurrenceRecord
	bindCells   map[storageBindCellKey]identity.ContentID
	assignments map[identity.ContentID]storageOccurrenceRecord
}

type storageTailValueRecord struct {
	id   identity.ContentID
	cell identity.ContentID
}

// StorageTransferCount reports Value's complete fixed Read/Bind/Write
// denominator. No Link computation projection is consulted.
func (schema *Schema) StorageTransferCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.storageTransfers)
}

// StorageTransferAt returns one deterministic Program-order relation.
func (schema *Schema) StorageTransferAt(index int) (StorageTransfer, bool) {
	if schema == nil || index < 0 || uint64(index) >= uint64(^uint32(0)) || index >= len(schema.storageTransfers) {
		return StorageTransfer{}, false
	}
	transfer := StorageTransfer{schema: schema, index: uint32(index)}
	return transfer, transfer.valid()
}

// StorageTransferForArtifactOccurrence resolves the exact Link-owned
// transfer operand for one mounted reusable Program occurrence. The mapping
// is sealed with the Value schema; hot callers never reopen Program or Link
// topology to reconstruct it.
func (schema *Schema) StorageTransferForArtifactOccurrence(mount, occurrence identity.ContentID) (StorageTransfer, bool) {
	if schema == nil || !mount.Available() || !occurrence.Available() || schema.storageTransferOccurrences == nil {
		return StorageTransfer{}, false
	}
	ordinal := schema.storageTransferOccurrences[storageTransferOccurrenceKey{mount: mount, occurrence: occurrence}]
	if ordinal == 0 {
		return StorageTransfer{}, false
	}
	transfer := StorageTransfer{schema: schema, index: ordinal - 1}
	return transfer, transfer.valid()
}

// FindStorageTransfer returns only a receipt issued by this exact Schema.
// StorageTransferRef retains the exact ModuleKey mount and Program occurrence
// from sealing, so equal content from another Link cannot replay through this
// inverse. The sealed ordinal map is the complete warm lookup authority; it
// never reopens Flow.
func (schema *Schema) FindStorageTransfer(ref StorageTransferRef) (StorageTransfer, bool) {
	if schema == nil || !ref.valid() || ref.linkID != schema.linkID {
		return StorageTransfer{}, false
	}
	ordinal := schema.storageTransferOrdinals[ref]
	if ordinal == 0 {
		return StorageTransfer{}, false
	}
	transfer := StorageTransfer{schema: schema, index: ordinal - 1}
	return transfer, transfer.valid()
}

func (ref StorageTransferRef) valid() bool {
	if !ref.linkID.Available() || !ref.mount.Available() || !ref.occurrence.Available() || !ref.kind.valid() {
		return false
	}
	return ref.kind.positional() || ref.position == 0
}

func (transfer StorageTransfer) valid() bool {
	if transfer.schema == nil || uint64(transfer.index) >= uint64(len(transfer.schema.storageTransfers)) {
		return false
	}
	row := transfer.schema.storageTransfers[transfer.index]
	if !row.ref.valid() || !row.id.Available() || !row.from.Valid() || !row.to.Valid() || !row.lifetime.Valid() {
		return false
	}
	// Program/Flow occurrence geometry is authenticated once by
	// addStorageTransfer while the Schema is sealing. A published handle is
	// thereafter the exact (Schema pointer, dense row) receipt; hot rule paths
	// must not reopen the mounted Program or inverse map to re-prove it.
	return uint64(row.ordinal) == uint64(transfer.index)+1 && row.ref.linkID == transfer.schema.linkID
}

// OwnsStorageTransfer is the complete Value owner fence for this operand.
func (schema *Schema) OwnsStorageTransfer(transfer StorageTransfer) bool {
	return schema != nil && transfer.schema == schema && transfer.valid()
}

// Ref returns the sole exact mounted inverse identity for the relation.
func (transfer StorageTransfer) Ref() (StorageTransferRef, bool) {
	if !transfer.valid() {
		return StorageTransferRef{}, false
	}
	return transfer.schema.storageTransfers[transfer.index].ref, true
}

// ID returns Value's fixed-width content identity for the canonical Program
// occurrence tuple. It is a pure domain-owned digest, not a Link row ID.
func (transfer StorageTransfer) ID() (identity.ContentID, bool) {
	if !transfer.valid() {
		return identity.ContentID{}, false
	}
	return transfer.schema.storageTransfers[transfer.index].id, true
}

// Occurrence returns the executable Program occurrence that owns this fixed
// transfer.  The transfer's Value-coordinate endpoints stay owned by Value,
// while control routing remains entirely in Flow; no from/to route is copied
// out of the sealed relation.
func (transfer StorageTransfer) Occurrence() (identity.ContentID, identity.ContentID, bool) {
	if !transfer.valid() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	ref := transfer.schema.storageTransfers[transfer.index].ref
	return ref.mount, ref.occurrence, true
}

// Endpoints returns Value's exact pre-existing source and destination
// coordinates. Neither coordinate is fabricated from a scalar projection.
func (transfer StorageTransfer) Endpoints() (from, to Coordinate, ok bool) {
	if !transfer.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	row := transfer.schema.storageTransfers[transfer.index]
	return row.from, row.to, true
}

// Lifetime returns the neutral destination-cell lifetime proved during cold
// sealing. Read occurrences carry Frame because their destination is the
// ephemeral read result rather than a persistent Cell.
func (transfer StorageTransfer) Lifetime() (lifecycle.StorageLifetime, bool) {
	if !transfer.valid() {
		return lifecycle.StorageLifetimeInvalid, false
	}
	return transfer.schema.storageTransfers[transfer.index].lifetime, true
}

// Persistent reports whether this occurrence stores into an authored Cell.
// A read's output Value is frame-local and must not drive a Placement escape.
func (transfer StorageTransfer) Persistent() bool {
	if !transfer.valid() {
		return false
	}
	return transfer.schema.storageTransfers[transfer.index].ref.kind != storageTransferRead
}

func (schema *valueBuilder) sealStorageTransfersWithFailure() SealFailure {
	if schema == nil || schema.sealProject() == nil || schema.storageTransfers != nil || schema.storageTransferOrdinals == nil || schema.storageTransferOccurrences == nil || schema.artifacts == nil {
		return SealFailureStorageTransferInput
	}
	globalCells, globalOK := schema.explicitGlobalStorageCells()
	if !globalOK {
		return SealFailureStorageTransferMount
	}
	for index := 0; index < schema.sealProject().Mounts().Count(); index++ {
		shard, shardOK := schema.sealProject().Mounts().At(index)
		module, moduleOK := schema.sealProject().ModuleKey(shard)
		mount, mountOK := schema.artifacts[module]
		if !shardOK || !moduleOK || !mountOK || !mount.Available() {
			return SealFailureStorageTransferMount
		}
		artifact := mount.Snapshot()
		program := artifact.Program()
		state, stateOK := program.ColdState()
		view, viewOK := lifecycle.NewView(state)
		lifetimeProof, lifetimeOK := storageLifetimeProofForProgram(view)
		if !stateOK || !viewOK || !lifetimeOK || !artifact.ArtifactID().Available() {
			return SealFailureStorageTransferMount
		}
		lifetimeProof.global = globalCells[module]
		occurrences, occurrenceOK := storageOccurrenceIndexForProgram(program, lifetimeProof.cells)
		if !occurrenceOK {
			return SealFailureStorageTransferMount
		}
		occurrenceCount, occurrenceCountOK := program.OccurrenceCount()
		if !occurrenceCountOK {
			return SealFailureStorageTransferMount
		}
		for rowIndex := 0; rowIndex < occurrenceCount; rowIndex++ {
			row, rowOK := program.OccurrenceAt(rowIndex)
			kind, kindOK := storageTransferKindForArtifact(row.Kind())
			if !rowOK || !kindOK {
				continue
			}
			position, fromID, toID, transferOK := storageTransferEndpoints(program, rowIndex, row, kind, occurrences, lifetimeProof.cells)
			if !transferOK {
				switch kind {
				case storageTransferRead:
					return SealFailureStorageTransferReadOccurrence
				case storageTransferBind:
					return SealFailureStorageTransferBind
				default:
					return SealFailureStorageTransferWrite
				}
			}
			// StorageTransferRef is the single authority on which
			// (kind, position) tuples name a relation; a non-positional
			// family carrying a list position is refused there.
			if failure := schema.addArtifactStorageTransfer(module, artifact.ArtifactID(), kind, row.ID(), position, fromID, toID, lifetimeProof); failure != SealFailureNone {
				return failure
			}
		}
	}
	return SealFailureNone
}

func (schema *valueBuilder) addArtifactStorageTransfer(module, artifactID identity.ContentID, kind storageTransferKind, occurrence identity.ContentID, position uint32, fromID, toID identity.ContentID, proof storageLifetimeProof) SealFailure {
	if schema == nil || schema.sealProject() == nil || schema.storageTransferOrdinals == nil || schema.storageTransferOccurrences == nil || !module.Available() || !artifactID.Available() || !kind.valid() || !occurrence.Available() || !fromID.Available() || !toID.Available() {
		return SealFailureStorageTransferAddInput
	}
	from, fromOK := schema.CoordinateForMountedSemantic(module, fromID)
	to, toOK := schema.CoordinateForMountedSemantic(module, toID)
	if !fromOK {
		return SealFailureStorageTransferAddFromValue
	}
	if !toOK {
		return SealFailureStorageTransferAddToValue
	}
	cellID := toID
	if kind == storageTransferRead {
		cellID = fromID
	}
	lifetime, lifetimeOK := schema.storageLifetimeForArtifact(module, kind, cellID, proof)
	if !lifetimeOK || !lifetime.Valid() {
		return SealFailureStorageTransferAddToCoordinate
	}
	ref := StorageTransferRef{linkID: schema.linkID, mount: module, occurrence: occurrence, kind: kind, position: position}
	if !ref.valid() {
		return SealFailureStorageTransferAddRef
	}
	id := storageTransferIdentityWithProof(ref, artifactID, fromID, toID, lifetime)
	if !id.Available() {
		return SealFailureStorageTransferAddIdentity
	}
	key := storageTransferOccurrenceKey{mount: module, occurrence: occurrence}
	if schema.storageTransferOrdinals[ref] != 0 {
		return SealFailureStorageTransferAddDuplicateRef
	}
	if schema.storageTransferOccurrences[key] != 0 {
		return SealFailureStorageTransferAddDuplicateOccurrence
	}
	if uint64(len(schema.storageTransfers)) >= uint64(^uint32(0)) {
		return SealFailureStorageTransferAddCapacity
	}
	schema.storageTransfers = append(schema.storageTransfers, storageTransferRow{ref: ref, id: id, from: from, to: to, lifetime: lifetime, ordinal: uint32(len(schema.storageTransfers) + 1)})
	schema.storageTransferOrdinals[ref] = uint32(len(schema.storageTransfers))
	schema.storageTransferOccurrences[key] = uint32(len(schema.storageTransfers))
	return SealFailureNone
}

// storageLifetimeProofForProgram materializes the Program Cell family once
// per mount. A published Unknown row is a valid conservative fact; a missing
// row is an unproven endpoint and therefore fails closed.
func storageLifetimeProofForProgram(view lifecycle.View) (storageLifetimeProof, bool) {
	proof := storageLifetimeProof{cells: make(map[identity.ContentID]lifecycle.StorageLifetime)}
	count, countOK := view.StorageCellLifetimeCount()
	if !countOK {
		return storageLifetimeProof{}, false
	}
	for index := 0; index < count; index++ {
		row, rowOK := view.StorageCellLifetimeAt(index)
		if !rowOK || !row.Available() {
			return storageLifetimeProof{}, false
		}
		id, lifetime := row.ID(), row.Lifetime()
		if _, duplicate := proof.cells[id]; duplicate {
			return storageLifetimeProof{}, false
		}
		proof.cells[id] = lifetime
	}
	return proof, true
}

func storageOccurrenceIndexForProgram(program programschema.Program, cells map[identity.ContentID]lifecycle.StorageLifetime) (storageOccurrenceIndex, bool) {
	index := storageOccurrenceIndex{
		values: make(map[identity.ContentID]storageOccurrenceRecord), members: make(map[storageValueMemberKey]storageValueMemberRecord), tailValues: make(map[storageValueMemberKey]storageTailValueRecord),
		binds: make(map[identity.ContentID]storageOccurrenceRecord), bindCells: make(map[storageBindCellKey]identity.ContentID), assignments: make(map[identity.ContentID]storageOccurrenceRecord),
	}
	resultCount, resultsOK := program.CallResultCount()
	if !resultsOK {
		return storageOccurrenceIndex{}, false
	}
	for resultIndex := 0; resultIndex < resultCount; resultIndex++ {
		result, resultOK := program.CallResultAt(resultIndex)
		valuesID := result.ValuesID()
		offset, slotCount, spanOK := result.SlotSpan()
		if !resultOK || !spanOK {
			return storageOccurrenceIndex{}, false
		}
		if !valuesID.Available() {
			continue
		}
		for child := uint32(0); child < slotCount; child++ {
			slot, slotOK := program.CallResultSlotAt(int(offset + child))
			position, positionOK := slot.ConsumerPosition()
			valueID, valueOK := slot.ValueID()
			if !slotOK || !positionOK {
				return storageOccurrenceIndex{}, false
			}
			if slot.SourceKind() != programschema.CallResultSlotSourceValuesTail || slot.ConsumerKind() != programschema.CallResultSlotConsumerCell {
				continue
			}
			key := storageValueMemberKey{values: valuesID, position: position}
			cellID := slot.ConsumerID()
			if !valueOK || !valueID.Available() || !cellID.Available() {
				return storageOccurrenceIndex{}, false
			}
			if _, duplicate := index.tailValues[key]; duplicate {
				return storageOccurrenceIndex{}, false
			}
			index.tailValues[key] = storageTailValueRecord{id: valueID, cell: cellID}
		}
	}
	count, countOK := program.OccurrenceCount()
	if !countOK || cells == nil {
		return storageOccurrenceIndex{}, false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		row, rowOK := program.OccurrenceAt(ordinal)
		body, bodyOK := row.BodyID()
		_, inputCount, spanOK := row.InputSpan()
		if !rowOK || !spanOK {
			return storageOccurrenceIndex{}, false
		}
		switch row.Kind() {
		case programschema.OccurrenceValues:
			if !bodyOK || inputCount != 0 {
				return storageOccurrenceIndex{}, false
			}
			id := row.ID()
			if _, duplicate := index.values[id]; duplicate {
				return storageOccurrenceIndex{}, false
			}
			index.values[id] = storageOccurrenceRecord{id: id, body: body, values: id}
		case programschema.OccurrenceValuesMember:
			if !bodyOK || inputCount != 2 || row.Code() > uint64(^uint32(0)) {
				return storageOccurrenceIndex{}, false
			}
			values, valuesOK := program.OccurrenceInputID(ordinal, 0)
			member, memberOK := program.OccurrenceInputID(ordinal, 1)
			key := storageValueMemberKey{values: values, position: uint32(row.Code())}
			if !valuesOK || !memberOK || !values.Available() || !member.Available() || !row.ID().Available() {
				return storageOccurrenceIndex{}, false
			}
			if _, duplicate := index.members[key]; duplicate {
				return storageOccurrenceIndex{}, false
			}
			index.members[key] = storageValueMemberRecord{id: row.ID(), body: body, values: values, position: uint32(row.Code())}
		case programschema.OccurrenceStorageBind:
			if !bodyOK || inputCount < 1 || row.Code() != 0 {
				return storageOccurrenceIndex{}, false
			}
			values, valuesOK := program.OccurrenceInputID(ordinal, 0)
			if !valuesOK {
				return storageOccurrenceIndex{}, false
			}
			for position := uint32(1); position < inputCount; position++ {
				cell, cellOK := program.OccurrenceInputID(ordinal, int(position))
				if _, exactCell := cells[cell]; !cellOK || !exactCell {
					return storageOccurrenceIndex{}, false
				}
				key := storageBindCellKey{bind: row.ID(), position: position - 1}
				if _, duplicate := index.bindCells[key]; duplicate {
					return storageOccurrenceIndex{}, false
				}
				index.bindCells[key] = cell
			}
			if _, duplicate := index.binds[row.ID()]; duplicate {
				return storageOccurrenceIndex{}, false
			}
			index.binds[row.ID()] = storageOccurrenceRecord{id: row.ID(), body: body, values: values, width: inputCount - 1}
		case programschema.OccurrenceStorageAssignment:
			if !bodyOK || inputCount != 1 || row.Code() != 0 {
				return storageOccurrenceIndex{}, false
			}
			values, valuesOK := program.OccurrenceInputID(ordinal, 0)
			if !valuesOK {
				return storageOccurrenceIndex{}, false
			}
			if _, duplicate := index.assignments[row.ID()]; duplicate {
				return storageOccurrenceIndex{}, false
			}
			index.assignments[row.ID()] = storageOccurrenceRecord{id: row.ID(), body: body, values: values}
		}
	}
	for key, member := range index.members {
		values, exists := index.values[key.values]
		if !exists || values.body != member.body || member.id == (identity.ContentID{}) {
			return storageOccurrenceIndex{}, false
		}
		if values.width == ^uint32(0) {
			return storageOccurrenceIndex{}, false
		}
		values.width++
		index.values[key.values] = values
	}
	for valuesID, values := range index.values {
		for position := uint32(0); position < values.width; position++ {
			member, memberOK := index.members[storageValueMemberKey{values: valuesID, position: position}]
			if !memberOK || member.values != valuesID || member.body != values.body || member.position != position {
				return storageOccurrenceIndex{}, false
			}
		}
	}
	return index, true
}

func storageTransferEndpoints(program programschema.Program, ordinal int, row programschema.Occurrence, kind storageTransferKind, index storageOccurrenceIndex, cells map[identity.ContentID]lifecycle.StorageLifetime) (uint32, identity.ContentID, identity.ContentID, bool) {
	if !program.Available() || !row.Available() || !kind.valid() || cells == nil || row.Code() > uint64(^uint32(0)) {
		return 0, identity.ContentID{}, identity.ContentID{}, false
	}
	position := uint32(row.Code())
	body, bodyOK := row.BodyID()
	_, inputCount, spanOK := row.InputSpan()
	if !bodyOK || !spanOK {
		return 0, identity.ContentID{}, identity.ContentID{}, false
	}
	input := func(at int) (identity.ContentID, bool) { return program.OccurrenceInputID(ordinal, at) }
	switch kind {
	case storageTransferRead:
		cell, cellOK := input(0)
		span, spanOK := input(1)
		_, exactCell := cells[cell]
		if inputCount != 2 || position != 0 || !cellOK || !spanOK || !exactCell || !span.Available() {
			return 0, identity.ContentID{}, identity.ContentID{}, false
		}
		return 0, cell, row.ID(), true
	case storageTransferBind, storageTransferWrite:
		parentID, parentOK := input(0)
		valueID, valueOK := input(1)
		cellID, cellOK := input(2)
		_, exactCell := cells[cellID]
		if !parentOK || !valueOK || !cellOK || !exactCell {
			return 0, identity.ContentID{}, identity.ContentID{}, false
		}
		var parent storageOccurrenceRecord
		var parentFound bool
		if kind == storageTransferBind {
			parent, parentFound = index.binds[parentID]
			if inputCount != 3 || !parentFound || position >= parent.width {
				return 0, identity.ContentID{}, identity.ContentID{}, false
			}
			parentCell, parentCellOK := index.bindCells[storageBindCellKey{bind: parentID, position: position}]
			if !parentCellOK || parentCell != cellID {
				return 0, identity.ContentID{}, identity.ContentID{}, false
			}
		} else {
			parent, parentFound = index.assignments[parentID]
			predecessor, predecessorOK := input(3)
			route, routeOK := input(4)
			if inputCount != 5 || !parentFound || !predecessorOK || !routeOK || !predecessor.Available() || !route.Available() {
				return 0, identity.ContentID{}, identity.ContentID{}, false
			}
		}
		key := storageValueMemberKey{values: parent.values, position: position}
		member, memberOK := index.members[key]
		tail, tailOK := index.tailValues[key]
		if parent.body != body {
			return 0, identity.ContentID{}, identity.ContentID{}, false
		}
		if tailOK && tail.id == valueID {
			if tail.cell != cellID {
				return 0, identity.ContentID{}, identity.ContentID{}, false
			}
		} else if memberOK {
			if member.body != body || member.id != valueID {
				return 0, identity.ContentID{}, identity.ContentID{}, false
			}
		} else {
			return 0, identity.ContentID{}, identity.ContentID{}, false
		}
		return position, valueID, cellID, true
	default:
		return 0, identity.ContentID{}, identity.ContentID{}, false
	}
}

// storageLifetimeForArtifact is the one cold join from a Value transfer to
// Program's neutral storage-cell family. The exact per-mount directory is
// supplied by the sealing pass, so this method never falls back to a lexical
// guess or scans the Program/Host once per transfer.
func (schema *valueBuilder) storageLifetimeForArtifact(module identity.ContentID, kind storageTransferKind, cellID identity.ContentID, proof storageLifetimeProof) (lifecycle.StorageLifetime, bool) {
	if schema == nil || !module.Available() || !cellID.Available() || proof.cells == nil {
		return lifecycle.StorageLifetimeInvalid, false
	}
	lifetime, found := proof.cells[cellID]
	if !found {
		return lifecycle.StorageLifetimeInvalid, false
	}
	if kind == storageTransferRead {
		return lifecycle.StorageLifetimeFrame, true
	}
	if lifetime == lifecycle.StorageLifetimeUnknown {
		if _, global := proof.global[cellID]; global {
			lifetime = lifecycle.StorageLifetimeGlobal
		}
	}
	return lifetime, lifetime.Valid()
}

// explicitGlobalStorageCells builds Host's exact global inverse once for the
// entire seal. It intentionally returns only canonical Program Cell IDs, so
// an equal-content or foreign Project mapping cannot refine an Unknown row.
func (schema *valueBuilder) explicitGlobalStorageCells() (map[identity.ContentID]map[identity.ContentID]struct{}, bool) {
	result := make(map[identity.ContentID]map[identity.ContentID]struct{})
	if schema == nil || schema.sealHost() == nil || schema.sealModule() == nil || schema.sealProject() == nil {
		return nil, false
	}
	globals := schema.sealHost().Globals()
	for index := 0; index < globals.Count(); index++ {
		binding, bindingOK := globals.At(index)
		if !bindingOK {
			return nil, false
		}
		analysis, _, cell, _, class, initial, mappingOK := globals.Mapping(binding)
		if !mappingOK || class == vocabulary.InitialBindingInvalid || initial == 0 {
			continue
		}
		canonical, canonicalOK := globals.For(analysis, cell)
		shard, _, _, rootOK := schema.sealModule().Roots().Mapping(analysis)
		if !canonicalOK || canonical != binding || !rootOK {
			continue
		}
		module, moduleOK := schema.sealProject().ModuleKey(shard)
		programID, programOK := schema.sealProject().Mounts().ProgramID(shard)
		owner, ownerOK := schema.sealProject().Mounts().Program(shard)
		if !moduleOK || !programOK || !ownerOK || owner == nil {
			continue
		}
		if _, exactOK := globals.ForProgramCell(shard, owner, cell); !exactOK {
			continue
		}
		candidate, candidateOK := lifecycle.StorageCellIdentity(programID, cell)
		if !candidateOK {
			continue
		}
		cells := result[module]
		if cells == nil {
			cells = make(map[identity.ContentID]struct{})
			result[module] = cells
		}
		cells[candidate] = struct{}{}
	}
	return result, true
}

// storageTransferIdentity retains the historical ref-only preimage for
// package-local laws that exercise the original identity contract. New sealed
// rows use storageTransferIdentityWithProof below, which authenticates every
// cold endpoint and the lifetime proof.
func storageTransferIdentity(ref StorageTransferRef) identity.ContentID {
	if !ref.valid() {
		return identity.ContentID{}
	}
	var payload [32 + 12*8]byte
	copy(payload[:32], ref.linkID[:])
	words := payload[32:]
	binary.BigEndian.PutUint64(words[0:8], 0x76616c2d73746f72) // "val-stor"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(ref.kind))
	binary.BigEndian.PutUint64(words[24:32], uint64(ref.position))
	copy(words[32:64], ref.mount[:])
	copy(words[64:96], ref.occurrence[:])
	return identity.ContentID(sha256.Sum256(payload[:]))
}

// storageTransferIdentityWithProof derives the current sealed identity. The
// explicit argument types are intentional: a transfer cannot silently accept
// a misordered or otherwise untyped compatibility payload.
func storageTransferIdentityWithProof(ref StorageTransferRef, artifactID, fromID, toID identity.ContentID, lifetime lifecycle.StorageLifetime) identity.ContentID {
	if !ref.valid() || !artifactID.Available() || !fromID.Available() || !toID.Available() || !lifetime.Valid() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.value.storage-transfer.v2\x00"))
	_, _ = hash.Write(ref.linkID[:])
	_, _ = hash.Write(ref.mount[:])
	_, _ = hash.Write(ref.occurrence[:])
	_, _ = hash.Write(artifactID[:])
	_, _ = hash.Write(fromID[:])
	_, _ = hash.Write(toID[:])
	var scalar [24]byte
	binary.BigEndian.PutUint64(scalar[0:8], uint64(ref.kind))
	binary.BigEndian.PutUint64(scalar[8:16], uint64(ref.position))
	binary.BigEndian.PutUint64(scalar[16:24], uint64(lifetime))
	_, _ = hash.Write(scalar[:])
	return identity.ContentID(sha256.Sum256(hash.Sum(nil)))
}
