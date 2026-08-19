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
	returnCursor := uint32(0)
	for index, row := range artifact.outcomes {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeUnavailable)
		}
		if _, exists := state.bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeBody)
		}
		if _, duplicate := state.outcomeRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeDuplicate)
		}
		outcomePoints := make(map[identity.ContentID]struct{}, len(row.points))
		for pointIndex, point := range row.points {
			if !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, known := state.pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			outcomePoints[point] = struct{}{}
		}
		if row.returnStart != returnCursor || uint64(row.returnEnd) > uint64(len(artifact.returnValues)) {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeRange)
		}
		for valueIndex := row.returnStart; valueIndex < row.returnEnd; valueIndex++ {
			value := artifact.returnValues[valueIndex]
			if !value.Available() {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueUnavailable)
			}
			if _, exists := state.valueRows[value.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueReference)
			}
		}
		state.outcomeRows[row.id] = index
		returnCursor = row.returnEnd
	}
	if returnCursor != uint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowReturnValue, -1, -1, CompileReasonOutcomeRange)
	}
	for index, row := range artifact.outcomes {
		if !row.hasPropagation {
			continue
		}
		nextIndex, exists := state.outcomeRows[row.propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := artifact.outcomes[nextIndex]
		if next.kind != row.kind || next.hasTarget != row.hasTarget || next.target != row.target {
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
