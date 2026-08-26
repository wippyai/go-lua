package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// TestArrangementAvailabilityIsSealedAtConstruction pins that repeated
// capability probes redeem the constructor verdict rather than revalidating a
// logical vector. The issued owner remains available without an O(width)
// revalidation.
func TestArrangementAvailabilityIsSealedAtConstruction(t *testing.T) {
	fence, handle, access, columns := coordinateClassIDs(t)
	layout, ok := newLayoutWithClass(fence, handle, access, columns, CoordinateClassStableCorrespondence)
	if !ok || !access.Available() || !layout.Available() {
		t.Fatal("sealed arrangement owner unavailable")
	}

	if allocs := testing.AllocsPerRun(200, func() {
		if !access.Available() || !layout.Available() {
			t.Fatal("sealed arrangement owner became unavailable")
		}
	}); allocs != 0 {
		t.Fatalf("repeated arrangement availability allocated %v per call", allocs)
	}
	if (Access{}).Available() || (Layout{}).Available() {
		t.Fatal("zero arrangement owner available")
	}
}

func sealedCompleteBindingLawValue(t *testing.T) CompleteBinding {
	t.Helper()
	fence, handle, access, columns := coordinateClassIDs(t)
	keyContent, ok := identity.DeriveContentID("arrangement/sealed-availability-law/key/v1", []byte("key"))
	if !ok {
		t.Fatal("key content")
	}
	key, ok := model.IssueKeyID(access.Relation(), keyContent)
	if !ok {
		t.Fatal("key")
	}
	keyAccess, ok := NewKeyAccess(key)
	if !ok {
		t.Fatal("key access")
	}
	keyLayout, ok := newLayoutWithClass(fence, handle, keyAccess, columns, CoordinateClassDeclaredKey)
	if !ok {
		t.Fatal("key layout")
	}
	denominator, ok := model.NewDenominatorRef(access.Relation(), key)
	if !ok {
		t.Fatal("denominator")
	}
	complete, ok := newCompleteBinding(denominator, keyLayout, columns)
	if !ok {
		t.Fatal("complete binding")
	}
	return complete
}

func TestCompleteBindingAvailabilityIsSealedAtConstruction(t *testing.T) {
	binding := sealedCompleteBindingLawValue(t)
	if !binding.Available() || len(binding.Columns()) == 0 {
		t.Fatal("sealed complete binding unavailable")
	}
	if allocs := testing.AllocsPerRun(200, func() {
		if !binding.Available() {
			t.Fatal("sealed complete binding became unavailable")
		}
	}); allocs != 0 {
		t.Fatalf("repeated complete availability allocated %v per call", allocs)
	}
	columns := binding.Columns()
	columns[0] = model.ColumnID{}
	if fresh := binding.Columns(); fresh[0] == (model.ColumnID{}) {
		t.Fatal("complete binding columns were not defensive")
	}
}

func TestArrangementHotScalarAccessorsDoNotAllocate(t *testing.T) {
	fence, handle, access, columns := coordinateClassIDs(t)
	layout, ok := newLayoutWithClass(fence, handle, access, columns, CoordinateClassStableCorrespondence)
	if !ok {
		t.Fatal("layout")
	}
	complete := sealedCompleteBindingLawValue(t)
	if allocs := testing.AllocsPerRun(200, func() {
		_ = access.Relation()
		_ = access.Key()
		_ = layout.Handle()
		_ = layout.KeyWidth()
		_ = layout.CoordinateClass()
		_ = complete.Denominator()
	}); allocs != 0 {
		t.Fatalf("sealed hot accessors allocated %v per call", allocs)
	}
}
