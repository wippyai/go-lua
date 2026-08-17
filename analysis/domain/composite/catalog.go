package composite

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

const ABIVersion uint64 = programartifact.GrammarABIVersion

type catalog struct {
	schema *engine.Schema
	// axisFragments holds each axis's opaque cold fragment at its slot. The
	// table is the only authority that reads it, and it hands one fragment
	// back only to the axis that produced it.
	axisFragments axisCells
	// ruleFragments holds each rule's opaque cold fragment at its slot. The
	// table is the only authority that reads it, and it hands one fragment
	// back only to the rule that produced it.
	ruleFragments ruleCells
	queries       QueryViews
}

// Compilation is an opaque proof of the exact sealed schema owner. It is
// sealed by construction: build assigns every field at once and only after the
// schema, digest, and query slots are established, so an accessor reads its
// field behind the zero-value fence instead of re-deriving what construction
// already settled. The digest is a view of the proof, never a constructor
// input.
type Compilation struct {
	catalog *catalog
	digest  identity.ContentID
	version uint64
}

// Available is the zero-value fence: a Compilation is either the zero value or
// the one build sealed.
func (compilation Compilation) Available() bool { return compilation.catalog != nil }

func (compilation Compilation) Digest() identity.ContentID { return compilation.digest }

func (compilation Compilation) Version() uint64 { return compilation.version }

// Schema is intentionally available only to sibling internal compiler code;
// the Compilation itself remains the authority fence.
func (compilation Compilation) Schema() *engine.Schema {
	if !compilation.Available() {
		return nil
	}
	return compilation.catalog.schema
}

var global struct {
	once        sync.Once
	compilation Compilation
	ok          bool
}

func Global() (Compilation, bool) {
	global.once.Do(func() { global.compilation, global.ok = build() })
	return global.compilation, global.ok
}

func build() (Compilation, bool) {
	// The declaration table seals before any schema slot exists, so a rule
	// inventory that violates its own laws never reaches the schema builder.
	if _, failure := Table(); failure.Available() {
		return Compilation{}, false
	}
	// The sealed table resolved every declared role once; the passes below read
	// that resolution rather than deriving identities of their own.
	roles, ok := SemanticRoles()
	if !ok {
		return Compilation{}, false
	}
	builder := engine.NewSchema()
	// Every axis's cold shape is recorded by one pass over the sealed table,
	// before the rule pass: a rule declares against the principals the axis
	// pass produces.
	axisFragments, _, ok := declareAxes(builder, roles)
	if !ok {
		return Compilation{}, false
	}
	owners, ok := axisFragments.coldPrincipals()
	if !ok {
		return Compilation{}, false
	}
	// Every rule's cold shape is recorded by one pass over the sealed table,
	// in the table's canonical order.
	fragments, _, ok := declareRules(builder, roles, owners)
	if !ok {
		return Compilation{}, false
	}
	queries, ok := declareQueries(builder, roles, owners)
	if !ok {
		return Compilation{}, false
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil || !schema.Available() {
		return Compilation{}, false
	}
	digest := identity.ContentID(schema.ID().Digest())
	if !digest.Available() {
		return Compilation{}, false
	}
	return Compilation{
		catalog: &catalog{
			schema:        schema,
			axisFragments: axisFragments,
			ruleFragments: fragments,
			queries:       queries,
		},
		digest:  digest,
		version: ABIVersion,
	}, true
}
