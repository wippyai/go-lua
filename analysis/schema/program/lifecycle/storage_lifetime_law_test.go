package lifecycle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
)

func storageLifetimeLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestStorageLifetimeRowsKeepUnknownDistinctFromInvalid(t *testing.T) {
	id := storageLifetimeLawID(1)
	unknown, ok := NewStorageCellLifetime(id, StorageLifetimeUnknown)
	if !ok || !unknown.Available() || unknown.ID() != id || unknown.Lifetime() != StorageLifetimeUnknown {
		t.Fatal("unknown storage lifetime was not preserved as a valid row")
	}
	invalid, ok := NewStorageCellLifetime(id, StorageLifetimeInvalid)
	if ok || invalid.Available() || invalid.ID().Available() || invalid.Lifetime() != StorageLifetimeInvalid {
		t.Fatal("invalid storage lifetime was admitted or exposed as a proof")
	}
	if StorageLifetimeInvalid.Valid() || StorageLifetimeUnknown.Valid() == false {
		t.Fatal("storage lifetime validity boundary moved")
	}
	closure, ok := NewStorageCellLifetime(id, StorageLifetimeClosure)
	if !ok || !closure.Available() || closure.Lifetime() != StorageLifetimeClosure || closure.Lifetime().String() != "closure" {
		t.Fatal("closure storage lifetime was not preserved as a valid row")
	}
	if closure.Lifetime() == StorageLifetimeModule {
		t.Fatal("closure storage lifetime collapsed into module ownership")
	}
}

func TestStorageLifetimeFamilyRejectsIncompleteRows(t *testing.T) {
	catalog, ok := programcatalog.CatalogID(storageLifetimeLawID(31))
	if !ok {
		t.Fatal("storage lifetime law catalog")
	}
	row, ok := NewStorageCellLifetime(storageLifetimeLawID(32), StorageLifetimeModule)
	if !ok {
		t.Fatal("storage lifetime law row")
	}
	if _, ok := StorageCellLifetimeFamily().Content([]StorageCellLifetime{row, StorageCellLifetime{}}, catalog); ok {
		t.Fatal("storage lifetime family admitted an unavailable row")
	}
}
