package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

// receiptQueryRuntime is the common solver query implementation for both
// catalog families. The only difference between the summary family and the
// exact one is the sealed Factor surface carried by the typed implementation.
type receiptQueryFactor[V any] interface {
	runtimeFactor
	receiptMatches(*schemaBindingState, *schemaBindingAuthority, uint64, composition.Key) bool
	stagedObserve(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) bool
}

type detailedReceiptQueryFactor[V any] interface {
	receiptQueryFactor[V]
	stagedObserveWithFailure(*carrier.Work, carrier.State, carrier.Unit, support.Mask, func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool)
}

type receiptQueryRuntime[V, R any] struct {
	identity equation.Query
	owner    *receiptQueryOwner
	factor   receiptQueryFactor[V]
	surface  equation.Surface
	unit     carrier.Unit
	project  func(OrderedCells[V]) R
	begin    func() R
	accum    func(R, OrderedCells[V]) (R, bool)
	result   FrozenResult[R]
}

func (runtime *receiptQueryRuntime[V, R]) query() equation.Query {
	if runtime == nil {
		return equation.Query{}
	}
	return runtime.identity
}

func (runtime *receiptQueryRuntime[V, R]) queryOwner() queryOwner {
	if runtime == nil {
		return nil
	}
	return runtime.owner
}

func (runtime *receiptQueryRuntime[V, R]) PublicationKey() (identity.ContentID, bool) {
	if runtime == nil {
		return identity.ContentID{}, false
	}
	key := solvedRowKey(runtime.identity.Key())
	return key, key.Available()
}

func (runtime *receiptQueryRuntime[V, R]) materialize(work *carrier.Work, state carrier.State) (*queryResult, bool) {
	if runtime == nil || runtime.owner == nil || runtime.factor == nil || (runtime.project == nil && (runtime.begin == nil || runtime.accum == nil)) || !validFrozenResult(runtime.result) || work == nil || !work.Checkpoint() || runtime.owner.state == nil || runtime.owner.authority == nil || runtime.owner.state.phase != schemaBindingSealed || runtime.owner.state.authority != runtime.owner.authority {
		return nil, false
	}
	frozen, ok := materializeReceiptProjection(work, state, runtime.owner.state, runtime.owner.authority, runtime.factor, runtime.unit, runtime.project, runtime.begin, runtime.accum, runtime.result)
	if !ok {
		return nil, false
	}
	return &queryResult{owner: runtime.owner, key: runtime.identity.Key(), value: frozen}, true
}

// materializeReceiptProjection is the sole typed execution path for both
// committed graph Queries and optional solve-local observations. The latter
// changes only when the projection is requested; Factor reads, correlation,
// freezing, and checkpoint discipline remain identical.
func materializeReceiptProjection[V, R any](work *carrier.Work, state carrier.State, binding *schemaBindingState, authority *schemaBindingAuthority, factor receiptQueryFactor[V], unit carrier.Unit, project func(OrderedCells[V]) R, begin func() R, accum func(R, OrderedCells[V]) (R, bool), result FrozenResult[R]) (frozenValue, bool) {
	value, _, ok := materializeReceiptProjectionWithFailure(work, state, binding, authority, factor, unit, project, begin, accum, result)
	return value, ok
}

