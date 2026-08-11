package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
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

func storageTransferKindFor(term keyspace.Term) (storageTransferKind, bool) {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyRead:
		return storageTransferRead, true
	case keyspace.FamilyBind:
		return storageTransferBind, true
	case keyspace.FamilyWrite:
		return storageTransferWrite, true
	default:
		return storageTransferInvalid, false
	}
}

// StorageTransferRef is the replay identity of one Value-owned fixed storage
// relation. The tuple names an existing Program occurrence; it never names a
// Link scalar row or allocation ordinal.
type StorageTransferRef struct {
	linkID       keyspace.ContentID
	shard        linkproject.Shard
	shardOrdinal uint32
	kind         storageTransferKind
	term         keyspace.Term
	position     uint32
}

// StorageTransfer is one sealed directed relation from an existing Value
// coordinate to another. Its dense ordinal is private to its issuing Schema.
type StorageTransfer struct {
	schema *Schema
	index  uint32
}

type storageTransferRow struct {
	ref  StorageTransferRef
	id   keyspace.ContentID
	from Coordinate
	to   Coordinate
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

// StorageTransferFor returns Value's sole fixed storage relation for one
// exact Program occurrence. The occurrence tuple is only an inverse into the
// already-sealed Read/Bind/Write denominator; it creates no compiler-owned
// row or alternate storage identity.
func (schema *Schema) StorageTransferFor(shard linkproject.Shard, occurrence keyspace.Term, position int) (StorageTransfer, bool) {
	if schema == nil || schema.source == nil || shard == (linkproject.Shard{}) || occurrence == 0 || position < 0 || uint64(position) > uint64(^uint32(0)) {
		return StorageTransfer{}, false
	}
	kind, kindOK := storageTransferKindFor(occurrence)
	if !kindOK {
		return StorageTransfer{}, false
	}
	if kind != storageTransferBind && position != 0 {
		return StorageTransfer{}, false
	}
	project := schema.source.Project()
	if project == nil {
		return StorageTransfer{}, false
	}
	mounts := project.Mounts()
	shardIndex, shardOK := mounts.Index(shard)
	if !shardOK || uint64(shardIndex) >= uint64(^uint32(0)) {
		return StorageTransfer{}, false
	}
	canonicalShard, canonicalOK := mounts.At(shardIndex)
	linkID := schema.source.ContentID()
	if !canonicalOK || canonicalShard != shard || !linkID.Available() {
		return StorageTransfer{}, false
	}
	if !schema.storageTransferOccurrenceExecutable(shard, kind, occurrence, uint32(position)) {
		return StorageTransfer{}, false
	}
	ref := StorageTransferRef{
		linkID:       linkID,
		shard:        shard,
		shardOrdinal: uint32(shardIndex + 1),
		kind:         kind,
		term:         occurrence,
		position:     uint32(position),
	}
	ordinal := schema.storageTransferOrdinals[ref]
	if ordinal == 0 {
		return StorageTransfer{}, false
	}
	// Keep the canonical map ordinal in its native unsigned domain: converting
	// through int would make this exact inverse architecture-dependent near the
	// dense uint32 ceiling.
	transfer := StorageTransfer{schema: schema, index: ordinal - 1}
	return transfer, transfer.valid()
}

// FindStorageTransfer rebinds only the canonical occurrence tuple to this
// schema. Equal-content Links replay through their own Value coordinates;
// foreign content fails before an ordinal is observed.
func (schema *Schema) FindStorageTransfer(ref StorageTransferRef) (StorageTransfer, bool) {
	if schema == nil || schema.source == nil || !ref.valid() || ref.linkID != schema.source.ContentID() {
		return StorageTransfer{}, false
	}
	project := schema.source.Project()
	if project == nil {
		return StorageTransfer{}, false
	}
	mounts := project.Mounts()
	// Project.Shard is intentionally owner-fenced and therefore cannot be a
	// replay-map key across independently sealed equal-content Projects.  The
	// persisted relation uses the canonical mount ordinal; reissue this
	// schema's exact Shard and match only the semantic tuple.
	if uint64(ref.shardOrdinal) > uint64(mounts.Count()) {
		return StorageTransfer{}, false
	}
	if localIndex, localOK := mounts.Index(ref.shard); localOK && uint64(localIndex)+1 != uint64(ref.shardOrdinal) {
		return StorageTransfer{}, false
	}
	shard, shardOK := mounts.At(int(ref.shardOrdinal - 1))
	if !shardOK {
		return StorageTransfer{}, false
	}
	if !schema.storageTransferOccurrenceExecutable(shard, ref.kind, ref.term, ref.position) {
		return StorageTransfer{}, false
	}
	localRef := ref
	localRef.shard = shard
	ordinal := schema.storageTransferOrdinals[localRef]
	if ordinal == 0 {
		return StorageTransfer{}, false
	}
	transfer := StorageTransfer{schema: schema, index: ordinal - 1}
	return transfer, transfer.valid()
}

func (ref StorageTransferRef) valid() bool {
	if !ref.linkID.Available() || ref.shard == (linkproject.Shard{}) || ref.shardOrdinal == 0 || ref.term == 0 || !ref.kind.valid() {
		return false
	}
	switch ref.kind {
	case storageTransferRead:
		return keyspace.TermFamily(ref.term) == keyspace.FamilyRead && ref.position == 0
	case storageTransferBind:
		return keyspace.TermFamily(ref.term) == keyspace.FamilyBind
	case storageTransferWrite:
		return keyspace.TermFamily(ref.term) == keyspace.FamilyWrite && ref.position == 0
	default:
		return false
	}
}

func (transfer StorageTransfer) valid() bool {
	if transfer.schema == nil || transfer.schema.source == nil || transfer.schema.storageTransferOrdinals == nil || uint64(transfer.index) >= uint64(len(transfer.schema.storageTransfers)) {
		return false
	}
	row := transfer.schema.storageTransfers[transfer.index]
	if !row.ref.valid() || !row.id.Available() || !row.from.Valid() || !row.to.Valid() {
		return false
	}
	project := transfer.schema.source.Project()
	if project == nil {
		return false
	}
	shardIndex, shardOK := project.Mounts().Index(row.ref.shard)
	if !shardOK || uint64(shardIndex)+1 != uint64(row.ref.shardOrdinal) ||
		!transfer.schema.storageTransferOccurrenceExecutable(row.ref.shard, row.ref.kind, row.ref.term, row.ref.position) {
		return false
	}
	ordinal := transfer.schema.storageTransferOrdinals[row.ref]
	return uint64(ordinal) == uint64(transfer.index)+1 && row.ref.linkID == transfer.schema.source.ContentID()
}

// OwnsStorageTransfer is the complete Value owner fence for this operand.
func (schema *Schema) OwnsStorageTransfer(transfer StorageTransfer) bool {
	return schema != nil && transfer.schema == schema && transfer.valid()
}

// Ref returns the sole replay identity for the exact relation.
func (transfer StorageTransfer) Ref() (StorageTransferRef, bool) {
	if !transfer.valid() {
		return StorageTransferRef{}, false
	}
	return transfer.schema.storageTransfers[transfer.index].ref, true
}

// ID returns Value's fixed-width content identity for the canonical Program
// occurrence tuple. It is a pure domain-owned digest, not a Link row ID.
func (transfer StorageTransfer) ID() (keyspace.ContentID, bool) {
	if !transfer.valid() {
		return keyspace.ContentID{}, false
	}
	return transfer.schema.storageTransfers[transfer.index].id, true
}

// Occurrence returns the executable Program occurrence that owns this fixed
// transfer.  The transfer's Value-coordinate endpoints stay owned by Value,
// while control routing remains entirely in Flow; no from/to route is copied
// out of the sealed relation.
func (transfer StorageTransfer) Occurrence() (linkproject.Shard, keyspace.Term, bool) {
	if !transfer.valid() {
		return linkproject.Shard{}, 0, false
	}
	ref := transfer.schema.storageTransfers[transfer.index].ref
	return ref.shard, ref.term, true
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

// storageTransferOccurrenceExecutable authenticates the tuple against the
// immutable Program source before the ordinal map is observed. The sealed map
// remains the only relation index; these constant-time checks merely reject a
// malformed, dead, or width-inconsistent occurrence supplied by a caller (or
// left behind by a forged in-package row).
func (schema *Schema) storageTransferOccurrenceExecutable(shard linkproject.Shard, kind storageTransferKind, occurrence keyspace.Term, position uint32) bool {
	if schema == nil || schema.source == nil || !kind.valid() || occurrence == 0 {
		return false
	}
	project := schema.source.Project()
	if project == nil {
		return false
	}
	p, ok := project.Mounts().Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(occurrence) {
		return false
	}
	storage := p.Flow().Authored().Storage()
	cells := storage.Cells()
	switch kind {
	case storageTransferRead:
		if keyspace.TermFamily(occurrence) != keyspace.FamilyRead || position != 0 {
			return false
		}
		_, cell, _, related := storage.Reads().Get(occurrence)
		if !related {
			return false
		}
		_, _, _, storageCell := cells.Get(cell)
		return storageCell
	case storageTransferBind:
		if keyspace.TermFamily(occurrence) != keyspace.FamilyBind {
			return false
		}
		owner, valuePack, related := storage.Binds().Get(occurrence)
		if !related || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			keyspace.TermFamily(valuePack) != keyspace.FamilyValues || keyspace.TermOrdinal(valuePack) == 0 {
			return false
		}
		width, sized := p.Source().Binds().Len(occurrence)
		if !sized || uint64(position) >= uint64(width) {
			return false
		}
		cell, bound := p.Source().Binds().At(occurrence, int(position))
		_, fixed := p.Flow().Authored().Values().Member(valuePack, int(position))
		if !bound || !fixed {
			return false
		}
		_, _, _, storageCell := cells.Get(cell)
		return storageCell
	case storageTransferWrite:
		if keyspace.TermFamily(occurrence) != keyspace.FamilyWrite || position != 0 {
			return false
		}
		assign, target, related := storage.Writes().Get(occurrence)
		if !related || !p.Flow().Executable().Contains(assign) {
			return false
		}
		_, _, _, storageCell := cells.Get(target)
		return storageCell
	default:
		return false
	}
}

func (schema *Schema) sealStorageTransfers() bool {
	if schema == nil || schema.source == nil || schema.storageTransfers != nil || schema.storageTransferOrdinals == nil {
		return false
	}
	for index := 0; index < schema.source.Project().Mounts().Count(); index++ {
		shard, shardOK := schema.source.Project().Mounts().At(index)
		p, programOK := schema.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil || !schema.sealReadTransfers(shard, p) || !schema.sealBindTransfers(shard, p) || !schema.sealWriteTransfers(shard, p) {
			return false
		}
	}
	return true
}

func (schema *Schema) sealReadTransfers(shard linkproject.Shard, p *program.Program) bool {
	reads := p.Flow().Authored().Storage().Reads()
	cells := p.Flow().Authored().Storage().Cells()
	for index := 0; index < reads.Count(); index++ {
		read, present := reads.At(index)
		_, cell, _, related := reads.Get(read)
		if !present || !related {
			return false
		}
		if !p.Flow().Executable().Contains(read) {
			continue
		}
		if _, _, _, storage := cells.Get(cell); !storage {
			continue
		}
		if !schema.addStorageTransfer(shard, storageTransferRead, read, 0, cell, read) {
			return false
		}
	}
	return true
}

func (schema *Schema) sealBindTransfers(shard linkproject.Shard, p *program.Program) bool {
	storage := p.Flow().Authored().Storage()
	binds := storage.Binds()
	values := p.Flow().Authored().Values()
	cells := storage.Cells()
	bindCells := p.Source().Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, present := binds.At(index)
		owner, valuePack, related := binds.Get(bind)
		width, sized := bindCells.Len(bind)
		if !present || !related || !sized || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 {
			return false
		}
		if !p.Flow().Executable().Contains(bind) {
			continue
		}
		for position := 0; position < width; position++ {
			cell, bound := bindCells.At(bind, position)
			value, fixed := values.Member(valuePack, position)
			if !bound {
				return false
			}
			if _, _, _, storage := cells.Get(cell); !storage {
				continue
			}
			if !fixed {
				continue
			}
			if !schema.addStorageTransfer(shard, storageTransferBind, bind, uint32(position), value, cell) {
				return false
			}
		}
	}
	return true
}

func (schema *Schema) sealWriteTransfers(shard linkproject.Shard, p *program.Program) bool {
	storage := p.Flow().Authored().Storage()
	writes := storage.Writes()
	assigns := storage.Assigns()
	valuePacks := p.Flow().Authored().Values()
	cells := storage.Cells()
	seenWrites := 0
	for index := 0; index < assigns.Count(); index++ {
		assign, present := assigns.At(index)
		owner, valuePack, assignOK := assigns.Get(assign)
		count, countOK := assigns.WriteCount(assign)
		expectedAssign := keyspace.MakeTerm(keyspace.FamilyAssign, uint32(index+1))
		if !present || assign != expectedAssign || !assignOK || !countOK || count <= 0 ||
			keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			keyspace.TermFamily(valuePack) != keyspace.FamilyValues || keyspace.TermOrdinal(valuePack) == 0 {
			return false
		}
		executable := p.Flow().Executable().Contains(assign)
		for position := 0; position < count; position++ {
			write, writeOK := assigns.WriteAt(assign, position)
			expectedWrite, expectedWriteOK := writes.At(seenWrites)
			writeAssign, cell, related := writes.Get(write)
			if !writeOK || !expectedWriteOK || write != expectedWrite || !related || writeAssign != assign {
				return false
			}
			seenWrites++
			if !executable {
				continue
			}
			if _, _, _, storage := cells.Get(cell); !storage {
				continue
			}
			value, fixed := valuePacks.Member(valuePack, position)
			if !fixed {
				continue
			}
			if !schema.addStorageTransfer(shard, storageTransferWrite, write, 0, value, cell) {
				return false
			}
		}
	}
	if seenWrites != writes.Count() {
		return false
	}
	return true
}

func (schema *Schema) addStorageTransfer(shard linkproject.Shard, kind storageTransferKind, term keyspace.Term, position uint32, fromTerm, toTerm keyspace.Term) bool {
	if schema == nil || schema.source == nil || schema.storageTransferOrdinals == nil || shard == (linkproject.Shard{}) || !kind.valid() || term == 0 || fromTerm == 0 || toTerm == 0 {
		return false
	}
	fromValue, fromValueOK := schema.source.Boundary().Values().Of(shard, fromTerm)
	toValue, toValueOK := schema.source.Boundary().Values().Of(shard, toTerm)
	from, fromOK := schema.CoordinateFor(fromValue)
	to, toOK := schema.CoordinateFor(toValue)
	shardIndex, shardOK := schema.source.Project().Mounts().Index(shard)
	ref := StorageTransferRef{linkID: schema.source.ContentID(), shard: shard, shardOrdinal: uint32(shardIndex + 1), kind: kind, term: term, position: position}
	id := storageTransferIdentity(ref)
	if !shardOK || !fromValueOK || !toValueOK || !fromOK || !toOK || !ref.valid() || !id.Available() || !schema.storageTransferOccurrenceExecutable(shard, kind, term, position) || schema.storageTransferOrdinals[ref] != 0 || uint64(len(schema.storageTransfers)) >= uint64(^uint32(0)) {
		return false
	}
	schema.storageTransfers = append(schema.storageTransfers, storageTransferRow{ref: ref, id: id, from: from, to: to})
	schema.storageTransferOrdinals[ref] = uint32(len(schema.storageTransfers))
	return true
}

func storageTransferIdentity(ref StorageTransferRef) keyspace.ContentID {
	if !ref.valid() {
		return keyspace.ContentID{}
	}
	var payload [32 + 6*8]byte
	copy(payload[:32], ref.linkID[:])
	words := payload[32:]
	binary.BigEndian.PutUint64(words[0:8], 0x76616c2d73746f72) // "val-stor"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(ref.shardOrdinal))
	binary.BigEndian.PutUint64(words[24:32], uint64(ref.kind))
	binary.BigEndian.PutUint64(words[32:40], uint64(ref.term))
	binary.BigEndian.PutUint64(words[40:48], uint64(ref.position))
	return sha256.Sum256(payload[:])
}
