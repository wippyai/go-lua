package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type compiler struct {
	input                      *program.Program
	key                        CompileKey
	pointAttachments           []PointAttachmentRow
	points                     map[identity.ContentID]struct{}
	environment                []EnvironmentEdge
	localTransfers             []LocalTransfer
	regions                    []Region
	events                     []WTOEvent
	values                     []ValuesRow
	calls                      []CallRow
	callOperands               []CallOperandRow
	callArguments              []CallArgumentRow
	callTypeArguments          []CallTypeArgumentRow
	bodies                     []BodyRow
	functionBoundaries         []FunctionBoundaryRow
	callTargets                []CallTargetRow
	boundaries                 []BoundaryRow
	outcomes                   []OutcomeRow
	returnValues               []ReturnValue
	heapAllocations            []HeapAllocationRow
	heapIndexes                []HeapIndexRow
	allocationRows             []allocationCompileRow
	occurrences                []OccurrenceRow
	exactScalarSummaries       []ExactScalarSummaryRow
	exactScalarStates          map[identity.ContentID]exactScalarState
	arithmeticSummaries        []ArithmeticSummaryRow
	unarySummaries             []UnarySummaryRow
	ruleOccurrences            map[RuleRole][]RuleOccurrence
	diagnosticObservations     []DiagnosticObservationRow
	staticTypeArguments        []StaticTypeArgumentRow
	staticTypeValues           []StaticTypeValueRow
	staticTypeNodes            []StaticTypeNodeRow
	staticExpressions          []StaticExpressionRow
	staticInputs               []StaticInputRow
	diagnosticObservationByID  map[identity.ContentID]int
	pointGeometry              map[identity.ContentID]Point
	occurrenceSpans            map[occurrenceLookup]occurrenceSpanGeometry
	routeOccurrences           map[identity.ContentID]identity.ContentID
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}

func (compiler *compiler) copyLocalWTO() bool {
	return !compiler.copyLocalWTOFailure().Available()
}

