// Package valuecore holds the concrete data layout of Effect's Fact type.
//
// It is internal to the effect axis family (domain/effect/...) so its
// constructors stay reachable only from the algebra that authenticates them
// (domain/effect/factor) while the type itself is named at the axis root
// (domain/effect) through a plain alias. valuecore depends on nothing but
// analysis/identity, so naming Value here carries no risk of dragging the
// factor algebra's heavier dependencies (link, pack, contract, static) back
// into the axis root.
package valuecore

import "github.com/wippyai/go-lua/analysis/identity"

// Atom is one opaque, algebra-local effect-template identity. It contains no
// decoded Target, Pack, Boundary, or Static payload. Owner is the algebra
// instance that minted it; validity is decided by that owner alone.
type Atom struct {
	owner any
	root  uint32
	id    identity.ContentID
}

// NewAtom constructs an Atom under owner. Only the effect axis family can
// reach this constructor.
func NewAtom(owner any, root uint32, id identity.ContentID) Atom {
	return Atom{owner: owner, root: root, id: id}
}

func (atom Atom) Owner() any             { return atom.owner }
func (atom Atom) Root() uint32           { return atom.root }
func (atom Atom) ID() identity.ContentID { return atom.id }

// Value is Bottom, an immutable sparse atom set, or Top.
type Value struct {
	owner any
	root  uint32
	top   bool
	atoms []Atom
	seal  uint64
}

// NewValue constructs a Value under owner. Only the effect axis family can
// reach this constructor; seal is the caller's already-computed constructor
// proof and is carried opaquely.
func NewValue(owner any, root uint32, top bool, atoms []Atom, seal uint64) Value {
	return Value{owner: owner, root: root, top: top, atoms: atoms, seal: seal}
}

func (value Value) Owner() any    { return value.owner }
func (value Value) Root() uint32  { return value.root }
func (value Value) IsTop() bool   { return value.top }
func (value Value) Atoms() []Atom { return value.atoms }
func (value Value) Seal() uint64  { return value.seal }
