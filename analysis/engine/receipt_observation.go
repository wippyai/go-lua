package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ReceiptObservation is one optional, solve-local, read-only projection. It is
// issued only while a ReceiptCompilation is open and is bound to an exact
// mounted graph point plus the sealed Factor/query implementation. Observation
// rows are not equation Queries: they never alter the reusable topology,
// query demand, schedule, or rule vocabulary. When present, their exact
// output Points are additional roots of this one Solver's demand closure;
// an observation-free Solver follows the original query-only closure.
type ReceiptObservation[R any] struct {
	owner   *receiptObservationOwner
	id      identity.ContentID
	ordinal uint64
}

// ReceiptObservationAttachFailure is the closed generic admission predicate
// for an optional observation. It contains no diagnostic rule or domain name.
type ReceiptObservationAttachFailure uint8

const (
	ReceiptObservationAttachFailureNone ReceiptObservationAttachFailure = iota
	ReceiptObservationAttachFailureArguments
	ReceiptObservationAttachFailureCompilation
	ReceiptObservationAttachFailureBinding
	ReceiptObservationAttachFailureProjection
	ReceiptObservationAttachFailurePoint
	ReceiptObservationAttachFailureMapping
	ReceiptObservationAttachFailureFactor
	ReceiptObservationAttachFailureUnit
	ReceiptObservationAttachFailureDuplicate
)

// Failure projects one observation attach boundary onto the engine's public
// failure vocabulary. The ordinal enters the site preimage and never leaves
// this package.
func (failure ReceiptObservationAttachFailure) Failure() SolveFailure {
	if failure == ReceiptObservationAttachFailureNone {
		return SolveFailure{}
	}
	return receiptFailure(SolveFailureFamilyObservation, "receipt-observation-attach", uint64(failure))
}

func (observation ReceiptObservation[R]) Available() bool {
	return observation.owner != nil && observation.id.Available() && observation.ordinal != ^uint64(0)
}

// MatchesID proves that id is the exact private observation identity issued
// by this handle. It deliberately exposes no observation payload, ordinal, or
// owner; outer detached receipts use it only to detect a provenance seal
// recomputed around a spliced Engine handle.
func (observation ReceiptObservation[R]) MatchesID(id identity.ContentID) bool {
	return observation.Available() && id.Available() && observation.id == id
}

func AttachRuleSummaryObservationWithFailure[V, R any](compilation *ReceiptCompilation, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, member ReceiptRuleMember) (ReceiptObservation[R], ReceiptObservationAttachFailure) {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || !compilation.graph.valid() || implementation == nil || !id.Available() || member.graph != compilation.graph || !compilation.graph.graph.OwnsMember(member.member) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureArguments
	}
	locator := member.locator
	resolvedMember, located := locator.Resolve(compilation.graph.graph)
	if !located || resolvedMember.Key() != member.member.Key() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureArguments
	}
	inner := compilation.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.mode != runtimeBindingReceipt || inner.runtime.graph != compilation.graph.graph || inner.byKey == nil || inner.observationIDs == nil {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureCompilation
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != inner.runtime.state || authority != inner.runtime.authority || !family.Available() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureBinding
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorSummary || !projection.Factor.Available() || !projection.Normalizer.Available() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureProjection
	}
	if inner.observationPoints == nil {
		var indexed bool
		inner.observationPoints, indexed = indexReceiptObservationPoints(compilation.graph.graph)
		if !indexed {
			return ReceiptObservation[R]{}, ReceiptObservationAttachFailurePoint
		}
	}
	point, resolved := inner.observationPoints[member.member.Key()]
	if !resolved || !compilation.graph.graph.OwnsPoint(point) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailurePoint
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadSummary, Semantic: projection.Normalizer, Normalizer: projection.Normalizer, Local: 1}
	if _, mappingOK := implementation.topologySummaryMapping(surface); !mappingOK {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureMapping
	}
	factorRuntime, factorOK := inner.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.receipt.factorOrdinal, projection.Factor) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureFactor
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureUnit
	}
	if _, duplicate := inner.observationIDs[id]; duplicate || uint64(len(inner.observations)) == ^uint64(0) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureDuplicate
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	ordinal := uint64(len(inner.observations))
	runtime := &receiptSummaryObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}
	inner.observations = append(inner.observations, runtime)
	inner.observationIDs[id] = struct{}{}
	inner.observationBuilders = append(inner.observationBuilders, func(next *receiptFactorCompilation) (runtimeObservation, bool) {
		resolved, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindReceiptSummaryObservationRuntime(next, implementation, id, resolved, owner)
	})
	return ReceiptObservation[R]{owner: owner, id: id, ordinal: ordinal}, ReceiptObservationAttachFailureNone
}

// bindReceiptSummaryObservationRuntime rebuilds one optional observation for
// an activation revision. It is the receipt counterpart to the member/query
// rebinding closures retained by receiptSolverCompiler.
func bindReceiptSummaryObservationRuntime[V, R any](compilation *receiptFactorCompilation, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, owner *receiptObservationOwner) (runtimeObservation, bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || compilation.runtime.mode != runtimeBindingReceipt || implementation == nil || owner == nil || !id.Available() || !compilation.runtime.graph.OwnsMember(member) {
		return nil, false
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != compilation.runtime.state || authority != compilation.runtime.authority || !family.Available() || owner.state != state || owner.authority != authority || owner.schema != state.schema || owner.query != queryOrdinal {
		return nil, false
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorSummary || !projection.Factor.Available() || !projection.Normalizer.Available() {
		return nil, false
	}
	if compilation.observationPoints == nil {
		var indexed bool
		compilation.observationPoints, indexed = indexReceiptObservationPoints(compilation.runtime.graph)
		if !indexed {
			return nil, false
		}
	}
	point, resolved := compilation.observationPoints[member.Key()]
	if !resolved || !compilation.runtime.graph.OwnsPoint(point) {
		return nil, false
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadSummary, Semantic: projection.Normalizer, Normalizer: projection.Normalizer, Local: 1}
	if _, mappingOK := implementation.topologySummaryMapping(surface); !mappingOK {
		return nil, false
	}
	factorRuntime, factorOK := compilation.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.receipt.factorOrdinal, projection.Factor) {
		return nil, false
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return nil, false
	}
	return &receiptSummaryObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}, true
}

