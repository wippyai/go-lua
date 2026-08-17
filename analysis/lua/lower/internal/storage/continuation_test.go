package storage

import "testing"

func TestStorageRunRequiresPendingContinuation(t *testing.T) {
	var writer Writer
	if err := writer.Run(); err == nil {
		t.Fatal("Run accepted an unavailable storage continuation")
	}
}
