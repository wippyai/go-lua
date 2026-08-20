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

func (artifact *Artifact) validUnsealed() bool {
	state := sealValidationState{}
	if !artifact.validateSealFoundation(&state) {
		return false
	}
	if !artifact.validateSealIndexes(&state) {
		return false
	}
	if !artifact.validateSealRows(&state) {
		return false
	}
	return artifact.validateSealFreeze(&state)
}

func (artifact *Artifact) validateSealFoundation(state *sealValidationState) bool {
	if artifact == nil || state == nil || !artifact.key.Available() || !artifact.id.Available() || !artifact.counts.Available() || artifact.sealed.Available() {
		return false
	}
	pointCount, pointsPublished := coldCount(artifact, programschema.PointFamily())
	if !pointsPublished {
		return false
	}
	state.pointRows = make(map[identity.ContentID]struct{}, pointCount)
	var previous identity.ContentID
	for index := 0; index < pointCount; index++ {
		row, held := coldRow(artifact, programschema.PointFamily(), index)
		decisionOffset, decisionCount, spanOK := row.DecisionSpan()
		if !held || !spanOK {
			return false
		}
		if index > 0 && !contentIDBefore(previous, row.ID()) {
			return false
		}
		previous = row.ID()
		var priorDecision identity.ContentID
		for decisionIndex := uint32(0); decisionIndex < decisionCount; decisionIndex++ {
			decision, decisionHeld := coldRow(artifact, programschema.PointDecisionFamily(), int(decisionOffset+decisionIndex))
			if !decisionHeld || decisionIndex > 0 && !contentIDBefore(priorDecision, decision.ID()) {
				return false
			}
			priorDecision = decision.ID()
		}
		state.pointRows[row.ID()] = struct{}{}
	}
	diagnosticCount, diagnosticsPublished := coldCount(artifact, programschema.DiagnosticObservationFamily())
	evidenceCount, evidencePublished := coldCount(artifact, programschema.DiagnosticEvidenceFamily())
	pathCount, pathsPublished := coldCount(artifact, programschema.DiagnosticPathFamily())
	if !diagnosticsPublished || !evidencePublished || !pathsPublished {
		return false
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
			return false
		}
		if _, duplicate := seenDiagnosticObservations[row.ID()]; duplicate || index > 0 {
			if index > 0 {
				prior, priorHeld := coldRow(artifact, programschema.DiagnosticObservationFamily(), index-1)
				if !priorHeld || !contentIDBefore(prior.ID(), row.ID()) {
					return false
				}
			}
			if duplicate {
				return false
			}
		}
		seenDiagnosticObservations[row.ID()] = struct{}{}
		seenEvidencePoints := make(map[identity.ContentID]struct{}, evidenceWidth)
		for pointIndex := uint32(0); pointIndex < evidenceWidth; pointIndex++ {
			ordinal := int(evidenceOffset + pointIndex)
			point, pointOK := coldRow(artifact, programschema.DiagnosticEvidenceFamily(), ordinal)
			if !pointOK || !point.Available() || usedEvidence[ordinal] {
				return false
			}
			if _, duplicate := seenEvidencePoints[point.PointID()]; duplicate {
				return false
			}
			seenEvidencePoints[point.PointID()] = struct{}{}
			usedEvidence[ordinal] = true
			if _, exists := state.pointRows[point.PointID()]; !exists {
				return false
			}
		}
		for pathIndex := uint32(0); pathIndex < pathWidth; pathIndex++ {
			ordinal := int(pathOffset + pathIndex)
			path, pathOK := coldRow(artifact, programschema.DiagnosticPathFamily(), ordinal)
			if !pathOK || !path.Available() || usedPaths[ordinal] {
				return false
			}
			usedPaths[ordinal] = true
		}
	}
	for index := range usedEvidence {
		if !usedEvidence[index] {
			return false
		}
	}
	for index := range usedPaths {
		if !usedPaths[index] {
			return false
		}
	}
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return false
	}
	state.valueRows = make(map[identity.ContentID]struct{}, valuesCount)
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		offset, members, spanOK := row.MemberSpan()
		if !held || !spanOK || !row.Available() {
			return false
		}
		if _, duplicate := state.valueRows[row.ID()]; duplicate {
			return false
		}
		state.valueRows[row.ID()] = struct{}{}
		memberRows := make(map[identity.ContentID]struct{}, members)
		for position := uint32(0); position < members; position++ {
			member, memberHeld := coldRow(artifact, programschema.ValuesMemberFamily(), int(offset+position))
			if !memberHeld || !member.Available() {
				return false
			}
			if _, duplicate := memberRows[member.ID()]; duplicate {
				return false
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
		return false
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
			return false
		}
		if _, duplicate := state.bodyRows[row.ID()]; duplicate {
			return false
		}
		state.bodyRows[row.ID()] = row
		for rootIndex := uint32(0); rootIndex < rootWidth; rootIndex++ {
			root, childHeld := coldRow(artifact, programschema.BodyRootFamily(), int(rootOffset+rootIndex))
			if !childHeld || root.BodyID() != row.ID() {
				return false
			}
			if _, duplicate := rootRows[root.ID()]; duplicate {
				return false
			}
			rootRows[root.ID()] = struct{}{}
		}
		entryRows := make(map[identity.ContentID]struct{}, entryWidth)
		for pointIndex := uint32(0); pointIndex < entryWidth; pointIndex++ {
			entry, childHeld := coldRow(artifact, programschema.BodyEntryFamily(), int(entryOffset+pointIndex))
			point := entry.PointID()
			if !childHeld || entry.BodyID() != row.ID() {
				return false
			}
			if _, known := state.pointRows[point]; !known {
				return false
			}
			if _, duplicate := entryRows[point]; duplicate {
				return false
			}
			entryRows[point] = struct{}{}
		}
		var mandatory [programschema.OutcomeCancel + 1]bool
		for childIndex := uint32(0); childIndex < outcomeWidth; childIndex++ {
			outcome, outcomeHeld := coldRow(artifact, programschema.OutcomeFamily(), int(outcomeOffset+childIndex))
			if !outcomeHeld || outcome.BodyID() != row.ID() {
				return false
			}
			switch outcome.Kind() {
			case programschema.OutcomeNormal, programschema.OutcomeThrow, programschema.OutcomeYield, programschema.OutcomeCancel:
				mandatory[outcome.Kind()] = true
			}
		}
		for _, kind := range [...]programschema.OutcomeKind{programschema.OutcomeNormal, programschema.OutcomeThrow, programschema.OutcomeYield, programschema.OutcomeCancel} {
			if !mandatory[kind] {
				return false
			}
		}
		entryCursor += entryWidth
		rootCursor += rootWidth
		state.outcomeCursor += outcomeWidth
	}
	if int(entryCursor) != bodyEntryCount || int(rootCursor) != bodyRootCount || int(state.outcomeCursor) != outcomeCount {
		return false
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
		return false
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
			return false
		}
		if _, duplicate := seenFunctions[row.ID()]; duplicate {
			return false
		}
		if _, duplicate := seenFunctionBodies[row.BodyID()]; duplicate {
			return false
		}
		seenFunctions[row.ID()], seenFunctionBodies[row.BodyID()] = struct{}{}, struct{}{}
		seenFormalIDs := make(map[identity.ContentID]struct{}, formalWidth)
		seenFormalCells := make(map[identity.ContentID]struct{}, formalWidth)
		seenFormalStorage := make(map[identity.ContentID]struct{}, formalWidth)
		for portIndex := uint32(0); portIndex < formalWidth; portIndex++ {
			port, portHeld := coldRow(artifact, programschema.FunctionFormalFamily(), int(formalOffset+portIndex))
			position, positionOK := port.Position()
			if !portHeld || !port.Available() || !positionOK || uint64(position) != uint64(portIndex) {
				return false
			}
			if _, duplicate := seenFormalIDs[port.ID()]; duplicate {
				return false
			}
			if _, duplicate := seenFormalCells[port.CellID()]; duplicate {
				return false
			}
			if _, duplicate := seenFormalStorage[port.StorageCellID()]; duplicate {
				return false
			}
			seenFormalIDs[port.ID()], seenFormalCells[port.CellID()], seenFormalStorage[port.StorageCellID()] = struct{}{}, struct{}{}, struct{}{}
		}
		if varargWidth == 1 {
			vararg, varargHeld := coldRow(artifact, programschema.FunctionVarargFamily(), int(varargOffset))
			if !varargHeld || !vararg.Available() {
				return false
			}
			if _, duplicate := seenVarargIDs[vararg.ID()]; duplicate {
				return false
			}
			seenVarargIDs[vararg.ID()] = struct{}{}
		}
		seenCaptureIDs := make(map[identity.ContentID]struct{}, captureWidth)
		for captureIndex := uint32(0); captureIndex < captureWidth; captureIndex++ {
			capture, captureHeld := coldRow(artifact, programschema.FunctionCaptureFamily(), int(captureOffset+captureIndex))
			position, positionOK := capture.Position()
			if !captureHeld || !capture.Available() || !positionOK || uint64(position) != uint64(captureIndex) || capture.InnerBodyID() != row.BodyID() {
				return false
			}
			if _, outerOK := state.bodyRows[capture.OuterBodyID()]; !outerOK {
				return false
			}
			if _, duplicate := seenCaptureIDs[capture.ID()]; duplicate {
				return false
			}
			seenCaptureIDs[capture.ID()] = struct{}{}
		}
		formalCursor += formalWidth
		varargCursor += varargWidth
		captureCursor += captureWidth
	}
	if formalCursor != uint32(formalCount) || varargCursor != uint32(varargCount) || captureCursor != uint32(captureCount) {
		return false
	}

	return true
}

func contentIDBefore(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
