package identity

// StoreID is the identity of one live solver-store instance. It is
// process-local by construction: it distinguishes two stores that exist at
// the same time, and says nothing about two stores in different processes.
//
// A StoreID is never derived from content. Two stores holding identical
// content are still two stores, and an address issued by one is not
// addressable in the other.
type StoreID uint64

// Available reports whether id names a store. The zero StoreID names none.
func (id StoreID) Available() bool { return id != 0 }

// Generation is a store revision fence. A store advances its Generation
// whenever a published slot's meaning can have changed, so a holder of a
// runtime-local address can decide whether that address still means what it
// meant when it was issued.
//
// Generation orders revisions of one store only. Comparing generations of
// different stores is meaningless, which is why Locator carries the StoreID
// alongside it and checks both.
type Generation uint64

// Available reports whether generation names a published revision. The zero
// Generation names none, so an unset fence never passes a validity check.
func (generation Generation) Available() bool { return generation != 0 }

// Next returns the revision following generation. It saturates to the
// unavailable zero at the width limit rather than wrapping, so an exhausted
// store fails closed instead of reusing a live revision number.
func (generation Generation) Next() Generation {
	if generation == ^Generation(0) {
		return 0
	}
	return generation + 1
}

// Precedes reports whether generation is an earlier revision than other.
// Unavailable generations precede nothing and are preceded by nothing.
func (generation Generation) Precedes(other Generation) bool {
	return generation.Available() && other.Available() && generation < other
}

// Locator is a generation-checked runtime coordinate: a slot in one live
// store at one revision of that store. It is an address, not an identity.
// Carrying one across a store boundary is meaningless and carrying one across
// a revision is unsafe, which is what Valid enforces.
//
// Slot is the owner's own coordinate type. This package neither interprets it
// nor constrains what it addresses; it only guarantees that the coordinate is
// checked against the store and revision that issued it.
type Locator[T comparable] struct {
	Store      StoreID
	Generation Generation
	Slot       T
}

// NewLocator issues an address into store at generation. An unavailable store
// or generation yields the zero Locator, which never validates.
func NewLocator[T comparable](store StoreID, generation Generation, slot T) Locator[T] {
	if !store.Available() || !generation.Available() {
		return Locator[T]{}
	}
	return Locator[T]{Store: store, Generation: generation, Slot: slot}
}

// Available reports whether locator carries a store and a revision. It says
// nothing about whether that revision is still current.
func (locator Locator[T]) Available() bool {
	return locator.Store.Available() && locator.Generation.Available()
}

// Valid reports whether locator still addresses store at generation. The
// generation match is exact rather than "not older": a slot's meaning is
// defined only at the revision that issued the address, so a store that has
// advanced has already invalidated every address it handed out.
func (locator Locator[T]) Valid(store StoreID, generation Generation) bool {
	return locator.Available() && locator.Store == store && locator.Generation == generation
}
