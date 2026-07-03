package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestArray(t *testing.T) {
	arr := NewArray(Number)

	if arr.Kind() != kind.Array {
		t.Errorf("Kind: got %v, want Array", arr.Kind())
	}

	if arr.Element != Number {
		t.Error("Element should be Number")
	}

	if arr.String() != "number[]" {
		t.Errorf("String: got %q, want %q", arr.String(), "number[]")
	}
}

func TestArrayNilElementDefaultsToUnknown(t *testing.T) {
	arr := NewArray(nil)

	if arr.Element != Unknown {
		t.Errorf("Element: got %v, want Unknown", arr.Element)
	}
}

func TestArrayEquality(t *testing.T) {
	arr1 := NewArray(Number)
	arr2 := NewArray(Number)
	arr3 := NewArray(String)

	if !arr1.Equals(arr2) {
		t.Error("number[] should equal number[]")
	}

	if arr1.Equals(arr3) {
		t.Error("number[] should not equal string[]")
	}

	if arr1.Hash() != arr2.Hash() {
		t.Error("Equal arrays should have same hash")
	}
}

func TestArrayNested(t *testing.T) {
	inner := NewArray(Number)
	outer := NewArray(inner)

	if outer.String() != "number[][]" {
		t.Errorf("String: got %q, want %q", outer.String(), "number[][]")
	}

	if !outer.Element.Equals(inner) {
		t.Error("Nested element should be inner array")
	}
}

func TestMap(t *testing.T) {
	m := NewMap(String, Number)

	if m.Kind() != kind.Map {
		t.Errorf("Kind: got %v, want Map", m.Kind())
	}

	if m.Key != String {
		t.Error("Key should be String")
	}

	if m.Value != Number {
		t.Error("Value should be Number")
	}

	if m.String() != "{[string]: number}" {
		t.Errorf("String: got %q, want %q", m.String(), "{[string]: number}")
	}
}

func TestMapNilKeyValueDefaultsToUnknown(t *testing.T) {
	m := NewMap(nil, nil)

	if m.Key != Unknown {
		t.Errorf("Key: got %v, want Unknown", m.Key)
	}
	if m.Value != Unknown {
		t.Errorf("Value: got %v, want Unknown", m.Value)
	}
}

func TestRebuildMapNilKeyValueDefaultsToUnknown(t *testing.T) {
	m := RebuildMap(nil, nil)

	if m.Key != Unknown {
		t.Errorf("Key: got %v, want Unknown", m.Key)
	}
	if m.Value != Unknown {
		t.Errorf("Value: got %v, want Unknown", m.Value)
	}
}

func TestRebuildMapMatchesNewMapHash(t *testing.T) {
	rebuilt := RebuildMap(String, Number)
	constructed := NewMap(String, Number)

	if rebuilt.Hash() != constructed.Hash() {
		t.Fatalf("RebuildMap hash = %d, want NewMap hash %d", rebuilt.Hash(), constructed.Hash())
	}
	if !rebuilt.Equals(constructed) {
		t.Fatal("RebuildMap should equal NewMap for identical key/value types")
	}
}

func TestRebuildMapCachesContainsFlagsFromKeyAndValue(t *testing.T) {
	key := NewTypeParam("K", nil)
	value := MaterializeOptional(Any)
	m := RebuildMap(key, value)

	if !m.containsTypeParam {
		t.Fatal("containsTypeParam should include key type")
	}
	if !m.containsAny {
		t.Fatal("containsAny should include value type")
	}
}

func TestMapKeyPreservesNilableKey(t *testing.T) {
	key := MaterializeOptional(String)
	m := NewMap(key, Number)
	if m.Key != key {
		t.Fatalf("map key = %v, want original key", m.Key)
	}
}

func TestReadonlyMapKeyPreservesNilableKey(t *testing.T) {
	key := MaterializeOptional(String)
	m := NewReadonlyMap(key, Number)
	if m.Key != key {
		t.Fatalf("readonly map key = %v, want original key", m.Key)
	}
}

func TestMapEquality(t *testing.T) {
	m1 := NewMap(String, Number)
	m2 := NewMap(String, Number)
	m3 := NewMap(String, String)
	m4 := NewMap(Number, Number)

	if !m1.Equals(m2) {
		t.Error("{[string]: number} should equal {[string]: number}")
	}

	if m1.Equals(m3) {
		t.Error("{[string]: number} should not equal {[string]: string}")
	}

	if m1.Equals(m4) {
		t.Error("{[string]: number} should not equal {[number]: number}")
	}

	if m1.Hash() != m2.Hash() {
		t.Error("Equal maps should have same hash")
	}
}

func TestReadonlyMap(t *testing.T) {
	m := NewReadonlyMap(String, Number)

	if m.Kind() != kind.ReadonlyMap {
		t.Errorf("Kind: got %v, want ReadonlyMap", m.Kind())
	}

	if m.Key != String {
		t.Error("Key should be String")
	}

	if m.Value != Number {
		t.Error("Value should be Number")
	}

	if m.String() != "readonly {[string]: number}" {
		t.Errorf("String: got %q, want %q", m.String(), "readonly {[string]: number}")
	}
}

