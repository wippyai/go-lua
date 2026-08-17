package program

import "testing"

func TestStorageIdentityQueriesKeepAbsentRowsOutOfTheSnapshot(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-storage-identity-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if id, ok := published.StorageCellIDAt(-1); ok || id.Available() {
		t.Fatalf("StorageCellIDAt(-1) = %x/%v; want unavailable", id, ok)
	}
	if read, span, term, ok := published.StorageReadIDAt(0); ok || read.Available() || span.Available() || term != 0 {
		t.Fatalf("StorageReadIDAt(0) = %x/%x/%v/%v; want absent row", read, span, term, ok)
	}
	if id, ok := published.StorageBindIDAt(0); ok || id.Available() {
		t.Fatalf("StorageBindIDAt(0) = %x/%v; want absent row", id, ok)
	}
	if id, ok := published.StorageBindTransferIDAt(0, 0); ok || id.Available() {
		t.Fatalf("StorageBindTransferIDAt(0,0) = %x/%v; want absent row", id, ok)
	}
	if id, ok := published.StorageAssignmentIDAt(0); ok || id.Available() {
		t.Fatalf("StorageAssignmentIDAt(0) = %x/%v; want absent row", id, ok)
	}
	if id, route, ok := published.AssignmentPredecessorID(0); ok || id.Available() || route.Available() {
		t.Fatalf("AssignmentPredecessorID(0) = %x/%x/%v; want absent row", id, route, ok)
	}
	if id, ok := published.StorageWriteTransferIDAt(0, 0); ok || id.Available() {
		t.Fatalf("StorageWriteTransferIDAt(0,0) = %x/%v; want absent row", id, ok)
	}
}
