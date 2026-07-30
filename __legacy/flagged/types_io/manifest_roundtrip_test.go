package io

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/types/typ"
)

func TestManifestEncodeDecodeRetainsLegacyErrorReturnCorrelation(t *testing.T) {
	legacy := NewManifest("registry")
	legacy.DefineSummary("registry.get", NewSummary(
		[]typ.Type{typ.String},
		[]typ.Type{
			typ.NewRecord().Field("id", typ.String).Build(),
			typ.NewOptional(typ.LuaError),
		},
	))

	data, err := legacy.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	sig, ok := decoded.ToCanonical().FunctionSignatures["registry.get"]
	if !ok {
		t.Fatal("decoded legacy manifest lost registry.get signature")
	}
	for _, label := range sig.Effect.Labels {
		if got, ok := effect.NormalizeLabel(label).(returns.ErrorReturn); ok && got == (returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}) {
			return
		}
	}
	t.Fatalf("registry.get effect = %v, want ErrorReturn(0, 1)", sig.Effect)
}
