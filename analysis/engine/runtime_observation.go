package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// exactObservationWriteMember is the narrow structural proof required to turn
// one committed exact output into the corresponding exact read. The caller
// never supplies a local coordinate: it is recovered from the selected
// graph-owned member.
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

// exactObservationReadSurfaceForFactor derives one exact read for a
// heterogeneous observation. A member may carry unrelated exact writes, but
// the requested Factor must have exactly one strong, unrouted exact write;
// duplicate writes for that Factor remain an authenticated-shape refusal.
func exactObservationReadSurfaceForFactor(member exactObservationWriteMember, factor composition.Key) (equation.Surface, bool) {
	if member == nil || !factor.Available() || member.WriteCount() == 0 {
		return equation.Surface{}, false
	}
	var matched equation.Surface
	matchedCount := 0
	for index := 0; index < member.WriteCount(); index++ {
		write, writeOK := member.WriteAt(index)
		if !writeOK || !write.Available() {
			return equation.Surface{}, false
		}
		if write.Factor != factor {
			continue
		}
		if write.Form != equation.SurfaceWriteExact || write.Mode != equation.TargetModeStrong || write.Local == 0 || write.Semantic.Available() || write.Normalizer.Available() {
			return equation.Surface{}, false
		}
		route, routeOK := member.WriteRouteRead(index)
		if !routeOK || route != 0 {
			return equation.Surface{}, false
		}
		matched = write
		matchedCount++
	}
	if matchedCount != 1 {
		return equation.Surface{}, false
	}
	read := equation.Surface{Factor: matched.Factor, Form: equation.SurfaceReadExact, Local: matched.Local}
	return read, read.Available()
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

func bindSummaryObservationRow[V, R any](plane *programPlane, implementation *SummaryQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context) (observationRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok || plane == nil || plane.runtime == nil || !plane.runtime.graph.OwnsMember(member) {
		return observationRow{}, false
	}
	projection, ok := row.state.schema.queryProjectionShapeAt(row.ordinal, 0)
	if !ok || projection.Kind != composition.QueryFactorSummary || !projection.Normalizer.Available() {
		return observationRow{}, false
	}
	surface := equation.Surface{Factor: projection.Factor, Form: equation.SurfaceReadSummary, Semantic: projection.Normalizer, Normalizer: projection.Normalizer, Local: 1}
	if _, ok := implementation.topologySummaryMapping(surface); !ok {
		return observationRow{}, false
	}
	return bindObservationRow(plane, id, point, context, row.state, row.ordinal, row.factorOrdinal, surface, composition.QueryFactorSummary, row.projection.materialize)
}

func bindExactObservationRow[V, R any](plane *programPlane, implementation *ExactQueryImplementation[V, R], id identity.ContentID, member equation.RuleMember, point equation.Point, context executioncontext.Context, explicit RuleReadSurface, explicitOK bool) (observationRow, bool) {
	row, ok := implementation.sealedRow()
	if !ok || plane == nil || plane.runtime == nil || !plane.runtime.graph.OwnsMember(member) {
		return observationRow{}, false
	}
	projection, ok := row.state.schema.queryProjectionShapeAt(row.ordinal, 0)
	if !ok || projection.Kind != composition.QueryFactorExact || projection.Normalizer.Available() {
		return observationRow{}, false
	}
	var surface equation.Surface
	if explicitOK {
		// A routed observation has no committed exact write to derive from. The
		// only admissible alternative is the owner-issued coordinate carried by
		// the admission. It must still belong to this sealed binding and name
		// this query's Factor; equal content from another owner is refused.
		if explicit.authority != row.state.authority || explicit.value.Factor != projection.Factor || explicit.value.Form != equation.SurfaceReadExact || explicit.value.Mode != equation.TargetModeNone || explicit.value.Local == 0 || explicit.value.Semantic.Available() || explicit.value.Normalizer.Available() || !validProgramQuerySurface(explicit.value, projection) {
			return observationRow{}, false
		}
		surface = explicit.value
	} else {
		var surfaceOK bool
		surface, surfaceOK = exactObservationReadSurface(member, projection.Factor)
		if !surfaceOK {
			return observationRow{}, false
		}
	}
	return bindObservationRow(plane, id, point, context, row.state, row.ordinal, row.factorOrdinal, surface, composition.QueryFactorExact, row.projection.materialize)
}

// bindObservationRow performs the one binding-generation fence and lowers the
// request to direct runtimeProgram handles. No state, authority, graph object,
// or factor interface is retained by the row.
func bindObservationRow(plane *programPlane, id identity.ContentID, point equation.Point, context executioncontext.Context, state *schemaBindingState, queryOrdinal, factorOrdinal uint64, surface equation.Surface, kind composition.QueryProjectionKind, exec queryExec) (observationRow, bool) {
	if plane == nil || !plane.frozen || plane.runtime == nil || plane.runtime.graph == nil || state == nil || state != plane.runtime.state || state.authority != plane.runtime.authority || !id.Available() || !plane.runtime.graph.OwnsPoint(point) || exec == nil || factorOrdinal >= uint64(len(plane.factors)) {
		return observationRow{}, false
	}
	stateOrdinal, stateOK := plane.observationState(point, context)
	if !stateOK {
		return observationRow{}, false
	}
	shape, shapeOK := state.schema.queryShapeAt(queryOrdinal)
	projection, projectionOK := state.schema.queryProjectionShapeAt(queryOrdinal, 0)
	if !shapeOK || !projectionOK || shape.ProjectionCount != 1 || projection.Kind != kind || projection.Factor != state.schema.factorSemanticAt(factorOrdinal) || !validProgramQuerySurface(surface, projection) {
		return observationRow{}, false
	}
	factor := plane.factors[factorOrdinal]
	unit, unitOK := factor.readUnit(surface)
	pointIndex, pointOK := plane.runtime.graph.PointIndex(point)
	if !unitOK || !pointOK {
		return observationRow{}, false
	}
	row := observationRow{id: id, queryOrdinal: queryOrdinal, factorOrdinal: factorOrdinal, point: int32(pointIndex), state: stateOrdinal, contextID: context.ID(), unit: unit, exec: exec}
	return row, row.valid()
}
