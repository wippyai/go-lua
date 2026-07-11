package service

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Mirrors command_bus: an ambient host module (channel) used as a BARE global
// (no require, no declared import), bound via manifest Globals + GlobalTypes.
// `channel.new(256)` must resolve, not report value.reference.unresolved.
func TestAmbientBareGlobalResolvesFromManifestGlobals(t *testing.T) {
	ctx := context.Background()
	document := embedding.RegistryDocument("fixture:command_bus/source:lua")
	content := []byte("local ch = channel.new(256)\nreturn ch\n")

	newFn := typ.Func().Param("size", typ.Number).Returns(typ.Any).Build()
	channelIface := typ.NewInterface("channel", []typ.Method{{Name: "new", Type: newFn}})
	ch := manifest.New("channel")
	ch.SetExport(channelIface)
	ch.DefineGlobalType("channel", channelIface) // adds "channel" to Globals + GlobalTypes

	input := UnitInput{
		ID:            "fixture:command_bus",
		ModulePath:    "fixture/command_bus",
		EntryDocument: document,
		Sources: map[embedding.DocumentID]embedding.SourceSnapshot{
			document: {Document: document, ProviderRevision: "rev-1", Content: content},
		},
		DocumentLabels: embedding.StaticLabels{document: "command_bus.lua"},
		Plan: embedding.UnitPlan{
			ID:      "fixture:command_bus",
			Entry:   document,
			Sources: []embedding.DocumentID{document},
		},
		ExternalManifests: map[string]*manifest.Manifest{"channel": ch},
		IncludeStdlib:     true,
		ResolutionDigest:  digestBytes([]byte("view-1")),
	}

	session := NewBatchSession()
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	tag := mustSolve(t, session, SolveRequest{UnitID: input.ID, Freshness: FreshnessRequireNew})
	completed, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(tag)})
	if !ok {
		t.Fatal("completed result missing")
	}
	for _, item := range completed.Judgments() {
		if string(item.Code) == "value.reference.unresolved" {
			t.Fatalf("bare ambient global channel reported unresolved despite manifest Globals binding: %s", item.Code)
		}
	}
}
