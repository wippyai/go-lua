package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Lattice states one owner's ascent mathematics for T. It is named exactly
// once per TypeID by a generated witness that spells the owner's own API, so
// this layer never guesses a method name and never carries a function value.
type Lattice[T any] interface {
	Join(left, right T) (T, bool)
	Widen(previous, next T) (T, bool)
	LessOrEq(left, right T) bool
}

// Column is the thin typed owner-column publisher: one TypeID, one solve-local
// store, and the only place a domain value crosses into a binding.ValueToken.
type Column[T any] struct {
	typeID model.TypeID
	store  *Store[T]
}

// NewColumn binds one owner-issued TypeID to its solve-local store.
func NewColumn[T any](typeID model.TypeID, store *Store[T]) (*Column[T], bool) {
	if !typeID.Available() || !store.Available() {
		return nil, false
	}
	return &Column[T]{typeID: typeID, store: store}, true
}

// Available reports whether the column carries a type and live storage.
func (column *Column[T]) Available() bool {
	return column != nil && column.typeID.Available() && column.store.Available()
}

// Type returns the owner-issued semantic type this column publishes.
func (column *Column[T]) Type() model.TypeID {
	if column == nil {
		return model.TypeID{}
	}
	return column.typeID
}

// Decode borrows the domain value behind token. A token of another type or
// another store refuses.
func (column *Column[T]) Decode(token binding.ValueToken) (T, bool) {
	var zero T
	if !column.Available() || !token.Available() || token.Type() != column.typeID {
		return zero, false
	}
	return column.store.Load(token.Opaque())
}

// Encode interns value and issues its runtime-fenced token.
func (column *Column[T]) Encode(issuer binding.Issuer, value T) (binding.ValueToken, bool) {
	if !column.Available() || !issuer.Available() {
		return binding.ValueToken{}, false
	}
	handle, ok := column.store.Intern(value)
	if !ok {
		return binding.ValueToken{}, false
	}
	return issuer.IssueValue(column.typeID, handle)
}

// Algebra resolves one TypeID's ascent authority independently of any
// operation signature, as the semantic ABI requires. L is instantiated with
// the owner's own lattice witness, so Join, Widen and LessOrEqual are static
// calls over concrete values: the payload is never boxed.
type Algebra[T any, L Lattice[T]] struct {
	column  *Column[T]
	issuer  binding.Issuer
	lattice L
}

// NewAlgebra binds one column's codec to its owner lattice.
func NewAlgebra[T any, L Lattice[T]](column *Column[T], issuer binding.Issuer, lattice L) (*Algebra[T, L], bool) {
	if !column.Available() || !issuer.Available() {
		return nil, false
	}
	return &Algebra[T, L]{column: column, issuer: issuer, lattice: lattice}, true
}

// Type returns the TypeID this algebra is the sole ascent authority for.
func (algebra *Algebra[T, L]) Type() model.TypeID {
	if algebra == nil {
		return model.TypeID{}
	}
	return algebra.column.Type()
}

// Join proves ordinary ascent over two authenticated values.
func (algebra *Algebra[T, L]) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	leftValue, rightValue, ok := algebra.pair(left, right)
	if !ok {
		return binding.ValueToken{}, false
	}
	joined, ok := algebra.lattice.Join(leftValue, rightValue)
	if !ok {
		return binding.ValueToken{}, false
	}
	return algebra.column.Encode(algebra.issuer, joined)
}

// Widen is invoked only at a certified recurrence head.
func (algebra *Algebra[T, L]) Widen(previous, next binding.ValueToken) (binding.ValueToken, bool) {
	previousValue, nextValue, ok := algebra.pair(previous, next)
	if !ok {
		return binding.ValueToken{}, false
	}
	widened, ok := algebra.lattice.Widen(previousValue, nextValue)
	if !ok {
		return binding.ValueToken{}, false
	}
	return algebra.column.Encode(algebra.issuer, widened)
}

// LessOrEqual is the ascent order the state layer validates a proposal with.
func (algebra *Algebra[T, L]) LessOrEqual(left, right binding.ValueToken) bool {
	leftValue, rightValue, ok := algebra.pair(left, right)
	if !ok {
		return false
	}
	return algebra.lattice.LessOrEq(leftValue, rightValue)
}

func (algebra *Algebra[T, L]) pair(left, right binding.ValueToken) (T, T, bool) {
	var zero T
	if algebra == nil || !algebra.column.Available() {
		return zero, zero, false
	}
	leftValue, ok := algebra.column.Decode(left)
	if !ok {
		return zero, zero, false
	}
	rightValue, ok := algebra.column.Decode(right)
	if !ok {
		return zero, zero, false
	}
	return leftValue, rightValue, true
}
