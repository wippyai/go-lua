package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ProgramObservation is the construction-plane observation handle.
type ProgramObservation[R any] struct {
	observation receiptObservation[R]
}

func (observation ProgramObservation[R]) Available() bool {
	return observation.observation.Available()
}

func (observation ProgramObservation[R]) SealedAs(id identity.ContentID) bool {
	return observation.observation.MatchesID(id)
}

// AttachMountedSummary binds one summary observation to the mounted member
// at the authored coordinates and reports SolveFailure.
func AttachMountedSummary[V, R any](compilation *ProgramConstruction, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramObservation[R], SolveFailure) {
	observation, failure := AttachMountedSummaryObservationWithFailure(compilation, implementation, id, role, mount, point, occurrence)
	return ProgramObservation[R]{observation: observation}, failure.Failure()
}

// AttachMountedExact binds one exact observation to the mounted member at
// the authored coordinates and reports SolveFailure.
func AttachMountedExact[V, R any](compilation *ProgramConstruction, implementation *ExactQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (ProgramObservation[R], SolveFailure) {
	if compilation == nil || compilation.committed == nil {
		return ProgramObservation[R]{}, ObservationAttachArguments()
	}
	member, memberOK := compilation.committed.MountedRuleMember(role, mount, point, occurrence)
	if !memberOK {
		return ProgramObservation[R]{}, ObservationAttachPoint()
	}
	observation, failure := AttachRuleExactObservationWithFailure(compilation, implementation, id, member)
	return ProgramObservation[R]{observation: observation}, failure.Failure()
}

type receiptObservation[R any] struct {
	owner   *receiptObservationOwner
	id      identity.ContentID
	ordinal uint64
}

// receiptObservationAttachFailure is the closed generic admission predicate
// for an optional observation. It contains no diagnostic rule or domain name.
type receiptObservationAttachFailure uint8

const (
	receiptObservationAttachFailureNone receiptObservationAttachFailure = iota
	receiptObservationAttachFailureArguments
	receiptObservationAttachFailureCompilation
	receiptObservationAttachFailureBinding
	receiptObservationAttachFailureProjection
	receiptObservationAttachFailurePoint
	receiptObservationAttachFailureMapping
	receiptObservationAttachFailureFactor
	receiptObservationAttachFailureUnit
	receiptObservationAttachFailureDuplicate
)

func (failure receiptObservationAttachFailure) Failure() SolveFailure {
	if failure == receiptObservationAttachFailureNone {
		return SolveFailure{}
	}
	return receiptFailure(SolveFailureFamilyObservation, "receipt-observation-attach", uint64(failure))
}

func (observation receiptObservation[R]) Available() bool {
	return observation.owner != nil && observation.id.Available() && observation.ordinal != ^uint64(0)
}

func (observation receiptObservation[R]) MatchesID(id identity.ContentID) bool {
	return observation.Available() && id.Available() && observation.id == id
}

func AttachMountedSummaryObservationWithFailure[V, R any](compilation *ProgramConstruction, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (receiptObservation[R], receiptObservationAttachFailure) {
	if compilation == nil || compilation.committed == nil {
		return receiptObservation[R]{}, receiptObservationAttachFailureArguments
	}
	member, memberOK := compilation.committed.MountedRuleMember(role, mount, point, occurrence)
	if !memberOK {
		return receiptObservation[R]{}, receiptObservationAttachFailurePoint
	}
	return AttachRuleSummaryObservationWithFailure(compilation, implementation, id, member)
}

func AttachRuleSummaryObservationWithFailure[V, R any](compilation *ProgramConstruction, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, member ProgramMember) (receiptObservation[R], receiptObservationAttachFailure) {
	if compilation == nil || compilation.inner == nil || compilation.committed == nil || !compilation.committed.valid() || implementation == nil || !id.Available() || !member.ownedBy(compilation.committed.graph, compilation.committed.topology) || !compilation.committed.graph.OwnsMember(member.member) {
		return receiptObservation[R]{}, receiptObservationAttachFailureArguments
	}
	locator := member.locator
	resolvedMember, located := locator.Resolve(compilation.committed.graph)
	if !located || resolvedMember.Key() != member.member.Key() {
		return receiptObservation[R]{}, receiptObservationAttachFailureArguments
	}
	inner := compilation.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.graph != compilation.committed.graph || inner.byKey == nil || inner.observationIDs == nil {
		return receiptObservation[R]{}, receiptObservationAttachFailureCompilation
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != inner.runtime.state || authority != inner.runtime.authority || !family.Available() {
		return receiptObservation[R]{}, receiptObservationAttachFailureBinding
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorSummary || !projection.Factor.Available() || !projection.Normalizer.Available() {
		return receiptObservation[R]{}, receiptObservationAttachFailureProjection
	}
	if inner.observationPoints == nil {
		var indexed bool
		inner.observationPoints, indexed = indexReceiptObservationPoints(compilation.committed.graph)
		if !indexed {
			return receiptObservation[R]{}, receiptObservationAttachFailurePoint
		}
	}
	point, resolved := inner.observationPoints[member.member.Key()]
	if !resolved || !compilation.committed.graph.OwnsPoint(point) {
		return receiptObservation[R]{}, receiptObservationAttachFailurePoint
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadSummary, Semantic: projection.Normalizer, Normalizer: projection.Normalizer, Local: 1}
	if _, mappingOK := implementation.topologySummaryMapping(surface); !mappingOK {
		return receiptObservation[R]{}, receiptObservationAttachFailureMapping
	}
	factorRuntime, factorOK := inner.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.binding.factorOrdinal, projection.Factor) {
		return receiptObservation[R]{}, receiptObservationAttachFailureFactor
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, borrow, transfer, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return receiptObservation[R]{}, receiptObservationAttachFailureUnit
	}
	if _, duplicate := inner.observationIDs[id]; duplicate || uint64(len(inner.observations)) == ^uint64(0) {
		return receiptObservation[R]{}, receiptObservationAttachFailureDuplicate
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	ordinal := uint64(len(inner.observations))
	runtime := &receiptSummaryObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, borrow: borrow, transfer: transfer, result: implementation.binding.cell.result}
	inner.observations = append(inner.observations, runtime)
	inner.observationIDs[id] = struct{}{}
	return receiptObservation[R]{owner: owner, id: id, ordinal: ordinal}, receiptObservationAttachFailureNone
}

func AttachMountedExactObservation[V, R any](compilation *ProgramConstruction, implementation *ExactQueryImplementation[V, R], id identity.ContentID, role RuleSlotCapability, mount, point, occurrence identity.ContentID) (receiptObservation[R], bool) {
	if compilation == nil || compilation.committed == nil {
		return receiptObservation[R]{}, false
	}
	member, memberOK := compilation.committed.MountedRuleMember(role, mount, point, occurrence)
	if !memberOK {
		return receiptObservation[R]{}, false
	}
	return AttachRuleExactObservation(compilation, implementation, id, member)
}

func AttachRuleExactObservation[V, R any](compilation *ProgramConstruction, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member ProgramMember) (receiptObservation[R], bool) {
	observation, failure := AttachRuleExactObservationWithFailure(compilation, implementation, id, member)
	return observation, failure == receiptObservationAttachFailureNone
}

func AttachRuleExactObservationWithFailure[V, R any](compilation *ProgramConstruction, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member ProgramMember) (receiptObservation[R], receiptObservationAttachFailure) {
	if compilation == nil || compilation.inner == nil || compilation.committed == nil || !compilation.committed.valid() || implementation == nil || !id.Available() || !member.ownedBy(compilation.committed.graph, compilation.committed.topology) || !compilation.committed.graph.OwnsMember(member.member) {
		return receiptObservation[R]{}, receiptObservationAttachFailureArguments
	}
	locator := member.locator
	resolvedMember, located := locator.Resolve(compilation.committed.graph)
	if !located || resolvedMember.Key() != member.member.Key() {
		return receiptObservation[R]{}, receiptObservationAttachFailureArguments
	}
	inner := compilation.inner
	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.closed || !inner.frozen || inner.runtime == nil || inner.runtime.graph != compilation.committed.graph || inner.byKey == nil || inner.observationIDs == nil {
		return receiptObservation[R]{}, receiptObservationAttachFailureCompilation
	}
	state, authority, family, queryOrdinal, receiptOK := implementation.boundTopologyQueryReceipt()
	if !receiptOK || state != inner.runtime.state || authority != inner.runtime.authority || !family.Available() {
		return receiptObservation[R]{}, receiptObservationAttachFailureBinding
	}
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !projectionOK || projection.Kind != composition.QueryFactorExact || !projection.Factor.Available() || projection.Normalizer.Available() {
		return receiptObservation[R]{}, receiptObservationAttachFailureProjection
	}
	if inner.observationPoints == nil {
		var indexed bool
		inner.observationPoints, indexed = indexReceiptObservationPoints(compilation.committed.graph)
		if !indexed {
			return receiptObservation[R]{}, receiptObservationAttachFailurePoint
		}
	}
	point, resolved := inner.observationPoints[member.member.Key()]
	if !resolved || !compilation.committed.graph.OwnsPoint(point) {
		return receiptObservation[R]{}, receiptObservationAttachFailurePoint
	}
	surface, surfaceOK := exactObservationReadSurface(resolvedMember, projection.Factor)
	if !surfaceOK {
		return receiptObservation[R]{}, receiptObservationAttachFailureMapping
	}
	factorRuntime, factorOK := inner.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.binding.factorOrdinal, projection.Factor) {
		return receiptObservation[R]{}, receiptObservationAttachFailureFactor
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, borrow, transfer, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return receiptObservation[R]{}, receiptObservationAttachFailureUnit
	}
	if _, duplicate := inner.observationIDs[id]; duplicate || uint64(len(inner.observations)) == ^uint64(0) {
		return receiptObservation[R]{}, receiptObservationAttachFailureDuplicate
	}
	owner := &receiptObservationOwner{state: state, authority: authority, schema: state.schema, query: queryOrdinal}
	ordinal := uint64(len(inner.observations))
	runtime := &receiptExactObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, borrow: borrow, transfer: transfer, result: implementation.binding.cell.result}
	inner.observations = append(inner.observations, runtime)
	inner.observationIDs[id] = struct{}{}
	return receiptObservation[R]{owner: owner, id: id, ordinal: ordinal}, receiptObservationAttachFailureNone
}

// ProgramCallStage is the construction-plane native Call stage proof.
type ProgramCallStage struct {
	receipt mountedCallStage
}

func (stage ProgramCallStage) Available() bool { return stage.receipt.Available() }

func (stage ProgramCallStage) Kind() rows.ArtifactRuleStage { return stage.receipt.Stage() }

func (stage ProgramCallStage) MountID() identity.ContentID { return stage.receipt.MountID() }

func (stage ProgramCallStage) OccurrenceID() identity.ContentID {
	return stage.receipt.OccurrenceID()
}

func (stage ProgramCallStage) PointID() identity.ContentID {
	return stage.receipt.ReusablePointID()
}

func (stage ProgramCallStage) HasMember() bool {
	_, ok := stage.receipt.RuleMember()
	return ok
}