func TestReadonlyMapNilKeyValueDefaultsToUnknown(t *testing.T) {
	m := NewReadonlyMap(nil, nil)

	if m.Key != Unknown {
		t.Errorf("Key: got %v, want Unknown", m.Key)
	}
	if m.Value != Unknown {
		t.Errorf("Value: got %v, want Unknown", m.Value)
	}
}

func TestRebuildReadonlyMapNilKeyValueDefaultsToUnknown(t *testing.T) {
	m := RebuildReadonlyMap(nil, nil)

	if m.Key != Unknown {
		t.Errorf("Key: got %v, want Unknown", m.Key)
	}
	if m.Value != Unknown {
		t.Errorf("Value: got %v, want Unknown", m.Value)
	}
}

func TestRebuildReadonlyMapMatchesNewReadonlyMapHash(t *testing.T) {
	rebuilt := RebuildReadonlyMap(String, Number)
	constructed := NewReadonlyMap(String, Number)

	if rebuilt.Hash() != constructed.Hash() {
		t.Fatalf("RebuildReadonlyMap hash = %d, want NewReadonlyMap hash %d", rebuilt.Hash(), constructed.Hash())
	}
	if !rebuilt.Equals(constructed) {
		t.Fatal("RebuildReadonlyMap should equal NewReadonlyMap for identical key/value types")
	}
}

func TestRebuildReadonlyMapCachesContainsFlagsFromKeyAndValue(t *testing.T) {
	key := MaterializeOptional(Any)
	value := NewTypeParam("V", nil)
	m := RebuildReadonlyMap(key, value)

	if !m.containsAny {
		t.Fatal("containsAny should include key type")
	}
	if !m.containsTypeParam {
		t.Fatal("containsTypeParam should include value type")
	}
}

func TestTupleEmpty(t *testing.T) {
	tup := NewTuple()

	if tup.Kind() != kind.Tuple {
		t.Errorf("Kind: got %v, want Tuple", tup.Kind())
	}

	if len(tup.Elements) != 0 {
		t.Errorf("Elements: got %d, want 0", len(tup.Elements))
	}

	if tup.String() != "()" {
		t.Errorf("String: got %q, want %q", tup.String(), "()")
	}
}

func TestTupleSingle(t *testing.T) {
	tup := NewTuple(Number)

	if len(tup.Elements) != 1 {
		t.Errorf("Elements: got %d, want 1", len(tup.Elements))
	}

	if tup.String() != "(number)" {
		t.Errorf("String: got %q, want %q", tup.String(), "(number)")
	}
}

func TestTupleMultiple(t *testing.T) {
	tup := NewTuple(Number, String, Boolean)

	if len(tup.Elements) != 3 {
		t.Errorf("Elements: got %d, want 3", len(tup.Elements))
	}

	if tup.String() != "(number, string, boolean)" {
		t.Errorf("String: got %q, want %q", tup.String(), "(number, string, boolean)")
	}
}

func TestTupleNilElementDefaultsToUnknown(t *testing.T) {
	tup := NewTuple(nil, Number)

	if len(tup.Elements) != 2 {
		t.Fatalf("Elements: got %d, want 2", len(tup.Elements))
	}
	if tup.Elements[0] != Unknown {
		t.Errorf("Element[0]: got %v, want Unknown", tup.Elements[0])
	}
}

func TestTupleEquality(t *testing.T) {
	t1 := NewTuple(Number, String)
	t2 := NewTuple(Number, String)
	t3 := NewTuple(String, Number)
	t4 := NewTuple(Number)

	if !t1.Equals(t2) {
		t.Error("(number, string) should equal (number, string)")
	}

	if t1.Equals(t3) {
		t.Error("(number, string) should not equal (string, number)")
	}

	if t1.Equals(t4) {
		t.Error("(number, string) should not equal (number)")
	}

	if t1.Hash() != t2.Hash() {
		t.Error("Equal tuples should have same hash")
	}
}

func TestContainerHashConsistency(t *testing.T) {
	arr1 := NewArray(Number)
	arr2 := NewArray(Number)

	if arr1.Hash() != arr2.Hash() {
		t.Error("identical arrays should have same hash")
	}

	m1 := NewMap(String, Number)
	m2 := NewMap(String, Number)

	if m1.Hash() != m2.Hash() {
		t.Error("identical maps should have same hash")
	}

	t1 := NewTuple(Number, String)
	t2 := NewTuple(Number, String)

	if t1.Hash() != t2.Hash() {
		t.Error("identical tuples should have same hash")
	}
}

func TestContainerNotEqualToPrimitive(t *testing.T) {
	if NewArray(Number).Equals(Number) {
		t.Error("number[] should not equal number")
	}

	if NewMap(String, Number).Equals(String) {
		t.Error("{[string]: number} should not equal string")
	}

	if NewTuple(Number).Equals(Number) {
		t.Error("(number) should not equal number")
	}
}
