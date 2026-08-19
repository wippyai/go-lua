package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// Solver-side observation runtime; the construction attach surface lives in
// runtime_program_observation.go.

type observationOwner struct {
	state     *schemaBindingState
	authority *schemaBindingAuthority
	schema    *Schema
	query     uint64
}

func (owner *observationOwner) valid(runtime *solverRuntime) bool {
	return owner != nil && runtime != nil && runtime.graph != nil && owner.state != nil && owner.authority != nil && owner.schema != nil &&
		owner.state.schema == owner.schema && owner.state.phase == schemaBindingSealed && owner.state.authority == owner.authority &&
		runtime.bindingState == owner.state && runtime.bindingAuthority == owner.authority &&
		owner.query < owner.schema.queryCount()
}

type runtimeObservation interface {
	observationID() identity.ContentID
	observationOwner() *observationOwner
	observationPoint() equation.Point
	materializeObservation(*carrier.Work, carrier.State) (*observationResult, solveBoundary, bool)
}

type observationResult struct {
	owner *observationOwner
	id    identity.ContentID
	value frozenValue
}

type summaryObservationRuntime[V, R any] struct {
	id       identity.ContentID
	owner    *observationOwner
	point    equation.Point
	factor   receiptQueryFactor[V]
	unit     carrier.Unit
	project  func(OrderedCells[V]) R
	begin    func() R
	accum    func(R, OrderedCells[V]) (R, bool)
	borrow   bool
	transfer bool
	result   FrozenResult[R]
}

// exactObservationRuntime is the exact-surface counterpart to the
// summary observation runtime. It is deliberately separate because an exact
// factor read has no normalizer: the sealed ExactQueryImplementation is the
// only authority for its factor, freezer, and projection callbacks.
type exactObservationRuntime[V, R any] struct {
	id       identity.ContentID
	owner    *observationOwner
	point    equation.Point
	factor   receiptQueryFactor[V]
	unit     carrier.Unit
	project  func(OrderedCells[V]) R
	begin    func() R
	accum    func(R, OrderedCells[V]) (R, bool)
	borrow   bool
	transfer bool
	result   FrozenResult[R]
}

// exactObservationWriteMember is the narrow structural proof required to turn
// one committed exact output into the corresponding exact read.  The caller
// never supplies a local coordinate: it is recovered from the selected
// graph-owned member and revalidated whenever a revision rebuilds it.
type exactObservationWriteMember interface {
	WriteCount() int
	WriteAt(int) (equation.Surface, bool)
	WriteRouteRead(int) (uint64, bool)
}

func exactObservationReadSurface(member exactObservationWriteMember, factor composition.Key) (equation.Surface, bool) {
	if member == nil || !factor.Available() || member.WriteCount() != 1 {
		return equation.Surface{}, false
	}
	write, writeOK := member.WriteAt(0)
	route, routeOK := member.WriteRouteRead(0)
	if !writeOK || !write.Available() || write.Factor != factor || write.Form != equation.SurfaceWriteExact || write.Mode != equation.TargetModeStrong || write.Local == 0 || write.Semantic.Available() || write.Normalizer.Available() || !routeOK || route != 0 {
		return equation.Surface{}, false
	}
	read := equation.Surface{Factor: write.Factor, Form: equation.SurfaceReadExact, Local: write.Local}
	return read, read.Available()
}

func (runtime *exactObservationRuntime[V, R]) observationID() identity.ContentID {
	if runtime == nil {
		return identity.ContentID{}
	}
	return runtime.id
}

func (runtime *exactObservationRuntime[V, R]) observationOwner() *observationOwner {
	if runtime == nil {
		return nil
	}
	return runtime.owner
}

func (runtime *exactObservationRuntime[V, R]) observationPoint() equation.Point {
	if runtime == nil {
		return equation.Point{}
	}
	return runtime.point
}