func (compiler *compiler) copyLocalWTOFailure() CompileFailure {
	wto := compiler.input.Flow().Local().WTO()
	regions := make(map[identity.ContentID]int, wto.Count())
	for index := 0; index < wto.Count(); index++ {
		parent, ok := wto.At(index)
		if !ok || !parent.Available() || !parent.ID().Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regions[parent.ID()]; exists {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		header, headerOK := parent.HeaderPoint()
		if !headerOK || !compiler.installPoint(header) {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionHeaderUnavailable)
		}
		members := make([]identity.ContentID, parent.PointCount())
		if len(members) == 0 {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionEmpty)
		}
		for pointIndex := range members {
			point, pointOK := parent.PointAt(pointIndex)
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberUnavailable)
			}
			members[pointIndex] = point.PathID()
			if pointIndex != 0 && members[pointIndex] == members[pointIndex-1] {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberDuplicate)
			}
		}
		if members[0] != header.PathID() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		regions[parent.ID()] = len(compiler.regions)
		compiler.regions = append(compiler.regions, Region{
			id: parent.ID(), head: header.PathID(), sourceHead: header.PathID(), parent: parent.ParentID(), cyclic: parent.Cyclic(), members: members,
		})
	}

	pointEvents := make(map[identity.ContentID]struct{}, len(compiler.points))
	entered := make([]bool, len(compiler.regions))
	exited := make([]bool, len(compiler.regions))
	type frame struct {
		region int
		next   int
	}
	stack := make([]frame, 0, len(compiler.regions))
	for index := 0; index < wto.EventCount(); index++ {
		parent, ok := wto.EventAt(index)
		if !ok || !parent.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		event := WTOEvent{}
		switch parent.Kind() {
		case flow.WTOEventEnter:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnavailable)
			}
			regionIndex, exists := regions[region.ID()]
			if !exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnknown)
			}
			if entered[regionIndex] || exited[regionIndex] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRegionRepeated)
			}
			row := compiler.regions[regionIndex]
			if len(stack) == 0 {
				if row.parent.Available() {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRootParent)
				}
			} else {
				parentFrame := stack[len(stack)-1]
				if !entered[parentFrame.region] || parentFrame.next == 0 || row.parent != compiler.regions[parentFrame.region].id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventParentMismatch)
				}
			}
			entered[regionIndex] = true
			stack = append(stack, frame{region: regionIndex})
			event.kind, event.region = WTOEventEnter, region.ID()
		case flow.WTOEventPoint:
			point, pointOK := parent.Point()
			// Parent LocalWTO may schedule an acyclic phase vertex outside every
			// cyclic Region.  It is still a total parent-issued point and must be
			// retained, rather than being treated as malformed merely because the
			// region stack is empty.
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointUnavailable)
			}
			id := point.PathID()
			if _, exists := pointEvents[id]; exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointRepeated)
			}
			if len(stack) != 0 {
				current := &stack[len(stack)-1]
				row := compiler.regions[current.region]
				if current.next >= len(row.members) || row.members[current.next] != id || current.next == 0 && row.head != id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.next, CompileReasonEventPointOrder)
				}
				current.next++
			}
			pointEvents[id] = struct{}{}
			event.kind, event.point = WTOEventPoint, id
		case flow.WTOEventExit:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() || len(stack) == 0 {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventExitUnavailable)
			}
			current := stack[len(stack)-1]
			if compiler.regions[current.region].id != region.ID() || current.next != len(compiler.regions[current.region].members) || exited[current.region] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.region, CompileReasonEventExitMismatch)
			}
			exited[current.region] = true
			stack = stack[:len(stack)-1]
			event.kind, event.region = WTOEventExit, region.ID()
		default:
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventKindUnknown)
		}
		if !event.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		compiler.events = append(compiler.events, event)
	}
	if len(stack) != 0 {
		return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, wto.EventCount(), len(stack), CompileReasonEventUnbalanced)
	}
	for index := range compiler.regions {
		if !entered[index] || !exited[index] {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionIncomplete)
		}
	}
	for point := range compiler.points {
		if _, exists := pointEvents[point]; !exists {
			return compileFailure(CompileStageLocalWTO, CompileRowPoint, -1, -1, CompileReasonPointUnscheduled)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) installPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	if compiler.pointGeometry == nil {
		compiler.pointGeometry = make(map[identity.ContentID]Point)
	}
	if existing, exists := compiler.pointGeometry[point.PathID()]; exists {
		return existing.Available()
	}
	entryBody, bodyOK := compiler.input.BodyAt(0)
	entrySite, entryOK := entryBody.EntrySite()
	if !bodyOK || !entryOK || !entrySite.Available() {
		return false
	}
	decisions := make(map[identity.ContentID]struct{})
	initial := false
	for index := 0; index < point.SiteCount(); index++ {
		site, siteOK := point.SiteAt(index)
		if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) {
			return false
		}
		if site.ContextID() == entrySite.ContextID() {
			initial = true
		}
		subject, subjectOK := site.Term()
		count, countOK := compiler.input.Flow().Continuation().GuardCount(subject)
		if !subjectOK || !countOK {
			return false
		}
		for guardIndex := 0; guardIndex < count; guardIndex++ {
			guard, guardOK := compiler.input.Flow().Continuation().GuardAt(subject, guardIndex)
			decisionID, decisionOK := compiler.input.Flow().SemanticTermPath(guard)
			if !guardOK || !decisionOK || !decisionID.Available() {
				return false
			}
			decisions[decisionID] = struct{}{}
		}
	}
	ordered := make([]identity.ContentID, 0, len(decisions))
	for decision := range decisions {
		ordered = append(ordered, decision)
	}
	identity.SortContentIDs(ordered)
	compiler.points[point.PathID()] = struct{}{}
	compiler.pointGeometry[point.PathID()] = Point{id: point.PathID(), decisions: ordered, initial: initial}
	return true
}

