package body

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestCallArgumentFunctionTypeProofPredicatesOwnSubtypeChecks(t *testing.T) {
	result := &Result{}
	fn := typ.Func().Returns(typ.String).Build()

	if !result.CallArgumentFunctionTypeAdmissible(fn, fn) {
		t.Fatal("matching contextual function type should be admissible")
	}
	if result.CallArgumentFunctionTypeAdmissible(fn, typ.String) {
		t.Fatal("unrelated expected type should not admit contextual function type")
	}
	if !result.CallArgumentFunctionTypeProvenMismatch(fn, typ.String) {
		t.Fatal("contextual function type should prove mismatch against unrelated expected type")
	}
	if result.CallArgumentFunctionTypeProvenMismatch(fn, fn) {
		t.Fatal("matching contextual function type should not prove mismatch")
	}
	if result.CallArgumentFunctionTypeProvenMismatch(fn, typ.Any) {
		t.Fatal("gradual expected type should not produce contextual function mismatch")
	}
}

func TestCallArgumentSolvedTypeProvenMismatchKeepsGradualBoundariesUnknown(t *testing.T) {
	result := &Result{}

	if !result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Number, false) {
		t.Fatal("trusted concrete solved type should prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.String, false) {
		t.Fatal("matching solved type should not prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Number, true) {
		t.Fatal("untrusted top-origin solved type should remain an unknown proof boundary")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.Any, typ.Number, false) {
		t.Fatal("gradual actual type should not prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Unknown, false) {
		t.Fatal("unknown expected type should not prove mismatch")
	}
}

func TestRecordInterfaceMismatchExplainsMissingMethod(t *testing.T) {
	result := &Result{}
	actual := typetable.NewRecord().Field("id", typ.String).Build()
	expected := typ.NewInterface("Closable", []typ.Method{
		{Name: "close", Type: typ.Func().Build()},
	})

	mismatch, ok := result.RecordInterfaceMismatch(actual, expected)
	if !ok {
		t.Fatal("record missing an interface method should produce mismatch evidence")
	}
	if mismatch.Kind != InterfaceMismatchMissingMethod {
		t.Fatalf("kind: got %v, want missing method", mismatch.Kind)
	}
	if mismatch.MethodName != "close" {
		t.Fatalf("method: got %q, want close", mismatch.MethodName)
	}
	if mismatch.Expected != expected.Methods[0].Type {
		t.Fatal("missing-method evidence should preserve the expected method type")
	}
}

func TestRecordInterfaceMismatchExplainsMethodType(t *testing.T) {
	result := &Result{}
	actualMethod := typ.Func().Returns(typ.String).Build()
	expectedMethod := typ.Func().Returns(typ.Number).Build()
	actual := typetable.NewRecord().Field("read", actualMethod).Build()
	expected := typ.NewInterface("Reader", []typ.Method{
		{Name: "read", Type: expectedMethod},
	})

	mismatch, ok := result.RecordInterfaceMismatch(actual, expected)
	if !ok {
		t.Fatal("record method with incompatible type should produce mismatch evidence")
	}
	if mismatch.Kind != InterfaceMismatchMethodType {
		t.Fatalf("kind: got %v, want method type", mismatch.Kind)
	}
	if mismatch.MethodName != "read" {
		t.Fatalf("method: got %q, want read", mismatch.MethodName)
	}
	if mismatch.Actual != actualMethod {
		t.Fatal("method-type evidence should preserve the actual method type")
	}
	if mismatch.Expected != expectedMethod {
		t.Fatal("method-type evidence should preserve the expected method type")
	}
}
