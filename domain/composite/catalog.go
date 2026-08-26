package composite

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

type catalog struct {
	// The fields above and below are one immutable declaration state. The
	// Compilation handle below points directly at this object; no nested state
	// mirror or second declaration projection is retained.
	sealed            *seal.Schema
	failure           schema.SealFailure
	templates         []*rule.Template
	ruleContributors  []RuleContributor[principals, authorities]
	axes              []*axisTemplate
	axisContributors  []axisContributor
	queries           []*query.Registration
	queryContributors []queryContributor
	observations      observation.Table
	queryPositions    map[schema.Key]queryPosition
	axisAdopters      []axisAdopter
	diagnostics       diagnostic.Table
	collections       DiagnosticCollections
	structure         structure.Table
	structureOK       bool
	roles             vocabulary.Roles
	slotsByKey        map[schema.Key]int
	declarations      analysiscatalog.Compilation

	digest    identity.ContentID
	schema    *engine.Schema
	execution programartifact.ExecutionSchemaID
	// axisFragments holds each axis's opaque cold fragment at its slot. The
	// table is the only authority that reads it, and it hands one fragment
	// back only to the axis that produced it.
	axisFragments axisCells
	// ruleFragments holds each rule's opaque cold fragment at its slot. The
	// table is the only authority that reads it, and it hands one fragment
	// back only to the rule that produced it.
	ruleFragments ruleCells
	// queryFragments holds each declared query family's opaque cold fragment at
	// its slot. The table is the only authority that reads it, and it hands one
	// fragment back only to the contributor that produced it.
	queryFragments queryCells
}

// queryPosition is the one sealed lookup witness for a query declaration. The
// authored family is the lookup key used by existing callers, while the
// owner-issued EntryID and one-based ordinal are retained beside the position
// so a lookup cannot silently pair a family with a different registration.
// There is no second family-to-identity map: this value is built once from the
// sealed registration slice and every consumer validates against it.
type queryPosition struct {
	Position        int
	EntryID         schema.EntryID
	Ordinal         uint32
	SelectedOrdinal uint32
}

// Compilation is an opaque proof of the exact sealed schema owner. It is
// sealed by construction: build assigns every field at once and only after the
// schema, digest, and query slots are established, so an accessor reads its
// field behind the zero-value fence instead of re-deriving what construction
// already settled. The digest is a view of the proof, never a constructor
// input.
type Compilation struct {
	catalog *catalog
}

// Available is the zero-value fence: a Compilation is either the zero value or
// the one build sealed.
func (compilation Compilation) Available() bool { return compilation.catalog != nil }

func (compilation Compilation) Digest() identity.ContentID {
	if !compilation.Available() {
		return identity.ContentID{}
	}
	return compilation.catalog.digest
}

// ExecutionSchemaID is the atomic foreign-consumer identity for this sealed
// compilation. It commits the cold engine meaning together with the
// order-sensitive publication schema, so declaration-axis reorder cannot
// reuse an artifact compiled under another snapshot layout.
func (compilation Compilation) ExecutionSchemaID() programartifact.ExecutionSchemaID {
	if !compilation.Available() || compilation.catalog == nil {
		return programartifact.ExecutionSchemaID{}
	}
	return compilation.catalog.execution
}

// Schema is intentionally available only to sibling internal compiler code;
// the Compilation itself remains the authority fence.
func (compilation Compilation) Schema() *engine.Schema {
	if !compilation.Available() {
		return nil
	}
	return compilation.catalog.schema
}

// Structure returns the vocabulary sealed by this exact compilation. It is
// retained by the declaration transaction rather than recovered through a
// second composition projection.
func (compilation Compilation) Structure() (structure.Table, bool) {
	if !compilation.Available() || compilation.catalog == nil {
		return structure.Table{}, false
	}
	return compilation.catalog.structure, compilation.catalog.structureOK
}

