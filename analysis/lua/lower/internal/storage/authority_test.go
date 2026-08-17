package storage

import "testing"

func TestStorageAuthorityStartsWithEmptyOwnedRanges(t *testing.T) {
	writer := New(nil, nil, nil, nil, nil, nil, nil, "storage.lua")
	if writer == nil || !writer.Clean() {
		t.Fatal("new storage writer was not clean")
	}
	if mark := writer.TargetMark(); mark.owner != writer || mark.target != 0 {
		t.Fatalf("initial TargetMark = %#v", mark)
	}
}