// AttachRuleExactObservation binds an optional, owner-fenced projection to
// the exact output of an already committed Rule member. The member fixes the
// output point; callers cannot provide a point, factor coordinate, atom, or
// domain-specific selector. As with summary observations, this adds only a
// solve-local demand root and does not alter reusable topology.
func AttachRuleExactObservation[V, R any](compilation *ReceiptCompilation, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member ReceiptRuleMember) (ReceiptObservation[R], bool) {
	observation, failure := AttachRuleExactObservationWithFailure(compilation, implementation, id, member)
	return observation, failure == ReceiptObservationAttachFailureNone
}

func AttachRuleExactObservationWithFailure[V, R any](compilation *ReceiptCompilation, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member ReceiptRuleMember) (ReceiptObservation[R], ReceiptObservationAttachFailure) {
	if compilation == nil || compilation.inner == nil || compilation.graph == nil || !compilation.graph.valid() || implementation == nil || !id.Available() || member.graph != compilation.graph || !compilation.graph.graph.OwnsMember(member.member) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureArguments
	}
	locator := member.locator
	resolvedMember, located := locator.Resolve(compilation.graph.graph)
	if !located || resolvedMember.Key() != member.member.Key() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureArguments
	}
	inner := compilation.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.mode != runtimeBindingReceipt || inner.runtime.graph != compilation.graph.graph || inner.byKey == nil || inner.observationIDs == nil {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureCompilation
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != inner.runtime.state || authority != inner.runtime.authority || !family.Available() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureBinding
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorExact || !projection.Factor.Available() || projection.Normalizer.Available() {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureProjection
	}
	if inner.observationPoints == nil {
		var indexed bool
		inner.observationPoints, indexed = indexReceiptObservationPoints(compilation.graph.graph)
		if !indexed {
			return ReceiptObservation[R]{}, ReceiptObservationAttachFailurePoint
		}
	}
	point, resolved := inner.observationPoints[member.member.Key()]
	if !resolved || !compilation.graph.graph.OwnsPoint(point) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailurePoint
	}
	surface, surfaceOK := exactObservationReadSurface(resolvedMember, projection.Factor)
	if !surfaceOK {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureMapping
	}
	factorRuntime, factorOK := inner.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.receipt.factorOrdinal, projection.Factor) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureFactor
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureUnit
	}
	if _, duplicate := inner.observationIDs[id]; duplicate || uint64(len(inner.observations)) == ^uint64(0) {
		return ReceiptObservation[R]{}, ReceiptObservationAttachFailureDuplicate
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	ordinal := uint64(len(inner.observations))
	runtime := &receiptExactObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}
	inner.observations = append(inner.observations, runtime)
	inner.observationIDs[id] = struct{}{}
	inner.observationBuilders = append(inner.observationBuilders, func(next *receiptFactorCompilation) (runtimeObservation, bool) {
		resolved, ok := locator.Resolve(next.runtime.graph)
		if !ok {
			return nil, false
		}
		return bindReceiptExactObservationRuntime(next, implementation, id, resolved, owner)
	})
	return ReceiptObservation[R]{owner: owner, id: id, ordinal: ordinal}, ReceiptObservationAttachFailureNone
}

// bindReceiptExactObservationRuntime rebuilds an exact rule observation for a
// later activation revision using the same committed member locator and
// sealed query implementation. It admits no caller-supplied point or factor.
func bindReceiptExactObservationRuntime[V, R any](compilation *receiptFactorCompilation, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, owner *receiptObservationOwner) (runtimeObservation, bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || compilation.runtime.mode != runtimeBindingReceipt || implementation == nil || owner == nil || !id.Available() || !compilation.runtime.graph.OwnsMember(member) {
		return nil, false
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != compilation.runtime.state || authority != compilation.runtime.authority || !family.Available() || owner.state != state || owner.authority != authority || owner.schema != state.schema || owner.query != queryOrdinal {
		return nil, false
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorExact || !projection.Factor.Available() || projection.Normalizer.Available() {
		return nil, false
	}
	if compilation.observationPoints == nil {
		var indexed bool
		compilation.observationPoints, indexed = indexReceiptObservationPoints(compilation.runtime.graph)
		if !indexed {
			return nil, false
		}
	}
	point, resolved := compilation.observationPoints[member.Key()]
	if !resolved || !compilation.runtime.graph.OwnsPoint(point) {
		return nil, false
	}
	surface, surfaceOK := exactObservationReadSurface(member, projection.Factor)
	if !surfaceOK {
		return nil, false
	}
	factorRuntime, factorOK := compilation.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.receipt.factorOrdinal, projection.Factor) {
		return nil, false
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return nil, false
	}
	return &receiptExactObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, result: implementation.receipt.cell.result}, true
}
