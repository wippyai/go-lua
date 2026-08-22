package contextfiber

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestAvailabilityIsSealedByTheConstructor pins that availability is a verdict
// reached exactly once, where the value can still be malformed: the constructor
// decides it, every copy carries the same answer, and the zero value is never
// available.  Accessor guards therefore read a settled fact instead of
// re-proving construction.
func TestAvailabilityIsSealedByTheConstructor(t *testing.T) {
	fixture := layoutDirectory(t, true, 3)
	owners := mountedOwnersForDirectory(t, fixture.directory, true)
	index, indexOK := New(fixture.directory, len(owners), identity.Generation(31))
	if indexOK != index.Available() {
		t.Fatalf("index verdict constructor=%v available=%v", indexOK, index.Available())
	}
	if !indexOK {
		t.Fatal("sealed index")
	}
	layout, layoutOK := NewLayout(index, fixture.directory, owners, identity.Generation(31))
	if layoutOK != layout.Available() {
		t.Fatalf("layout verdict constructor=%v available=%v", layoutOK, layout.Available())
	}
	if !layoutOK {
		t.Fatal("sealed layout")
	}

	indexCopy, layoutCopy := index, layout
	if !indexCopy.Available() || !layoutCopy.Available() {
		t.Fatal("sealed verdict lost across a copy")
	}
	if indexCopy.ContextCount() != index.ContextCount() || layoutCopy.StateCount() != layout.StateCount() {
		t.Fatal("sealed projection lost across a copy")
	}

	if (Index{}).Available() || (Layout{}).Available() || (StateCell{}).Available() || (PointOwner{}).Available() {
		t.Fatal("zero value available")
	}

	cell, cellOK := layout.StateAt(0)
	if !cellOK || !cell.Available() || !cell.OwnedBy(layout) {
		t.Fatal("sealed state cell")
	}
	cellCopy := cell
	if !cellCopy.Available() || !cellCopy.OwnedBy(layout) {
		t.Fatal("sealed cell verdict lost across a copy")
	}
}

// TestPointOwnerAuthenticationIsIssuedNotRederived pins that only the owner
// constructor issues an authenticated owner.  A struct literal that reproduces
// the derived digest is still not an issued owner, so owner authority cannot be
// manufactured by anyone able to replay the derivation.
func TestPointOwnerAuthenticationIsIssuedNotRederived(t *testing.T) {
	module := fiberLawID(t, "sealed-owner-module")
	issued, issuedOK := Mounted(module)
	if !issuedOK || !issued.Available() {
		t.Fatal("issued owner")
	}
	digest, digestOK := identity.DeriveContentID(pointOwnerDomain, []byte{byte(PointOwnerMounted)}, module[:])
	if !digestOK || digest != issued.ID() {
		t.Fatal("owner derivation")
	}
	replayed := PointOwner{kind: PointOwnerMounted, key: module, id: digest}
	if replayed.Available() || replayed.Mounted() || replayed.ModuleKey().Available() || replayed.ID().Available() {
		t.Fatal("replayed owner digest authenticated an unissued owner")
	}
}
