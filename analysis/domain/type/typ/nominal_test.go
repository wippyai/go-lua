package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
)

func TestInterface(t *testing.T) {
	methods := []Method{
		{Name: "read", Type: Func().Returns(String).Build()},
	}
	i := NewInterface("Reader", methods)

	if i.Kind() != kind.Interface {
		t.Errorf("Kind: got %v, want Interface", i.Kind())
	}

	if i.Name != "Reader" {
		t.Errorf("Name: got %q, want %q", i.Name, "Reader")
	}

	if len(i.Methods) != 1 {
		t.Errorf("Methods: got %d, want 1", len(i.Methods))
	}
}

func TestInterfaceString(t *testing.T) {
	// Named interface renders as just the name
	methods := []Method{
		{Name: "read", Type: Func().Returns(String).Build()},
		{Name: "write", Type: Func().Param("data", String).Build()},
	}
	i := NewInterface("IO", methods)

	s := i.String()
	if s != "IO" {
		t.Errorf("String: got %q, want %q", s, "IO")
	}

	// Anonymous interface expands methods
	anon := NewInterface("", methods)
	anonStr := anon.String()
	if anonStr != "interface { read: fun() -> string; write: fun(data: string) }" {
		t.Errorf("Anonymous String: got %q", anonStr)
	}
}

func TestInterfaceEquality(t *testing.T) {
	m1 := []Method{{Name: "foo", Type: Func().Build()}}
	m2 := []Method{{Name: "foo", Type: Func().Build()}}
	m3 := []Method{{Name: "bar", Type: Func().Build()}}

	i1 := NewInterface("I", m1)
	i2 := NewInterface("I", m2)
	i3 := NewInterface("I", m3)
	i4 := NewInterface("J", m1)

	if !i1.Equals(i2) {
		t.Error("same interfaces should be equal")
	}

	if i1.Equals(i3) {
		t.Error("different methods should not be equal")
	}

	if i1.Equals(i4) {
		t.Error("different names should not be equal")
	}
}

func TestInterfaceNotEqualToPrimitive(t *testing.T) {
	i := NewInterface("I", nil)
	if i.Equals(Number) {
		t.Error("interface should not equal primitive")
	}
}
