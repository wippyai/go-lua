package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func stateLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-state-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func stateLawFrozen(t *testing.T) (snapshot.Frozen, identity.ContentID) {
	t.Helper()
	catalog := stateLawID(t, "catalog")
	builder := snapshot.NewFrozen(catalog, identity.StoreID(1))
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal empty frozen: %v", err)
	}
	return frozen, catalog
}

// A sealed publication under its owner-issued catalog is the only input that
// can become a readable state. State stores the Frozen handle, not a second
// row or catalog representation.
func TestNewAcceptsMatchingSealedCatalog(t *testing.T) {
	frozen, catalog := stateLawFrozen(t)
	got, ok := New(frozen, catalog)
	if !ok || !got.Available() {
		t.Fatal("matching sealed catalog was rejected")
	}
	if got.CatalogID() != catalog {
		t.Fatalf("catalog = %v, want %v", got.CatalogID(), catalog)
	}
	if published := got.Frozen().Published(); !published {
		t.Fatal("state lost its sealed publication")
	}
	if schema := got.Frozen().Schema(); schema != catalog {
		t.Fatalf("frozen schema = %v, want %v", schema, catalog)
	}
}

// An expected catalog is a fence, not a label: a Frozen sealed under another
// catalog cannot be reinterpreted as this program's publication.
func TestNewRejectsForeignCatalog(t *testing.T) {
	frozen, catalog := stateLawFrozen(t)
	foreign := stateLawID(t, "foreign-catalog")
	if foreign == catalog {
		t.Fatal("foreign catalog collided with source catalog")
	}
	if _, ok := New(frozen, foreign); ok {
		t.Fatal("foreign catalog authenticated a Frozen")
	}
	if _, ok := New(frozen, identity.ContentID{}); ok {
		t.Fatal("unavailable catalog authenticated a Frozen")
	}
}

// An unpublished or zero value never exposes a publication or catalog
// identity to a child reader.
func TestInvalidStateFailsClosed(t *testing.T) {
	var zero State
	if zero.Available() {
		t.Fatal("zero state is available")
	}
	if zero.CatalogID().Available() {
		t.Fatal("invalid state exposed a catalog")
	}
	if zero.Frozen().Published() {
		t.Fatal("invalid state exposed a publication")
	}

	catalog := stateLawID(t, "unpublished-catalog")
	if _, ok := New(snapshot.Frozen{}, catalog); ok {
		t.Fatal("unpublished Frozen authenticated")
	}
}

// State copies share the immutable publication and retain no mutable input
// surface. A child can read through either copy, while a foreign axis remains
// rejected by snapshot's schema fence.
func TestStateCopiesPreserveTheFrozenFence(t *testing.T) {
	frozen, catalog := stateLawFrozen(t)
	first, ok := New(frozen, catalog)
	if !ok {
		t.Fatal("state")
	}
	second := first
	if !second.Available() || second.CatalogID() != first.CatalogID() {
		t.Fatal("state copy changed its authenticated catalog")
	}

	firstFrozen := first.Frozen()
	secondFrozen := second.Frozen()
	foreign := stateLawID(t, "foreign-axis")
	axis := snapshot.Axis[uint32, int]{SchemaID: foreign, Slot: 0}
	if _, status := snapshot.ReadFrozen(&firstFrozen, axis, 0); status != snapshot.ReadInvalid {
		t.Fatalf("foreign axis through first state returned %v", status)
	}
	if _, status := snapshot.ReadFrozen(&secondFrozen, axis, 0); status != snapshot.ReadInvalid {
		t.Fatalf("foreign axis through copied state returned %v", status)
	}
}