func materializeReceiptProjectionWithFailure[V, R any](work *carrier.Work, state carrier.State, binding *schemaBindingState, authority *schemaBindingAuthority, factor receiptQueryFactor[V], unit carrier.Unit, project func(OrderedCells[V]) R, begin func() R, accum func(R, OrderedCells[V]) (R, bool), result FrozenResult[R]) (frozenValue, solveBoundary, bool) {
	if factor == nil || (project == nil && (begin == nil || accum == nil)) || !validFrozenResult(result) || work == nil || !work.Checkpoint() || binding == nil || authority == nil || binding.phase != schemaBindingSealed || binding.authority != authority {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	var value R
	if begin != nil {
		value = begin()
	}
	observations := 0
	projectionFailure := boundaryNone
	visit := func(observation factbinding.Observation[V], _ support.Mask) bool {
		cells, ok := orderedCellsFromObservation(observation)
		if !ok {
			projectionFailure = refused(SolveFailureFamilyObservation, "shape")
			return false
		}
		observations++
		if accum != nil {
			next, ok := accum(value, cells)
			if !ok {
				projectionFailure = refused(SolveFailureFamilyObservation, "projection")
				return false
			}
			value = next
			if !work.Checkpoint() {
				projectionFailure = refused(SolveFailureFamilyObservation, "preflight")
				return false
			}
			return true
		}
		value = project(cells)
		if !work.Checkpoint() {
			projectionFailure = refused(SolveFailureFamilyObservation, "preflight")
			return false
		}
		return true
	}
	valid := false
	failure := stagedObservationFailureNone
	if detailed, ok := factor.(detailedReceiptQueryFactor[V]); ok {
		failure, valid = detailed.stagedObserveWithFailure(work, state, unit, state.Support(), visit)
	} else {
		valid = factor.stagedObserve(work, state, unit, state.Support(), visit)
	}
	if !valid || (accum == nil && observations > 1) || !work.Checkpoint() {
		boundary := refused(SolveFailureFamilyObservation, "projection")
		if projectionFailure != boundaryNone {
			boundary = projectionFailure
		} else if failure == stagedObservationFailureArguments {
			boundary = refused(SolveFailureFamilyObservation, "preflight")
		} else if failure == stagedObservationFailureCheckpoint || failure == stagedObservationFailureSlot || failure == stagedObservationFailureWork {
			boundary = refused(SolveFailureFamilyObservation, "work")
		} else if failure == stagedObservationFailureUnit {
			boundary = refused(SolveFailureFamilyObservation, "unit")
		} else if failure == stagedObservationFailureSupport {
			boundary = refused(SolveFailureFamilyObservation, "support")
		} else if failure == stagedObservationFailureRoot {
			boundary = refused(SolveFailureFamilyObservation, "root")
		} else if failure == stagedObservationFailureCarrier {
			boundary = refused(SolveFailureFamilyObservation, "carrier")
		} else if failure == stagedObservationFailureDecode {
			boundary = refused(SolveFailureFamilyObservation, "decode")
		}
		return nil, boundary, false
	}
	if observations == 0 && begin == nil {
		// A Project callback has no lawful empty-row contract. Empty
		// observations must be represented by a typed Begin fold state.
		return nil, refused(SolveFailureFamilyObservation, "shape"), false
	}
	frozen := result.Freeze(value)
	if !work.Checkpoint() {
		return nil, refused(SolveFailureFamilyObservation, "freeze"), false
	}
	return &typedFrozenValue[R]{value: frozen, freeze: result}, boundaryNone, true
}

func orderedCellsFromObservation[V any](observation factbinding.Observation[V]) (OrderedCells[V], bool) {
	if !observation.Valid() {
		return OrderedCells[V]{}, false
	}
	cells := make([]summaryCell[V], observation.Count())
	for index := range cells {
		entry, ok := observation.At(index)
		if !ok {
			return OrderedCells[V]{}, false
		}
		value, present := entry.Read()
		cells[index] = summaryCell[V]{value: value, present: present}
	}
	record := newOrderedCellsRecord(cells)
	return OrderedCells[V]{record: record}, true
}

// The plane-bound query binds. They are the whole join between a sealed typed
// query implementation and one graph's Query identity, and an activation
// revision replays them against the plane of a later graph, so they live with
// the query runtime they produce rather than with the construction that first
// called them.
// bindReceiptExactQuery is the receipt compiler's exact-query join. It is
// intentionally private until the receipt Solver lane consumes it; keeping
// the join here prevents a caller from supplying a parallel declaration schema or a
// second projection plan.
func bindReceiptExactQuery[V, R any](compilation *programPlane, implementation *ExactQueryImplementation[V, R], identity equation.Query) (*receiptExactQueryRuntime[V, R], bool) {
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
func bindReceiptSummaryQuery[V, R any](compilation *programPlane, implementation *SummaryQueryImplementation[V, R], identity equation.Query) (*receiptSummaryQueryRuntime[V, R], bool) {
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
	if !surface.Available() || surface.Factor != implementation.receipt.factor || surface.Form != equation.SurfaceReadSummary || !surface.Semantic.Available() || surface.Semantic != implementation.receipt.normalizer || surface.Normalizer != implementation.receipt.normalizer || surface.Mode != equation.TargetModeNone {
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

func bindReceiptExactQueryRuntime[V, R any](compilation *programPlane, implementation *ExactQueryImplementation[V, R], identity equation.Query) (runtimeQuery, bool) {
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

func bindReceiptSummaryQueryRuntime[V, R any](compilation *programPlane, implementation *SummaryQueryImplementation[V, R], identity equation.Query) (runtimeQuery, bool) {
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
