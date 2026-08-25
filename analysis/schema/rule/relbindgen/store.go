package relbindgen

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// storeTagWidth is the prefix of an opaque handle that names the issuing
// store. The next eight bytes carry the store epoch and the last eight carry
// the one-based slot.
const storeTagWidth = 16

const storeEpochOffset = storeTagWidth

const storeSlotOffset = storeTagWidth + 8

// Store is the solve-local arena for one owner payload type.
//
// Go requires exactly one boundary between a heterogeneous domain payload and
// generic runtime storage, and this is it. T stays concrete, so a domain value
// is never boxed into an interface and never copied through reflection, while
// the engine carries only the opaque handle inside a binding.ValueToken.
//
// A handle is runtime data fenced to one store and one epoch. It is never a
// logical identity: rows, columns and types are owner-issued elsewhere.
type Store[T any] struct {
	tag    [storeTagWidth]byte
	epoch  uint64
	values []T
}

// NewStore adopts one owner-issued token as the store fence and reserves
// capacity for reserve values. Interning within that reservation does not
// allocate.
func NewStore[T any](tag identity.ContentID, reserve int) (*Store[T], bool) {
	if !tag.Available() || reserve < 0 {
		return nil, false
	}
	fence := [storeTagWidth]byte(tag[:storeTagWidth])
	if fence == ([storeTagWidth]byte{}) {
		return nil, false
	}
	return &Store[T]{tag: fence, epoch: 1, values: make([]T, 0, reserve)}, true
}

// Available reports whether the store carries a fence and live storage.
func (store *Store[T]) Available() bool {
	return store != nil && store.tag != [storeTagWidth]byte{} && store.epoch != 0 && store.values != nil
}

// Len returns the number of values interned in the current epoch.
func (store *Store[T]) Len() int {
	if !store.Available() {
		return 0
	}
	return len(store.values)
}

// Intern adopts one value and returns its opaque handle.
func (store *Store[T]) Intern(value T) (identity.ContentID, bool) {
	if !store.Available() {
		return identity.ContentID{}, false
	}
	store.values = append(store.values, value)
	return store.handle(uint64(len(store.values))), true
}

// Load borrows the value behind handle. A foreign store, a stale epoch and an
// out-of-range slot each refuse.
func (store *Store[T]) Load(handle identity.ContentID) (T, bool) {
	var zero T
	if !store.Available() {
		return zero, false
	}
	if [storeTagWidth]byte(handle[:storeTagWidth]) != store.tag {
		return zero, false
	}
	if binary.BigEndian.Uint64(handle[storeEpochOffset:storeSlotOffset]) != store.epoch {
		return zero, false
	}
	slot := binary.BigEndian.Uint64(handle[storeSlotOffset:])
	if slot == 0 || slot > uint64(len(store.values)) {
		return zero, false
	}
	return store.values[slot-1], true
}

// Reset opens the next epoch. Every handle issued before it refuses, the
// interned values are released to the collector, and the reservation is
// retained for reuse.
func (store *Store[T]) Reset() bool {
	if !store.Available() {
		return false
	}
	var zero T
	for index := range store.values {
		store.values[index] = zero
	}
	store.values = store.values[:0]
	store.epoch++
	if store.epoch == 0 {
		store.epoch = 1
	}
	return true
}

func (store *Store[T]) handle(slot uint64) identity.ContentID {
	var handle identity.ContentID
	copy(handle[:storeTagWidth], store.tag[:])
	binary.BigEndian.PutUint64(handle[storeEpochOffset:storeSlotOffset], store.epoch)
	binary.BigEndian.PutUint64(handle[storeSlotOffset:], slot)
	return handle
}
