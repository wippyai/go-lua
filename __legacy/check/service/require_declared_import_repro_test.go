package service

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// Mirrors the runtime resolver's output for `modules: [uuid]` +
// `local uuid = require("uuid")`: the unit declares uuid as a Plan import AND
// carries uuid's export manifest. require("uuid").v4() must yield (string,
// LuaError?), so new_id returns string and the concat is clean.
func TestRequireOfDeclaredHostImportRehydratesExport(t *testing.T) {
	ctx := context.Background()
	document := embedding.RegistryDocument("fixture:uuid_client/source:lua")
	content := []byte("local uuid = require(\"uuid\")\n" +
		"local function new_id()\n" +
		"    local id, err = uuid.v4()\n" +
		"    if err then return tostring(err) end\n" +
		"    return id\n" +
		"end\n" +
		"return \"ui.response.\" .. new_id()\n")

	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	v4 := typ.Func().Returns(typ.String, typeexpr.Optional(errorType)).Build()
	uuid := manifest.New("uuid")
	uuid.DefineType("Error", errorType)
	uuid.SetExport(typ.NewInterface("uuid", []typ.Method{{Name: "v4", Type: v4}}))

	input := UnitInput{
		ID:            "fixture:uuid_client",
		ModulePath:    "fixture/uuid_client",
		EntryDocument: document,
		Sources: map[embedding.DocumentID]embedding.SourceSnapshot{
			document: {Document: document, ProviderRevision: "rev-1", Content: content},
		},
		DocumentLabels: embedding.StaticLabels{document: "uuid_client.lua"},
		Plan: embedding.UnitPlan{
			ID:      "fixture:uuid_client",
			Entry:   document,
			Sources: []embedding.DocumentID{document},
			Imports: []embedding.UnitImport{{Alias: "uuid", TargetUnit: "builtin:uuid", ManifestDigest: digestBytes([]byte("uuid-manifest"))}},
		},
		ExternalManifests: map[string]*manifest.Manifest{"uuid": uuid},
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
		if string(item.Code) == "operator.concat.operand" {
			t.Fatalf("uuid.v4 first return not preserved as string via require: %s", item.Code)
		}
	}
}
