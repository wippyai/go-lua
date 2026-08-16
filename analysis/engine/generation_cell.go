package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
)

// generationCell is one lock-free live-stamp cell. It holds exactly one
// Generation: the stamp of the execution that currently owns the cell, or the
// unavailable zero once that execution is revoked. Readers compare stamps; they
// never inspect a raw counter, so a revoked or superseded holder fails closed.
type generationCell struct{ value atomic.Uint64 }

// live returns the stamp this cell currently admits.
func (cell *generationCell) live() identity.Generation {
	return identity.Generation(cell.value.Load())
}

// holds reports whether generation is the stamp this cell currently admits. An
// unavailable stamp is admitted by nothing, including a revoked cell.
func (cell *generationCell) holds(generation identity.Generation) bool {
	return generation.Available() && cell.live() == generation
}

// open installs the stamp of a starting execution.
func (cell *generationCell) open(generation identity.Generation) {
	cell.value.Store(uint64(generation))
}

// claim installs generation only while the cell is unavailable, so exactly one
// caller at a time may open a nested token inside an already open execution.
func (cell *generationCell) claim(generation identity.Generation) bool {
	return generation.Available() && cell.value.CompareAndSwap(0, uint64(generation))
}

// advance issues the next stamp of this cell and installs it as the live one.
// It is the form used where the cell is its own sequence: only the newest
// stamp is ever admitted, and an escaped older one fails closed.
func (cell *generationCell) advance() (identity.Generation, bool) {
	generation := identity.Generation(cell.value.Add(1))
	return generation, generation.Available()
}

// revoke retires exactly the named stamp, leaving the cell unavailable. It
// reports whether this call performed the retirement.
func (cell *generationCell) revoke(generation identity.Generation) bool {
	return generation.Available() && cell.value.CompareAndSwap(uint64(generation), 0)
}

// idSequence issues the append-only numbers of one identity space. Issue never
// reuses a live number: a saturated sequence fails closed instead of wrapping
// to a number some retained holder still names.
type idSequence[T ~uint64] struct{ value atomic.Uint64 }

func (sequence *idSequence[T]) issue() (T, bool) {
	issued := T(sequence.value.Add(1))
	return issued, issued != 0
}

// generationSequence issues the stamps of one store.
type generationSequence = idSequence[identity.Generation]
