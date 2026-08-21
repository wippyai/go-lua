package factor

import "github.com/wippyai/go-lua/analysis/identity"

// MountedPublicationBatchIndex is Effect's sealed directory of publication
// batches.  It retains one batch for every mounted call, including calls whose
// publication row set is empty.  The batch itself remains the canonical
// Effect denominator; this type only indexes already-issued batches and does
// not derive any Placement, Heap, or runtime consequence.
//
// The maps are deliberately built at seal time.  Hot consumers authenticate
// their operand against the owner's sealed scalar and these maps; they do not
// replay batch Rows or the full batch Valid proof on every lookup.
type MountedPublicationBatchIndex struct {
	owner *Algebra

	batches  []MountedPublicationBatch
	byID     map[identity.ContentID]uint32
	byScalar map[uint64]uint32
	byCall   map[mountedPublicationBatchCallKey]uint32
	sealed   bool
}

type mountedPublicationBatchCallKey struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

// NewMountedPublicationBatchIndex enumerates every cold-issued mounted call
// in the Effect algebra and seals its complete publication batch.  A malformed
// batch, duplicate sealed identity, scalar collision, or duplicate
// module/occurrence coordinate rejects the whole index; no row is dropped.
func NewMountedPublicationBatchIndex(owner *Algebra) (*MountedPublicationBatchIndex, bool) {
	if owner == nil || !owner.Valid() {
		return nil, false
	}
	count := owner.MountedCallCount()
	index := &MountedPublicationBatchIndex{
		owner:    owner,
		batches:  make([]MountedPublicationBatch, 0, count),
		byID:     make(map[identity.ContentID]uint32, count),
		byScalar: make(map[uint64]uint32, count),
		byCall:   make(map[mountedPublicationBatchCallKey]uint32, count),
		sealed:   true,
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		mounted, mountedOK := owner.MountedCallAt(ordinal)
		if !mountedOK {
			return nil, false
		}
		batch, batchOK := owner.PublicationBatchForMountedCall(mounted)
		if !batchOK || !batch.Valid() {
			return nil, false
		}
		id, idOK := batch.SealedContentID()
		module, occurrence, provenanceOK := batch.CallProvenance()
		batchMounted, batchMountedOK := batch.MountedCall()
		if !idOK || !module.Available() || !occurrence.Available() || !provenanceOK || !batchMountedOK || batchMounted != mounted {
			return nil, false
		}
		scalar := mountedPublicationBatchScalar(id)
		if scalar == 0 {
			return nil, false
		}
		callKey := mountedPublicationBatchCallKey{module: module, occurrence: occurrence}
		if _, duplicate := index.byID[id]; duplicate {
			return nil, false
		}
		if _, duplicate := index.byScalar[scalar]; duplicate {
			// A scalar collision would make the O(1) hot lookup ambiguous even
			// when the full 256-bit sealed IDs differ.
			return nil, false
		}
		if _, duplicate := index.byCall[callKey]; duplicate {
			return nil, false
		}
		index.batches = append(index.batches, batch)
		slot := uint32(len(index.batches))
		index.byID[id] = slot
		index.byScalar[scalar] = slot
		index.byCall[callKey] = slot
	}
	return index, index.Valid()
}

// ready is intentionally O(1).  It is used by hot accessors instead of
// Valid, whose structural audit is reserved for seal-time/law checks.
func (index *MountedPublicationBatchIndex) ready() bool {
	return index != nil && index.sealed && index.owner != nil && index.owner.Valid() &&
		len(index.batches) == len(index.byID) && len(index.batches) == len(index.byScalar) &&
		len(index.batches) == len(index.byCall)
}

