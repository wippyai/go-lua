package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// bindReceiptExactQuery is the receipt compiler's exact-query join. It is
// intentionally private until the receipt Solver lane consumes it; keeping
// the join here prevents a caller from supplying a parallel declaration schema or a
// second projection plan.
func bindReceiptExactQuery[V, R any](compilation *receiptFactorCompilation, implementation *ExactQueryImplementation[V, R], identity equation.Query) (*receiptExactQueryRuntime[V, R], bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || compilation.runtime.mode != runtimeBindingReceipt || compilation.runtime.graph == nil || implementation == nil || !implementation.receipt.valid() || implementation.receipt.state != compilation.runtime.state || implementation.receipt.authority != compilation.runtime.authority || !compilation.runtime.graph.OwnsQuery(identity) || !identity.Key().Available() || !identity.Family().Available() || identity.Family() != implementation.receipt.state.schema.querySemanticAt(implementation.receipt.queryOrdinal) {
		return nil, false
	}
	shape, ok := implementation.receipt.state.schema.queryShapeAt(implementation.receipt.queryOrdinal)
	if !ok || shape.ProjectionCount != 1 {
		return nil, false
	}
	projection, ok := implementation.receipt.state.schema.queryProjectionShapeAt(implementation.receipt.queryOrdinal, 0)
	if !ok || projection.Kind != composition.QueryFactorExact || projection.Factor != implementation.receipt.factor {
		return nil, false
	}
	surfaces := identity.Surfaces()
	if len(surfaces) != 1 {
		return nil, false
	}
	surface := surfaces[0]
	if !surface.Available() || surface.Factor != implementation.receipt.factor || surface.Form != equation.SurfaceReadExact || surface.Local == 0 || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Mode != equation.TargetModeNone {
		return nil, false
	}
	runtime, ok := compilation.byKey[implementation.receipt.factor]
	if !ok || runtime == nil {
		return nil, false
	}
	factor, ok := runtime.(receiptQueryFactor[V])
	if !ok || !factor.receiptMatches(implementation.receipt.state, implementation.receipt.authority, implementation.receipt.factorOrdinal, implementation.receipt.factor) {
		return nil, false
	}
	unit, ok := factor.readUnit(surface)
	if !ok {
		return nil, false
	}
	return &receiptExactQueryRuntime[V, R]{identity: identity, receipt: implementation.receipt, factor: factor, surface: surface, unit: unit}, true
}

// bindReceiptSummaryQuery is the summary counterpart of bindReceiptExactQuery.
// It joins only the graph-owned summary surface and the exact sealed form
// normalizer; no read-form reconstruction is admitted.
func bindReceiptSummaryQuery[V, R any](compilation *receiptFactorCompilation, implementation *SummaryQueryImplementation[V, R], identity equation.Query) (*receiptSummaryQueryRuntime[V, R], bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || compilation.runtime.mode != runtimeBindingReceipt || compilation.runtime.graph == nil || implementation == nil || !implementation.receipt.valid() || implementation.receipt.state != compilation.runtime.state || implementation.receipt.authority != compilation.runtime.authority || !compilation.runtime.graph.OwnsQuery(identity) || !identity.Key().Available() || !identity.Family().Available() || identity.Family() != implementation.receipt.state.schema.querySemanticAt(implementation.receipt.queryOrdinal) {
		return nil, false
	}
	shape, ok := implementation.receipt.state.schema.queryShapeAt(implementation.receipt.queryOrdinal)
	if !ok || shape.ProjectionCount != 1 {
		return nil, false
	}
	projection, ok := implementation.receipt.state.schema.queryProjectionShapeAt(implementation.receipt.queryOrdinal, 0)
	if !ok || projection.Kind != composition.QueryFactorSummary || projection.Factor != implementation.receipt.factor || projection.Normalizer != implementation.receipt.normalizer {
		return nil, false
	}
	surfaces := identity.Surfaces()
	if len(surfaces) != 1 {
		return nil, false
	}
	surface := surfaces[0]
	if !surface.Available() || surface.Factor != implementation.receipt.factor || surface.Form != equation.SurfaceReadSummary || !surface.Semantic.Available() || surface.Semantic != implementation.receipt.normalizer || surface.Normalizer != implementation.receipt.normalizer || surface.Local == 0 || surface.Mode != equation.TargetModeNone {
		return nil, false
	}
	runtime, ok := compilation.byKey[implementation.receipt.factor]
	if !ok || runtime == nil {
		return nil, false
	}
	factor, ok := runtime.(receiptQueryFactor[V])
	if !ok || !factor.receiptMatches(implementation.receipt.state, implementation.receipt.authority, implementation.receipt.factorOrdinal, implementation.receipt.factor) {
		return nil, false
	}
	unit, ok := factor.readUnit(surface)
	if !ok {
		return nil, false
	}
	return &receiptSummaryQueryRuntime[V, R]{identity: identity, receipt: implementation.receipt, factor: factor, surface: surface, unit: unit}, true
}

func bindReceiptExactQueryRuntime[V, R any](compilation *receiptFactorCompilation, implementation *ExactQueryImplementation[V, R], identity equation.Query) (runtimeQuery, bool) {
	evidence, ok := bindReceiptExactQuery[V, R](compilation, implementation, identity)
	if !ok || evidence == nil {
		return nil, false
	}
	project, ok := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if !ok && !hasAccumulator || ok && hasAccumulator {
		return nil, false
	}
	return &receiptQueryRuntime[V, R]{identity: identity, owner: &receiptQueryOwner{state: implementation.receipt.state, authority: implementation.receipt.authority, schema: implementation.receipt.state.schema, ordinal: implementation.receipt.queryOrdinal}, factor: evidence.factor, surface: evidence.surface, unit: evidence.unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}, true
}

func bindReceiptSummaryQueryRuntime[V, R any](compilation *receiptFactorCompilation, implementation *SummaryQueryImplementation[V, R], identity equation.Query) (runtimeQuery, bool) {
	evidence, ok := bindReceiptSummaryQuery[V, R](compilation, implementation, identity)
	if !ok || evidence == nil {
		return nil, false
	}
	project, _ := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if project == nil && !hasAccumulator {
		return nil, false
	}
	return &receiptQueryRuntime[V, R]{identity: identity, owner: &receiptQueryOwner{state: implementation.receipt.state, authority: implementation.receipt.authority, schema: implementation.receipt.state.schema, ordinal: implementation.receipt.queryOrdinal}, factor: evidence.factor, surface: evidence.surface, unit: evidence.unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}, true
}
