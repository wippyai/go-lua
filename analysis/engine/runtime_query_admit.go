package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// declareMountedQuery states one mounted query row. The row is addressed by
// the mount and reusable artifact point ID; the dense Point address is
// resolved by the constructor, so no query ever names an equation reference.
func (implementation *SummaryQueryImplementation[V, R]) declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, id, mount, point identity.ContentID) (declaredQueryRow, *ruleSummaryMapping, bool) {
	return declareMountedQueryRow[V, R](state, authority, implementation, id, mount, point, composition.QueryFactorSummary)
}

func (implementation *SummaryQueryImplementation[V, R]) bindConstruction(construction *ProgramConstruction, id identity.ContentID) bool {
	return AttachSummaryQuery(construction, implementation, id)
}

func (implementation *ExactQueryImplementation[V, R]) declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, id, mount, point identity.ContentID) (declaredQueryRow, *ruleSummaryMapping, bool) {
	return declareMountedQueryRow[V, R](state, authority, implementation, id, mount, point, composition.QueryFactorExact)
}

func (implementation *ExactQueryImplementation[V, R]) bindConstruction(construction *ProgramConstruction, id identity.ContentID) bool {
	return AttachExactQuery(construction, implementation, id)
}

func declareMountedQueryRow[V, R any](state *schemaBindingState, authority *schemaBindingAuthority, implementation bindingQueryReceipt, id, mount, reusable identity.ContentID, kind composition.QueryProjectionKind) (declaredQueryRow, *ruleSummaryMapping, bool) {
	if state == nil || implementation == nil || !id.Available() || !mount.Available() || !reusable.Available() {
		return declaredQueryRow{}, nil, false
	}
	receiptState, receiptAuthority, family, ordinal, ok := implementation.boundTopologyQueryReceipt()
	if !ok || receiptState != state || receiptAuthority != authority || !family.Available() {
		return declaredQueryRow{}, nil, false
	}
	projection, ok := state.schema.queryProjectionShapeAt(ordinal, 0)
	if !ok || projection.Kind != kind || !projection.Factor.Available() {
		return declaredQueryRow{}, nil, false
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadExact, Local: 1}
	var summary *ruleSummaryMapping
	if kind == composition.QueryFactorSummary {
		surface.Form = equation.SurfaceReadSummary
		surface.Semantic = projection.Normalizer
		surface.Normalizer = projection.Normalizer
		summaryImplementation, summaryOK := implementation.(*SummaryQueryImplementation[V, R])
		if !summaryOK {
			return declaredQueryRow{}, nil, false
		}
		mapping, mappingOK := summaryImplementation.topologySummaryMapping(surface)
		if !mappingOK {
			return declaredQueryRow{}, nil, false
		}
		summary = &ruleSummaryMapping{receipt: summaryImplementation, surface: mapping.Surface, keys: mapping.Keys}
	}
	return declaredQueryRow{
		ID: id, Mount: mount, Point: reusable,
		Row: equation.QueryInstance{Family: family, Surfaces: []equation.Surface{surface}},
	}, summary, true
}
