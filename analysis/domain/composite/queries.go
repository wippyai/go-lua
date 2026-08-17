package composite

import (
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// QueryViews is the sealed schema owner's typed query surface and the
// immutable projection a later hot binder consumes. It exposes no callbacks or
// mutable catalog state, and it is the catalog's one domain-typed composition
// so the package root names no semantic domain.
type QueryViews struct {
	valueQuery  *engine.QuerySlot[valuedomain.ValueSummaryObservation]
	valueRead   engine.SchemaReadForm[valuedomain.Value]
	effectQuery *engine.QuerySlot[effectfactor.EffectObservation]
	effectRead  engine.SchemaReadForm[effectfactor.Value]
}

func (views QueryViews) available() bool {
	return views.valueQuery != nil && views.effectQuery != nil
}

func (views QueryViews) Value() (*engine.QuerySlot[valuedomain.ValueSummaryObservation], engine.SchemaReadForm[valuedomain.Value], bool) {
	return views.valueQuery, views.valueRead, views.valueQuery != nil && views.valueRead.Schema() != nil
}

func (views QueryViews) Effect() (*engine.QuerySlot[effectfactor.EffectObservation], engine.SchemaReadForm[effectfactor.Value], bool) {
	return views.effectQuery, views.effectRead, views.effectQuery != nil && views.effectRead.Schema() != nil
}

func (compilation Compilation) Queries() (QueryViews, bool) {
	if !compilation.Available() || !compilation.catalog.queries.available() {
		return QueryViews{}, false
	}
	return compilation.catalog.queries, true
}

// declareQueries opens the two cold query slots against the axis principals.
// Query payloads are deliberately marker-free schema slots. Their typed hot
// projectors belong to analysis binding, while these two cold query identities
// remain part of this sole schema owner.
func declareQueries(builder *engine.SchemaBuilder, v vocabulary.Bundle, owners principals) (QueryViews, bool) {
	valueRead := owners.value.FoldSummaryRead()
	effectRead := owners.effect.ExactRead()
	if valueRead.Schema() != nil || effectRead.Schema() != nil {
		return QueryViews{}, false
	}
	valueQuery, ok := engine.NewQuerySlot[valuedomain.ValueSummaryObservation](builder, engine.SchemaQuerySpec{Semantic: v.ValueQuery, Freezer: v.ValueCodec})
	if !ok || !engine.SchemaQueryRead(valueQuery, valueRead) {
		return QueryViews{}, false
	}
	effectQuery, ok := engine.NewQuerySlot[effectfactor.EffectObservation](builder, engine.SchemaQuerySpec{Semantic: v.EffectQuery, Freezer: v.EffectCodec})
	if !ok || !engine.SchemaQueryRead(effectQuery, effectRead) {
		return QueryViews{}, false
	}
	return QueryViews{valueQuery: valueQuery, valueRead: valueRead, effectQuery: effectQuery, effectRead: effectRead}, true
}
