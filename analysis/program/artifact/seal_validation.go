package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// sealValidationState carries the indexes built during the immutable seal
// pass. Each index is constructed once and consumed by later validation
// phases; no consumer rebuilds a parallel artifact view.
type sealValidationState struct {
	pointRows                  map[identity.ContentID]struct{}
	valueRows                  map[identity.ContentID]struct{}
	bodyRows                   map[identity.ContentID]programschema.Body
	outcomeRows                map[identity.ContentID]int
	outcomeCursor              uint32
	callableBodies             int
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
	occurrenceRows             map[OccurrenceKind]map[identity.ContentID]struct{}
	valuesRows                 map[identity.ContentID]struct{}
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
	seenDiagnosticObservations := make(map[identity.ContentID]struct{}, len(artifact.diagnosticObservations))
	for index, row := range artifact.diagnosticObservations {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		if row.Kind() == structure.DiagnosticObservationBranchCondition {
			branch, branchOK := row.BranchCondition()
			if !branchOK {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
			}
			for pointIndex := 0; pointIndex < branch.EvidencePointCount(); pointIndex++ {
				point, pointOK := branch.EvidencePointAt(pointIndex)
				if !pointOK || !point.Available() {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, pointIndex, CompileReasonRouteEndpoints)
				}
				if _, exists := state.pointRows[point]; !exists {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, pointIndex, CompileReasonRouteEndpoints)
				}
			}
		}
		if _, duplicate := seenDiagnosticObservations[row.id]; duplicate || index > 0 && !contentIDBefore(artifact.diagnosticObservations[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		seenDiagnosticObservations[row.id] = struct{}{}
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
	seenFunctions := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	seenFunctionBodies := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	for functionIndex, row := range artifact.functionBoundaries {
		body, bodyOK := state.bodyRows[row.body]
		function, _ := body.FunctionContextID()
		formal, _ := body.CallFormalID()
		if !row.Available() || !bodyOK || !body.Callable() || body.OutcomeCount() == 0 || body.ContextID() != row.bodyContext || body.EntryID() != row.entry ||
			function != row.id || formal != row.callFormal {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenFunctions[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenFunctionBodies[row.body]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		seenFunctions[row.id], seenFunctionBodies[row.body] = struct{}{}, struct{}{}
		seenFormalIDs := make(map[identity.ContentID]struct{}, len(row.formals))
		seenFormalCells := make(map[identity.ContentID]struct{}, len(row.formals))
		seenFormalStorage := make(map[identity.ContentID]struct{}, len(row.formals))
		for portIndex, port := range row.formals {
			if !port.Available() || uint64(port.position) != uint64(portIndex) {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenFormalIDs[port.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalCells[port.cell]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalStorage[port.storage]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			seenFormalIDs[port.id], seenFormalCells[port.cell], seenFormalStorage[port.storage] = struct{}{}, struct{}{}, struct{}{}
		}
		seenCaptureIDs := make(map[identity.ContentID]struct{}, len(row.captures))
		for captureIndex, capture := range row.captures {
			if !capture.Available() || capture.innerBody != row.body {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if _, outerOK := state.bodyRows[capture.outerBody]; !outerOK {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenCaptureIDs[capture.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyDuplicate)
			}
			seenCaptureIDs[capture.id] = struct{}{}
		}
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
