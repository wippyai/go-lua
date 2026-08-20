// Package lifetime owns the lock-free generation and append-only sequence
// primitives shared by engine execution owners.
package lifetime

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Cell is one lock-free live-stamp cell. It holds exactly one Generation: the
// stamp of the execution that currently owns the cell, or the unavailable zero
// once that execution is revoked. Readers compare stamps; they never inspect
// a raw counter, so a revoked or superseded holder fails closed.
type Cell struct{ value atomic.Uint64 }

// live returns the stamp this cell currently admits.
func (cell *Cell) live() identity.Generation {
	return identity.Generation(cell.value.Load())
}

// Holds reports whether generation is the stamp this cell currently admits.
// An unavailable stamp is admitted by nothing, including a revoked cell.
func (cell *Cell) Holds(generation identity.Generation) bool {
	return generation.Available() && cell.live() == generation
}

// Open installs the stamp of a starting execution.
func (cell *Cell) Open(generation identity.Generation) {
	cell.value.Store(uint64(generation))
}

// Claim installs generation only while the cell is unavailable, so exactly
// one caller at a time may open a nested token inside an already open
// execution.
func (cell *Cell) Claim(generation identity.Generation) bool {
	return generation.Available() && cell.value.CompareAndSwap(0, uint64(generation))
}

// Advance issues the next stamp of this cell and installs it as the live one.
// It is the form used where the cell is its own sequence: only the newest
// stamp is ever admitted, and an escaped older one fails closed. Saturation
// leaves the maximum value installed, so later calls cannot wrap and reuse an
// earlier stamp.
func (cell *Cell) Advance() (identity.Generation, bool) {
	for {
		current := cell.value.Load()
		if current == ^uint64(0) {
			return 0, false
		}
		next := current + 1
		if cell.value.CompareAndSwap(current, next) {
			generation := identity.Generation(next)
			return generation, generation.Available()
		}
	}
}

// Revoke retires exactly the named stamp, leaving the cell unavailable. It
// reports whether this call performed the retirement.
func (cell *Cell) Revoke(generation identity.Generation) bool {
	return generation.Available() && cell.value.CompareAndSwap(uint64(generation), 0)
}

// Sequence issues the append-only numbers of one identity space. Issue never
// reuses a live number: a saturated sequence fails closed instead of wrapping
// to a number some retained holder still names. Saturation leaves the maximum
// value installed, so later calls remain refused.
type Sequence[T ~uint64] struct{ value atomic.Uint64 }

// Issue returns the next number in this identity space.
func (sequence *Sequence[T]) Issue() (T, bool) {
	for {
		current := sequence.value.Load()
		if current == ^uint64(0) {
			var zero T
			return zero, false
		}
		next := current + 1
		if sequence.value.CompareAndSwap(current, next) {
			return T(next), true
		}
	}
}

// GenerationSequence issues the stamps of one store.
type GenerationSequence = Sequence[identity.Generation]
