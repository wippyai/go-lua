package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/internal/canonical"
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
// carrier during schema binding.
// Ref is an opaque exact-key capability issued by a sealed Factor. Its key
// remains zero-based even though later equation rows use one-based locals.
// The sealed Factor row is carried directly beside that dense key; no second
// provenance record is minted for the route.
//
// Ref is only a cold identity capability. It is not a Program handle, an
// equation coordinate, or a runtime binding handle.
type Ref[K ~uint32 | ~uint64] struct {
	row schemaFactorBinding
	raw K
	_   [0]func()
}

type exactRef interface {
	factorRow() schemaFactorBinding
	rawAddress() uint64
}

func (ref Ref[K]) factorRow() schemaFactorBinding { return ref.row }
func (ref Ref[K]) rawAddress() uint64             { return uint64(ref.raw) }

const (
	summaryVectorDigestDomain         = "analysis/engine/summary-vector"
	summaryVectorDigestVersion uint64 = 2
)

// summaryVectorDigest is the immutable identity digest for a canonical key
// vector. Its preimage is framed under its own domain and records the key
// width and the vector length ahead of the keys, so a vector of a different
// key type, of a different length, or from another identity space can never
// reach the same digest.
func summaryVectorDigest[K ~uint32 | ~uint64](keys []K) [32]byte {
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

// OrderedCells is the typed, read-only Factor observation handed to an E
// callback. D can name it in a form but cannot construct one. A materialized
// projector owns a record; a synchronous fold may instead borrow the exact
// generation-fenced view issued by the Factor binding.
type OrderedCells[V any] struct {
	record *orderedCellsRecord[V]
	view   orderedCellsView[V]
}

// orderedCellsView is the private borrowed half of OrderedCells. The issuing
// owner decides its lifetime; Count and At fail closed after that generation
// ends. Keeping the view private prevents a domain from constructing or
// extending an observation lifetime.
type orderedCellsView[V any] interface {
	Count() int
	At(int) (V, bool, bool)
}

// Count reports the exact finite typed observation width while its Product or
// Query frame is live. A revoked observation reports zero rather than leaking
// stale abstract values into a later transfer or proof check.
func (cells OrderedCells[V]) Count() int {
	if cells.record != nil {
		if !cells.record.live.Load() {
			return 0
		}
		return len(cells.record.cells)
	}
	if cells.view == nil {
		return 0
	}
	return cells.view.Count()
}

// At returns one exact typed observation cell and its presence bit. It is a
// read-only snapshot capability: callers cannot mutate the underlying Factor
// root and a revoked Product/Query frame rejects the read.
func (cells OrderedCells[V]) At(index int) (V, bool, bool) {
	var zero V
	if cells.record != nil {
		if !cells.record.live.Load() || index < 0 || index >= len(cells.record.cells) {
			return zero, false, false
		}
		cell := cells.record.cells[index]
		return cell.value, cell.present, true
	}
	if cells.view == nil {
		return zero, false, false
	}
	return cells.view.At(index)
}

// Value returns one exact typed observation without its presence bit. It is
// the accessor a read declaring ReadSparseFactorDefault is delivered through:
// the engine has already substituted the Factor's declared default at every
// unwritten coordinate, so there is no absent case left for a Fold to branch
// on. A read that must tell a written Bottom from an unwritten coordinate
// declares ReadSparseExplicit and reads At instead.
func (cells OrderedCells[V]) Value(index int) (V, bool) {
	value, present, available := cells.At(index)
	return value, available && present
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

// newOrderedCellsRecord adopts cells: every caller builds the slice locally
// and hands it over, so the record owns it without a copy and revoke may zero
// it in place.
func newOrderedCellsRecord[V any](cells []summaryCell[V]) *orderedCellsRecord[V] {
	record := &orderedCellsRecord[V]{cells: cells}
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