func (compiler *compiler) containsPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	_, exists := compiler.points[point.PathID()]
	return exists
}

// admitPointDecision joins an exact guarded final-route decision into the
// already parent-issued source point scope. Point Site attachments describe
// ambient continuation guards, while the route proof is the sole authority
// for its edge-local guard. Keeping both sources in one canonical set avoids
// reconstructing scope from Terms at Link time.
func (compiler *compiler) admitPointDecision(point, decision identity.ContentID) bool {
	geometry, exists := compiler.pointGeometry[point]
	if !exists || !geometry.Available() || !decision.Available() {
		return false
	}
	if compiler.pointDecisionAdds == nil {
		compiler.pointDecisionAdds = make(map[identity.ContentID][]identity.ContentID)
	}
	compiler.pointDecisionAdds[point] = append(compiler.pointDecisionAdds[point], decision)
	return true
}

// canonicalizePointDecisionsFailure batches route-local decision admission by
// owner point. The old path inserted each decision into a sorted slice, moving
// O(D) tail elements for every route. One radix pass and in-place duplicate
// removal gives the same canonical owner-local order in O(D) work.
func (compiler *compiler) canonicalizePointDecisionsFailure() CompileFailure {
	for point, additions := range compiler.pointDecisionAdds {
		geometry, known := compiler.pointGeometry[point]
		if !known || !geometry.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
		}
		if len(additions) == 0 {
			continue
		}
		decisions := make([]identity.ContentID, 0, len(geometry.decisions)+len(additions))
		decisions = append(decisions, geometry.decisions...)
		decisions = append(decisions, additions...)
		identity.SortContentIDs(decisions)
		unique := 0
		for _, decision := range decisions {
			if !decision.Available() {
				return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
			}
			if unique == 0 || decisions[unique-1] != decision {
				decisions[unique] = decision
				unique++
			}
		}
		geometry.decisions = decisions[:unique]
		compiler.pointGeometry[point] = geometry
	}
	return CompileFailure{}
}

