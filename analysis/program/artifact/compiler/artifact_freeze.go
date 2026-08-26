package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	if fault := compiler.localTransfer.Seal(); fault.Available() {
		return CompileFailure{construction: fault}
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
	compiler.pointGeometry = nil
	if compiler.bodyBoundary == nil || compiler.allocations == nil || compiler.localTransfer == nil || compiler.exactScalar == nil {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	allocations, allocationFields, allocationsOK := compiler.allocations.TakeCanonicalPlanes()
	pointRows, pointDecisions, pointsOK := coldPointPlanes(points)
	edges, resets, edgesOK := coldEnvironmentPlanes(compiler.environment)
	transfers, transferWrites, transferFault := compiler.localTransfer.TakeCanonicalPlanes()
	if transferFault.Available() {
		return nil, CompileFailure{construction: transferFault}
	}
	regions, regionMembers, events, regionsOK := coldRegionPlanes(compiler.regions, compiler.events)
	if !allocationsOK || !pointsOK || !edgesOK || !regionsOK {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	for index, row := range compiler.publication.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, compiler.publication.OccurrencePoints, compiler.publication.OccurrenceInputs) ||
			uint64(index) > uint64(^uint32(0)) {
			return nil, compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index, row := range compiler.publication.RuleOccurrences {
		if !row.Available() {
			return nil, compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	bodyPlanes, bodyPlanesOK := compiler.bodyBoundary.TakeCanonicalPlanes()
	if !bodyPlanesOK {
		return nil, compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	// ProgramPublication is the sole canonical row inventory. Only the
	// compile-only child bundles and nested draft planes are transferred here;
	// every other family was emitted directly into this schema-owned value.
	compiler.publication.EntryBodyID = bodyPlanes.EntryBodyID
	compiler.publication.HeapAllocations, compiler.publication.HeapFields = allocations, allocationFields
	compiler.publication.Points, compiler.publication.PointDecisions = pointRows, pointDecisions
	compiler.publication.EnvironmentEdges, compiler.publication.EnvironmentResets = edges, resets
	compiler.publication.LocalTransfers, compiler.publication.LocalTransferWrites = transfers, transferWrites
	compiler.publication.ExactScalarSummaries = compiler.exactScalar.Rows()
	compiler.publication.Regions, compiler.publication.RegionMembers, compiler.publication.WTOEvents = regions, regionMembers, events
	compiler.publication.Bodies = bodyPlanes.Bodies
	compiler.publication.BodyEntries = bodyPlanes.BodyEntries
	compiler.publication.BodyRoots = bodyPlanes.BodyRoots
	compiler.publication.Outcomes = bodyPlanes.Outcomes
	compiler.publication.OutcomeReturnValues = bodyPlanes.OutcomeReturnValues
	compiler.publication.OutcomePoints = bodyPlanes.OutcomePoints
	compiler.publication.FunctionBoundaries = bodyPlanes.FunctionBoundaries
	compiler.publication.FunctionFormals = bodyPlanes.FunctionFormals
	compiler.publication.FunctionVarargs = bodyPlanes.FunctionVarargs
	compiler.publication.FunctionCaptures = bodyPlanes.FunctionCaptures
	result, published := programartifact.Publish(compiler.key, compiler.publication, compiler.counts)
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
			row.id, row.from, row.Departure(), row.to, row.route, row.guard, row.decision, row.condition,
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