// Valid reports that the sealed index retains one authenticated batch per
// mounted call and that every sealed lookup key resolves to its retained
// batch.  The constructor performs the expensive full batch audits; this
// method verifies the immutable index structure without walking batch rows.
func (index *MountedPublicationBatchIndex) Valid() bool {
	if !index.ready() || len(index.batches) != index.owner.MountedCallCount() {
		return false
	}
	for ordinal, batch := range index.batches {
		if !batch.available() {
			return false
		}
		id := batch.id
		scalar := batch.sealedScalar
		module, occurrence := batch.module, batch.occurrence
		slot := uint32(ordinal + 1)
		if index.byID[id] != slot || index.byScalar[scalar] != slot || index.byCall[mountedPublicationBatchCallKey{module: module, occurrence: occurrence}] != slot {
			return false
		}
	}
	return true
}

// Count returns the number of mounted-call publication batches, including
// zero-row batches.
func (index *MountedPublicationBatchIndex) Count() int {
	if !index.ready() {
		return 0
	}
	return len(index.batches)
}

// BatchAt returns the sealed batch at canonical mounted-call ordinal.  It is
// an O(1) sealed access and does not replay the batch's structural proof.
func (index *MountedPublicationBatchIndex) BatchAt(ordinal int) (MountedPublicationBatch, bool) {
	if !index.ready() || ordinal < 0 || ordinal >= len(index.batches) {
		return MountedPublicationBatch{}, false
	}
	batch := index.batches[ordinal]
	return batch, batch.available()
}

// BatchForContentID performs an O(1) sealed-ID lookup.  The scalar index is
// only a fast bucket; the full ID comparison closes its collision boundary.
func (index *MountedPublicationBatchIndex) BatchForContentID(id identity.ContentID) (MountedPublicationBatch, bool) {
	if !index.ready() || !id.Available() {
		return MountedPublicationBatch{}, false
	}
	scalar := mountedPublicationBatchScalar(id)
	slot, found := index.byScalar[scalar]
	if !found || slot == 0 || int(slot) > len(index.batches) {
		return MountedPublicationBatch{}, false
	}
	batch := index.batches[slot-1]
	return batch, batch.available() && batch.id == id
}

// BatchForCall performs an O(1) lookup by the exact mounted module and call
// occurrence coordinates retained by Effect.
func (index *MountedPublicationBatchIndex) BatchForCall(module, occurrence identity.ContentID) (MountedPublicationBatch, bool) {
	if !index.ready() || !module.Available() || !occurrence.Available() {
		return MountedPublicationBatch{}, false
	}
	slot, found := index.byCall[mountedPublicationBatchCallKey{module: module, occurrence: occurrence}]
	if !found || slot == 0 || int(slot) > len(index.batches) {
		return MountedPublicationBatch{}, false
	}
	batch := index.batches[slot-1]
	return batch, batch.available() && batch.module == module && batch.occurrence == occurrence
}

// BatchForMountedCall is the owner-fenced convenience form for consumers
// that already hold Effect's opaque mounted-call receipt.
func (index *MountedPublicationBatchIndex) BatchForMountedCall(mounted MountedCall) (MountedPublicationBatch, bool) {
	if !index.ready() || mounted.owner != index.owner || !mounted.Valid() {
		return MountedPublicationBatch{}, false
	}
	_, module, occurrence, ok := index.owner.MountedCallIdentity(mounted)
	if !ok {
		return MountedPublicationBatch{}, false
	}
	return index.BatchForCall(module, occurrence)
}

// Owns reports whether batch is the exact sealed batch issued by this index's
// Effect owner.  It uses the retained scalar and IDs only; no row replay is
// needed for the ownership check.
func (index *MountedPublicationBatchIndex) Owns(batch MountedPublicationBatch) bool {
	if !index.ready() || batch.owner != index.owner || !batch.available() {
		return false
	}
	slot, found := index.byScalar[batch.sealedScalar]
	if !found || slot == 0 || int(slot) > len(index.batches) {
		return false
	}
	owned := index.batches[slot-1]
	return owned.id == batch.id && owned.sealedScalar == batch.sealedScalar && owned.owner == batch.owner
}
