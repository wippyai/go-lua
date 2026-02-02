package returns

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestJoinReturnVectorsPreferNonSoft_Empty(t *testing.T) {
	result := JoinReturnVectorsPreferNonSoft(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestJoinReturnVectorsPreferNonSoft_AEmpty(t *testing.T) {
	b := []typ.Type{typ.String}
	result := JoinReturnVectorsPreferNonSoft(nil, b)
	if len(result) != 1 || result[0] != typ.String {
		t.Errorf("expected [string], got %v", result)
	}
}

func TestJoinReturnVectorsPreferNonSoft_BEmpty(t *testing.T) {
	a := []typ.Type{typ.Number}
	result := JoinReturnVectorsPreferNonSoft(a, nil)
	if len(result) != 1 || result[0] != typ.Number {
		t.Errorf("expected [number], got %v", result)
	}
}

func TestJoinReturnVectorsPreferNonSoft_SameLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	result := JoinReturnVectorsPreferNonSoft(a, b)
	if len(result) != 1 {
		t.Errorf("expected length 1, got %d", len(result))
	}
}

func TestJoinReturnVectorsPreferNonSoft_DifferentLengths(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.Boolean}
	result := JoinReturnVectorsPreferNonSoft(a, b)
	if len(result) != 2 {
		t.Errorf("expected length 2, got %d", len(result))
	}
}

func TestReturnTypesEqual_Empty(t *testing.T) {
	if !ReturnTypesEqual(nil, nil) {
		t.Error("nil slices should be equal")
	}
}

func TestReturnTypesEqual_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String, typ.Number}
	if ReturnTypesEqual(a, b) {
		t.Error("different lengths should not be equal")
	}
}

func TestReturnTypesEqual_Same(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String, typ.Number}
	if !ReturnTypesEqual(a, b) {
		t.Error("same types should be equal")
	}
}

func TestReturnTypesEqual_Different(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	if ReturnTypesEqual(a, b) {
		t.Error("different types should not be equal")
	}
}

func TestReturnTypesRefine_EmptyA(t *testing.T) {
	b := []typ.Type{typ.String}
	if ReturnTypesRefine(nil, b) {
		t.Error("empty a should not refine b")
	}
}

func TestReturnTypesRefine_EmptyB(t *testing.T) {
	a := []typ.Type{typ.String}
	if !ReturnTypesRefine(a, nil) {
		t.Error("a should refine empty b")
	}
}

func TestReturnTypesRefine_Same(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if !ReturnTypesRefine(a, b) {
		t.Error("same types should refine")
	}
}

func TestReturnTypesRefine_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String}
	if ReturnTypesRefine(a, b) {
		t.Error("different lengths should not refine")
	}
}

func TestReturnTypesExtendRecord_Empty(t *testing.T) {
	if ReturnTypesExtendRecord(nil, nil) {
		t.Error("empty vectors should not extend")
	}
}

func TestReturnTypesExtendRecord_NotRecords(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if ReturnTypesExtendRecord(a, b) {
		t.Error("non-records should not extend")
	}
}

func TestReturnTypesExtendRecord_RecordExtends(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with more fields should extend")
	}
}

func TestReturnTypesElideOptional_Empty(t *testing.T) {
	if ReturnTypesElideOptional(nil, nil) {
		t.Error("empty vectors should not elide")
	}
}

func TestTypeExtendsRecord_NilTypes(t *testing.T) {
	if TypeExtendsRecord(nil, typ.String) {
		t.Error("nil a should not extend")
	}
	if TypeExtendsRecord(typ.String, nil) {
		t.Error("nil b should not extend")
	}
}

func TestTypeExtendsRecord_NotRecord(t *testing.T) {
	if TypeExtendsRecord(typ.String, typ.String) {
		t.Error("non-record should not extend")
	}
}

func TestNormalizeReturnVector_Empty(t *testing.T) {
	result := NormalizeReturnVector(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestNormalizeReturnVector_ReplacesNil(t *testing.T) {
	input := []typ.Type{typ.String, nil, typ.Number}
	result := NormalizeReturnVector(input)
	if len(result) != 3 {
		t.Fatalf("expected length 3, got %d", len(result))
	}
	if result[0] != typ.String {
		t.Error("first element should be string")
	}
	if result[1] != typ.Nil {
		t.Error("nil should be replaced with typ.Nil")
	}
	if result[2] != typ.Number {
		t.Error("third element should be number")
	}
}

// Regression tests for recordSuperset map component handling.

func TestRecordSuperset_BothHaveMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Field("x", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with same map component and additional fields should extend")
	}
}

func TestRecordSuperset_OldHasNoMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with additional fields should extend record without map component")
	}
}

func TestRecordSuperset_NewHasMapComponentOldDoesNot(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).MapComponent(typ.String, typ.Any).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with additional map component should extend record without it")
	}
}

func TestRecordSuperset_IncompatibleMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.Number, typ.String).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if ReturnTypesExtendRecord(a, b) {
		t.Error("record with incompatible map component should not extend")
	}
}

// Regression: recordSuperset should use && not || for map component check.
// This test verifies the fix by checking that the code uses HasMapComponent semantics.
func TestTypeExtendsRecord_MapComponentConsistency(t *testing.T) {
	// When old has map component, new must have compatible map component
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Build()
	if TypeExtendsRecord(newRec, oldRec) {
		t.Error("record without map component should not extend record with map component")
	}
}
