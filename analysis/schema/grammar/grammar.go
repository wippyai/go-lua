// Package grammar owns the one process-global sealed analyzer grammar.
// Its receipt is the only authority accepted by the Program transformer;
// callers cannot manufacture an equivalent authority from a digest.
//
// Neither the axis nor the rule inventory is written here: they are the two
// surfaces of the analyzer declaration table, and this package composes that
// table's cold declaration pass.
package grammar

import (
	"sync"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const ReceiptVersion uint64 = programartifact.GrammarABIVersion

type catalog struct {
	schema *engine.Schema
	// axisFragments holds each axis's opaque cold fragment at its writer
	// principal. The table is the only authority that reads it, and it hands
	// one fragment back only to the axis that produced it.
	axisFragments axisCells
	// ruleFragments holds each rule's opaque cold fragment at its role. The
	// table is the only authority that reads it, and it hands one fragment
	// back only to the rule that produced it.
	ruleFragments [ruleRoleLimit]rule.Cell
	valueQuery    *engine.QuerySlot[query.ValueSummaryObservation]
	valueRead     engine.SchemaReadForm[valuedomain.Value]
	effectQuery   *engine.QuerySlot[query.EffectObservation]
	effectRead    engine.SchemaReadForm[effectfactor.Value]
}

// CompilationReceipt is an opaque proof of the exact sealed schema owner.
// The digest is a view of the proof, never a constructor input.
type CompilationReceipt struct {
	catalog *catalog
	digest  identity.ContentID
	version uint64
}

func (receipt CompilationReceipt) Available() bool {
	return receipt.catalog != nil && receipt.catalog.schema != nil && receipt.catalog.schema.Available() && receipt.digest.Available() && receipt.version == ReceiptVersion
}

func (receipt CompilationReceipt) Digest() identity.ContentID {
	if !receipt.Available() {
		return identity.ContentID{}
	}
	return receipt.digest
}

func (receipt CompilationReceipt) Version() uint64 {
	if !receipt.Available() {
		return 0
	}
	return receipt.version
}

// Schema is intentionally available only to sibling internal compiler code;
// the receipt itself remains the authority fence.
func (receipt CompilationReceipt) Schema() *engine.Schema {
	if !receipt.Available() {
		return nil
	}
	return receipt.catalog.schema
}

// QueryViews is the immutable typed projection needed by a later hot binder.
// It exposes no callbacks or mutable catalog state.
type QueryViews struct {
	valueQuery  *engine.QuerySlot[query.ValueSummaryObservation]
	valueRead   engine.SchemaReadForm[valuedomain.Value]
	effectQuery *engine.QuerySlot[query.EffectObservation]
	effectRead  engine.SchemaReadForm[effectfactor.Value]
}

func (receipt CompilationReceipt) Queries() (QueryViews, bool) {
	if !receipt.Available() || receipt.catalog.valueQuery == nil || receipt.catalog.effectQuery == nil {
		return QueryViews{}, false
	}
	return QueryViews{valueQuery: receipt.catalog.valueQuery, valueRead: receipt.catalog.valueRead, effectQuery: receipt.catalog.effectQuery, effectRead: receipt.catalog.effectRead}, true
}

func (views QueryViews) Value() (*engine.QuerySlot[query.ValueSummaryObservation], engine.SchemaReadForm[valuedomain.Value], bool) {
	return views.valueQuery, views.valueRead, views.valueQuery != nil && views.valueRead.Schema() != nil
}

func (views QueryViews) Effect() (*engine.QuerySlot[query.EffectObservation], engine.SchemaReadForm[effectfactor.Value], bool) {
	return views.effectQuery, views.effectRead, views.effectQuery != nil && views.effectRead.Schema() != nil
}

var global struct {
	once    sync.Once
	receipt CompilationReceipt
	ok      bool
}

func Global() (CompilationReceipt, bool) {
	global.once.Do(func() { global.receipt, global.ok = build() })
	return global.receipt, global.ok
}

func build() (CompilationReceipt, bool) {
	v, ok := vocabulary.New()
	if !ok {
		return CompilationReceipt{}, false
	}
	// The declaration table seals before any schema slot exists, so a rule
	// inventory that violates its own laws never reaches the schema builder.
	if _, failure := Table(); failure.Available() {
		return CompilationReceipt{}, false
	}
	builder := engine.NewSchema()
	// Every axis's cold shape is recorded by one pass over the sealed table,
	// before the rule pass: a rule declares against the principals the axis
	// pass produces.
	axisFragments, _, ok := declareAxes(builder, v)
	if !ok {
		return CompilationReceipt{}, false
	}
	owners, ok := axisFragments.coldPrincipals()
	if !ok {
		return CompilationReceipt{}, false
	}
	// Every rule's cold shape is recorded by one pass over the sealed table,
	// in the table's canonical order.
	fragments, _, ok := declareRules(builder, v, owners)
	if !ok {
		return CompilationReceipt{}, false
	}
	// Query payloads are deliberately marker-free schema slots. Their typed
	// hot projectors belong to analysis binding, while these two cold query
	// identities remain part of this sole schema owner.
	valueRead := owners.value.FoldSummaryRead()
	effectRead := owners.effect.ExactRead()
	if valueRead.Schema() != nil || effectRead.Schema() != nil {
		return CompilationReceipt{}, false
	}
	valueQuery, ok := engine.NewQuerySlot[query.ValueSummaryObservation](builder, engine.SchemaQuerySpec{Semantic: v.ValueQuery, Freezer: v.ValueCodec})
	if !ok || !engine.SchemaQueryRead(valueQuery, valueRead) {
		return CompilationReceipt{}, false
	}
	effectQuery, ok := engine.NewQuerySlot[query.EffectObservation](builder, engine.SchemaQuerySpec{Semantic: v.EffectQuery, Freezer: v.EffectCodec})
	if !ok || !engine.SchemaQueryRead(effectQuery, effectRead) {
		return CompilationReceipt{}, false
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil || !schema.Available() {
		return CompilationReceipt{}, false
	}
	digest := identity.ContentID(schema.ID().Digest())
	if !digest.Available() {
		return CompilationReceipt{}, false
	}
	receipt := CompilationReceipt{
		catalog: &catalog{
			schema:        schema,
			axisFragments: axisFragments,
			ruleFragments: fragments,
			valueQuery:    valueQuery,
			valueRead:     valueRead,
			effectQuery:   effectQuery,
			effectRead:    effectRead,
		},
		digest:  digest,
		version: ReceiptVersion,
	}
	return receipt, receipt.Available()
}