// Diagnostics returns the diagnostic declaration table sealed by this exact
// compilation. Runtime collectors consume it explicitly instead of reaching
// another compilation's declaration state.
func (compilation Compilation) Diagnostics() (diagnostic.Table, bool) {
	if !compilation.Available() || compilation.catalog == nil || !compilation.catalog.diagnostics.Available() {
		return diagnostic.Table{}, false
	}
	return compilation.catalog.diagnostics, true
}

// DiagnosticCollections returns the branch collection joins sealed by this
// exact compilation. The returned rows do not alias the catalog's site slices.
func (compilation Compilation) DiagnosticCollections() (DiagnosticCollections, bool) {
	if !compilation.Available() || compilation.catalog == nil || !compilation.catalog.collections.Available() {
		return DiagnosticCollections{}, false
	}
	return compilation.catalog.collections, true
}

// Publication is the immutable snapshot column plan compiled from this
// compilation's sealed declaration schema. It is carried by the compilation,
// not discovered through another composition projection.
func (compilation Compilation) Publication() (analysiscatalog.Publication, bool) {
	if !compilation.Available() || compilation.catalog == nil {
		return analysiscatalog.Publication{}, false
	}
	return compilation.catalog.declarations.Publication()
}

// RulePlans returns the single post-seal Rule projection retained by this
// compilation. Runtime construction consumes these dense rows directly; it
// does not reopen the declaration schema or reconstruct rule geometry.
func (compilation Compilation) RulePlans() (ruleplan.Catalog, bool) {
	if !compilation.Available() || compilation.catalog == nil {
		return ruleplan.Catalog{}, false
	}
	return compilation.catalog.declarations.RulePlans()
}

// Build seals one independent concrete analyzer compilation. The caller owns
// its lifetime; this package retains no compilation cache.
func Build() (Compilation, bool) {
	// The declaration table seals before any schema slot exists, so a rule
	// inventory that violates its own laws never reaches the schema builder.
	state, failure := newCatalog()
	if state == nil || failure.Available() || state.sealed == nil {
		return Compilation{}, false
	}
	// The sealed table resolved every declared role once; the passes below read
	// that resolution rather than deriving identities of their own.
	roles := state.roles
	if !roles.Available() {
		return Compilation{}, false
	}
	builder := engine.NewSchema()
	// Every axis's cold shape is recorded by one pass over the sealed table,
	// before the rule pass: a rule declares against the principals the axis
	// pass produces.
	axisFragments, _, ok := declareAxes(state, builder, roles)
	if !ok {
		return Compilation{}, false
	}
	owners, ok := axisFragments.coldPrincipals(state)
	if !ok {
		return Compilation{}, false
	}
	// Every rule's cold shape is recorded by one pass over the sealed table,
	// in the table's canonical order.
	fragments, _, ok := declareRules(state, builder, roles, owners)
	if !ok {
		return Compilation{}, false
	}
	queryFragments, ok := declareQueries(state, builder, axisFragments)
	if !ok {
		return Compilation{}, false
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil || !schema.Available() {
		return Compilation{}, false
	}
	publication, publicationOK := state.declarations.Publication()
	publicationSchemaID, schemaIDOK := publication.SchemaID()
	if !publicationOK || !schemaIDOK {
		return Compilation{}, false
	}
	if !state.structureOK {
		return Compilation{}, false
	}
	if !state.diagnostics.Available() {
		return Compilation{}, false
	}
	collections, collectionsOK := diagnosticCollectionDirectory(state.diagnostics, observationIssuance(state), queryIssuance(state), state)
	if !collectionsOK {
		return Compilation{}, false
	}
	state.collections = collections
	digest := identity.ContentID(schema.ID().Digest())
	if !digest.Available() {
		return Compilation{}, false
	}
	execution, executionOK := programartifact.NewExecutionSchemaID(digest, publicationSchemaID, programartifact.GrammarABIVersion)
	if !executionOK {
		return Compilation{}, false
	}
	state.schema = schema
	state.digest = digest
	state.execution = execution
	state.axisFragments = axisFragments
	state.ruleFragments = fragments
	state.queryFragments = queryFragments
	return Compilation{catalog: state}, true
}
