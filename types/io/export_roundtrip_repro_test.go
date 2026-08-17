package io

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// A host module export is an interface of methods returning (value, LuaError?).
// The Encode/Decode clone in the checker service must preserve those method
// signatures, or require("uuid") rehydrates an export whose v4() yields nil.
func TestManifestExportInterfaceRoundTripPreservesMethods(t *testing.T) {
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	v4 := typ.Func().Returns(typ.String, typeexpr.Optional(errorType)).Build()

	m := New("uuid")
	m.DefineType("Error", errorType)
	m.SetExport(typ.NewInterface("uuid", []typ.Method{{Name: "v4", Type: v4}}))

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Export == nil {
		t.Fatal("decoded export is nil")
	}
	if !m.Export.Equals(got.Export) {
		t.Fatalf("decoded export = %s, want %s (interface methods must survive round-trip)", got.Export.String(), m.Export.String())
	}
}
