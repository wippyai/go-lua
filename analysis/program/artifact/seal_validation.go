package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// sealValidationState carries the indexes built during the immutable seal
// pass. Each index is constructed once and consumed by later validation
// phases; no consumer rebuilds a parallel artifact view.
type sealValidationState struct {
	pointRows                  map[identity.ContentID]struct{}
	valueRows                  map[identity.ContentID]struct{}
	bodyRows                   map[identity.ContentID]BodyRow
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
	if artifact == nil || !artifact.key.Available() || !artifact.id.Available() || !artifact.counts.Available() || artifact.sealed.Available() {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	if !sortedPoints(artifact.points) {
		return compileFailure(CompileStageSeal, CompileRowPoint, -1, -1, CompileReasonPointOrder)
	}
	state.pointRows = make(map[identity.ContentID]struct{}, len(artifact.points))
	for index, row := range artifact.points {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		for decisionIndex, decision := range row.decisions {
			if !decision.Available() || decisionIndex > 0 && !contentIDBefore(row.decisions[decisionIndex-1], decision) {
				return compileFailure(CompileStageSeal, CompileRowPoint, index, decisionIndex, CompileReasonPointUnavailable)
			}
		}
		state.pointRows[row.id] = struct{}{}
	}
	seenAttachments := make(map[struct {
		site  identity.ContentID
		point identity.ContentID
	}]struct{}, len(artifact.pointAttachments))
	for index, row := range artifact.pointAttachments {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		if _, known := state.pointRows[row.point]; !known {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		key := struct {
			site  identity.ContentID
			point identity.ContentID
		}{site: row.site, point: row.point}
		if _, duplicate := seenAttachments[key]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		seenAttachments[key] = struct{}{}
	}
	seenDiagnosticObservations := make(map[identity.ContentID]struct{}, len(artifact.diagnosticObservations))
	for index, row := range artifact.diagnosticObservations {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		if row.Kind() == DiagnosticObservationBranchCondition {
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
	state.valueRows = make(map[identity.ContentID]struct{}, len(artifact.values))
	for index, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, duplicate := state.valueRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		state.valueRows[row.id] = struct{}{}
		memberRows := make(map[identity.ContentID]struct{}, len(row.members))
		for memberIndex, member := range row.members {
			if !member.Available() {
				return compileFailure(CompileStageSeal, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			if _, duplicate := memberRows[member.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowValues, index, memberIndex, CompileReasonValuesDuplicate)
			}
			memberRows[member.id] = struct{}{}
		}
	}
	state.bodyRows = make(map[identity.ContentID]BodyRow, len(artifact.bodies))
	rootRows := make(map[identity.ContentID]struct{})
	state.outcomeRows = make(map[identity.ContentID]int, len(artifact.outcomes))
	if len(artifact.bodies) == 0 || !fitsUint32(len(artifact.bodies)) || !fitsUint32(len(artifact.outcomes)) || !fitsUint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	state.outcomeCursor = uint32(0)
	for bodyIndex, row := range artifact.bodies {
		if !row.Available() || row.outcomeStart != state.outcomeCursor || uint64(row.outcomeEnd) > uint64(len(artifact.outcomes)) {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		if _, duplicate := state.bodyRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		state.bodyRows[row.id] = row
		for rootIndex, root := range row.roots {
			if !root.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := rootRows[root.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyDuplicate)
			}
			rootRows[root.id] = struct{}{}
		}
		if len(row.entryPoints) == 0 {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		entryRows := make(map[identity.ContentID]struct{}, len(row.entryPoints))
		for pointIndex, point := range row.entryPoints {
			if !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			if _, known := state.pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := entryRows[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			entryRows[point] = struct{}{}
		}
		var mandatory [OutcomeCancel + 1]bool
		for outcomeIndex := row.outcomeStart; outcomeIndex < row.outcomeEnd; outcomeIndex++ {
			outcome := artifact.outcomes[outcomeIndex]
			if outcome.body != row.id {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, int(outcomeIndex-row.outcomeStart), CompileReasonOutcomeBody)
			}
			switch outcome.kind {
			case OutcomeNormal, OutcomeThrow, OutcomeYield, OutcomeCancel:
				mandatory[outcome.kind] = true
			}
		}
		for _, kind := range [...]OutcomeKind{OutcomeNormal, OutcomeThrow, OutcomeYield, OutcomeCancel} {
			if !mandatory[kind] {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeKind)
			}
		}
		state.outcomeCursor = row.outcomeEnd
	}
	state.callableBodies = 0
	for _, body := range artifact.bodies {
		if body.Callable() {
			state.callableBodies++
		}
	}
	seenFunctions := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	seenFunctionBodies := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	for functionIndex, row := range artifact.functionBoundaries {
		body, bodyOK := state.bodyRows[row.body]
		if !row.Available() || !bodyOK || !body.Callable() || body.context != row.bodyContext || body.entry != row.entry ||
			body.function != row.id || body.formal != row.callFormal {
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
		if len(row.outcomes) != body.OutcomeCount() {
			return compileFailure(CompileStageSeal, CompileRowOutcome, functionIndex, -1, CompileReasonBodyRange)
		}
		for outcomeIndex, id := range row.outcomes {
			artifactIndex := uint64(body.outcomeStart) + uint64(outcomeIndex)
			if artifactIndex >= uint64(len(artifact.outcomes)) || artifact.outcomes[artifactIndex].id != id {
				return compileFailure(CompileStageSeal, CompileRowOutcome, functionIndex, outcomeIndex, CompileReasonOutcomeReference)
			}
		}
	}

	return CompileFailure{}
}

func sortedPoints(rows []Point) bool {
	for index := 1; index < len(rows); index++ {
		if !contentIDBefore(rows[index-1].id, rows[index].id) {
			return false
		}
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
