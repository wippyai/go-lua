package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func coordinateClassIDs(t *testing.T) (address.Fence, Handle, Access, []model.ColumnID) {
	t.Helper()
	ownerContent, ok := identity.DeriveContentID("arrangement/coordinate-class-law/owner/v1", []byte("owner"))
	if !ok {
		t.Fatal("owner content")
	}
	owner, ok := model.IssueOwnerID(ownerContent)
	if !ok {
		t.Fatal("owner")
	}
	schemaContent, ok := identity.DeriveContentID("arrangement/coordinate-class-law/schema/v1", []byte("schema"))
	if !ok {
		t.Fatal("schema content")
	}
	schema, ok := model.IssueSchemaID(owner, schemaContent)
	if !ok {
		t.Fatal("schema")
	}
	relationContent, ok := identity.DeriveContentID("arrangement/coordinate-class-law/relation/v1", []byte("relation"))
	if !ok {
		t.Fatal("relation content")
	}
	relation, ok := model.IssueRelationID(owner, relationContent)
	if !ok {
		t.Fatal("relation")
	}
	columnContent, ok := identity.DeriveContentID("arrangement/coordinate-class-law/column/v1", []byte("column"))
	if !ok {
		t.Fatal("column content")
	}
	column, ok := model.IssueColumnID(relation, columnContent)
	if !ok {
		t.Fatal("column")
	}
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	certificateDigest, ok := identity.DeriveContentID("arrangement/coordinate-class-law/certificate/v1", []byte("certificate"))
	if !ok {
		t.Fatal("certificate digest")
	}
	fence, ok := address.NewFence(schema, certificateDigest, storeID, identity.MountID{0: 1}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	handle, ok := NewHandle(fence, 1)
	if !ok {
		t.Fatal("handle")
	}
	access, ok := NewVectorAccess(relation, []model.ColumnID{column})
	if !ok {
		t.Fatal("access")
	}
	return fence, handle, access, []model.ColumnID{column}
}

func TestPhysicalVectorRefusesMixedCoordinateClasses(t *testing.T) {
	_, _, access, columns := coordinateClassIDs(t)
	state := deriveState{}
	if !state.addPhysicalVector(access, columns, CoordinateClassLookupOnly) {
		t.Fatal("initial mutable correspondence was refused")
	}
	if state.addPhysicalVector(access, columns, CoordinateClassStableCorrespondence) {
		t.Fatal("mixed mutable/stable correspondence was silently upgraded")
	}
	_, class, ok := state.physicalCoordinate(access)
	if !ok || class != CoordinateClassLookupOnly {
		t.Fatalf("mixed correspondence changed class to %v", class)
	}
}

func TestLayoutCoordinateClassIsDigestSensitive(t *testing.T) {
	fence, handle, access, columns := coordinateClassIDs(t)
	if layout, ok := newLayout(fence, handle, access, columns); ok || layout.Available() {
		t.Fatal("unkeyed physical coordinate was accepted without a sealed class")
	}
	stable, ok := newLayoutWithClass(fence, handle, access, columns, CoordinateClassStableCorrespondence)
	if !ok || !stable.Available() {
		t.Fatal("stable correspondence layout")
	}
	lookup, ok := newLayoutWithClass(fence, handle, access, columns, CoordinateClassLookupOnly)
	if !ok || !lookup.Available() {
		t.Fatal("lookup-only correspondence layout")
	}
	if stable.Digest() == lookup.Digest() || stable.Equal(lookup) {
		t.Fatal("coordinate ownership class did not affect layout identity")
	}
}
