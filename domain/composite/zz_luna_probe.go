package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
)

// CodexLunaCatalogProbe exposes the construction refusal only while the
// bounded diagnostic probe is being run; this file is removed immediately.
func CodexLunaCatalogProbe() (schema.SealFailure, bool) {
	state, failure := newCatalog()
	return failure, state != nil && state.sealed != nil
}

func CodexLunaBuildProbe() string {
	state, failure := newCatalog()
	if state == nil || failure.Available() || state.sealed == nil {
		return "catalog"
	}
	roles := state.roles
	if !roles.Available() {
		return "roles"
	}
	builder := engine.NewSchema()
	axisFragments, _, ok := declareAxes(state, builder, roles)
	if !ok {
		return "declareAxes"
	}
	owners, ok := axisFragments.coldPrincipals(state)
	if !ok {
		return "coldPrincipals"
	}
	fragments, _, ok := declareRules(state, builder, roles, owners)
	if !ok {
		return "declareRules"
	}
	queryFragments, ok := declareQueries(state, builder, axisFragments)
	if !ok {
		return "declareQueries"
	}
	sealedSchema, ok := builder.Seal()
	if !ok || sealedSchema == nil || !sealedSchema.Available() {
		return "builder.Seal"
	}
	publication, publicationOK := state.declarations.Publication()
	publicationSchemaID, schemaIDOK := publication.SchemaID()
	if !publicationOK || !schemaIDOK {
		return "publication"
	}
	if !state.structureOK {
		return "structure"
	}
	collections, collectionsOK := diagnosticCollectionDirectory(state.diagnostics, observationIssuance(state), queryIssuance(state), state)
	if !collectionsOK {
		return "collections"
	}
	state.collections = collections
	digest := identity.ContentID(sealedSchema.ID().Digest())
	if !digest.Available() {
		return "digest"
	}
	execution, executionOK := programartifact.NewExecutionSchemaID(digest, publicationSchemaID, programartifact.GrammarABIVersion)
	if !executionOK {
		return "execution"
	}
	state.schema = sealedSchema
	state.digest = digest
	state.execution = execution
	state.axisFragments = axisFragments
	state.ruleFragments = fragments
	state.queryFragments = queryFragments
	return "ok"
}
