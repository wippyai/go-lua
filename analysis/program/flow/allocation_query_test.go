package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestAllocationsExposeOnlyOwnedExecutableOccurrences(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "allocation-query.lua",
		Text: []byte("local value = {}\nreturn value"),
	})
	if err != nil {
		t.Fatal(err)
	}
	view := program.Flow()
	allocations := view.Allocations()
	if allocations.Count() == 0 {
		t.Fatal("table literal did not produce an executable allocation")
	}
	for index := 0; index < allocations.Count(); index++ {
		allocation, ok := allocations.At(index)
		if !ok || !allocation.Available() || !allocation.Owns(view) {
			t.Fatalf("allocation[%d] = %#v/%v, want an owned sealed occurrence", index, allocation, ok)
		}
		if !allocation.ID().Available() || allocation.FieldCount() == 0 {
			// Empty tables legitimately have no fields; the occurrence itself is
			// still the required semantic proof.
			if !allocation.ID().Available() {
				t.Fatalf("allocation[%d] has no parent-issued identity", index)
			}
			continue
		}
		field, ok := allocation.FieldAt(0)
		if !ok || !field.Available() || !field.BelongsTo(allocation) || !allocations.OwnsField(field) {
			t.Fatalf("allocation[%d] field = %#v/%v, want an owned field", index, field, ok)
		}
	}
}
