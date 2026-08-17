package storage

import "testing"

func TestTableWriterRequiresAllTypedOwners(t *testing.T) {
	writer := NewTable(nil, nil, nil, nil, "storage.lua")
	if err := writer.Schedule(nil, 1, writer.span(nil)); err == nil {
		t.Fatal("TableWriter accepted an incomplete authority")
	}
}
