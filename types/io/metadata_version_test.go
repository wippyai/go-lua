package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestPreviousManifestVersionDefaultsSemanticMetadata(t *testing.T) {
	m := NewManifest("legacy")
	m.Export = typ.NewRecord().OptReadonlyField("field", typ.NewArray(typ.String)).
		MapComponent(typ.String, typ.NewMap(typ.String, typ.Number)).SetOpen(true).SetDeclared(true).Build()
	old, err := m.encodeVersion(13)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(old)
	if err != nil {
		t.Fatal(err)
	}
	if !typ.TypeEquals(m.Export, decoded.Export) {
		t.Fatalf("v13 defaults changed: want %#v, got %#v", m.Export, decoded.Export)
	}
	if decoded.Export.(*typ.Record).Complete || decoded.Export.(*typ.Record).MapInferredPresence || decoded.Export.(*typ.Record).GetField("field").InferredPresence {
		t.Fatal("v13 metadata defaults are incorrect")
	}
}

func TestPreviousManifestVersionDefaultsMissingPresenceBits(t *testing.T) {
	m := NewManifest("legacy-flags")
	m.Export = typ.NewRecord().AddField(typ.Field{Name: "field", Type: typ.NewInferredArray(typ.String).WithExplicitNilWrite(), Optional: true, InferredPresence: true}).
		MapComponentWithFlags(typ.String, typ.NewInferredMap(typ.String, typ.Number).WithExplicitNilWrite(), true, true).
		SetComplete(true).Build()
	old, err := m.encodeVersion(13)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(old)
	if err != nil {
		t.Fatal(err)
	}
	r := decoded.Export.(*typ.Record)
	if r.Complete || r.MapInferredPresence || r.MapExplicitNilWrite || r.GetField("field").InferredPresence {
		t.Fatalf("v13 must default absent record flags to false: %#v", r)
	}
	a := r.GetField("field").Type.(*typ.Array)
	mapping := r.MapValue.(*typ.Map)
	if a.InferredPresence || a.ExplicitNilWrite || mapping.InferredPresence || mapping.ExplicitNilWrite {
		t.Fatalf("v13 must default absent container flags to false: array=%#v map=%#v", a, mapping)
	}
}
