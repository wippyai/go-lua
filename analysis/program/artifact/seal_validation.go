package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// sealValidationState carries the indexes built during the immutable seal
// pass. Each index is constructed once and consumed by later validation
// phases; no consumer rebuilds a parallel artifact view.
type sealValidationState struct {
	pointRows      map[identity.ContentID]struct{}
	valueRows      map[identity.ContentID]struct{}
	bodyRows       map[identity.ContentID]programschema.Body
	outcomeRows    map[identity.ContentID]int
	outcomeCursor  uint32
	callableBodies int
	occurrenceRows map[programschema.OccurrenceKind]map[identity.ContentID]struct{}
	valuesRows     map[identity.ContentID]struct{}
}

func (artifact *Artifact) validUnsealedFailure() CompileFailure {
	state := sealValidationState{}
	if failure := artifact.validateSealFoundation(&state); failure.Available() {
		return failure
	}
	if failure := artifact.validateSealIndexes(&state); failure.Available() {
		return failure
	}
	if failure := artifact.validateSealRows(&state); failure.Available() {
		return failure
	}
	return artifact.validateSealFreeze(&state)
}

func (artifact *Artifact) validateSealFoundation(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil || !artifact.key.Available() || !artifact.id.Available() || !artifact.counts.Available() || artifact.sealed.Available() {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	pointCount, pointsPublished := coldCount(artifact, programschema.PointFamily())
	if !pointsPublished {
		return compileFailure(CompileStageSeal, CompileRowPoint, -1, -1, CompileReasonPointUnavailable)
	}
	state.pointRows = make(map[identity.ContentID]struct{}, pointCount)
	previous := Point{}
	for index := 0; index < pointCount; index++ {
		row, held := artifact.pointRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		if !pointsOrdered(previous, row, index == 0) {
			return compileFailure(CompileStageSeal, CompileRowPoint, -1, -1, CompileReasonPointOrder)
		}
		previous = row
		for decisionIndex, decision := range row.decisions {
			if !decision.Available() || decisionIndex > 0 && !contentIDBefore(row.decisions[decisionIndex-1], decision) {
				return compileFailure(CompileStageSeal, CompileRowPoint, index, decisionIndex, CompileReasonPointUnavailable)
			}
		}
		state.pointRows[row.id] = struct{}{}
	}
	diagnosticCount, diagnosticsPublished := coldCount(artifact, programschema.DiagnosticObservationFamily())
	evidenceCount, evidencePublished := coldCount(artifact, programschema.DiagnosticEvidenceFamily())
	pathCount, pathsPublished := coldCount(artifact, programschema.DiagnosticPathFamily())
	if !diagnosticsPublished || !evidencePublished || !pathsPublished {
		return compileFailure(CompileStageSeal, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
	}
	seenDiagnosticObservations := make(map[identity.ContentID]struct{}, diagnosticCount)
	usedEvidence := make([]bool, evidenceCount)
	usedPaths := make([]bool, pathCount)
	for index := 0; index < diagnosticCount; index++ {
		row, held := coldRow(artifact, programschema.DiagnosticObservationFamily(), index)
		evidenceOffset, evidenceWidth, evidenceSpanOK := row.EvidenceSpan()
		pathOffset, pathWidth, pathSpanOK := row.PathSpan()
		if !held || !row.Available() || !evidenceSpanOK || !pathSpanOK ||
			uint64(evidenceOffset)+uint64(evidenceWidth) > uint64(evidenceCount) || uint64(pathOffset)+uint64(pathWidth) > uint64(pathCount) {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		if _, duplicate := seenDiagnosticObservations[row.ID()]; duplicate || index > 0 {
			if index > 0 {
				prior, priorHeld := coldRow(artifact, programschema.DiagnosticObservationFamily(), index-1)
				if !priorHeld || !contentIDBefore(prior.ID(), row.ID()) {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
				}
			}
			if duplicate {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
			}
		}
		seenDiagnosticObservations[row.ID()] = struct{}{}
		seenEvidencePoints := make(map[identity.ContentID]struct{}, evidenceWidth)
		for pointIndex := uint32(0); pointIndex < evidenceWidth; pointIndex++ {
			ordinal := int(evidenceOffset + pointIndex)
			point, pointOK := coldRow(artifact, programschema.DiagnosticEvidenceFamily(), ordinal)
			if !pointOK || !point.Available() || usedEvidence[ordinal] {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, int(pointIndex), CompileReasonRouteEndpoints)
			}
			if _, duplicate := seenEvidencePoints[point.PointID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, int(pointIndex), CompileReasonRouteEndpoints)
			}
			seenEvidencePoints[point.PointID()] = struct{}{}
			usedEvidence[ordinal] = true
			if _, exists := state.pointRows[point.PointID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, int(pointIndex), CompileReasonRouteEndpoints)
			}
		}
		for pathIndex := uint32(0); pathIndex < pathWidth; pathIndex++ {
			ordinal := int(pathOffset + pathIndex)
			path, pathOK := coldRow(artifact, programschema.DiagnosticPathFamily(), ordinal)
			if !pathOK || !path.Available() || usedPaths[ordinal] {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, int(pathIndex), CompileReasonRouteGuard)
			}
			usedPaths[ordinal] = true
		}
	}
	for index := range usedEvidence {
		if !usedEvidence[index] {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	for index := range usedPaths {
		if !usedPaths[index] {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	state.valueRows = make(map[identity.ContentID]struct{}, valuesCount)
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		offset, members, spanOK := row.MemberSpan()
		if !held || !spanOK || !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, duplicate := state.valueRows[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		state.valueRows[row.ID()] = struct{}{}
		memberRows := make(map[identity.ContentID]struct{}, members)
		for position := uint32(0); position < members; position++ {
			member, memberHeld := coldRow(artifact, programschema.ValuesMemberFamily(), int(offset+position))
			if !memberHeld || !member.Available() {
				return compileFailure(CompileStageSeal, CompileRowValues, index, int(position), CompileReasonValuesMember)
			}
			if _, duplicate := memberRows[member.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowValues, index, int(position), CompileReasonValuesDuplicate)
			}
			memberRows[member.ID()] = struct{}{}
		}
	}
	bodyCount, bodiesPublished := coldCount(artifact, programschema.BodyFamily())
	bodyEntryCount, bodyEntriesPublished := coldCount(artifact, programschema.BodyEntryFamily())
	bodyRootCount, bodyRootsPublished := coldCount(artifact, programschema.BodyRootFamily())
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	_, returnsPublished := coldCount(artifact, programschema.OutcomeReturnValueFamily())
	_, outcomePointsPublished := coldCount(artifact, programschema.OutcomePointFamily())
	if !bodiesPublished || !bodyEntriesPublished || !bodyRootsPublished || !outcomesPublished || !returnsPublished || !outcomePointsPublished || bodyCount == 0 {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	state.bodyRows = make(map[identity.ContentID]programschema.Body, bodyCount)
	rootRows := make(map[identity.ContentID]struct{})
	state.outcomeRows = make(map[identity.ContentID]int, outcomeCount)
	entryCursor, rootCursor := uint32(0), uint32(0)
	state.outcomeCursor = uint32(0)
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		row, held := coldRow(artifact, programschema.BodyFamily(), bodyIndex)
		entryOffset, entryWidth, entriesOK := row.EntrySpan()
		rootOffset, rootWidth, rootsOK := row.RootSpan()
		outcomeOffset, outcomeWidth, bodyOutcomesOK := row.OutcomeSpan()
		if !held || !entriesOK || !rootsOK || !bodyOutcomesOK || entryOffset != entryCursor || rootOffset != rootCursor || outcomeOffset != state.outcomeCursor ||
			uint64(entryOffset)+uint64(entryWidth) > uint64(bodyEntryCount) || uint64(rootOffset)+uint64(rootWidth) > uint64(bodyRootCount) || uint64(outcomeOffset)+uint64(outcomeWidth) > uint64(outcomeCount) {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		if _, duplicate := state.bodyRows[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		state.bodyRows[row.ID()] = row
		for rootIndex := uint32(0); rootIndex < rootWidth; rootIndex++ {
			root, childHeld := coldRow(artifact, programschema.BodyRootFamily(), int(rootOffset+rootIndex))
			if !childHeld || root.BodyID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, int(rootIndex), CompileReasonBodyUnavailable)
			}
			if _, duplicate := rootRows[root.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, int(rootIndex), CompileReasonBodyDuplicate)
			}
			rootRows[root.ID()] = struct{}{}
		}
		entryRows := make(map[identity.ContentID]struct{}, entryWidth)
		for pointIndex := uint32(0); pointIndex < entryWidth; pointIndex++ {
			entry, childHeld := coldRow(artifact, programschema.BodyEntryFamily(), int(entryOffset+pointIndex))
			point := entry.PointID()
			if !childHeld || entry.BodyID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, int(pointIndex), CompileReasonBodyUnavailable)
			}
			if _, known := state.pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, int(pointIndex), CompileReasonBodyUnavailable)
			}
			if _, duplicate := entryRows[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, int(pointIndex), CompileReasonBodyUnavailable)
			}
			entryRows[point] = struct{}{}
		}
		var mandatory [programschema.OutcomeCancel + 1]bool
		for childIndex := uint32(0); childIndex < outcomeWidth; childIndex++ {
			outcome, outcomeHeld := coldRow(artifact, programschema.OutcomeFamily(), int(outcomeOffset+childIndex))
			if !outcomeHeld || outcome.BodyID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, int(childIndex), CompileReasonOutcomeBody)
			}
			switch outcome.Kind() {
			case programschema.OutcomeNormal, programschema.OutcomeThrow, programschema.OutcomeYield, programschema.OutcomeCancel:
				mandatory[outcome.Kind()] = true
			}
		}
		for _, kind := range [...]programschema.OutcomeKind{programschema.OutcomeNormal, programschema.OutcomeThrow, programschema.OutcomeYield, programschema.OutcomeCancel} {
			if !mandatory[kind] {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeKind)
			}
		}
		entryCursor += entryWidth
		rootCursor += rootWidth
		state.outcomeCursor += outcomeWidth
	}
	if int(entryCursor) != bodyEntryCount || int(rootCursor) != bodyRootCount || int(state.outcomeCursor) != outcomeCount {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	state.callableBodies = 0
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, _ := coldRow(artifact, programschema.BodyFamily(), bodyIndex)
		if body.Callable() {
			state.callableBodies++
		}
	}
	functionCount, functionsPublished := coldCount(artifact, programschema.FunctionBoundaryFamily())
	formalCount, formalsPublished := coldCount(artifact, programschema.FunctionFormalFamily())
	varargCount, varargsPublished := coldCount(artifact, programschema.FunctionVarargFamily())
	captureCount, capturesPublished := coldCount(artifact, programschema.FunctionCaptureFamily())
	if !functionsPublished || !formalsPublished || !varargsPublished || !capturesPublished || functionCount != state.callableBodies {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	seenFunctions := make(map[identity.ContentID]struct{}, functionCount)
	seenFunctionBodies := make(map[identity.ContentID]struct{}, functionCount)
	formalCursor, varargCursor, captureCursor := uint32(0), uint32(0), uint32(0)
	seenVarargIDs := make(map[identity.ContentID]struct{}, varargCount)
	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		row, rowHeld := coldRow(artifact, programschema.FunctionBoundaryFamily(), functionIndex)
		formalOffset, formalWidth, formalSpanOK := row.FormalSpan()
		varargOffset, varargWidth, varargSpanOK := row.VarargSpan()
		captureOffset, captureWidth, captureSpanOK := row.CaptureSpan()
		body, bodyOK := state.bodyRows[row.BodyID()]
		function, _ := body.FunctionContextID()
		callFormal, _ := body.CallFormalID()
		if !rowHeld || !row.Available() || !formalSpanOK || !varargSpanOK || !captureSpanOK ||
			formalOffset != formalCursor || varargOffset != varargCursor || captureOffset != captureCursor ||
			uint64(formalOffset)+uint64(formalWidth) > uint64(formalCount) ||
			uint64(varargOffset)+uint64(varargWidth) > uint64(varargCount) ||
			uint64(captureOffset)+uint64(captureWidth) > uint64(captureCount) ||
			!bodyOK || !body.Callable() || body.OutcomeCount() == 0 || body.ContextID() != row.BodyContextID() || body.EntryID() != row.EntryID() ||
			function != row.ID() || callFormal != row.CallFormalID() || varargWidth > 1 {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenFunctions[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenFunctionBodies[row.BodyID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		seenFunctions[row.ID()], seenFunctionBodies[row.BodyID()] = struct{}{}, struct{}{}
		seenFormalIDs := make(map[identity.ContentID]struct{}, formalWidth)
		seenFormalCells := make(map[identity.ContentID]struct{}, formalWidth)
		seenFormalStorage := make(map[identity.ContentID]struct{}, formalWidth)
		for portIndex := uint32(0); portIndex < formalWidth; portIndex++ {
			port, portHeld := coldRow(artifact, programschema.FunctionFormalFamily(), int(formalOffset+portIndex))
			position, positionOK := port.Position()
			if !portHeld || !port.Available() || !positionOK || uint64(position) != uint64(portIndex) {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(portIndex), CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenFormalIDs[port.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(portIndex), CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalCells[port.CellID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(portIndex), CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalStorage[port.StorageCellID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(portIndex), CompileReasonBodyDuplicate)
			}
			seenFormalIDs[port.ID()], seenFormalCells[port.CellID()], seenFormalStorage[port.StorageCellID()] = struct{}{}, struct{}{}, struct{}{}
		}
		if varargWidth == 1 {
			vararg, varargHeld := coldRow(artifact, programschema.FunctionVarargFamily(), int(varargOffset))
			if !varargHeld || !vararg.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenVarargIDs[vararg.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
			}
			seenVarargIDs[vararg.ID()] = struct{}{}
		}
		seenCaptureIDs := make(map[identity.ContentID]struct{}, captureWidth)
		for captureIndex := uint32(0); captureIndex < captureWidth; captureIndex++ {
			capture, captureHeld := coldRow(artifact, programschema.FunctionCaptureFamily(), int(captureOffset+captureIndex))
			position, positionOK := capture.Position()
			if !captureHeld || !capture.Available() || !positionOK || uint64(position) != uint64(captureIndex) || capture.InnerBodyID() != row.BodyID() {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(captureIndex), CompileReasonBodyUnavailable)
			}
			if _, outerOK := state.bodyRows[capture.OuterBodyID()]; !outerOK {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(captureIndex), CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenCaptureIDs[capture.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(captureIndex), CompileReasonBodyDuplicate)
			}
			seenCaptureIDs[capture.ID()] = struct{}{}
		}
		formalCursor += formalWidth
		varargCursor += varargWidth
		captureCursor += captureWidth
	}
	if formalCursor != uint32(formalCount) || varargCursor != uint32(varargCount) || captureCursor != uint32(captureCount) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}

	return CompileFailure{}
}

// pointsOrdered states the point plane's order law over the adjacent pair the
// seal holds. The plane is read one row at a time out of the publication, so
// the law is stated over successive rows rather than over a retained slice.
func pointsOrdered(previous, current Point, first bool) bool {
	return first || contentIDBefore(previous.id, current.id)
}

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
