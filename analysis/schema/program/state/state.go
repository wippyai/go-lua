// Package state owns the immutable read capability for one sealed Program
// publication. It carries the snapshot itself and derives the cold catalog
// identity from that publication; it does not retain Program metadata, rows,
// indexes, or any mutable construction state.
package state

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// State is the immutable cold publication a semantic Program child reads.
//
// The catalog argument accepted by New is validation input only. The sealed
// Frozen already carries the catalog identity in its schema field, so State
// deliberately stores no second catalog field that could disagree with it.
// Copying State copies the Frozen value, which shares the sealed publication
// without copying any rows.
type State struct {
	frozen snapshot.Frozen
}

// New authenticates one sealed cold publication against the catalog derived
// by its owner. An unpublished Frozen, an unavailable expected catalog, or a
// catalog different from Frozen.Schema is not a readable state.
func New(frozen snapshot.Frozen, expectedCatalog identity.ContentID) (State, bool) {
	if !frozen.Published() || !expectedCatalog.Available() || frozen.Schema() != expectedCatalog {
		return State{}, false
	}
	return State{frozen: frozen}, true
}

// Available reports whether this value is an authenticated sealed
// publication.
func (state State) Available() bool {
	return state.frozen.Published() && state.frozen.Schema().Available()
}

// Frozen returns the immutable publication carried by this state. Invalid
// state fails closed with the zero Frozen value.
func (state State) Frozen() snapshot.Frozen {
	if !state.Available() {
		return snapshot.Frozen{}
	}
	return state.frozen
}

// CatalogID returns the cold catalog identity sealed into Frozen. It is read
// from the publication rather than retained as a second authority.
func (state State) CatalogID() identity.ContentID {
	if !state.Available() {
		return identity.ContentID{}
	}
	return state.frozen.Schema()
}