func (runtime *exactObservationRuntime[V, R]) materializeObservation(work *carrier.Work, state carrier.State) (*observationResult, solveBoundary, bool) {
	if runtime == nil || runtime.owner == nil || !runtime.id.Available() || !runtime.point.Available() || runtime.factor == nil {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	value, boundary, ok := materializeReceiptProjectionWithFailure(work, state, runtime.owner.state, runtime.owner.authority, runtime.factor, runtime.unit, runtime.project, runtime.begin, runtime.accum, runtime.borrow, runtime.transfer, runtime.result)
	if !ok || value == nil {
		return nil, boundary, false
	}
	return &observationResult{owner: runtime.owner, id: runtime.id, value: value}, boundaryNone, true
}

func (runtime *summaryObservationRuntime[V, R]) observationID() identity.ContentID {
	if runtime == nil {
		return identity.ContentID{}
	}
	return runtime.id
}

func (runtime *summaryObservationRuntime[V, R]) observationOwner() *observationOwner {
	if runtime == nil {
		return nil
	}
	return runtime.owner
}

func (runtime *summaryObservationRuntime[V, R]) observationPoint() equation.Point {
	if runtime == nil {
		return equation.Point{}
	}
	return runtime.point
}

func (runtime *summaryObservationRuntime[V, R]) materializeObservation(work *carrier.Work, state carrier.State) (*observationResult, solveBoundary, bool) {
	if runtime == nil || runtime.owner == nil || !runtime.id.Available() || !runtime.point.Available() || runtime.factor == nil {
		return nil, refused(SolveFailureFamilyObservation, "preflight"), false
	}
	value, boundary, ok := materializeReceiptProjectionWithFailure(work, state, runtime.owner.state, runtime.owner.authority, runtime.factor, runtime.unit, runtime.project, runtime.begin, runtime.accum, runtime.borrow, runtime.transfer, runtime.result)
	if !ok || value == nil {
		return nil, boundary, false
	}
	return &observationResult{owner: runtime.owner, id: runtime.id, value: value}, boundaryNone, true
}

// observationOwner is the sealed query authority one optional observation
// answers under. It is derived from the query cell alone, so an inventory can
// name coordinates but never a second query authority.
func (implementation *SummaryQueryImplementation[V, R]) observationOwner() (*observationOwner, bool) {
	state, authority, family, ordinal, ok := implementation.boundTopologyQueryReceipt()
	if !ok || state == nil || !family.Available() {
		return nil, false
	}
	return &observationOwner{state: state, authority: authority, schema: state.schema, query: ordinal}, true
}

func (implementation *ExactQueryImplementation[V, R]) observationOwner() (*observationOwner, bool) {
	state, authority, family, ordinal, ok := implementation.boundTopologyQueryReceipt()
	if !ok || state == nil || !family.Available() {
		return nil, false
	}
	return &observationOwner{state: state, authority: authority, schema: state.schema, query: ordinal}, true
}

// indexObservationPoints is constructed lazily only when optional
// observations are requested. It is linear in the committed member plane and
// gives O(1) output lookup for every subsequent selected occurrence.
func indexObservationPoints(graph *equation.Graph) (map[composition.Key]equation.Point, bool) {
	if graph == nil || graph.GroupCount() == 0 {
		return nil, false
	}
	result := make(map[composition.Key]equation.Point)
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, groupOK := graph.HyperedgeAt(groupIndex)
		if !groupOK || !graph.OwnsGroup(group) || !graph.OwnsPoint(group.Output()) {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !graph.OwnsMember(member) || !member.Key().Available() {
				return nil, false
			}
			if _, duplicate := result[member.Key()]; duplicate {
				return nil, false
			}
			result[member.Key()] = group.Output()
		}
	}
	return result, len(result) != 0
}

// bindSummaryObservationRuntime binds one optional observation against a
// sealed factor plane. It is the observation counterpart of the member and
// query binds an activation revision replays.
func bindSummaryObservationRuntime[V, R any](compilation *programPlane, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, point equation.Point, owner *observationOwner) (runtimeObservation, bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || implementation == nil || owner == nil || !id.Available() || !compilation.runtime.graph.OwnsMember(member) || !compilation.runtime.graph.OwnsPoint(point) {
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
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadSummary, Semantic: projection.Normalizer, Normalizer: projection.Normalizer, Local: 1}
	if _, mappingOK := implementation.topologySummaryMapping(surface); !mappingOK {
		return nil, false
	}
	factorRuntime, factorOK := compilation.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.binding.factorOrdinal, projection.Factor) {
		return nil, false
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, borrow, transfer, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return nil, false
	}
	return &summaryObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, borrow: borrow, transfer: transfer, result: implementation.binding.cell.result}, true
}

// bindExactObservationRuntime rebuilds an exact rule observation for a
// later activation revision using the same committed member locator and
// sealed query implementation. It admits no caller-supplied point or factor.
func bindExactObservationRuntime[V, R any](compilation *programPlane, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, point equation.Point, owner *observationOwner) (runtimeObservation, bool) {
	if compilation == nil || !compilation.frozen || compilation.runtime == nil || implementation == nil || owner == nil || !id.Available() || !compilation.runtime.graph.OwnsMember(member) || !compilation.runtime.graph.OwnsPoint(point) {
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
	surface, surfaceOK := exactObservationReadSurface(member, projection.Factor)
	if !surfaceOK {
		return nil, false
	}
	factorRuntime, factorOK := compilation.byKey[projection.Factor]
	factor, typed := factorRuntime.(receiptQueryFactor[V])
	if !factorOK || !typed || factor == nil || !factor.receiptMatches(state, authority, implementation.binding.factorOrdinal, projection.Factor) {
		return nil, false
	}
	unit, unitOK := factor.readUnit(surface)
	project, _ := implementation.projector()
	begin, accum, borrow, transfer, hasAccumulator := implementation.accumulator()
	if !unitOK || project == nil && !hasAccumulator || project != nil && hasAccumulator {
		return nil, false
	}
	return &exactObservationRuntime[V, R]{id: id, owner: owner, point: point, factor: factor, unit: unit, project: project, begin: begin, accum: accum, borrow: borrow, transfer: transfer, result: implementation.binding.cell.result}, true
}