func (compiler *compiler) emitRoutesFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteUnavailable)
	}
	routes := compiler.input.Flow().Causal().Successors()
	for index := 0; index < routes.TotalCount(); index++ {
		route, ok := routes.FinalAt(index)
		if !ok {
			return compileFailure(CompileStageRoutes, CompileRowRoute, index, -1, CompileReasonRouteUnavailable)
		}
		if failure := compiler.admitEnvironmentFailure(route, index); failure.Available() {
			return failure
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) admitEnvironment(route flow.FinalRoute) bool {
	return !compiler.admitEnvironmentFailure(route, -1).Available()
}

func (compiler *compiler) admitEnvironmentFailure(route flow.FinalRoute, rowIndex int) CompileFailure {
	if compiler == nil || !compiler.input.Available() || !route.Available() || !compiler.input.Flow().Causal().OwnsFinalRoute(route) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteForeign)
	}
	from, fromOK := route.FromPoint()
	to, toOK := route.ToPoint()
	_, fromSiteOK := route.From()
	_, toSiteOK := route.To()
	routeID, routeOK := route.ID()
	arm, armOK := route.Arm()
	kind, kindOK := routeKind(arm)
	if !fromOK || !toOK || !fromSiteOK || !toSiteOK || !compiler.containsPoint(from) || !compiler.containsPoint(to) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteEndpoints)
	}
	if !routeOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	if !armOK || !kindOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteArm)
	}
	occurrenceID := environmentRouteOccurrenceID(compiler.input.ContentID(), routeID, arm)
	if !occurrenceID.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	guardID, conditionID, guarded, truth := identity.ContentID{}, identity.ContentID{}, false, false
	decisionID := identity.ContentID{}
	if route.Guarded() {
		guard, guardOK := route.GuardProof()
		truthValue, truthOK := guard.Truth()
		if !guardOK || !truthOK || !guard.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		guardID, guarded, truth = guard.ContextID(), true, truthValue
		if !guardID.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		var decisionOK bool
		decisionID, decisionOK = guard.DecisionPathID()
		if !decisionOK || !decisionID.Available() || !compiler.admitPointDecision(from.PathID(), decisionID) {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		identityValue, identityOK := route.Identity()
		if !identityOK {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		conditionID = compiler.conditionValueSpanID(identityValue.Decision())
	}

	component, resetDigest := identity.ContentID{}, identity.ContentID{}
	hasReset := route.HasResetWitness()
	mu, hasMu := identity.ContentID{}, route.HasMu()
	if hasMu {
		var muOK bool
		mu, muOK = route.MuPathID()
		if !muOK || !mu.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMu)
		}
	}
	resets := []identity.ContentID(nil)
	if recurrence, recurrenceOK := route.Recurrence(); recurrenceOK {
		proofRouteID, proofRouteIDOK := recurrence.RouteID()
		if !proofRouteIDOK || proofRouteID != routeID {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		component = recurrence.ComponentID()
		if !component.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		if hasReset {
			// ResetCount is a Mu/reset-witness capability. Ordinary
			// intra-component routes intentionally have no witness and their
			// parent proof returns (0,false), rather than fabricating an empty
			// reset interval. Only consume the count when the witness exists.
			count, countOK := recurrence.ResetCount()
			if !countOK || count < 0 {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			var resetOK bool
			resetDigest, resetOK = recurrence.ResetDigest()
			routeResetCount, routeResetOK := route.ResetCount()
			if !resetOK || !resetDigest.Available() || !routeResetOK || routeResetCount != count {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			resets = make([]identity.ContentID, count)
			for index := range resets {
				path, pathOK := route.ResetPathAt(index)
				if !pathOK || !path.Available() {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetMember)
				}
				resets[index] = path
				if index != 0 && !contentIDBefore(resets[index-1], resets[index]) {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetOrder)
				}
			}
		}
	} else if hasMu || hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
	}
	if hasMu && !component.Available() || hasMu != hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMuResetMismatch)
	}

	row := EnvironmentEdge{
		id: occurrenceID, from: from.PathID(), to: to.PathID(), route: routeID,
		guard: guardID, decision: decisionID, condition: conditionID, guarded: guarded, truth: truth, component: component,
		mu: mu, hasMu: hasMu, reset: resetDigest, resets: resets, hasReset: hasReset, arm: kind,
	}
	if !row.Available() {
		return compileFailure(CompileStageRoutes, CompileRowEnvironment, rowIndex, -1, CompileReasonEnvironmentUnavailable)
	}
	compiler.environment = append(compiler.environment, row)
	if compiler.routeOccurrences == nil {
		compiler.routeOccurrences = make(map[identity.ContentID]identity.ContentID)
	}
	if prior, duplicate := compiler.routeOccurrences[row.route]; duplicate && prior != occurrenceID {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	compiler.routeOccurrences[row.route] = occurrenceID
	if _, exists := compiler.environmentByRoute[row.route]; exists {
		compiler.environmentRouteDuplicates[row.route] = struct{}{}
	} else {
		compiler.environmentByRoute[row.route] = row
	}
	return CompileFailure{}
}

// environmentRouteOccurrenceID is the artifact-owned construction identity
// for one emitted environment row. It preserves the established route
// occurrence equation while consuming only the sealed route digest and arm
// supplied by Flow.
func environmentRouteOccurrenceID(programID, routeID identity.ContentID, arm flow.BoundaryArmKind) identity.ContentID {
	if !programID.Available() || !routeID.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/structural-route", 1) != nil ||
		writer.Record(1) != nil || writer.Bytes(programID[:]) != nil ||
		writer.Bytes(routeID[:]) != nil || writer.Uint(uint64(arm)) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var result identity.ContentID
	if sum := hash.Sum(result[:0]); len(sum) != len(result) {
		return identity.ContentID{}
	}
	return result
}

