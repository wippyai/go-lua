package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestFunctionEmpty(t *testing.T) {
	f := Func().Build()

	if f.Kind() != kind.Function {
		t.Errorf("Kind: got %v, want Function", f.Kind())
	}

	if len(f.Params) != 0 {
		t.Errorf("Params: got %d, want 0", len(f.Params))
	}

	if len(f.Returns) != 0 {
		t.Errorf("Returns: got %d, want 0", len(f.Returns))
	}

	if f.String() != "fun()" {
		t.Errorf("String: got %q, want %q", f.String(), "fun()")
	}
}

func TestFunctionWithParams(t *testing.T) {
	f := Func().
		Param("x", Number).
		Param("y", String).
		Build()

	if len(f.Params) != 2 {
		t.Errorf("Params: got %d, want 2", len(f.Params))
	}

	if f.Params[0].Name != "x" || f.Params[0].Type != Number {
		t.Error("first param should be x: number")
	}

	if f.Params[1].Name != "y" || f.Params[1].Type != String {
		t.Error("second param should be y: string")
	}

	if f.String() != "fun(x: number, y: string)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithOptParam(t *testing.T) {
	f := Func().
		Param("x", Number).
		OptParam("y", String).
		Build()

	if !f.Params[1].Optional {
		t.Error("second param should be optional")
	}

	if f.String() != "fun(x: number, y: string?)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithVariadic(t *testing.T) {
	f := Func().
		Param("x", Number).
		Variadic(String).
		Build()

	if f.Variadic != String {
		t.Error("Variadic should be String")
	}

	if f.String() != "fun(x: number, ...string)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionWithReturns(t *testing.T) {
	f := Func().
		Param("x", Number).
		Returns(Boolean).
		Build()

	if len(f.Returns) != 1 {
		t.Errorf("Returns: got %d, want 1", len(f.Returns))
	}

	if f.String() != "fun(x: number) -> boolean" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionMultiReturn(t *testing.T) {
	f := Func().
		Returns(Number, String, Boolean).
		Build()

	if len(f.Returns) != 3 {
		t.Errorf("Returns: got %d, want 3", len(f.Returns))
	}

	if f.String() != "fun() -> (number, string, boolean)" {
		t.Errorf("String: got %q", f.String())
	}
}

func TestFunctionEquality(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(Boolean).Build()
	f2 := Func().Param("y", Number).Returns(Boolean).Build()
	f3 := Func().Param("x", String).Returns(Boolean).Build()
	f4 := Func().Param("x", Number).Returns(String).Build()

	if !f1.Equals(f2) {
		t.Error("functions with same types should be equal (names ignored)")
	}

	if f1.Equals(f3) {
		t.Error("functions with different param types should not be equal")
	}

	if f1.Equals(f4) {
		t.Error("functions with different return types should not be equal")
	}
}

func TestFunctionEqualityOptional(t *testing.T) {
	f1 := Func().Param("x", Number).Build()
	f2 := Func().OptParam("x", Number).Build()

	if f1.Equals(f2) {
		t.Error("required vs optional param should differ")
	}
}

func TestFunctionEqualityVariadic(t *testing.T) {
	f1 := Func().Variadic(Number).Build()
	f2 := Func().Build()
	f3 := Func().Variadic(String).Build()

	if f1.Equals(f2) {
		t.Error("variadic vs non-variadic should differ")
	}

	if f1.Equals(f3) {
		t.Error("different variadic types should differ")
	}
}

func TestFunctionHashConsistency(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(Boolean).Build()
	f2 := Func().Param("y", Number).Returns(Boolean).Build()

	if f1.Hash() != f2.Hash() {
		t.Error("functions with same signature should have same hash")
	}

	f3 := Func().Param("x", Number).Build()
	f4 := Func().Param("x", String).Build()

	if f3.Hash() == f4.Hash() {
		t.Error("functions with different param types should have different hash")
	}
}

func TestFunctionNotEqualToPrimitive(t *testing.T) {
	f := Func().Returns(Number).Build()
	if f.Equals(Number) {
		t.Error("function should not equal primitive")
	}
}

func TestFunctionBuild_PanicsOnNilReturn(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		Func().Returns(Number, nil, String).Build()
	})
}

func TestFunctionBuild_PanicsOnTypedNilReturn(t *testing.T) {
	var nilRecord *Record
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		Func().Returns(Number, nilRecord, String).Build()
	})
}

func TestFunctionBuild_PanicsOnNilParamType(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in params; normalize before building", func() {
		Func().Param("x", nil).Build()
	})
}

func TestFunctionBuild_PanicsOnTypedNilParamType(t *testing.T) {
	var nilRecord *Record
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in params; normalize before building", func() {
		Func().Param("x", nilRecord).Build()
	})
}

func TestFunctionBuild_TreatsTypedNilVariadicAsAbsent(t *testing.T) {
	var nilRecord *Record
	f := Func().Variadic(nilRecord).Build()

	if f.Variadic != nil {
		t.Fatalf("typed nil variadic should normalize to absent, got %v", f.Variadic)
	}
	if f.String() != "fun()" {
		t.Fatalf("typed nil variadic should not render a variadic slot: %q", f.String())
	}
}

func TestRebuildFunction_PanicsOnNilTypeParam(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in type params; normalize before building", func() {
		RebuildFunction(FunctionParts{TypeParams: []*TypeParam{nil}})
	})
}

func TestRebuildFunction_PanicsOnNilReturn(t *testing.T) {
	expectFunctionBuildPanic(t, "typ.RebuildFunction: nil entry in returns; normalize before building", func() {
		RebuildFunction(FunctionParts{Returns: []Type{Number, nil, String}})
	})
}

func expectFunctionBuildPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != want {
			t.Fatalf("unexpected panic message: %v", r)
		}
	}()

	fn()
}

func TestFunctionBuild_ValidReturns(t *testing.T) {
	f := Func().Returns(Number, String).Build()
	if len(f.Returns) != 2 {
		t.Fatalf("expected 2 returns, got %d", len(f.Returns))
	}
	if f.Returns[0] != Number {
		t.Errorf("expected Number at index 0, got %v", f.Returns[0])
	}
	if f.Returns[1] != String {
		t.Errorf("expected String at index 1, got %v", f.Returns[1])
	}
}
