package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema/cold"
)

func (artifact *Artifact) validateSealRows(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	outcomeCount, outcomesPublished := coldCount(artifact, cold.OutcomeFamily())
	returnValueCount, returnsPublished := coldCount(artifact, cold.OutcomeReturnValueFamily())
	outcomePointCount, pointsPublished := coldCount(artifact, cold.OutcomePointFamily())
	if !outcomesPublished || !returnsPublished || !pointsPublished {
		return compileFailure(CompileStageSeal, CompileRowOutcome, -1, -1, CompileReasonOutcomeUnavailable)
	}
	returnCursor, pointCursor := uint32(0), uint32(0)
	for index := 0; index < outcomeCount; index++ {
		row, held := coldRow(artifact, cold.OutcomeFamily(), index)
		returnOffset, returnWidth, returnSpanOK := row.ReturnValueSpan()
		pointOffset, pointWidth, pointSpanOK := row.PointSpan()
		if !held || !returnSpanOK || !pointSpanOK {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeUnavailable)
		}
		if _, exists := state.bodyRows[row.BodyID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeBody)
		}
		if _, duplicate := state.outcomeRows[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeDuplicate)
		}
		if pointOffset != pointCursor || uint64(pointOffset)+uint64(pointWidth) > uint64(outcomePointCount) {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeRange)
		}
		outcomePoints := make(map[identity.ContentID]struct{}, pointWidth)
		for pointIndex := uint32(0); pointIndex < pointWidth; pointIndex++ {
			child, childHeld := coldRow(artifact, cold.OutcomePointFamily(), int(pointOffset+pointIndex))
			point := child.PointID()
			if !childHeld || child.OutcomeID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, int(pointIndex), CompileReasonOutcomeUnavailable)
			}
			if _, known := state.pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, int(pointIndex), CompileReasonOutcomeUnavailable)
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, int(pointIndex), CompileReasonOutcomeUnavailable)
			}
			outcomePoints[point] = struct{}{}
		}
		if returnOffset != returnCursor || uint64(returnOffset)+uint64(returnWidth) > uint64(returnValueCount) {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeRange)
		}
		for valueIndex := uint32(0); valueIndex < returnWidth; valueIndex++ {
			value, valueHeld := coldRow(artifact, cold.OutcomeReturnValueFamily(), int(returnOffset+valueIndex))
			if !valueHeld || value.OutcomeID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex), CompileReasonReturnValueUnavailable)
			}
			if _, exists := state.valueRows[value.ValuesID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex), CompileReasonReturnValueReference)
			}
		}
		state.outcomeRows[row.ID()] = index
		returnCursor += returnWidth
		pointCursor += pointWidth
	}
	if int(returnCursor) != returnValueCount || int(pointCursor) != outcomePointCount {
		return compileFailure(CompileStageSeal, CompileRowReturnValue, -1, -1, CompileReasonOutcomeRange)
	}
	for index := 0; index < outcomeCount; index++ {
		row, _ := coldRow(artifact, cold.OutcomeFamily(), index)
		propagation, propagated := row.PropagationID()
		if !propagated {
			continue
		}
		nextIndex, exists := state.outcomeRows[propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next, nextHeld := coldRow(artifact, cold.OutcomeFamily(), nextIndex)
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if !nextHeld || next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	edgeCount, edgesPublished := coldCount(artifact, cold.EnvironmentEdgeFamily())
	if !edgesPublished {
		return compileFailure(CompileStageSeal, CompileRowEnvironment, -1, -1, CompileReasonEnvironmentUnavailable)
	}
	state.environmentByRoute = make(map[identity.ContentID]EnvironmentEdge, edgeCount)
	state.environmentRouteDuplicates = make(map[identity.ContentID]struct{})
	for index := 0; index < edgeCount; index++ {
		row, held := artifact.environmentEdgeRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := state.environmentByRoute[row.route]; exists {
			state.environmentRouteDuplicates[row.route] = struct{}{}
		} else {
			state.environmentByRoute[row.route] = row
		}
		if _, exists := state.pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := state.pointRows[row.to]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		for resetIndex, reset := range row.resets {
			if !reset.Available() {
				return compileFailure(CompileStageSeal, CompileRowEnvironment, index, resetIndex, CompileReasonRouteResetMember)
			}
			if resetIndex != 0 && !contentIDBefore(row.resets[resetIndex-1], reset) {
				return compileFailure(CompileStageSeal, CompileRowEnvironment, index, resetIndex, CompileReasonRouteResetOrder)
			}
		}
	}
	seenLocalTransfers := make(map[identity.ContentID]struct{}, len(artifact.localTransfers))
	for index, row := range artifact.localTransfers {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := state.pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := state.pointRows[row.to]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, duplicate := seenLocalTransfers[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
		seenLocalTransfers[row.id] = struct{}{}
	}
	regionCount, regionsPublished := coldCount(artifact, cold.RegionFamily())
	if !regionsPublished {
		return compileFailure(CompileStageSeal, CompileRowRegion, -1, -1, CompileReasonRegionUnavailable)
	}
	regionRows := make(map[identity.ContentID]struct{}, regionCount)
	for index := 0; index < regionCount; index++ {
		row, held := artifact.regionRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regionRows[row.id]; exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		regionRows[row.id] = struct{}{}
		if _, exists := state.pointRows[row.members[0]]; !exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		for memberIndex, member := range row.members {
			if _, exists := state.pointRows[member]; !exists || memberIndex != 0 && member == row.members[memberIndex-1] {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, memberIndex, CompileReasonRegionReference)
			}
		}
	}
	for index := 0; index < regionCount; index++ {
		row, held := artifact.regionRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if row.parent.Available() {
			if _, exists := regionRows[row.parent]; !exists {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionReference)
			}
		}
	}
	eventCount, eventsPublished := coldCount(artifact, cold.WTOEventFamily())
	if !eventsPublished {
		return compileFailure(CompileStageSeal, CompileRowWTOEvent, -1, -1, CompileReasonEventUnavailable)
	}
	for index := 0; index < eventCount; index++ {
		event, held := artifact.wtoEventRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		if event.kind == WTOEventPoint {
			if _, exists := state.pointRows[event.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
			}
		} else if _, exists := regionRows[event.region]; !exists {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
		}
	}
	state.occurrenceRows = make(map[OccurrenceKind]map[identity.ContentID]struct{})
	for index, row := range artifact.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if row.body.Available() {
			if _, exists := state.bodyRows[row.body]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		for pointIndex, point := range row.points {
			if _, exists := state.pointRows[point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, pointIndex, CompileReasonOccurrenceUnavailable)
			}
		}
		rows := state.occurrenceRows[row.kind]
		if rows == nil {
			rows = make(map[identity.ContentID]struct{})
			state.occurrenceRows[row.kind] = rows
		}
		if _, duplicate := rows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		rows[row.id] = struct{}{}
	}
	exactCount, exactPublished := cold.ExactScalarSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !exactPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorExact identity.ContentID
	for index := 0; index < exactCount; index++ {
		row, rowOK := cold.ExactScalarSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorExact, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		priorExact = row.ID()
		_, exists := state.occurrenceRows[OccurrenceBinaryArithmetic][row.OccurrenceID()]
		if !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.OccurrenceID()}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) || artifact.occurrences[binary].body != row.BodyPathID() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		left, right, _, endpointsOK := artifact.occurrences[binary].BinaryArithmetic()
		wantSubject := artifact.occurrences[binary].ID()
		switch row.Role() {
		case cold.ExactScalarSummaryLeft:
			wantSubject = left
		case cold.ExactScalarSummaryRight:
			wantSubject = right
		case cold.ExactScalarSummaryResult:
		default:
			endpointsOK = false
		}
		if !endpointsOK || row.SubjectID() != wantSubject {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	arithmeticCount, arithmeticPublished := cold.ArithmeticSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !arithmeticPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorArithmetic identity.ContentID
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := cold.ArithmeticSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorArithmetic, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		priorArithmetic = row.ID()
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.OccurrenceID()}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[binary]
		_, _, op, endpointsOK := occurrence.BinaryArithmetic()
		if !endpointsOK || occurrence.body != row.BodyPathID() || op != flowkind.BinaryOp(row.Operator()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	unaryCount, unaryPublished := cold.UnarySummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !unaryPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorUnary identity.ContentID
	for index := 0; index < unaryCount; index++ {
		row, rowOK := cold.UnarySummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorUnary, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
		}
		priorUnary = row.ID()
		unary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceUnary, id: row.OccurrenceID()}]
		if !found || uint64(unary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[unary]
		if occurrence.body != row.BodyPathID() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
		}
		if flowkind.UnaryOp(occurrence.code) != flowkind.UnaryOp(row.Operator()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 3, CompileReasonOccurrenceUnavailable)
		}
		pointFound := false
		for _, point := range occurrence.points {
			pointFound = pointFound || point == row.OutputPointID()
		}
		if !pointFound {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 5, CompileReasonOccurrenceUnavailable)
		}
	}

	return CompileFailure{}
}
