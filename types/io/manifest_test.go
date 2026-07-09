package io

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEncodeDecodeTypeUsesCanonicalManifestCodec(t *testing.T) {
	want := typ.NewRecord().
		Field("name", typ.String).
		OptField("age", typ.Number).
		Build()

	data, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("decoded type = %s, want %s", got, want)
	}
}

func TestManifestEncodeDecodePreservesCanonicalFields(t *testing.T) {
	m := NewManifest("app.mod")
	m.Version = 7
	m.DefineType("UserID", typ.String)
	m.SetExport(typ.NewRecord().
		Field("get", typ.Func().Param("id", typ.String).Returns(typ.Any).Build()).
		Build())
	m.AddGlobal("legacy", typ.Boolean)

	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Manifest.Encode failed: %v", err)
	}
	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}

	if got.Path != m.Path || got.Version != m.Version {
		t.Fatalf("decoded manifest identity = %s/%d, want %s/%d", got.Path, got.Version, m.Path, m.Version)
	}
	if _, ok := got.LookupType("UserID"); !ok {
		t.Fatalf("decoded manifest missing type UserID")
	}
	if !typ.TypeEquals(got.Export, m.Export) {
		t.Fatalf("decoded export = %s, want %s", got.Export, m.Export)
	}
	if globals := got.AllGlobals(); len(globals) != 0 {
		t.Fatalf("globals should remain legacy in-memory only, got %v", globals)
	}
}

func TestFunctionSummaryParamEscapesDeriveFromParamRelations(t *testing.T) {
	s := NewSummary([]typ.Type{typ.Any, typ.Any, typ.Any}, nil)
	s.ParamEscapes = []bool{true, true, true}
	s.SetParamRelations([]signature.ParamRelation{
		{
			Param:                0,
			EscapeClass:          signature.EscapeNone,
			PlacementConsequence: signature.PlacementConsequenceKeep,
		},
		{
			Param:                1,
			EscapeClass:          signature.EscapeBorrow,
			PlacementConsequence: signature.PlacementConsequenceKeep,
		},
		{
			Param:                2,
			EscapeClass:          signature.EscapeStore,
			PlacementConsequence: signature.PlacementConsequenceOwnedHeap,
		},
	})
	if got := s.ParamEscapes; len(got) != 3 || got[0] || got[1] || !got[2] {
		t.Fatalf("ParamEscapes = %#v, want derived [false false true]", got)
	}
	clone := s.Clone()
	if got := clone.ParamEscapes; len(got) != 3 || got[0] || got[1] || !got[2] {
		t.Fatalf("clone ParamEscapes = %#v, want derived [false false true]", got)
	}
}
