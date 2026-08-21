package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
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
	if compiler.localTransfer == nil {
		compiler.localTransfer = localtransfer.New(artifactFormat())
	}
	if fault := compiler.localTransfer.Seal(); fault.Failed() {
		return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, fault.Index(), -1, CompileReasonEnvironmentDuplicate)
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
	// points is the complete canonical materialization of the compile-only
	// point directory. The map is not consulted by publication construction.
	compiler.pointGeometry = nil
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

// coldPointPlanes flattens the sealed point sequence into the two dense
// planes the publication holds. Each base point owns one final decision
// vector, identified by decisionScope; synthetic rows reference that scope
// without retaining a copied vector. The first encounter of each scope in the
// already canonical point order emits its half-open span, and every later row
// reuses the same span.
type pointDecisionScopePlane struct {
	owner     identity.ContentID
	decisions []identity.ContentID
	offset    uint32
	count     uint32
	emitted   bool
}

func coldPointPlanes(rows []pointDraft) ([]programschema.Point, []programschema.PointDecision, bool) {
	points := make([]programschema.Point, 0, len(rows))
	decisions := make([]programschema.PointDecision, 0, len(rows))
	scopes := make(map[identity.ContentID]pointDecisionScopePlane, len(rows))
	for _, row := range rows {
		if !row.Available() {
			return nil, nil, false
		}
		scope, exists := scopes[row.decisionScope]
		if !exists {
			scope = pointDecisionScopePlane{}
		}
		if row.id == row.decisionScope {
			if scope.owner.Available() {
				return nil, nil, false
			}
			scope.owner = row.id
			scope.decisions = row.decisions
		} else if len(row.decisions) != 0 {
			// A non-owner row may name the owner's scope, but it may not
			// smuggle another decision vector into the publication.
			return nil, nil, false
		}
		scopes[row.decisionScope] = scope
	}
	for scopeID, scope := range scopes {
		if !scopeID.Available() || !scope.owner.Available() || scope.owner != scopeID {
			return nil, nil, false
		}
	}
	for _, row := range rows {
		scope := scopes[row.decisionScope]
		if !scope.emitted {
			if !fitsUint32(len(decisions)) || !fitsUint32(len(scope.decisions)) ||
				uint64(len(decisions))+uint64(len(scope.decisions)) > uint64(^uint32(0)) {
				return nil, nil, false
			}
			scope.offset = uint32(len(decisions))
			scope.count = uint32(len(scope.decisions))
			for _, decision := range scope.decisions {
				converted, ok := programschema.NewPointDecision(decision)
				if !ok {
					return nil, nil, false
				}
				decisions = append(decisions, converted)
			}
			scope.emitted = true
			scopes[row.decisionScope] = scope
		}
		point, ok := programschema.NewPoint(row.id, row.initial, scope.offset, scope.count)
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

// freezeColdPublication seals the families that have moved onto the shared
// publication substrate. The compiler's own slices are transient build state
// and are not carried into the artifact beside it: the sealed publication is
// the single authority for every family it holds.
func canonicalPublication(compiler *compiler, pointRows []pointDraft) (programpublication.Publication, bool) {
	if compiler == nil {
		return programpublication.Publication{}, false
	}
	if compiler.bodyBoundary == nil {
		return programpublication.Publication{}, false
	}
	if compiler.allocations == nil {
		return programpublication.Publication{}, false
	}
	allocations, allocationFields, allocationsOK := compiler.allocations.TakeCanonicalPlanes()
	values, valuesMembers := compiler.values, compiler.valuesMembers
	indexes := compiler.heapIndexes
	points, pointDecisions, pointsOK := coldPointPlanes(pointRows)
	edges, resets, edgesOK := coldEnvironmentPlanes(compiler.environment)
	if compiler.localTransfer == nil {
		return programpublication.Publication{}, false
	}
	transfers, transferWrites, transfersOK := compiler.localTransfer.TakeCanonicalPlanes()
	typeValues := compiler.staticTypeValues
	regions, regionMembers, events, regionsOK := coldRegionPlanes(compiler.regions, compiler.events)
	if !allocationsOK || !pointsOK || !edgesOK || !transfersOK || !regionsOK {
		return programpublication.Publication{}, false
	}
	for index, row := range compiler.occurrences {
		if !programschema.OccurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			return programpublication.Publication{}, false
		}
		if uint64(index) > uint64(^uint32(0)) {
			return programpublication.Publication{}, false
		}
	}
	for _, row := range compiler.ruleOccurrences {
		if !row.Available() {
			return programpublication.Publication{}, false
		}
	}
	bodyPlanes, bodyPlanesOK := compiler.bodyBoundary.TakeCanonicalPlanes()
	if !bodyPlanesOK {
		return programpublication.Publication{}, false
	}
	publication := programpublication.Publication{
		EntryBodyID:     bodyPlanes.EntryBodyID,
		CallTargets:     compiler.callTargets,
		HeapAllocations: allocations, HeapFields: allocationFields,
		Values: values, ValuesMembers: valuesMembers,
		HeapIndexes:          indexes,
		ExactScalarSummaries: compiler.exactScalar.Rows(),
		ArithmeticSummaries:  compiler.arithmeticSummaries,
		UnarySummaries:       compiler.unarySummaries,
		Points:               points,
		PointDecisions:       pointDecisions,
		EnvironmentEdges:     edges,
		EnvironmentResets:    resets,
		LocalTransfers:       transfers,
		LocalTransferWrites:  transferWrites,
		Diagnostic:           compiler.diagnostic,
		Occurrences:          compiler.occurrences,
		OccurrencePoints:     compiler.occurrencePoints,
		OccurrenceInputs:     compiler.occurrenceInputs,
		RuleOccurrences:      compiler.ruleOccurrences,
		StaticTypeValues:     typeValues,
		StaticExpressions:    compiler.staticExpressions,
		StaticInputs:         compiler.staticInputs,
		Static: staticnode.Publication{
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
		},
		Lifecycle: lifecycle.Publication{
			StorageCellLifetimes: compiler.storageCellLifetimes,
			SubjectLifetimes:     compiler.subjectLifetimes,
			SubjectEvents:        compiler.subjectEvents,
			AliasRouteScopes:     compiler.aliasRouteScopes,
			AliasRouteMembers:    compiler.aliasRouteScopeMembers,
			AliasCandidates:      compiler.aliasCandidates,
		},
		Regions:                  regions,
		RegionMembers:            regionMembers,
		WTOEvents:                events,
		Calls:                    compiler.calls,
		CallResults:              compiler.callResults,
		CallResultSlots:          compiler.callResultSlots,
		CallOperands:             compiler.callOperands,
		CallArguments:            compiler.callArguments,
		CallTypeArguments:        compiler.callTypeArguments,
		ModuleImports:            compiler.moduleImports,
		ModuleRequests:           compiler.moduleRequests,
		ModuleEntries:            compiler.moduleEntries,
		ModuleEntryRootCells:     compiler.moduleEntryRootCells,
		ModuleEntryMembers:       compiler.moduleEntryMembers,
		ModuleEntryRootFunctions: compiler.moduleEntryRootFunctions,
		Bodies:                   bodyPlanes.Bodies,
		BodyEntries:              bodyPlanes.BodyEntries,
		BodyRoots:                bodyPlanes.BodyRoots,
		Outcomes:                 bodyPlanes.Outcomes,
		OutcomeReturnValues:      bodyPlanes.OutcomeReturnValues,
		OutcomePoints:            bodyPlanes.OutcomePoints,
		FunctionBoundaries:       bodyPlanes.FunctionBoundaries,
		FunctionFormals:          bodyPlanes.FunctionFormals,
		FunctionVarargs:          bodyPlanes.FunctionVarargs,
		FunctionCaptures:         bodyPlanes.FunctionCaptures,
	}
	return publication, true
}
