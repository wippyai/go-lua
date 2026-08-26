package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Lookup-only coordinates are the mount projection used by a correlated
// Apply child.  The delivered vector may contain more cells than the
// correlation projection; only the explicitly issued projection becomes the
// trie coordinate.  In particular, the denominator/order key is not inferred
// or used as a prefix coordinate.
func TestLookupOnlyCoordinateUsesExactOwnerProjection(t *testing.T) {
	fence, handle, single, singleColumns := coordinateClassIDs(t)
	secondContent, ok := identity.DeriveContentID("arrangement/lookup-coordinate-law/column/v1", []byte("second"))
	if !ok {
		t.Fatal("second column content")
	}
	second, ok := model.IssueColumnID(single.Relation(), secondContent)
	if !ok {
		t.Fatal("second column")
	}
	delivered, ok := NewVectorAccess(single.Relation(), []model.ColumnID{singleColumns[0], second})
	if !ok {
		t.Fatal("delivered vector")
	}
	correlation := []model.ColumnID{singleColumns[0]}
	layout, ok := newLayoutWithClass(fence, handle, delivered, correlation, CoordinateClassLookupOnly)
	if !ok || !layout.Available() {
		t.Fatal("lookup-only layout")
	}
	if layout.Access().Key().Available() {
		t.Fatal("query-site projection fabricated a relation key")
	}
	if got := layout.Columns(); len(got) != 2 || got[0] != singleColumns[0] || got[1] != second {
		t.Fatalf("delivered vector = %v", got)
	}
	if got := layout.KeyColumns(); len(got) != 1 || got[0] != singleColumns[0] {
		t.Fatalf("correlation coordinate = %v", got)
	}
	if layout.CoordinateClass() != CoordinateClassLookupOnly {
		t.Fatalf("coordinate class = %v", layout.CoordinateClass())
	}

	// A different owner-issued projection is a different physical lookup,
	// even though the delivered row and mounted handle are unchanged.
	otherCoordinate := []model.ColumnID{second}
	other, ok := newLayoutWithClass(fence, handle, delivered, otherCoordinate, CoordinateClassLookupOnly)
	if !ok || !other.Available() || other.Digest() == layout.Digest() {
		t.Fatal("lookup coordinate did not participate in layout identity")
	}

	// A projection cannot smuggle a column that was not delivered by the exact
	// mounted vector.  This prevents a later Reader.Lookup from inventing a
	// correlation component or relying on a denominator prefix.
	foreignContent, ok := identity.DeriveContentID("arrangement/lookup-coordinate-law/column/v1", []byte("foreign"))
	if !ok {
		t.Fatal("foreign column content")
	}
	foreign, ok := model.IssueColumnID(single.Relation(), foreignContent)
	if !ok {
		t.Fatal("foreign column")
	}
	if invalid, accepted := newLayoutWithClass(fence, handle, delivered, []model.ColumnID{foreign}, CoordinateClassLookupOnly); accepted || invalid.Available() {
		t.Fatal("undelivered lookup coordinate accepted")
	}
}
