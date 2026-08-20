package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (artifact *Artifact) validateSealRows(state *sealValidationState) bool {
	if artifact == nil || state == nil {
		return false
	}
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	returnValueCount, returnsPublished := coldCount(artifact, programschema.OutcomeReturnValueFamily())
	outcomePointCount, pointsPublished := coldCount(artifact, programschema.OutcomePointFamily())
	if !outcomesPublished || !returnsPublished || !pointsPublished {
		return false
	}
	returnCursor, pointCursor := uint32(0), uint32(0)
	for index := 0; index < outcomeCount; index++ {
		row, held := coldRow(artifact, programschema.OutcomeFamily(), index)
		returnOffset, returnWidth, returnSpanOK := row.ReturnValueSpan()
		pointOffset, pointWidth, pointSpanOK := row.PointSpan()
		if !held || !returnSpanOK || !pointSpanOK {
			return false
		}
		if _, exists := state.bodyRows[row.BodyID()]; !exists {
			return false
		}
		if _, duplicate := state.outcomeRows[row.ID()]; duplicate {
			return false
		}
		if pointOffset != pointCursor || uint64(pointOffset)+uint64(pointWidth) > uint64(outcomePointCount) {
			return false
		}
		outcomePoints := make(map[identity.ContentID]struct{}, pointWidth)
		for pointIndex := uint32(0); pointIndex < pointWidth; pointIndex++ {
			child, childHeld := coldRow(artifact, programschema.OutcomePointFamily(), int(pointOffset+pointIndex))
			point := child.PointID()
			if !childHeld || child.OutcomeID() != row.ID() {
				return false
			}
			if _, known := state.pointRows[point]; !known {
				return false
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return false
			}
			outcomePoints[point] = struct{}{}
		}
		if returnOffset != returnCursor || uint64(returnOffset)+uint64(returnWidth) > uint64(returnValueCount) {
			return false
		}
		for valueIndex := uint32(0); valueIndex < returnWidth; valueIndex++ {
			value, valueHeld := coldRow(artifact, programschema.OutcomeReturnValueFamily(), int(returnOffset+valueIndex))
			if !valueHeld || value.OutcomeID() != row.ID() {
				return false
			}
			if _, exists := state.valueRows[value.ValuesID()]; !exists {
				return false
			}
		}
		state.outcomeRows[row.ID()] = index
		returnCursor += returnWidth
		pointCursor += pointWidth
	}
	if int(returnCursor) != returnValueCount || int(pointCursor) != outcomePointCount {
		return false
	}
	for index := 0; index < outcomeCount; index++ {
		row, _ := coldRow(artifact, programschema.OutcomeFamily(), index)
		propagation, propagated := row.PropagationID()
		if !propagated {
			continue
		}
		nextIndex, exists := state.outcomeRows[propagation]
		if !exists || nextIndex == index {
			return false
		}
		next, nextHeld := coldRow(artifact, programschema.OutcomeFamily(), nextIndex)
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if !nextHeld || next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return false
		}
	}
	edgeCount, edgesPublished := coldCount(artifact, programschema.EnvironmentEdgeFamily())
	if !edgesPublished {
		return false
	}
	for index := 0; index < edgeCount; index++ {
		row, held := artifact.environmentEdgeRowAt(index)
		if !held {
			return false
		}
		if _, exists := state.pointRows[row.from]; !exists {
			return false
		}
		if _, exists := state.pointRows[row.to]; !exists {
			return false
		}
		for resetIndex, reset := range row.resets {
			if !reset.Available() {
				return false
			}
			if resetIndex != 0 && !contentIDBefore(row.resets[resetIndex-1], reset) {
				return false
			}
		}
	}
	localTransferCount, localTransfersPublished := coldCount(artifact, programschema.LocalTransferFamily())
	localTransferWriteCount, localTransferWritesPublished := coldCount(artifact, programschema.LocalTransferWriteFamily())
	if !localTransfersPublished || !localTransferWritesPublished {
		return false
	}
	seenLocalTransfers := make(map[identity.ContentID]struct{}, localTransferCount)
	consumedTransferWrites := uint32(0)
	for index := 0; index < localTransferCount; index++ {
		row, held := coldRow(artifact, programschema.LocalTransferFamily(), index)
		offset, writeCount, spanOK := row.WriteSpan()
		if !held || !spanOK || offset != consumedTransferWrites || uint64(offset)+uint64(writeCount) > uint64(localTransferWriteCount) {
			return false
		}
		if _, exists := state.pointRows[row.From()]; !exists {
			return false
		}
		if _, exists := state.pointRows[row.To()]; !exists {
			return false
		}
		if _, duplicate := seenLocalTransfers[row.ID()]; duplicate {
			return false
		}
		seenLocalTransfers[row.ID()] = struct{}{}
		var prior schema.Key
		for child := uint32(0); child < writeCount; child++ {
			write, writeHeld := coldRow(artifact, programschema.LocalTransferWriteFamily(), int(offset+child))
			key, keyOK := write.Key()
			if !writeHeld || !keyOK || child != 0 && prior >= key {
				return false
			}
			prior = key
		}
		consumedTransferWrites += writeCount
	}
	if uint64(consumedTransferWrites) != uint64(localTransferWriteCount) {
		return false
	}
	regionCount, regionsPublished := coldCount(artifact, programschema.RegionFamily())
	if !regionsPublished {
		return false
	}
	regionRows := make(map[identity.ContentID]struct{}, regionCount)
	for index := 0; index < regionCount; index++ {
		row, held := artifact.regionRowAt(index)
		if !held {
			return false
		}
		if _, exists := regionRows[row.id]; exists {
			return false
		}
		regionRows[row.id] = struct{}{}
		if _, exists := state.pointRows[row.members[0]]; !exists {
			return false
		}
		for memberIndex, member := range row.members {
			if _, exists := state.pointRows[member]; !exists || memberIndex != 0 && member == row.members[memberIndex-1] {
				return false
			}
		}
	}
	for index := 0; index < regionCount; index++ {
		row, held := artifact.regionRowAt(index)
		if !held {
			return false
		}
		if row.parent.Available() {
			if _, exists := regionRows[row.parent]; !exists {
				return false
			}
		}
	}
	eventCount, eventsPublished := coldCount(artifact, programschema.WTOEventFamily())
	if !eventsPublished {
		return false
	}
	for index := 0; index < eventCount; index++ {
		event, held := artifact.wtoEventRowAt(index)
		if !held {
			return false
		}
		if event.kind == WTOEventPoint {
			if _, exists := state.pointRows[event.point]; !exists {
				return false
			}
		} else if _, exists := regionRows[event.region]; !exists {
			return false
		}
	}
	state.occurrenceRows = make(map[programschema.OccurrenceKind]map[identity.ContentID]struct{})
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		return false
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || !row.Available() {
			return false
		}
		body, hasBody := row.BodyID()
		if hasBody {
			if _, exists := state.bodyRows[body]; !exists {
				return false
			}
		}
		pointOffset, pointCount, pointSpanOK := row.PointSpan()
		inputOffset, inputCount, inputSpanOK := row.InputSpan()
		if !pointSpanOK || !inputSpanOK {
			return false
		}
		for pointIndex := uint32(0); pointIndex < pointCount; pointIndex++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + pointIndex))
			if !pointOK || !point.Available() {
				return false
			}
			if _, exists := state.pointRows[point.PointID()]; !exists {
				return false
			}
		}
		for inputIndex := uint32(0); inputIndex < inputCount; inputIndex++ {
			input, inputOK := program.OccurrenceInputAt(int(inputOffset + inputIndex))
			if !inputOK || !input.Available() {
				return false
			}
		}
		kind := row.Kind()
		rows := state.occurrenceRows[kind]
		if rows == nil {
			rows = make(map[identity.ContentID]struct{})
			state.occurrenceRows[kind] = rows
		}
		if _, duplicate := rows[row.ID()]; duplicate {
			return false
		}
		rows[row.ID()] = struct{}{}
	}
	exactCount, exactPublished := programschema.ExactScalarSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !exactPublished {
		return false
	}
	var priorExact identity.ContentID
	for index := 0; index < exactCount; index++ {
		row, rowOK := programschema.ExactScalarSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorExact, row.ID()) {
			return false
		}
		priorExact = row.ID()
		_, exists := state.occurrenceRows[programschema.OccurrenceBinaryArithmetic][row.OccurrenceID()]
		if !exists {
			return false
		}
		binary, found := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return false
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
			return false
		}
	}
	arithmeticCount, arithmeticPublished := programschema.ArithmeticSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !arithmeticPublished {
		return false
	}
	var priorArithmetic identity.ContentID
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := programschema.ArithmeticSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorArithmetic, row.ID()) {
			return false
		}
		priorArithmetic = row.ID()
		binary, found := program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return false
		}
		body, bodyOK := binary.BodyID()
		if !bodyOK || body != row.BodyPathID() || flowkind.BinaryOp(binary.Code()) != flowkind.BinaryOp(row.Operator()) {
			return false
		}
	}
	unaryCount, unaryPublished := programschema.UnarySummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !unaryPublished {
		return false
	}
	var priorUnary identity.ContentID
	for index := 0; index < unaryCount; index++ {
		row, rowOK := programschema.UnarySummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorUnary, row.ID()) {
			return false
		}
		priorUnary = row.ID()
		unary, found := program.OccurrenceForID(programschema.OccurrenceUnary, row.OccurrenceID())
		if !found {
			return false
		}
		body, bodyOK := unary.BodyID()
		if !bodyOK || body != row.BodyPathID() {
			return false
		}
		if flowkind.UnaryOp(unary.Code()) != flowkind.UnaryOp(row.Operator()) {
			return false
		}
		pointFound := false
		pointOffset, pointCount, pointSpanOK := unary.PointSpan()
		for position := uint32(0); pointSpanOK && position < pointCount; position++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + position))
			pointFound = pointFound || pointOK && point.PointID() == row.OutputPointID()
		}
		if !pointFound {
			return false
		}
	}

	return true
}
