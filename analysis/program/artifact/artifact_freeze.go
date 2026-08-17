package artifact

import "github.com/wippyai/go-lua/analysis/identity"

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
