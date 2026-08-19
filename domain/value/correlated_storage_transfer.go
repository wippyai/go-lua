package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	ref     StorageTransferRef
	id      identity.ContentID
	from    Coordinate
	to      Coordinate
	ordinal uint32
}

// storageTransferOccurrenceKey joins a reusable Program occurrence identity
// to the exact Link mount that issued its Value operand. The mount qualifier
// is mandatory: one reusable Program may appear in several Link modules.
type storageTransferOccurrenceKey struct {
	mount      identity.ContentID
	occurrence identity.ContentID
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
	if !row.ref.valid() || !row.id.Available() || !row.from.Valid() || !row.to.Valid() {
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

func (schema *valueBuilder) sealStorageTransfersWithFailure() SealFailure {
	if schema == nil || schema.sealProject() == nil || schema.storageTransfers != nil || schema.storageTransferOrdinals == nil || schema.storageTransferOccurrences == nil || schema.artifacts == nil {
		return SealFailureStorageTransferInput
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
			// StorageTransferRef is the single authority on which
			// (kind, position) tuples name a relation; a non-positional
			// family carrying a list position is refused there.
			position := uint32(row.Code())
			var fromID, toID identity.ContentID
			switch kind {
			case storageTransferRead:
				input, inputOK := program.OccurrenceInputFor(rowIndex, 0)
				if !inputOK {
					return SealFailureStorageTransferMount
				}
				fromID = input.InputID()
				toID = row.ID()
			case storageTransferBind, storageTransferWrite:
				from, fromOK := program.OccurrenceInputFor(rowIndex, 1)
				to, toOK := program.OccurrenceInputFor(rowIndex, 2)
				if !fromOK || !toOK {
					return SealFailureStorageTransferMount
				}
				fromID, toID = from.InputID(), to.InputID()
			}
			if failure := schema.addArtifactStorageTransfer(module, kind, row.ID(), position, fromID, toID); failure != SealFailureNone {
				return failure
			}
		}
	}
	return SealFailureNone
}

func (schema *valueBuilder) addArtifactStorageTransfer(module identity.ContentID, kind storageTransferKind, occurrence identity.ContentID, position uint32, fromID, toID identity.ContentID) SealFailure {
	if schema == nil || schema.sealProject() == nil || schema.storageTransferOrdinals == nil || schema.storageTransferOccurrences == nil || !module.Available() || !kind.valid() || !occurrence.Available() || !fromID.Available() || !toID.Available() {
		return SealFailureStorageTransferAddInput
	}
	fromValue, fromValueOK := schema.sealBoundary().Values().ForMountedSemantic(module, fromID)
	toValue, toValueOK := schema.sealBoundary().Values().ForMountedSemantic(module, toID)
	if !fromValueOK {
		return SealFailureStorageTransferAddFromValue
	}
	if !toValueOK {
		return SealFailureStorageTransferAddToValue
	}
	from, fromOK := schema.coordinateForCold(fromValue)
	to, toOK := schema.coordinateForCold(toValue)
	if !fromOK {
		return SealFailureStorageTransferAddFromCoordinate
	}
	if !toOK {
		return SealFailureStorageTransferAddToCoordinate
	}
	ref := StorageTransferRef{linkID: schema.linkID, mount: module, occurrence: occurrence, kind: kind, position: position}
	if !ref.valid() {
		return SealFailureStorageTransferAddRef
	}
	id := storageTransferIdentity(ref)
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
	schema.storageTransfers = append(schema.storageTransfers, storageTransferRow{ref: ref, id: id, from: from, to: to, ordinal: uint32(len(schema.storageTransfers) + 1)})
	schema.storageTransferOrdinals[ref] = uint32(len(schema.storageTransfers))
	schema.storageTransferOccurrences[key] = uint32(len(schema.storageTransfers))
	return SealFailureNone
}

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
	return sha256.Sum256(payload[:])
}
