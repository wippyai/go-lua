package flow

import "testing"

func TestCountRowsPublishesCanonicalFlowEntries(t *testing.T) {
	assembly := openProjectionAssembly(t, "flow-counts.lua")
	_, component, _, _, err := assembly.Take()
	if err != nil {
		t.Fatalf("Assembly.Take: %v", err)
	}
	rows, err := CountRows(component.View())
	if err != nil || !rows.Available() || rows.Count() == 0 {
		t.Fatalf("CountRows = %d/%v/%v, want sealed Flow rows", rows.Count(), rows.Available(), err)
	}
	for index := 0; index < rows.Count(); index++ {
		row, ok := rows.At(index)
		if !ok || !row.ID().Available() {
			t.Fatalf("CountRows.At(%d) = %#v/%v, want an available generated identity", index, row, ok)
		}
		if value, ok := rows.Value(row.ID()); !ok || value != row.Count() {
			t.Fatalf("CountRows.Value(%v) = %d/%v, want %d", row.ID(), value, ok, row.Count())
		}
	}
	var unavailable View
	if _, err := CountRows(unavailable); err == nil {
		t.Fatal("CountRows accepted an unavailable Flow View")
	}
}