// conditionValueSpanID joins a Branch guard's canonical authored condition to
// its existing transformer Span only while the artifact row is being built.
// The resulting Span identity is copied into the immutable edge column; no
// authored term or live span is retained by Artifact.
func (compiler *compiler) conditionValueSpanID(decision keyspace.Term) identity.ContentID {
	if compiler == nil || !compiler.input.Available() || keyspace.TermFamily(decision) != keyspace.FamilyBranch {
		return identity.ContentID{}
	}
	_, condition, _, _, ok := compiler.input.Flow().Authored().Control().Branches().Get(decision)
	span, spanOK := compiler.input.Span(condition)
	if !ok || !spanOK || !compiler.input.OwnsSpan(span) {
		return identity.ContentID{}
	}
	return span.ContextID()
}

func (compiler *compiler) finalizeFailure() CompileFailure {
	// Least-significant key first preserves canonical (From, To, ID) order.
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.environment); index++ {
		if compiler.environment[index-1].id == compiler.environment[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row DiagnosticObservationRow) identity.ContentID { return row.id })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].id == compiler.diagnosticObservations[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) sealArtifact() (*Artifact, CompileFailure) {
	pointIDs := make([]identity.ContentID, 0, len(compiler.points))
	for id := range compiler.points {
		pointIDs = append(pointIDs, id)
	}
	identity.SortContentIDs(pointIDs)
	points := make([]Point, len(pointIDs))
	for index, id := range pointIDs {
		point, ok := compiler.pointGeometry[id]
		if !ok || !point.Available() {
			return nil, compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		points[index] = point
	}
	occurrenceByID := make(map[occurrenceLookup]uint32, len(compiler.occurrences))
	occurrenceByKind := make(map[OccurrenceKind][]uint32)
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			return nil, compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrenceByID[occurrenceLookup{kind: row.kind, id: row.id}] = uint32(index)
		occurrenceByKind[row.kind] = append(occurrenceByKind[row.kind], uint32(index))
	}
	functionBoundaryByBody := make(map[identity.ContentID]uint32, len(compiler.functionBoundaries))
	for index, row := range compiler.functionBoundaries {
		functionBoundaryByBody[row.BodyID()] = uint32(index)
	}
	artifact := &Artifact{
		key: compiler.key, pointAttachments: compiler.pointAttachments, points: points, environment: compiler.environment, localTransfers: compiler.localTransfers,
		regions: compiler.regions, events: compiler.events, values: compiler.values, calls: compiler.calls, callOperands: compiler.callOperands, callArguments: compiler.callArguments, callTypeArguments: compiler.callTypeArguments,
		bodies: compiler.bodies, functionBoundaries: compiler.functionBoundaries, callTargets: compiler.callTargets, outcomes: compiler.outcomes, returnValues: compiler.returnValues,
		boundaries:      compiler.boundaries,
		heapAllocations: compiler.heapAllocations, heapIndexes: compiler.heapIndexes,
		occurrences: compiler.occurrences, exactScalarSummaries: compiler.exactScalarSummaries, arithmeticSummaries: compiler.arithmeticSummaries, unarySummaries: compiler.unarySummaries, occurrenceByID: occurrenceByID, occurrenceByKind: occurrenceByKind, functionBoundaryByBody: functionBoundaryByBody, ruleOccurrences: compiler.ruleOccurrences,
		diagnosticObservations: compiler.diagnosticObservations, staticTypeArguments: compiler.staticTypeArguments, staticTypeValues: compiler.staticTypeValues, staticTypeNodes: compiler.staticTypeNodes, staticExpressions: compiler.staticExpressions, staticInputs: compiler.staticInputs,
	}
	artifact.id = artifactID(artifact)
	if failure := artifact.validUnsealedFailure(); failure.Available() {
		return nil, failure
	}
	artifact.sealed = artifact.id
	return artifact, CompileFailure{}
}
