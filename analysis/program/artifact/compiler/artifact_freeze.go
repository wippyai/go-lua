package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) finalizeFailure() CompileFailure {
	// Least-significant key first preserves canonical (From, To, ID) order.
	identity.SortByContentID(compiler.environment, func(row environmentEdgeDraft) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.environment, func(row environmentEdgeDraft) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.environment, func(row environmentEdgeDraft) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.environment); index++ {
		if compiler.environment[index-1].id == compiler.environment[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.localTransfers, func(row localTransferDraft) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row programschema.DiagnosticObservation) identity.ContentID { return row.ID() })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].ID() == compiler.diagnosticObservations[index].ID() {
			return compileFailure(CompileStageCanonicalize, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) sealArtifact() (*programartifact.Artifact, CompileFailure) {
	pointIDs := make([]identity.ContentID, 0, len(compiler.pointGeometry))
	for id := range compiler.pointGeometry {
		pointIDs = append(pointIDs, id)
	}
	identity.SortContentIDs(pointIDs)
	points := make([]pointDraft, len(pointIDs))
	for index, id := range pointIDs {
		point, ok := compiler.pointGeometry[id]
		if !ok || !point.Available() {
			return nil, compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		points[index] = point
	}
	publication, publicationOK := canonicalPublication(compiler, points)
	if !publicationOK {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	result, published := programartifact.Publish(compiler.key, publication, compiler.counts)
	if !published {
		return nil, compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	return result, CompileFailure{}
}

// coldHeapPlanes flattens the compiler's nested allocation rows into the two
// dense planes the publication holds. Each allocation names its fields by the
// half-open span it occupies in the field plane, so the canonical field order
// the compiler emitted is the ordinal order of the plane and no allocation
// retains a slice header.
func coldHeapPlanes(rows []heapAllocationDraft) ([]programschema.HeapAllocation, []programschema.HeapField, bool) {
	allocations := make([]programschema.HeapAllocation, 0, len(rows))
	fields := make([]programschema.HeapField, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(fields)) || !fitsUint32(len(row.fields)) {
			return nil, nil, false
		}
		offset := uint32(len(fields))
		for _, field := range row.fields {
			converted, ok := programschema.NewHeapField(
				field.id, uint8(field.kind), field.fieldSpan, field.selectorSpan, field.valuesSpan, field.valuesID,
				field.width, field.finalOpen, field.sharesFirstValueCell, uint64(field.normalized), field.normalizedOK,
			)
			if !ok {
				return nil, nil, false
			}
			fields = append(fields, converted)
		}
		allocation, ok := programschema.NewHeapAllocation(row.id, uint8(row.role), uint8(row.form), row.rootSpan, offset, uint32(len(row.fields)))
		if !ok {
			return nil, nil, false
		}
		allocations = append(allocations, allocation)
	}
	return allocations, fields, true
}

// coldPointPlanes flattens the sealed point sequence into the two dense
// planes the publication holds. Each point names its decisions by the
// half-open span it occupies in the decision plane, so the canonical decision
// order is the ordinal order of the plane and no point retains a slice header.
func coldPointPlanes(rows []pointDraft) ([]programschema.Point, []programschema.PointDecision, bool) {
	points := make([]programschema.Point, 0, len(rows))
	decisions := make([]programschema.PointDecision, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(decisions)) || !fitsUint32(len(row.decisions)) {
			return nil, nil, false
		}
		offset := uint32(len(decisions))
		for _, decision := range row.decisions {
			converted, ok := programschema.NewPointDecision(decision)
			if !ok {
				return nil, nil, false
			}
			decisions = append(decisions, converted)
		}
		point, ok := programschema.NewPoint(row.id, row.initial, offset, uint32(len(row.decisions)))
		if !ok {
			return nil, nil, false
		}
		points = append(points, point)
	}
	return points, decisions, true
}

// coldEnvironmentPlanes flattens the compiler's nested environment rows into
// the two dense planes the publication holds. Each edge names its reset
// witnesses by the half-open span it occupies in the reset plane, so the
// canonical witness order is the ordinal order of the plane and no edge
// retains a slice header.
func coldEnvironmentPlanes(rows []environmentEdgeDraft) ([]programschema.EnvironmentEdge, []programschema.EnvironmentReset, bool) {
	edges := make([]programschema.EnvironmentEdge, 0, len(rows))
	resets := make([]programschema.EnvironmentReset, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(resets)) || !fitsUint32(len(row.resets)) {
			return nil, nil, false
		}
		offset := uint32(len(resets))
		for _, reset := range row.resets {
			converted, ok := programschema.NewEnvironmentReset(reset)
			if !ok {
				return nil, nil, false
			}
			resets = append(resets, converted)
		}
		edge, ok := programschema.NewEnvironmentEdge(
			row.id, row.from, row.to, row.route, row.guard, row.decision, row.condition,
			row.reset, row.component, row.mu, offset, uint32(len(row.resets)), uint8(row.arm),
			row.guarded, row.truth, row.hasReset, row.hasMu,
		)
		if !ok {
			return nil, nil, false
		}
		edges = append(edges, edge)
	}
	return edges, resets, true
}

// coldLocalTransfers copies the compiler's transient transport drafts into
// the canonical Program family. The draft order is already the canonical
// (From, To, ID) order established by finalizeFailure; no resort or identity
// remint occurs at this boundary.
func coldLocalTransfers(rows []localTransferDraft) ([]programschema.LocalTransfer, []programschema.LocalTransferWrite, bool) {
	transfers := make([]programschema.LocalTransfer, 0, len(rows))
	writes := make([]programschema.LocalTransferWrite, 0)
	for _, row := range rows {
		if !fitsUint32(len(writes)) || !fitsUint32(len(row.writes)) {
			return nil, nil, false
		}
		offset := uint32(len(writes))
		for _, key := range row.writes {
			write, ok := programschema.NewLocalTransferWrite(key)
			if !ok {
				return nil, nil, false
			}
			writes = append(writes, write)
		}
		converted, ok := programschema.NewLocalTransfer(row.id, row.from, row.to, row.full, offset, uint32(len(row.writes)))
		if !ok {
			return nil, nil, false
		}
		transfers = append(transfers, converted)
	}
	return transfers, writes, true
}

// coldStaticTypeValues copies the remaining local authored TypeValue rows
// into their canonical family. StaticExpression and StaticInput rows are
// emitted canonically by the static compiler child and enter Publication
// directly.
func coldStaticTypeValues(values []staticTypeValueDraft) ([]programschema.StaticTypeValue, bool) {
	typeValues := make([]programschema.StaticTypeValue, 0, len(values))
	for _, row := range values {
		converted, ok := programschema.NewStaticTypeValue(row.id, row.body, row.reference, row.root, row.name)
		if !ok {
			return nil, false
		}
		typeValues = append(typeValues, converted)
	}
	return typeValues, true
}

// coldRegionPlanes flattens the compiler's nested region rows into the two
// dense planes the publication holds, and copies the event bracket sequence
// one for one. Each region names its members by the half-open span it
// occupies in the member plane, so the canonical member order is the ordinal
// order of the plane and no region retains a slice header.
func coldRegionPlanes(rows []regionDraft, events []wtoEventDraft) ([]programschema.Region, []programschema.RegionMember, []programschema.WTOEvent, bool) {
	regions := make([]programschema.Region, 0, len(rows))
	members := make([]programschema.RegionMember, 0, len(rows))
	for _, row := range rows {
		if !fitsUint32(len(members)) || !fitsUint32(len(row.members)) {
			return nil, nil, nil, false
		}
		offset := uint32(len(members))
		for _, member := range row.members {
			converted, ok := programschema.NewRegionMember(member)
			if !ok {
				return nil, nil, nil, false
			}
			members = append(members, converted)
		}
		region, ok := programschema.NewRegion(row.id, row.parent, offset, uint32(len(row.members)), row.cyclic)
		if !ok {
			return nil, nil, nil, false
		}
		regions = append(regions, region)
	}
	brackets := make([]programschema.WTOEvent, 0, len(events))
	for _, event := range events {
		converted, ok := programschema.NewWTOEvent(uint8(event.kind), event.region, event.point)
		if !ok {
			return nil, nil, nil, false
		}
		brackets = append(brackets, converted)
	}
	return regions, members, brackets, true
}

// coldHeapIndexes copies the compiler's index-access rows one for one; the
// row is already flat, so the conversion is a change of vocabulary only.
func coldHeapIndexes(rows []heapIndexDraft) ([]programschema.HeapIndex, bool) {
	indexes := make([]programschema.HeapIndex, 0, len(rows))
	for _, row := range rows {
		converted, ok := programschema.NewHeapIndex(
			row.id, row.read, row.baseSpan, row.resultSpan, row.keySpan,
			row.lensKind, uint64(row.exactKey), row.valuesSpan, row.valuesID, row.position,
		)
		if !ok {
			return nil, false
		}
		indexes = append(indexes, converted)
	}
	return indexes, true
}

// freezeColdPublication seals the families that have moved onto the shared
// publication substrate. The compiler's own slices are transient build state
// and are not carried into the artifact beside it: the sealed publication is
// the single authority for every family it holds.
func canonicalPublication(compiler *compiler, pointRows []pointDraft) (programschema.Publication, bool) {
	if compiler == nil {
		return programschema.Publication{}, false
	}
	allocations, allocationFields, heapOK := coldHeapPlanes(compiler.heapAllocations)
	values, valuesMembers := compiler.values, compiler.valuesMembers
	indexes, indexesOK := coldHeapIndexes(compiler.heapIndexes)
	points, pointDecisions, pointsOK := coldPointPlanes(pointRows)
	edges, resets, edgesOK := coldEnvironmentPlanes(compiler.environment)
	transfers, transferWrites, transfersOK := coldLocalTransfers(compiler.localTransfers)
	typeValues, staticOK := coldStaticTypeValues(compiler.staticTypeValues)
	regions, regionMembers, events, regionsOK := coldRegionPlanes(compiler.regions, compiler.events)
	if !heapOK || !indexesOK || !pointsOK || !edgesOK || !transfersOK || !staticOK || !regionsOK {
		return programschema.Publication{}, false
	}
	for index, row := range compiler.occurrences {
		if !occurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			return programschema.Publication{}, false
		}
		if uint64(index) > uint64(^uint32(0)) {
			return programschema.Publication{}, false
		}
	}
	for _, row := range compiler.ruleOccurrences {
		if !row.Available() {
			return programschema.Publication{}, false
		}
	}
	publication := programschema.Publication{
		CallTargets:     compiler.callTargets,
		HeapAllocations: allocations, HeapFields: allocationFields,
		Values: values, ValuesMembers: valuesMembers,
		HeapIndexes:                              indexes,
		ExactScalarSummaries:                     compiler.exactScalarSummaries,
		ArithmeticSummaries:                      compiler.arithmeticSummaries,
		UnarySummaries:                           compiler.unarySummaries,
		Points:                                   points,
		PointDecisions:                           pointDecisions,
		EnvironmentEdges:                         edges,
		EnvironmentResets:                        resets,
		LocalTransfers:                           transfers,
		LocalTransferWrites:                      transferWrites,
		DiagnosticObservations:                   compiler.diagnosticObservations,
		DiagnosticEvidence:                       compiler.diagnosticEvidence,
		DiagnosticPaths:                          compiler.diagnosticPaths,
		Occurrences:                              compiler.occurrences,
		OccurrencePoints:                         compiler.occurrencePoints,
		OccurrenceInputs:                         compiler.occurrenceInputs,
		RuleOccurrences:                          compiler.ruleOccurrences,
		StaticTypeValues:                         typeValues,
		StaticExpressions:                        compiler.staticExpressions,
		StaticInputs:                             compiler.staticInputs,
		StaticTypeNodes:                          compiler.staticTypeNodes,
		StaticTypeNodeUnionMembers:               compiler.staticTypeNodeUnionMembers,
		StaticTypeNodeIntersectionMembers:        compiler.staticTypeNodeIntersectionMembers,
		StaticTypeNodeGenericArguments:           compiler.staticTypeNodeGenericArguments,
		StaticTypeNodeAliasParameters:            compiler.staticTypeNodeAliasParameters,
		StaticTypeNodeInterfaceExtends:           compiler.staticTypeNodeInterfaceExtends,
		StaticTypeNodeInterfaceMembers:           compiler.staticTypeNodeInterfaceMembers,
		StaticTypeNodeTypeFunctionTypeParameters: compiler.staticTypeNodeTypeFunctionTypeParameters,
		StaticTypeNodeTypeFunctionParameters:     compiler.staticTypeNodeTypeFunctionParameters,
		StaticTypeNodeTypeFunctionReturns:        compiler.staticTypeNodeTypeFunctionReturns,
		StaticTypeNodeRecordFields:               compiler.staticTypeNodeRecordFields,
		StaticTypeNodeReferenceSourceKeys:        compiler.staticTypeNodeReferenceSourceKeys,
		StaticTypeNodeReferenceCanonicalKeys:     compiler.staticTypeNodeReferenceCanonicalKeys,
		Regions:                                  regions,
		RegionMembers:                            regionMembers,
		WTOEvents:                                events,
		Calls:                                    compiler.calls,
		CallResults:                              compiler.callResults,
		CallOperands:                             compiler.callOperands,
		CallArguments:                            compiler.callArguments,
		CallTypeArguments:                        compiler.callTypeArguments,
		Bodies:                                   compiler.bodies,
		BodyEntries:                              compiler.bodyEntries,
		BodyRoots:                                compiler.bodyRoots,
		Outcomes:                                 compiler.outcomes,
		OutcomeReturnValues:                      compiler.outcomeReturnValues,
		OutcomePoints:                            compiler.outcomePoints,
		FunctionBoundaries:                       compiler.functionBoundaries,
		FunctionFormals:                          compiler.functionFormals,
		FunctionVarargs:                          compiler.functionVarargs,
		FunctionCaptures:                         compiler.functionCaptures,
	}
	return publication, true
}
