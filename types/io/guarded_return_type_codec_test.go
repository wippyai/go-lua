package io

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestGuardedReturnTypeSurvivesManifestCodec(t *testing.T) {
	target := typ.NewRecord().Field("category", typ.String).Build()
	fn := typ.Func().Returns(typ.Boolean, typ.NewUnion(target, typ.String)).
		Spec(contract.NewSpec().WithEffects(effect.GuardedReturnType{
			GuardIndex: 0, TargetIndex: 1, TargetHash: target.Hash(), TargetType: target,
		})).Build()
	manifest := NewManifest("producer")
	manifest.SetExport(typ.NewRecord().Field("choose", fn).Build())
	data, err := manifest.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	record := decoded.EnrichedExport().(*typ.Record)
	spec := contract.ExtractSpec(record.GetField("choose").Type)
	if spec == nil {
		t.Fatal("missing function spec")
	}
	for _, label := range spec.Effects.Labels {
		if relation, ok := label.(effect.GuardedReturnType); ok {
			got, ok := relation.TargetType.(typ.Type)
			if !ok || got.Hash() != target.Hash() || relation.TargetHash != target.Hash() {
				t.Fatalf("guarded target changed across codec: %+v", relation)
			}
			return
		}
	}
	t.Fatal("guarded return effect was lost")
}
