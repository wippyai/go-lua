package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func TestManifestReturnConvention_ExternalOnlyAcrossCodec(t *testing.T) {
	fn := typ.Func().Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String)).Build()
	for _, bodyBacked := range []bool{false, true} {
		manifest := NewManifest("source")
		manifest.BodyBacked = bodyBacked
		manifest.SetExport(typ.NewRecord().Field("get", fn).Build())
		data, err := manifest.Encode()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeManifest(data)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.BodyBacked != bodyBacked {
			t.Fatalf("manifest body provenance was lost: %v", bodyBacked)
		}
		record, ok := decoded.EnrichedExport().(*typ.Record)
		if !ok || record.GetField("get") == nil {
			t.Fatal("missing get export")
		}
		field := record.GetField("get").Type
		hasRelation := contract.ExtractSpec(field) != nil && contract.ExtractSpec(field).Effects.GetErrorReturn(0) != nil
		if hasRelation == bodyBacked {
			t.Fatalf("relation for bodyBacked=%v: got %v", bodyBacked, hasRelation)
		}
	}
}

func TestManifestReturnConvention_ExternalNamedType(t *testing.T) {
	manifest := NewManifest("sql")
	query := typ.Func().Returns(typ.NewOptional(typ.Number), typ.NewOptional(typ.String)).Build()
	manifest.DefineType("DB", typ.NewRecord().Field("query", query).Build())
	got, ok := manifest.LookupType("DB")
	if !ok {
		t.Fatal("missing DB type")
	}
	record := got.(*typ.Record)
	if spec := contract.ExtractSpec(record.GetField("query").Type); spec == nil || spec.Effects.GetErrorReturn(0) == nil {
		t.Fatal("external method declaration must carry its return relation")
	}
}
