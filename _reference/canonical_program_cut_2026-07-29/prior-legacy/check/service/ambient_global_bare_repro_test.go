package service

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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

// A module import and an ambient global may intentionally have the same public
// name while denoting different values. The bare root must be seeded from
// GlobalTypes; ModuleExports is authority only for a lexical require result.
func TestAmbientBareGlobalDoesNotUseSameNamedModuleExport(t *testing.T) {
	ctx := context.Background()
	document := embedding.RegistryDocument("fixture:ambient-module-collision/source:lua")
	ambientType := typetable.NewRecord().Field("value", typ.Func().Returns(typ.String).Build()).Build()
	moduleType := typetable.NewRecord().Field("value", typ.Func().Returns(typ.Number).Build()).Build()
	processModule := manifest.New("process")
	processModule.SetExport(moduleType)
	processModule.DefineGlobalType("process", ambientType)
	input := UnitInput{
		ID:            "fixture:ambient-module-collision",
		ModulePath:    "fixture/ambient-module-collision",
		EntryDocument: document,
		Sources: map[embedding.DocumentID]embedding.SourceSnapshot{
			document: {Document: document, ProviderRevision: "rev-1", Content: []byte("local process_module = require(\"process\")\nlocal module_value: number = process_module.value()\nlocal global_value: string = process.value()\nreturn { module_value = module_value, global_value = global_value }\n")},
		},
		DocumentLabels: embedding.StaticLabels{document: "ambient_module_collision.lua"},
		Plan: embedding.UnitPlan{
			ID:      "fixture:ambient-module-collision",
			Entry:   document,
			Sources: []embedding.DocumentID{document},
		},
		ExternalManifests: map[string]*manifest.Manifest{"process": processModule},
		Globals:           []string{"process"},
		GlobalTypes:       map[string]typ.Type{"process": ambientType},
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
	for _, item := range completed.RenderedDiagnostics() {
		if item.Code == "type.assignment" {
			t.Fatalf("bare ambient global was typed from same-named module export: %s", item.Message)
		}
	}
}
