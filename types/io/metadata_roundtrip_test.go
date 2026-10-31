package io_test

import (
	"testing"

	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

func TestSemanticMetadataSurvivesTransformsAndManifest(t *testing.T) {
	p := typ.NewTypeParam("T", nil)
	meta := typ.NewRecord().Field("tag", typ.String).Build()
	tests := []struct {
		name string
		make func(typ.Type) typ.Type
	}{
		{"array inferred", func(v typ.Type) typ.Type { return typ.NewInferredArray(v) }},
		{"array nil write", func(v typ.Type) typ.Type { return typ.NewArray(v).WithExplicitNilWrite() }},
		{"map inferred", func(v typ.Type) typ.Type { return typ.NewInferredMap(typ.String, v) }},
		{"map nil write", func(v typ.Type) typ.Type { return typ.NewMap(typ.String, v).WithExplicitNilWrite() }},
		{"record open", func(v typ.Type) typ.Type { return typ.NewRecord().Field("value", v).SetOpen(true).Build() }},
		{"record complete", func(v typ.Type) typ.Type { return typ.NewRecord().Field("value", v).SetComplete(true).Build() }},
		{"record declared", func(v typ.Type) typ.Type { return typ.NewRecord().Field("value", v).SetDeclared(true).Build() }},
		{"field optional", func(v typ.Type) typ.Type { return typ.NewRecord().OptField("value", v).Build() }},
		{"field inferred", func(v typ.Type) typ.Type {
			return typ.NewRecord().AddField(typ.Field{Name: "value", Type: v, Optional: true, InferredPresence: true}).Build()
		}},
		{"field readonly", func(v typ.Type) typ.Type { return typ.NewRecord().ReadonlyField("value", v).Build() }},
		{"record metatable", func(v typ.Type) typ.Type { return typ.NewRecord().Field("value", v).Metatable(meta).Build() }},
		{"record map component", func(v typ.Type) typ.Type {
			return typ.NewRecord().Field("value", v).MapComponent(typ.String, v).Build()
		}},
		{"record map inferred", func(v typ.Type) typ.Type {
			return typ.NewRecord().Field("value", v).MapComponentWithFlags(typ.String, v, true, false).Build()
		}},
		{"record map nil write", func(v typ.Type) typ.Type {
			return typ.NewRecord().Field("value", v).MapComponentWithFlags(typ.String, v, false, true).Build()
		}},
		{"record flags", func(v typ.Type) typ.Type {
			return typ.NewRecord().AddField(typ.Field{Name: "value", Type: v, Optional: true, InferredPresence: true, Readonly: true}).
				Metatable(meta).MapComponentWithFlags(typ.String, v, true, true).
				SetOpen(true).SetComplete(true).SetDeclared(true).Build()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.make(typ.Number)
			check := func(operation string, got typ.Type) {
				t.Helper()
				if !typ.TypeEquals(want, got) {
					t.Errorf("%s lost semantic metadata: want %#v, got %#v", operation, want, got)
				}
			}
			check("rewrite identity", typ.Rewrite(want, func(typ.Type) (typ.Type, bool) { return nil, false }))
			check("rewrite child", typ.Rewrite(tc.make(p), func(v typ.Type) (typ.Type, bool) {
				if v == p {
					return typ.Number, true
				}
				return nil, false
			}))
			check("substitute no-op", subst.Params(want, []*typ.TypeParam{p}, []typ.Type{typ.Number}))
			check("substitute child", subst.Params(tc.make(p), []*typ.TypeParam{p}, []typ.Type{typ.Number}))
			generic := typ.NewGeneric("Box", []*typ.TypeParam{p}, tc.make(p))
			check("instantiate", subst.ExpandInstantiated(typ.Instantiate(generic, typ.Number)))
			check("resolve no-op", typ.Resolve(want, want))
			check("resolve child", typ.Resolve(tc.make(typ.Unresolved), want))
			encodedType, err := typeio.Encode(want)
			if err != nil {
				t.Fatal(err)
			}
			decodedType, err := typeio.Decode(encodedType)
			if err != nil {
				t.Fatal(err)
			}
			check("type codec", decodedType)
			m := typeio.NewManifest("metadata")
			m.Export = want
			data, err := m.Encode()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := typeio.DecodeManifest(data)
			if err != nil {
				t.Fatal(err)
			}
			check("manifest", decoded.Export)
		})
	}
}

func TestDeclarationMarkingPreservesContainerMetadata(t *testing.T) {
	child := typ.NewRecord().Field("leaf", typ.String).Build()
	array := typ.NewInferredArray(child).WithExplicitNilWrite()
	mapping := typ.NewInferredMap(typ.String, child).WithExplicitNilWrite()
	root := typ.NewRecord().Field("array", array).Field("map", mapping).
		MapComponentWithFlags(typ.String, child, true, true).SetComplete(true).Build()
	marked := typ.MarkDeclaredShared(root)[0].(*typ.Record)
	if !marked.Declared || !marked.Complete || !marked.MapInferredPresence || !marked.MapExplicitNilWrite {
		t.Fatalf("record metadata lost during declaration marking: %#v", marked)
	}
	if got := marked.GetField("array").Type.(*typ.Array); !got.InferredPresence || !got.ExplicitNilWrite {
		t.Fatalf("array metadata lost during declaration marking: %#v", got)
	}
	if got := marked.GetField("map").Type.(*typ.Map); !got.InferredPresence || !got.ExplicitNilWrite {
		t.Fatalf("map metadata lost during declaration marking: %#v", got)
	}
}
