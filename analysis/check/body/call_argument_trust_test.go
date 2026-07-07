package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
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
