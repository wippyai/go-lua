package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (artifact *Artifact) validateSealRows(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	returnValueCount, returnsPublished := coldCount(artifact, programschema.OutcomeReturnValueFamily())
	outcomePointCount, pointsPublished := coldCount(artifact, programschema.OutcomePointFamily())
	if !outcomesPublished || !returnsPublished || !pointsPublished {
		return compileFailure(CompileStageSeal, CompileRowOutcome, -1, -1, CompileReasonOutcomeUnavailable)
	}
	returnCursor, pointCursor := uint32(0), uint32(0)
	for index := 0; index < outcomeCount; index++ {
		row, held := coldRow(artifact, programschema.OutcomeFamily(), index)
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
			child, childHeld := coldRow(artifact, programschema.OutcomePointFamily(), int(pointOffset+pointIndex))
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
			value, valueHeld := coldRow(artifact, programschema.OutcomeReturnValueFamily(), int(returnOffset+valueIndex))
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
		row, _ := coldRow(artifact, programschema.OutcomeFamily(), index)
		propagation, propagated := row.PropagationID()
		if !propagated {
			continue
		}
		nextIndex, exists := state.outcomeRows[propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next, nextHeld := coldRow(artifact, programschema.OutcomeFamily(), nextIndex)
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if !nextHeld || next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	edgeCount, edgesPublished := coldCount(artifact, programschema.EnvironmentEdgeFamily())
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
	localTransferCount, localTransfersPublished := coldCount(artifact, programschema.LocalTransferFamily())
	localTransferWriteCount, localTransferWritesPublished := coldCount(artifact, programschema.LocalTransferWriteFamily())
	if !localTransfersPublished || !localTransferWritesPublished {
		return compileFailure(CompileStageSeal, CompileRowEnvironment, -1, -1, CompileReasonEnvironmentUnavailable)
	}
	seenLocalTransfers := make(map[identity.ContentID]struct{}, localTransferCount)
	consumedTransferWrites := uint32(0)
	for index := 0; index < localTransferCount; index++ {
		row, held := coldRow(artifact, programschema.LocalTransferFamily(), index)
		offset, writeCount, spanOK := row.WriteSpan()
		if !held || !spanOK || offset != consumedTransferWrites || uint64(offset)+uint64(writeCount) > uint64(localTransferWriteCount) {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := state.pointRows[row.From()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := state.pointRows[row.To()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, duplicate := seenLocalTransfers[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
		seenLocalTransfers[row.ID()] = struct{}{}
		var prior schema.Key
		for child := uint32(0); child < writeCount; child++ {
			write, writeHeld := coldRow(artifact, programschema.LocalTransferWriteFamily(), int(offset+child))
			key, keyOK := write.Key()
			if !writeHeld || !keyOK || child != 0 && prior >= key {
				return compileFailure(CompileStageSeal, CompileRowEnvironment, index, int(child), CompileReasonEnvironmentUnavailable)
			}
			prior = key
		}
		consumedTransferWrites += writeCount
	}
	if uint64(consumedTransferWrites) != uint64(localTransferWriteCount) {
		return compileFailure(CompileStageSeal, CompileRowEnvironment, -1, -1, CompileReasonEnvironmentUnavailable)
	}
	regionCount, regionsPublished := coldCount(artifact, programschema.RegionFamily())
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
	eventCount, eventsPublished := coldCount(artifact, programschema.WTOEventFamily())
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
	state.occurrenceRows = make(map[programschema.OccurrenceKind]map[identity.ContentID]struct{})
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		body, hasBody := row.BodyID()
		if hasBody {
			if _, exists := state.bodyRows[body]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		pointOffset, pointCount, pointSpanOK := row.PointSpan()
		inputOffset, inputCount, inputSpanOK := row.InputSpan()
		if !pointSpanOK || !inputSpanOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + pointIndex))
			if !pointOK || !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(pointIndex), CompileReasonOccurrenceUnavailable)
			}
			if _, exists := state.pointRows[point.PointID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(pointIndex), CompileReasonOccurrenceUnavailable)
			}
		}
		for inputIndex := uint32(0); inputIndex < inputCount; inputIndex++ {
			input, inputOK := program.OccurrenceInputAt(int(inputOffset + inputIndex))
			if !inputOK || !input.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(inputIndex), CompileReasonOccurrenceUnavailable)
			}
		}
		kind := row.Kind()
		rows := state.occurrenceRows[kind]
		if rows == nil {
			rows = make(map[identity.ContentID]struct{})
			state.occurrenceRows[kind] = rows
		}
		if _, duplicate := rows[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		rows[row.ID()] = struct{}{}
	}
	exactCount, exactPublished := programschema.ExactScalarSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !exactPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorExact identity.ContentID
	for index := 0; index < exactCount; index++ {
		row, rowOK := programschema.ExactScalarSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorExact, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		priorExact = row.ID()
		_, exists := state.occurrenceRows[programschema.OccurrenceBinaryArithmetic][row.OccurrenceID()]
		if !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binaryIndex := -1
		for candidate := 0; candidate < occurrenceCount; candidate++ {
			candidateRow, candidateOK := program.OccurrenceAt(candidate)
			if candidateOK && candidateRow.ID() == binary.ID() && candidateRow.Kind() == programschema.OccurrenceBinaryArithmetic {
				binaryIndex = candidate
				break
			}
		}
		leftRow, leftOK := program.OccurrenceInputFor(binaryIndex, 0)
		rightRow, rightOK := program.OccurrenceInputFor(binaryIndex, 1)
		left, right, endpointsOK := leftRow.InputID(), rightRow.InputID(), leftOK && rightOK
		body, bodyOK := binary.BodyID()
		endpointsOK = endpointsOK && bodyOK && body == row.BodyPathID()
		wantSubject := binary.ID()
		switch row.Role() {
		case programschema.ExactScalarSummaryLeft:
			wantSubject = left
		case programschema.ExactScalarSummaryRight:
			wantSubject = right
		case programschema.ExactScalarSummaryResult:
		default:
			endpointsOK = false
		}
		if !endpointsOK || row.SubjectID() != wantSubject {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	arithmeticCount, arithmeticPublished := programschema.ArithmeticSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !arithmeticPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorArithmetic identity.ContentID
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := programschema.ArithmeticSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorArithmetic, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		priorArithmetic = row.ID()
		binary, found := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		body, bodyOK := binary.BodyID()
		if !bodyOK || body != row.BodyPathID() || flowkind.BinaryOp(binary.Code()) != flowkind.BinaryOp(row.Operator()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	unaryCount, unaryPublished := programschema.UnarySummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !unaryPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	var priorUnary identity.ContentID
	for index := 0; index < unaryCount; index++ {
		row, rowOK := programschema.UnarySummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorUnary, row.ID()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
		}
		priorUnary = row.ID()
		unary, found := program.OccurrenceForID(programschema.OccurrenceUnary, row.OccurrenceID())
		if !found {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
		}
		body, bodyOK := unary.BodyID()
		if !bodyOK || body != row.BodyPathID() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
		}
		if flowkind.UnaryOp(unary.Code()) != flowkind.UnaryOp(row.Operator()) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 3, CompileReasonOccurrenceUnavailable)
		}
		pointFound := false
		pointOffset, pointCount, pointSpanOK := unary.PointSpan()
		for position := uint32(0); pointSpanOK && position < pointCount; position++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + position))
			pointFound = pointFound || pointOK && point.PointID() == row.OutputPointID()
		}
		if !pointFound {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 5, CompileReasonOccurrenceUnavailable)
		}
	}

	return CompileFailure{}
}
