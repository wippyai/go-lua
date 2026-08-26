package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/population"
)

// declareMountedQuery states one mounted query row. The row is addressed by
// the mount and reusable artifact point ID; the dense point handle is resolved
// only when runtimeProgram is built.
func (implementation *SummaryQueryImplementation[V, R]) declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, context executioncontext.Context, id, mount, point identity.ContentID) (declaredQueryRow, []*ruleSummaryMapping, bool) {
	row, ok := implementation.sealedRow()
	if !ok || row.state != state || state.authority != authority || !context.Available() {
		return declaredQueryRow{}, nil, false
	}
	declared, surface, ok := declareMountedQueryRow(state, row.ordinal, context, id, mount, point, composition.QueryFactorSummary)
	if !ok {
		return declaredQueryRow{}, nil, false
	}
	mapping, ok := implementation.topologySummaryMapping(surface)
	if !ok {
		return declaredQueryRow{}, nil, false
	}
	projection, _ := state.schema.queryProjectionShapeAt(row.ordinal, 0)
	return declared, []*ruleSummaryMapping{{
		state: state, authority: authority, factor: projection.Factor, normalizer: projection.Normalizer,
		surface: mapping.Surface, keys: newSummaryKeyVector(mapping.Keys),
	}}, true
}

func (implementation *ExactQueryImplementation[V, R]) declareMountedQuery(state *schemaBindingState, authority *schemaBindingAuthority, context executioncontext.Context, id, mount, point identity.ContentID) (declaredQueryRow, []*ruleSummaryMapping, bool) {
	row, ok := implementation.sealedRow()
	if !ok || row.state != state || state.authority != authority || !context.Available() {
		return declaredQueryRow{}, nil, false
	}
	declared, _, ok := declareMountedQueryRow(state, row.ordinal, context, id, mount, point, composition.QueryFactorExact)
	return declared, nil, ok
}

func declareMountedQueryRow(state *schemaBindingState, ordinal uint64, context executioncontext.Context, id, mount, reusable identity.ContentID, kind composition.QueryProjectionKind) (declaredQueryRow, equation.Surface, bool) {
	if state == nil || state.schema == nil || !context.Available() || !id.Available() || !mount.Available() || !reusable.Available() || ordinal >= state.schema.queryCount() {
		return declaredQueryRow{}, equation.Surface{}, false
	}
	family := state.schema.querySemanticAt(ordinal)
	projection, ok := state.schema.queryProjectionShapeAt(ordinal, 0)
	shape, shapeOK := state.schema.queryShapeAt(ordinal)
	if !ok || !shapeOK || shape.Population != population.SelectedPoint || !family.Available() || projection.Kind != kind || !projection.Factor.Available() {
		return declaredQueryRow{}, equation.Surface{}, false
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadExact, Local: 1}
	if kind == composition.QueryFactorSummary {
		surface.Form = equation.SurfaceReadSummary
		surface.Semantic = projection.Normalizer
		surface.Normalizer = projection.Normalizer
	}
	if !surface.Available() {
		return declaredQueryRow{}, equation.Surface{}, false
	}
	return declaredQueryRow{
		Context: context, ID: id, Mount: mount, Point: reusable,
		Row: equation.QueryInstance{Context: context.ID(), Family: family, Surfaces: []equation.Surface{surface}},
	}, surface, true
}

func (implementation *SummaryQueryImplementation[V, R]) bindProgramQuery(plane *programPlane, query equation.Query) (queryRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok {
		return queryRow{}, false
	}
	return bindProgramQueryRow(plane, query, row.state, row.ordinal, row.factorOrdinal, composition.QueryFactorSummary, row.projection.materialize)
}

func (implementation *ExactQueryImplementation[V, R]) bindProgramQuery(plane *programPlane, query equation.Query) (queryRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok {
		return queryRow{}, false
	}
	return bindProgramQueryRow(plane, query, row.state, row.ordinal, row.factorOrdinal, composition.QueryFactorExact, row.projection.materialize)
}

// bindProgramQueryRow lowers one graph query to direct Schema ordinals and
// store-local handles. Binding-state identity is checked here once and is not
// retained by the live row.
func bindProgramQueryRow(plane *programPlane, query equation.Query, state *schemaBindingState, queryOrdinal, factorOrdinal uint64, kind composition.QueryProjectionKind, exec queryExec) (queryRow, bool) {
	if plane == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || state == nil || state != plane.runtime.state || state.authority != plane.runtime.authority || !plane.runtime.graph.OwnsQuery(query) || !query.Key().Available() || exec == nil || factorOrdinal >= uint64(len(plane.factors)) {
		return queryRow{}, false
	}
	schema := state.schema
	shape, shapeOK := schema.queryShapeAt(queryOrdinal)
	projection, projectionOK := schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !shapeOK || shape.Population != population.SelectedPoint || !projectionOK || shape.ProjectionCount != 1 || projection.Kind != kind || projection.Factor != schema.factorSemanticAt(factorOrdinal) || query.Family() != schema.querySemanticAt(queryOrdinal) {
		return queryRow{}, false
	}
	surfaces := query.Surfaces()
	if len(surfaces) != 1 || !validProgramQuerySurface(surfaces[0], projection) {
		return queryRow{}, false
	}
	factor := plane.factors[factorOrdinal]
	unit, unitOK := factor.readUnit(surfaces[0])
	point, pointOK := plane.runtime.graph.PointIndex(query.Point())
	if !unitOK || !pointOK {
		return queryRow{}, false
	}
	stateOrdinal, stateOK := plane.queryState(query)
	if !stateOK {
		return queryRow{}, false
	}
	row := queryRow{queryOrdinal: queryOrdinal, factorOrdinal: factorOrdinal, point: int32(point), state: stateOrdinal, unit: unit, exec: exec}
	return row, row.valid()
}

func validProgramQuerySurface(surface equation.Surface, projection composition.QueryProjectionShape) bool {
	if !surface.Available() || surface.Factor != projection.Factor || surface.Mode != equation.TargetModeNone {
		return false
	}
	switch projection.Kind {
	case composition.QueryFactorExact:
		return surface.Form == equation.SurfaceReadExact && surface.Local != 0 && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.QueryFactorSummary:
		return surface.Form == equation.SurfaceReadSummary && surface.Semantic == projection.Normalizer && surface.Normalizer == projection.Normalizer
	default:
		return false
	}
}
