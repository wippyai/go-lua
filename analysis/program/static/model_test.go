package static

import "testing"

func TestStaticModelPrimitiveVocabularyAndRuntimeBoundary(t *testing.T) {
	names := map[string]PrimitiveKind{
		"nil": PrimitiveNil, "boolean": PrimitiveBoolean, "number": PrimitiveNumber,
		"integer": PrimitiveInteger, "string": PrimitiveString, "function": PrimitiveFunction,
		"any": PrimitiveAny, "unknown": PrimitiveUnknown, "never": PrimitiveNever, "self": PrimitiveSelf,
	}
	for name, want := range names {
		got, ok := PrimitiveKindForName(name)
		if !ok || got != want || !got.valid() {
			t.Fatalf("PrimitiveKindForName(%q) = %v/%v, want %v/true", name, got, ok, want)
		}
	}
	if _, ok := PrimitiveKindForName("user-defined"); ok {
		t.Fatal("open primitive spelling entered the closed model vocabulary")
	}
	if PrimitiveFunction.RuntimeLoadable() || PrimitiveSelf.RuntimeLoadable() {
		t.Fatal("static-only primitive entered the runtime-loadable subset")
	}
	if !PrimitiveAny.RuntimeLoadable() || !PrimitiveInteger.RuntimeLoadable() {
		t.Fatal("runtime-loadable primitive was excluded from the model subset")
	}
}
