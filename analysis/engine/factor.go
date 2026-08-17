package engine

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// Measure is a key-aware, well-founded transition witness for a Factor's
// recurrence operation. It is semantic input, not a work budget. Wave D
// retains the law but does not attach it to a carrier or scheduler.
type Measure[K ~uint32 | ~uint64, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

// Factor is a typed cold owner capability. K and V remain available to its
// owner and the later template binder, but they never enter a mixed engine
// carrier during receipt schema binding.
// Ref is an opaque, sealed exact-key capability issued by a Factor. Its key
// remains zero-based even though later private equation binding may choose a
// different representation. It records canonical sealed identities plus one
// private live seal-authority ticket, has no public constructor or inspection
// surface, and the zero-sized function member deliberately makes it
// uncomparable.
//
// Ref is only a cold identity capability. It is not a Program handle, an
// equation coordinate, or a runtime binding handle.
type Ref[K ~uint32 | ~uint64] struct {
	compositionID    CompositionID
	bindingAuthority *schemaBindingAuthority
	factorKey        composition.Key
	factorIndex      uint64
	raw              K
	_                [0]func()
}

// ClosedRefs is one Factor-issued, seal-once vector of exact Ref
// capabilities. It deliberately exposes append/close rather than raw
// coordinates: callers with an owner-private K can construct it by type
// inference, while only Assembly later reads its canonical Ref vector.
type ClosedRefs[K ~uint32 | ~uint64] struct {
	receipt factorRuntimeReceipt
	digest  [32]byte
	refs    []Ref[K]
	closed  bool
}

func (refs *ClosedRefs[K]) validIssuer() bool {
	return refs != nil && refs.receipt.valid()
}

const (
	summaryVectorDigestDomain         = "analysis/engine/summary-vector"
	summaryVectorDigestVersion uint64 = 2
)

// SummaryVectorDigest is the immutable evidence digest for a canonical key
// vector. Its preimage is framed under its own domain and records the key
// width and the vector length ahead of the keys, so a vector of a different
// key type, of a different length, or from another identity space can never
// reach the same digest.
func SummaryVectorDigest[K ~uint32 | ~uint64](keys []K) [32]byte {
	digest, ok := framedDigest(summaryVectorDigestDomain, summaryVectorDigestVersion, func(writer *canonical.DigestWriter) bool {
		if writer.Uint(summaryKeyWidth[K]()) != nil || writer.Count(uint64(len(keys))) != nil {
			return false
		}
		for _, key := range keys {
			if writer.Uint(uint64(key)) != nil {
				return false
			}
		}
		return true
	})
	if !ok {
		return [32]byte{}
	}
	return digest
}

// summaryKeyWidth reports the bit width of the vector's key type. It reads the
// width from the type itself, so no caller can present a narrow vector as a
// wide one.
func summaryKeyWidth[K ~uint32 | ~uint64]() uint64 {
	if uint64(^K(0)) == uint64(^uint32(0)) {
		return 32
	}
	return 64
}

// Append records one exact Ref from this vector's sole issuing Factor. It is
// legal only before Close; duplicates are rejected before they can alter the
// canonical summary set.
func (refs *ClosedRefs[K]) Append(ref Ref[K]) bool {
	if refs == nil || refs.closed || !refs.receipt.valid() || !validateRefForReceipt(refs.receipt, ref) {
		return false
	}
	for _, present := range refs.refs {
		if present.raw == ref.raw {
			return false
		}
	}
	refs.refs = append(refs.refs, ref)
	return true
}

// Close fixes one immutable canonical Ref order. It is intentionally
// idempotence-free: a vector has one admission episode and any second close
// is rejected rather than treated as a parallel construction path.
func (refs *ClosedRefs[K]) Close() bool {
	if refs == nil || refs.closed || !refs.receipt.valid() || len(refs.refs) == 0 {
		return false
	}
	for _, ref := range refs.refs {
		if !validateRefForReceipt(refs.receipt, ref) {
			return false
		}
	}
	sort.Slice(refs.refs, func(left, right int) bool { return refs.refs[left].raw < refs.refs[right].raw })
	for index := 1; index < len(refs.refs); index++ {
		if refs.refs[index-1].raw >= refs.refs[index].raw {
			return false
		}
	}
	keys := make([]uint64, len(refs.refs))
	for index, ref := range refs.refs {
		keys[index] = uint64(ref.raw)
	}
	digest := SummaryVectorDigest(keys)
	if digest == ([32]byte{}) {
		return false
	}
	refs.digest = digest
	refs.closed = true
	return true
}

// OrderedCells is the typed, read-only Factor observation handed to an E
// callback. D can name it in a form but cannot construct one.
type OrderedCells[V any] struct{ record *orderedCellsRecord[V] }

// Count reports the exact finite typed observation width while its Product or
// Query frame is live. A revoked observation reports zero rather than leaking
// stale abstract values into a later transfer or proof check.
func (cells OrderedCells[V]) Count() int {
	if cells.record == nil || !cells.record.live.Load() {
		return 0
	}
	return len(cells.record.cells)
}

// At returns one exact typed observation cell and its presence bit. It is a
// read-only snapshot capability: callers cannot mutate the underlying Factor
// root and a revoked Product/Query frame rejects the read.
func (cells OrderedCells[V]) At(index int) (V, bool, bool) {
	var zero V
	if cells.record == nil || !cells.record.live.Load() || index < 0 || index >= len(cells.record.cells) {
		return zero, false, false
	}
	cell := cells.record.cells[index]
	return cell.value, cell.present, true
}

// summaryCell is private runtime observation storage. Its fields stay hidden
// from domain declarations; an OrderedCells value is valid only while its
// synchronous Product or Query frame remains active.
type summaryCell[V any] struct {
	value   V
	present bool
}

type orderedCellsRecord[V any] struct {
	cells []summaryCell[V]
	live  atomic.Bool
}

func equalOrderedCellRecords[V any](left, right *orderedCellsRecord[V], equal func(V, V) bool) bool {
	if left == nil || right == nil || equal == nil || !left.live.Load() || !right.live.Load() || len(left.cells) != len(right.cells) {
		return false
	}
	for index := range left.cells {
		if left.cells[index].present != right.cells[index].present || left.cells[index].present && !equal(left.cells[index].value, right.cells[index].value) {
			return false
		}
	}
	return true
}

func fingerprintOrderedCellRecord[V any](record *orderedCellsRecord[V], fingerprint func(V) uint64) uint64 {
	if record == nil || fingerprint == nil || !record.live.Load() {
		return 0
	}
	result := uint64(len(record.cells)) ^ 0x9e3779b97f4a7c15
	for _, cell := range record.cells {
		value := uint64(0x517cc1b727220a95)
		if cell.present {
			value = fingerprint(cell.value) ^ 0x94d049bb133111eb
		}
		result ^= value + 0x9e3779b97f4a7c15 + result<<6 + result>>2
	}
	return result
}

func newOrderedCellsRecord[V any](cells []summaryCell[V]) *orderedCellsRecord[V] {
	record := &orderedCellsRecord[V]{cells: append([]summaryCell[V](nil), cells...)}
	record.live.Store(true)
	return record
}

func (record *orderedCellsRecord[V]) revoke() {
	if record == nil || !record.live.CompareAndSwap(true, false) {
		return
	}
	var zero V
	for index := range record.cells {
		record.cells[index].value = zero
		record.cells[index].present = false
	}
	record.cells = nil
}
