package io

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	checker "github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDecodeLegacyManifestReturnsMigrationSignal(t *testing.T) {
	// Produced by go-lua v1.5.16's binary manifest codec from a representative
	// record/type/global manifest. Keep the fixture immutable so detection stays
	// compatible with real persisted cache entries.
	const encoded = "SU5BTQgAAAAAAAAAAA0AAABjb21wYXQvbW9kdWxlAQ8CAAAABQAAAGNvdW50AwEABAAAAG5hbWUEAAAAAAABAAAABwAAAFBheWxvYWQOBAUAAAAAAQAAAA0AAABjb21wYXRfZ2xvYmFsAQ=="
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if _, err := DecodeManifest(data); !errors.Is(err, ErrLegacyManifestWire) {
		t.Fatalf("DecodeManifest error = %v, want ErrLegacyManifestWire", err)
	}
}

func TestDecodeLegacyStandaloneTypeReturnsMigrationSignal(t *testing.T) {
	// v1 binary type encoding for typ.String is its one-byte kind tag.
	if _, err := Decode([]byte{4}); !errors.Is(err, ErrLegacyTypeWire) {
		t.Fatalf("Decode error = %v, want ErrLegacyTypeWire", err)
	}
}

func TestDecodeMalformedNonLegacyTypeDoesNotReturnMigrationSignal(t *testing.T) {
	if _, err := Decode([]byte{0xff}); err == nil || errors.Is(err, ErrLegacyTypeWire) {
		t.Fatalf("Decode error = %v, want non-legacy syntax error", err)
	}
}

func TestCompatibilityManifestPreservesCanonicalFunctionSignatures(t *testing.T) {
	fn := typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()
	want := signature.Function{
		Type:   fn,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}
	m := NewManifest("compat/signature")
	m.DefineFunctionSignature("send", want)

	encoded, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest: %v", err)
	}
	got, ok := decoded.AllFunctionSignatures()["send"]
	if !ok || !got.Equals(want) {
		t.Fatalf("signature = %#v/%v, want %#v", got, ok, want)
	}
}

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
	if globals := got.AllGlobals(); !typ.TypeEquals(globals["legacy"], typ.Boolean) {
		t.Fatalf("decoded global legacy = %v, want boolean", globals["legacy"])
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

func TestLegacyManifestToCanonicalPreservesRegistryGetSignature(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	get := typ.Func().
		Param("id", typ.String).
		Returns(entry, typ.NewOptional(typ.LuaError)).
		Build()

	legacy := NewManifest("registry")
	legacy.DefineType("Entry", entry)
	legacy.SetExport(typ.NewInterface("registry", []typ.Method{{Name: "get", Type: get}}))
	legacy.DefineSummary("get", NewSummary(
		[]typ.Type{typ.String},
		[]typ.Type{entry, typ.NewOptional(typ.LuaError)},
	))

	canonical := legacy.ToCanonical()
	legacyGet, ok := canonical.FunctionSignatures["get"]
	if !ok {
		t.Fatalf("legacy conversion dropped registry.get summary: %#v", canonical.FunctionSignatures)
	}
	assertRegistryGetSignature(t, legacyGet, entry)
	assertRegistryConsumerClean(t, canonical)

	equivalent := manifest.New("registry")
	equivalent.DefineType("Entry", entry)
	equivalent.SetExport(typ.NewInterface("registry", []typ.Method{{Name: "get", Type: get}}))
	equivalent.DefineFunctionSignature("get", signature.Function{
		Type:   get,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	})
	assertRegistryConsumerClean(t, equivalent)
}

func TestManifestEncodeDecodeRetainsLegacyFunctionSummary(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	legacy := NewManifest("registry")
	legacy.DefineSummary("registry.get", NewSummary(
		[]typ.Type{typ.String},
		[]typ.Type{entry, typ.NewOptional(typ.LuaError)},
	))

	data, err := legacy.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	summary, ok := decoded.LookupSummary("registry.get")
	if !ok {
		t.Fatalf("decoded summaries = %#v, want registry.get", decoded.AllSummaries())
	}
	if len(summary.Params) != 1 || !typ.TypeEquals(summary.Params[0], typ.String) || len(summary.Returns) != 2 || summary.Returns[1].String() != "Error?" {
		t.Fatalf("decoded summary = %#v (returns %v), want registry.get(string) -> (Entry, LuaError?)", summary, summary.Returns)
	}
	decodedEntry, ok := summary.Returns[0].(*typ.Record)
	if !ok || decodedEntry.GetField("id") == nil || !typ.TypeEquals(decodedEntry.GetField("id").Type, typ.String) {
		t.Fatalf("decoded registry.get entry return = %v, want Entry{id: string}", summary.Returns[0])
	}
}

func assertRegistryGetSignature(t *testing.T, sig signature.Function, entry typ.Type) {
	t.Helper()
	if sig.Type == nil || len(sig.Type.Params) != 1 || !typ.TypeEquals(sig.Type.Params[0].Type, typ.String) {
		t.Fatalf("registry.get params = %#v, want one string parameter", sig.Type)
	}
	if len(sig.Type.Returns) != 2 || !typ.TypeEquals(sig.Type.Returns[0], entry) ||
		!typ.TypeEquals(sig.Type.Returns[1], typ.NewOptional(typ.LuaError)) {
		t.Fatalf("registry.get returns = %#v, want (Entry, LuaError?)", sig.Type.Returns)
	}
	record, ok := sig.Type.Returns[0].(*typ.Record)
	if !ok || record.GetField("id") == nil || !typ.TypeEquals(record.GetField("id").Type, typ.String) {
		t.Fatalf("registry.get first return = %v, want Entry{id: string}", sig.Type.Returns[0])
	}
	for _, label := range sig.Effect.Labels {
		if got, ok := effect.NormalizeLabel(label).(returns.ErrorReturn); ok && got == (returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}) {
			return
		}
	}
	t.Fatalf("registry.get effect = %v, want ErrorReturn(0, 1)", sig.Effect)
}

func assertRegistryConsumerClean(t *testing.T, registry *manifest.Manifest) {
	t.Helper()
	chunk, err := parse.ParseString(`
local e, err = registry.get("k")
if err == nil then
    local s: string = e.id
end
`, "consumer.lua")
	if err != nil {
		t.Fatalf("parse consumer: %v", err)
	}
	database := db.New()
	database.Connect("registry", registry)
	session := checker.NewChecker(database, checker.Deps{}).CheckChunkWithImports(chunk, "consumer.lua", map[string]*manifest.Manifest{
		"registry": registry,
	})
	if len(session.Diagnostics) != 0 {
		t.Fatalf("registry consumer diagnostics = %#v, want none", session.Diagnostics)
	}
}
