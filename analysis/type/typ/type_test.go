package typ

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestPrimitives(t *testing.T) {
	primitives := []struct {
		typ  Type
		k    kind.Kind
		name string
	}{
		{Nil, kind.Nil, "nil"},
		{Boolean, kind.Boolean, "boolean"},
		{Number, kind.Number, "number"},
		{Integer, kind.Integer, "integer"},
		{String, kind.String, "string"},
		{Any, kind.Any, "any"},
		{Unknown, kind.Unknown, "unknown"},
		{Never, kind.Never, "never"},
		{Self, kind.Self, "self"},
	}

	for _, tc := range primitives {
		t.Run(tc.name, func(t *testing.T) {
			if tc.typ.Kind() != tc.k {
				t.Errorf("Kind: got %v, want %v", tc.typ.Kind(), tc.k)
			}

			if tc.typ.String() != tc.name {
				t.Errorf("String: got %q, want %q", tc.typ.String(), tc.name)
			}

			if tc.typ.Hash() != uint64(tc.k) {
				t.Errorf("Hash: got %d, want %d", tc.typ.Hash(), tc.k)
			}
		})
	}
}

func TestBuiltinPrimitiveVocabulary(t *testing.T) {
	tests := []struct {
		name string
		want Type
	}{
		{name: "nil", want: Nil},
		{name: "boolean", want: Boolean},
		{name: "number", want: Number},
		{name: "integer", want: Integer},
		{name: "string", want: String},
		{name: "function", want: Func().Build()},
		{name: "any", want: Any},
		{name: "unknown", want: Unknown},
		{name: "never", want: Never},
		{name: "self", want: Self},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !BuiltinPrimitiveName(tc.name) {
				t.Fatalf("BuiltinPrimitiveName(%q) = false, want true", tc.name)
			}
			got, ok := BuiltinPrimitiveType(tc.name)
			if !ok {
				t.Fatalf("BuiltinPrimitiveType(%q) returned ok=false", tc.name)
			}
			if !TypeEquals(got, tc.want) {
				t.Fatalf("BuiltinPrimitiveType(%q) = %#v, want %#v", tc.name, got, tc.want)
			}
		})
	}
}

func TestBuiltinPrimitiveRejectsNonBuiltinNames(t *testing.T) {
	for _, name := range []string{"", "User", "booleanish", "Number", "nil?", "selfish"} {
		t.Run(name, func(t *testing.T) {
			if BuiltinPrimitiveName(name) {
				t.Fatalf("BuiltinPrimitiveName(%q) = true, want false", name)
			}
			if got, ok := BuiltinPrimitiveType(name); ok || got != nil {
				t.Fatalf("BuiltinPrimitiveType(%q) = %#v/%v, want nil/false", name, got, ok)
			}
		})
	}
}

func TestBuiltinTableTopMarker(t *testing.T) {
	got := BuiltinTableTopMarker()
	if !IsBuiltinTableTopMarker(got) {
		t.Fatalf("IsBuiltinTableTopMarker(BuiltinTableTopMarker()) = false, want true")
	}
	if IsBuiltinTableTopMarker(NewInterface(BuiltinTableTopName, []Method{{Name: "open", Type: Func().Build()}})) {
		t.Fatalf("non-empty %q interface reported as builtin table top marker", BuiltinTableTopName)
	}
	if IsBuiltinTableTopMarker(NewInterface("not-table", nil)) {
		t.Fatal("unrelated empty interface reported as builtin table top marker")
	}
}

func TestPrimitiveEquality(t *testing.T) {
	if !Nil.Equals(Nil) {
		t.Error("Nil should equal Nil")
	}

	if Nil.Equals(Boolean) {
		t.Error("Nil should not equal Boolean")
	}

	if !Boolean.Equals(Boolean) {
		t.Error("Boolean should equal Boolean")
	}

	if Boolean.Equals(Number) {
		t.Error("Boolean should not equal Number")
	}

	if !Number.Equals(Number) {
		t.Error("Number should equal Number")
	}

	if Number.Equals(Integer) {
		t.Error("Number should not equal Integer")
	}

	if !Integer.Equals(Integer) {
		t.Error("Integer should equal Integer")
	}

	if !String.Equals(String) {
		t.Error("String should equal String")
	}

	if !Any.Equals(Any) {
		t.Error("Any should equal Any")
	}

	if !Unknown.Equals(Unknown) {
		t.Error("Unknown should equal Unknown")
	}

	if !Never.Equals(Never) {
		t.Error("Never should equal Never")
	}

	if !Self.Equals(Self) {
		t.Error("Self should equal Self")
	}
}

func TestPrimitivesAreSingletons(t *testing.T) {
	for _, name := range []string{"nil", "boolean", "number", "integer", "string", "any", "unknown", "never", "self"} {
		first, ok := BuiltinPrimitiveType(name)
		if !ok {
			t.Fatalf("BuiltinPrimitiveType(%q) returned ok=false", name)
		}
		second, ok := BuiltinPrimitiveType(name)
		if !ok {
			t.Fatalf("second BuiltinPrimitiveType(%q) returned ok=false", name)
		}
		if reflect.ValueOf(first).Kind() != reflect.Ptr {
			t.Fatalf("BuiltinPrimitiveType(%q) = %T, want pointer-backed singleton", name, first)
		}
		if first != second {
			t.Fatalf("BuiltinPrimitiveType(%q) returned distinct singleton pointers: %p and %p", name, first, second)
		}
	}
	if Nil == Boolean {
		t.Fatal("different primitive singletons Nil and Boolean share a pointer")
	}
	if Number == String {
		t.Fatal("different primitive singletons Number and String share a pointer")
	}
}

func TestPrimitiveHashUniqueness(t *testing.T) {
	primitives := []Type{Nil, Boolean, Number, Integer, String, Any, Unknown, Never, Self}
	hashes := make(map[uint64]Type)

	for _, p := range primitives {
		h := p.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s both have hash %d", existing.String(), p.String(), h)
		}

		hashes[h] = p
	}
}
