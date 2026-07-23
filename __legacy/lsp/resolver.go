package lsp

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/service"
	"github.com/wippyai/go-lua/analysis/embedding"
)

// MaterializedDocument is the immutable overlay snapshot given to a module
// resolver. The resolver selects a unit plan and configured manifests; it does
// not read files, resolve imports during solving, or run checker analysis.
type MaterializedDocument struct {
	Document embedding.DocumentID
	Version  int64
	Snapshot embedding.SourceSnapshot
}

// ModuleResolver is the product extension point for translating a materialized
// document into a complete service.UnitInput. Wippy can supply registry-aware
// resolution here while the server remains protocol/scheduling-only.
type ModuleResolver interface {
	Resolve(context.Context, MaterializedDocument) (service.UnitInput, error)
}

// FileConventionsResolver is the v1 single-document resolver. Template carries
// configured service inputs such as external manifests, global types, profile,
// and policies. The resolver replaces every source identity with the current
// overlay document and preserves only explicit, already-materialized imports.
type FileConventionsResolver struct {
	Template              service.UnitInput
	UnitIDForDocument     func(embedding.DocumentID) embedding.UnitID
	ModulePathForDocument func(embedding.DocumentID) string
}

func (r FileConventionsResolver) Resolve(ctx context.Context, document MaterializedDocument) (service.UnitInput, error) {
	if err := ctx.Err(); err != nil {
		return service.UnitInput{}, err
	}
	if document.Document.Scheme != embedding.DocumentSchemeFile || !document.Document.Valid() {
		return service.UnitInput{}, fmt.Errorf("lsp: file resolver cannot resolve document %q", document.Document)
	}
	verified, err := document.Snapshot.Verify()
	if err != nil {
		return service.UnitInput{}, err
	}
	if verified.Document != document.Document {
		return service.UnitInput{}, fmt.Errorf("lsp: snapshot document %q does not match %q", verified.Document, document.Document)
	}
	input := r.Template
	unitID := embedding.UnitID(document.Document.String())
	if r.UnitIDForDocument != nil {
		unitID = r.UnitIDForDocument(document.Document)
	}
	if unitID == "" {
		return service.UnitInput{}, fmt.Errorf("lsp: resolver produced an empty unit id for %q", document.Document)
	}
	modulePath := document.Document.OpaqueKey
	if r.ModulePathForDocument != nil {
		modulePath = r.ModulePathForDocument(document.Document)
	}
	if modulePath == "" {
		modulePath = string(unitID)
	}
	input.ID = unitID
	input.ModulePath = modulePath
	input.EntryDocument = document.Document
	input.Sources = map[embedding.DocumentID]embedding.SourceSnapshot{document.Document: verified}
	input.DocumentLabels = embedding.StaticLabels{document.Document: document.Document.OpaqueKey}
	input.EntryFile = ""
	input.SourceFiles = nil
	input.DocumentVersion = document.Version
	plan := input.Plan.Clone()
	plan.ID = unitID
	plan.ModulePath = modulePath
	plan.Entry = document.Document
	plan.Sources = []embedding.DocumentID{document.Document}
	input.Plan = plan
	return input, nil
}
