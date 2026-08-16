package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func (artifact *Artifact) validUnsealedFailure() CompileFailure {
	if artifact == nil || !artifact.key.Available() || !artifact.id.Available() || artifact.sealed.Available() {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	if !sortedPoints(artifact.points) {
		return compileFailure(CompileStageSeal, CompileRowPoint, -1, -1, CompileReasonPointOrder)
	}
	pointRows := make(map[identity.ContentID]struct{}, len(artifact.points))
	for index, row := range artifact.points {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		for decisionIndex, decision := range row.decisions {
			if !decision.Available() || decisionIndex > 0 && !contentIDBefore(row.decisions[decisionIndex-1], decision) {
				return compileFailure(CompileStageSeal, CompileRowPoint, index, decisionIndex, CompileReasonPointUnavailable)
			}
		}
		pointRows[row.id] = struct{}{}
	}
	seenAttachments := make(map[struct {
		site  identity.ContentID
		point identity.ContentID
	}]struct{}, len(artifact.pointAttachments))
	for index, row := range artifact.pointAttachments {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		if _, known := pointRows[row.point]; !known {
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
				if _, exists := pointRows[point]; !exists {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, pointIndex, CompileReasonRouteEndpoints)
				}
			}
		}
		if _, duplicate := seenDiagnosticObservations[row.id]; duplicate || index > 0 && !contentIDBefore(artifact.diagnosticObservations[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		seenDiagnosticObservations[row.id] = struct{}{}
	}
	valueRows := make(map[identity.ContentID]struct{}, len(artifact.values))
	for index, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, duplicate := valueRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		valueRows[row.id] = struct{}{}
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
	bodyRows := make(map[identity.ContentID]BodyRow, len(artifact.bodies))
	rootRows := make(map[identity.ContentID]struct{})
	outcomeRows := make(map[identity.ContentID]int, len(artifact.outcomes))
	if len(artifact.bodies) == 0 || !fitsUint32(len(artifact.bodies)) || !fitsUint32(len(artifact.outcomes)) || !fitsUint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	outcomeCursor := uint32(0)
	for bodyIndex, row := range artifact.bodies {
		if !row.Available() || row.outcomeStart != outcomeCursor || uint64(row.outcomeEnd) > uint64(len(artifact.outcomes)) {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		if _, duplicate := bodyRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		bodyRows[row.id] = row
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
			if _, known := pointRows[point]; !known {
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
		outcomeCursor = row.outcomeEnd
	}
	callableBodies := 0
	for _, body := range artifact.bodies {
		if body.Callable() {
			callableBodies++
		}
	}
	seenFunctions := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	seenFunctionBodies := make(map[identity.ContentID]struct{}, len(artifact.functionBoundaries))
	for functionIndex, row := range artifact.functionBoundaries {
		body, bodyOK := bodyRows[row.body]
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
			if _, outerOK := bodyRows[capture.outerBody]; !outerOK {
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
	if len(artifact.functionBoundaries) != callableBodies || !artifact.packReceipt.Available() || !artifact.packReceipt.validAgainst(artifact) {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceStorage)
	}
	seenCallAllocations := make(map[identity.ContentID]struct{}, len(artifact.callTargets))
	seenCallBodies := make(map[identity.ContentID]struct{}, len(artifact.callTargets))
	bodyByContext := make(map[identity.ContentID]BodyRow, len(artifact.bodies))
	for _, body := range artifact.bodies {
		if _, duplicate := bodyByContext[body.context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyDuplicate)
		}
		bodyByContext[body.context] = body
	}
	for index, target := range artifact.callTargets {
		body, bodyOK := bodyByContext[target.context]
		if !target.Available() || !bodyOK || !body.Callable() || body.ID() != target.body ||
			body.context != target.context || body.function != target.function || body.formal != target.formal {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenCallAllocations[target.allocation]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenCallBodies[target.context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenCallAllocations[target.allocation], seenCallBodies[target.context] = struct{}{}, struct{}{}
	}
	seenBoundaries := make(map[identity.ContentID]struct{}, len(artifact.boundaries))
	for index, row := range artifact.boundaries {
		if !row.Available() || (row.kind == BoundaryCapture && uint64(row.position) > uint64(^uint32(0))) {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenBoundaries[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenBoundaries[row.id] = struct{}{}
	}
	if outcomeCursor != uint32(len(artifact.outcomes)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	for index, row := range artifact.values {
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
	}
	seenStaticArguments := make(map[identity.ContentID]struct{}, len(artifact.staticTypeArguments))
	for index, row := range artifact.staticTypeArguments {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticArguments[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticArguments[row.id] = struct{}{}
	}
	seenStaticValues := make(map[identity.ContentID]struct{}, len(artifact.staticTypeValues))
	for index, row := range artifact.staticTypeValues {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticValues[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticValues[row.id] = struct{}{}
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	seenStaticNodes := make(map[identity.ContentID]struct{}, len(artifact.staticTypeNodes))
	for index, row := range artifact.staticTypeNodes {
		// A TypeRefUnresolved is a complete Static leaf: Static sealed its
		// targetless disposition and ProgramArtifact retained its exact lexical
		// proof as a DiagnosticObservation. All other references must retain
		// their resolved/canonical target edge.
		zeroChildAllowed := row.Kind() == StaticNodePrimitive || row.Kind() == StaticNodeLiteral || row.Kind() == StaticNodeUnknown || row.Kind() == StaticNodeTypeParam || row.Kind() == StaticNodeInterface || row.Kind() == StaticNodeTypeFunction ||
			row.Kind() == StaticNodeReference && row.Resolution() == uint8(programstatic.TypeRefUnresolved)
		if !row.Available() || row.ChildCount() == 0 && !zeroChildAllowed || row.Kind() == StaticNodeTypeOf && !row.operand.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticNodes[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticNodes[row.id] = struct{}{}
	}
	for functionIndex, function := range artifact.functionBoundaries {
		for formalIndex, formal := range function.formals {
			if formal.declared.Available() {
				if _, exists := seenStaticNodes[formal.declared]; !exists {
					return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, formalIndex, CompileReasonBodyUnavailable)
				}
			}
		}
	}
	for index, row := range artifact.staticTypeNodes {
		for childIndex := 0; childIndex < row.ChildCount(); childIndex++ {
			child, ok := row.ChildAt(childIndex)
			if !ok {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex, CompileReasonOccurrenceUnavailable)
			}
			if _, exists := seenStaticNodes[child]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	seenStaticExpressions := make(map[identity.ContentID]struct{}, len(artifact.staticExpressions))
	for index, row := range artifact.staticExpressions {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticExpressions[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticExpressions[row.id] = struct{}{}
		if _, exists := seenStaticNodes[row.reference]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	seenStaticInputs := make(map[identity.ContentID]struct{}, len(artifact.staticInputs))
	for index, row := range artifact.staticInputs {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticInputs[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticInputs[row.id] = struct{}{}
		if _, exists := seenStaticExpressions[row.expression]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	returnCursor := uint32(0)
	for index, row := range artifact.outcomes {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeUnavailable)
		}
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeBody)
		}
		if _, duplicate := outcomeRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeDuplicate)
		}
		outcomePoints := make(map[identity.ContentID]struct{}, len(row.points))
		for pointIndex, point := range row.points {
			if !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, known := pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			outcomePoints[point] = struct{}{}
		}
		if row.returnStart != returnCursor || uint64(row.returnEnd) > uint64(len(artifact.returnValues)) {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeRange)
		}
		for valueIndex := row.returnStart; valueIndex < row.returnEnd; valueIndex++ {
			value := artifact.returnValues[valueIndex]
			if !value.Available() {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueUnavailable)
			}
			if _, exists := valueRows[value.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueReference)
			}
		}
		outcomeRows[row.id] = index
		returnCursor = row.returnEnd
	}
	if returnCursor != uint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowReturnValue, -1, -1, CompileReasonOutcomeRange)
	}
	for index, row := range artifact.outcomes {
		if !row.hasPropagation {
			continue
		}
		nextIndex, exists := outcomeRows[row.propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := artifact.outcomes[nextIndex]
		if next.kind != row.kind || next.hasTarget != row.hasTarget || next.target != row.target {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	environmentByRoute := make(map[identity.ContentID]EnvironmentEdge, len(artifact.environment))
	environmentRouteDuplicates := make(map[identity.ContentID]struct{})
	for index, row := range artifact.environment {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := environmentByRoute[row.route]; exists {
			environmentRouteDuplicates[row.route] = struct{}{}
		} else {
			environmentByRoute[row.route] = row
		}
		if _, exists := pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := pointRows[row.to]; !exists {
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
	seenLocalTransfers := make(map[identity.ContentID]struct{}, len(artifact.localTransfers))
	for index, row := range artifact.localTransfers {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := pointRows[row.to]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, duplicate := seenLocalTransfers[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
		seenLocalTransfers[row.id] = struct{}{}
	}
	regionRows := make(map[identity.ContentID]struct{}, len(artifact.regions))
	for index, row := range artifact.regions {
		if !row.id.Available() || !row.head.Available() || !row.sourceHead.Available() || len(row.members) == 0 {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regionRows[row.id]; exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		regionRows[row.id] = struct{}{}
		if _, exists := pointRows[row.head]; !exists || row.members[0] != row.head {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		if _, exists := pointRows[row.sourceHead]; !exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		for memberIndex, member := range row.members {
			if _, exists := pointRows[member]; !exists || memberIndex != 0 && member == row.members[memberIndex-1] {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, memberIndex, CompileReasonRegionReference)
			}
		}
	}
	for index, row := range artifact.regions {
		if row.parent.Available() {
			if _, exists := regionRows[row.parent]; !exists {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionReference)
			}
		}
	}
	for index, event := range artifact.events {
		if !event.Available() {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		if event.kind == WTOEventPoint {
			if _, exists := pointRows[event.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
			}
		} else if _, exists := regionRows[event.region]; !exists {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
		}
	}
	occurrenceRows := make(map[OccurrenceKind]map[identity.ContentID]struct{})
	for index, row := range artifact.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if row.body.Available() {
			if _, exists := bodyRows[row.body]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		for pointIndex, point := range row.points {
			if _, exists := pointRows[point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, pointIndex, CompileReasonOccurrenceUnavailable)
			}
		}
		rows := occurrenceRows[row.kind]
		if rows == nil {
			rows = make(map[identity.ContentID]struct{})
			occurrenceRows[row.kind] = rows
		}
		if _, duplicate := rows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		rows[row.id] = struct{}{}
	}
	for index, row := range artifact.exactScalarSummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.exactScalarSummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		_, exists := occurrenceRows[OccurrenceBinaryArithmetic][row.occurrence]
		if !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.occurrence}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) || artifact.occurrences[binary].body != row.body {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		left, right, _, endpointsOK := artifact.occurrences[binary].BinaryArithmetic()
		wantSubject := artifact.occurrences[binary].ID()
		switch row.role {
		case ExactScalarSummaryLeft:
			wantSubject = left
		case ExactScalarSummaryRight:
			wantSubject = right
		case ExactScalarSummaryResult:
		default:
			endpointsOK = false
		}
		if !endpointsOK || row.subject != wantSubject {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index, row := range artifact.arithmeticSummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.arithmeticSummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.occurrence}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[binary]
		_, _, op, endpointsOK := occurrence.BinaryArithmetic()
		if !endpointsOK || occurrence.body != row.body || op != row.op {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index, row := range artifact.unarySummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.unarySummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
		}
		unary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceUnary, id: row.occurrence}]
		if !found || uint64(unary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[unary]
		if occurrence.body != row.body {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
		}
		if flowkind.UnaryOp(occurrence.code) != row.op {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 3, CompileReasonOccurrenceUnavailable)
		}
		pointFound := false
		for _, point := range occurrence.points {
			pointFound = pointFound || point == row.point
		}
		if !pointFound {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 5, CompileReasonOccurrenceUnavailable)
		}
	}
	valuesRows := make(map[identity.ContentID]struct{}, len(artifact.values))
	for _, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
		}
		valuesRows[row.ID()] = struct{}{}
	}
	seenHeapAllocations := make(map[identity.ContentID]struct{}, len(artifact.heapAllocations))
	for index, allocation := range artifact.heapAllocations {
		if !allocation.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, exists := occurrenceRows[OccurrenceAllocation][allocation.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, duplicate := seenHeapAllocations[allocation.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		seenHeapAllocations[allocation.id] = struct{}{}
		for fieldIndex, field := range allocation.fields {
			if !field.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := occurrenceRows[OccurrenceAllocationField][field.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := valuesRows[field.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	seenHeapIndexes := make(map[identity.ContentID]struct{}, len(artifact.heapIndexes))
	for index, access := range artifact.heapIndexes {
		if !access.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		kind := OccurrenceIndexWrite
		if access.read {
			kind = OccurrenceIndexRead
		}
		if _, exists := occurrenceRows[kind][access.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if _, duplicate := seenHeapIndexes[access.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !access.read {
			if _, exists := valuesRows[access.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
			}
		}
		seenHeapIndexes[access.id] = struct{}{}
	}
	if artifact.ruleOccurrences == nil {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for role, rows := range artifact.ruleOccurrences {
		if !role.valid() || !ruleRoleSupported(role) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		for index, occurrence := range rows {
			if !occurrence.Available() || occurrence.role != role || int(occurrence.occurrence) >= len(artifact.occurrences) {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if _, exists := pointRows[occurrence.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
			}
			if occurrence.input.Available() {
				if _, exists := pointRows[occurrence.input]; !exists {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
				}
			}
			if occurrence.inputKind == RuleInputPredecessor {
				if _, duplicate := environmentRouteDuplicates[occurrence.route]; duplicate {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
				edge, found := environmentByRoute[occurrence.route]
				if !found || edge.from != occurrence.input {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
			}
		}
	}
	return CompileFailure{}
}

func routeKind(kind flow.BoundaryArmKind) (RouteKind, bool) {
	switch kind {
	case flow.BoundaryLocal:
		return RouteLocal, true
	case flow.BoundaryResume:
		return RouteResume, true
	case flow.BoundarySelectTrue:
		return RouteSelectTrue, true
	case flow.BoundarySelectFalse:
		return RouteSelectFalse, true
	case flow.BoundaryTail:
		return RouteTail, true
	case flow.BoundaryThrow:
		return RouteThrow, true
	case flow.BoundaryYield:
		return RouteYield, true
	case flow.BoundaryCancel:
		return RouteCancel, true
	default:
		return RouteInvalid, false
	}
}

type field struct {
	bytes []byte
	uint  uint64
	kind  uint8
}

const (
	fieldBytes uint8 = iota + 1
	fieldUint
	fieldBool
)

func bytesField(value identity.ContentID) field { return field{bytes: value[:], kind: fieldBytes} }
func uintField(value uint64) field              { return field{uint: value, kind: fieldUint} }
func boolField(value bool) field {
	if value {
		return field{uint: 1, kind: fieldBool}
	}
	return field{kind: fieldBool}
}

func digest(domain string, version uint64, fields ...field) identity.ContentID {
	var writer canonical.DigestWriter
	if writer.Reset(domain, version) != nil {
		return identity.ContentID{}
	}
	for _, value := range fields {
		var err error
		switch value.kind {
		case fieldBytes:
			err = writer.Bytes(value.bytes)
		case fieldUint, fieldBool:
			err = writer.Uint(value.uint)
		default:
			return identity.ContentID{}
		}
		if err != nil {
			return identity.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(writer.Sum())
}

func artifactID(artifact *Artifact) identity.ContentID {
	fields := append([]field{bytesField(artifact.key.ID())}, artifact.key.identityFields()...)
	fields = append(fields, uintField(pointGeometryLawVersion))
	fields = append(fields, uintField(pointAttachmentLawVersion))
	fields = append(fields, uintField(uint64(len(artifact.points))))
	for _, point := range artifact.points {
		fields = append(fields, bytesField(point.id), boolField(point.initial), uintField(uint64(len(point.decisions))))
		for _, decision := range point.decisions {
			fields = append(fields, bytesField(decision))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.pointAttachments))))
	for _, row := range artifact.pointAttachments {
		fields = append(fields, bytesField(row.site), bytesField(row.point))
	}
	fields = append(fields, uintField(uint64(len(artifact.values))))
	for _, row := range artifact.values {
		fields = append(fields, bytesField(row.id), bytesField(row.body), uintField(uint64(len(row.members))))
		for _, member := range row.members {
			fields = append(fields, bytesField(member.id))
		}
		fields = append(fields, boolField(row.tail.present), uintField(uint64(row.tail.kind)), bytesField(row.tail.id))
	}
	fields = append(fields, uintField(packReceiptLawVersion), uintField(uint64(artifact.packReceipt.BindCount())))
	for index := 0; index < artifact.packReceipt.BindCount(); index++ {
		row, _ := artifact.packReceipt.BindAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.BodyID()), bytesField(row.ValuesID()), uintField(uint64(row.CellCount())))
		for cellIndex := 0; cellIndex < row.CellCount(); cellIndex++ {
			cell, _ := row.CellAt(cellIndex)
			fields = append(fields, bytesField(cell))
		}
	}
	fields = append(fields, uintField(uint64(artifact.packReceipt.BodyCount())))
	for index := 0; index < artifact.packReceipt.BodyCount(); index++ {
		row, _ := artifact.packReceipt.BodyAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.ContextID()), boolField(row.Callable()), uintField(uint64(row.FormalCount())))
		for formalIndex := 0; formalIndex < row.FormalCount(); formalIndex++ {
			formal, _ := row.FormalAt(formalIndex)
			fields = append(fields, bytesField(formal.FormalID()), bytesField(formal.CellID()), bytesField(formal.StorageCellID()))
		}
	}
	fields = append(fields, uintField(uint64(artifact.packReceipt.CallCount())))
	for index := 0; index < artifact.packReceipt.CallCount(); index++ {
		row, _ := artifact.packReceipt.CallAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.BodyID()), bytesField(row.FormalID()), bytesField(row.ValuesID()), bytesField(row.TypeArgumentsID()), bytesField(row.CalleeID()), bytesField(row.ActualsID()), uintField(uint64(row.Form())))
		receiver, hasReceiver := row.ReceiverID()
		tail, hasTail := row.TailID()
		fields = append(fields, boolField(hasReceiver), bytesField(receiver), boolField(hasTail), bytesField(tail), uintField(uint64(row.ArgumentCount())))
		for argumentIndex := 0; argumentIndex < row.ArgumentCount(); argumentIndex++ {
			argument, _ := row.ArgumentAt(argumentIndex)
			fields = append(fields, bytesField(argument))
		}
		fields = append(fields, uintField(uint64(row.TypeArgumentCount())))
		for argumentIndex := 0; argumentIndex < row.TypeArgumentCount(); argumentIndex++ {
			argument, _ := row.TypeArgumentAt(argumentIndex)
			fields = append(fields, bytesField(argument))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.bodies))))
	for _, body := range artifact.bodies {
		fields = append(fields, bytesField(body.id), bytesField(body.context), bytesField(body.entry), boolField(body.callable), bytesField(body.function), bytesField(body.formal), uintField(uint64(len(body.entryPoints))))
		for _, point := range body.entryPoints {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(body.roots))))
		for _, root := range body.roots {
			fields = append(fields, bytesField(root.id), uintField(uint64(root.family)))
		}
		fields = append(fields, uintField(uint64(body.outcomeStart)), uintField(uint64(body.outcomeEnd)))
	}
	fields = append(fields, uintField(functionBoundaryLawVersion), uintField(uint64(len(artifact.functionBoundaries))))
	for _, boundary := range artifact.functionBoundaries {
		fields = append(fields,
			bytesField(boundary.id), bytesField(boundary.body), bytesField(boundary.bodyContext), bytesField(boundary.entry), bytesField(boundary.callFormal),
			uintField(uint64(len(boundary.formals))),
		)
		for _, port := range boundary.formals {
			fields = append(fields, bytesField(port.id), bytesField(port.cell), bytesField(port.storage), bytesField(port.declared), uintField(uint64(port.position)))
		}
		fields = append(fields, boolField(boundary.hasVararg), bytesField(boundary.vararg.id), bytesField(boundary.vararg.cell), uintField(uint64(len(boundary.captures))))
		for _, capture := range boundary.captures {
			fields = append(fields,
				bytesField(capture.id), bytesField(capture.inner), bytesField(capture.outer), bytesField(capture.innerBody), bytesField(capture.outerBody), uintField(uint64(capture.position)),
			)
		}
		fields = append(fields, uintField(uint64(len(boundary.outcomes))))
		for _, outcome := range boundary.outcomes {
			fields = append(fields, bytesField(outcome))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.callTargets))))
	for _, target := range artifact.callTargets {
		fields = append(fields, bytesField(target.allocation), bytesField(target.body), bytesField(target.context), bytesField(target.function), bytesField(target.formal))
	}
	fields = append(fields, uintField(uint64(len(artifact.boundaries))))
	for _, row := range artifact.boundaries {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.owner), uintField(uint64(row.position)), boolField(row.eligible))
	}
	fields = append(fields, uintField(uint64(len(artifact.outcomes))))
	for _, outcome := range artifact.outcomes {
		fields = append(fields,
			bytesField(outcome.id), bytesField(outcome.body), uintField(uint64(outcome.kind)),
			boolField(outcome.hasTarget), bytesField(outcome.target),
			boolField(outcome.hasPropagation), bytesField(outcome.propagation),
			uintField(uint64(outcome.returnStart)), uintField(uint64(outcome.returnEnd)), uintField(uint64(len(outcome.points))),
		)
		for _, point := range outcome.points {
			fields = append(fields, bytesField(point))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.returnValues))))
	for _, value := range artifact.returnValues {
		fields = append(fields, bytesField(value.id))
	}
	fields = append(fields, uintField(uint64(len(artifact.occurrences))))
	for _, row := range artifact.occurrences {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.body), uintField(row.code), uintField(uint64(len(row.points))))
		for _, point := range row.points {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(row.inputs))))
		for _, input := range row.inputs {
			fields = append(fields, bytesField(input))
		}
		fields = append(fields, uintField(uint64(row.literalFamily)), boolField(row.literalOK), uintField(uint64(row.literal.Kind)), boolField(row.literal.Bool), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits), field{bytes: []byte(row.literal.String), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.exactScalarSummaries))))
	for _, row := range artifact.exactScalarSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.subject), bytesField(row.body),
			uintField(uint64(row.role)), uintField(uint64(row.literal.Kind)), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits))
	}
	fields = append(fields, uintField(uint64(len(artifact.arithmeticSummaries))))
	for _, row := range artifact.arithmeticSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), uintField(uint64(row.op)),
			uintField(uint64(row.left)), uintField(uint64(row.right)), uintField(uint64(row.result)), uintField(uint64(row.divisor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.unarySummaries))))
	for _, row := range artifact.unarySummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), bytesField(row.point), uintField(uint64(row.op)),
			uintField(uint64(row.operand)), uintField(uint64(row.result)))
	}
	fields = append(fields, uintField(uint64(len(artifact.heapAllocations))))
	for _, allocation := range artifact.heapAllocations {
		fields = append(fields, bytesField(allocation.id), uintField(uint64(allocation.role)), uintField(uint64(allocation.form)), bytesField(allocation.rootSpan), uintField(uint64(len(allocation.fields))))
		for _, field := range allocation.fields {
			fields = append(fields, bytesField(field.id), uintField(uint64(field.kind)), bytesField(field.fieldSpan), bytesField(field.selectorSpan), bytesField(field.valuesSpan), bytesField(field.valuesID), uintField(uint64(field.width)), boolField(field.finalOpen), boolField(field.sharesFirstValueCell), uintField(uint64(field.normalized)), boolField(field.normalizedOK))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.heapIndexes))))
	for _, access := range artifact.heapIndexes {
		fields = append(fields, bytesField(access.id), boolField(access.read), bytesField(access.baseSpan), bytesField(access.resultSpan), bytesField(access.keySpan), uintField(uint64(access.lensKind)), uintField(uint64(access.exactKey)), bytesField(access.valuesSpan), bytesField(access.valuesID), uintField(uint64(access.position+1)))
	}
	fields = append(fields, uintField(diagnosticLawVersion), uintField(uint64(len(artifact.diagnosticObservations))))
	for _, row := range artifact.diagnosticObservations {
		fields = append(fields,
			bytesField(row.id), uintField(uint64(row.kind)),
			field{bytes: []byte(row.location.File), kind: fieldBytes}, uintField(uint64(row.location.StartLine)),
			uintField(uint64(row.location.StartCol)), uintField(uint64(row.location.EndLine)), uintField(uint64(row.location.EndCol)),
		)
		switch row.kind {
		case DiagnosticObservationBranchCondition:
			fields = append(fields, bytesField(row.branch.decision), bytesField(row.branch.value), uintField(uint64(len(row.branch.points))))
			for _, point := range row.branch.points {
				fields = append(fields, bytesField(point))
			}
		case DiagnosticObservationTypeReferenceUnresolved:
			fields = append(fields, bytesField(row.unresolved.reference), bytesField(row.unresolved.root), uintField(uint64(len(row.unresolved.path))))
			for _, component := range row.unresolved.path {
				fields = append(fields, field{bytes: []byte(component), kind: fieldBytes})
			}
		case DiagnosticObservationValueReferenceUnresolved:
			fields = append(fields, bytesField(row.value.read), bytesField(row.value.cell), field{bytes: []byte(row.value.name), kind: fieldBytes})
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeArguments))))
	for _, row := range artifact.staticTypeArguments {
		fields = append(fields, bytesField(row.id), bytesField(row.call), bytesField(row.types), bytesField(row.reference), uintField(uint64(row.index)))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeValues))))
	for _, row := range artifact.staticTypeValues {
		fields = append(fields, bytesField(row.id), bytesField(row.body), bytesField(row.reference), bytesField(row.root), field{bytes: []byte(row.name), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeNodes))))
	for _, row := range artifact.staticTypeNodes {
		exact := row.exact
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), field{bytes: []byte(row.name), kind: fieldBytes}, uintField(uint64(row.key)), uintField(uint64(row.literal)), uintField(row.bits), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, boolField(row.flag), uintField(uint64(row.resolution)), uintField(uint64(row.assertParam)), bytesField(row.declaration), bytesField(row.operand), bytesField(row.scope), bytesField(row.assertionNarrow), uintField(uint64(row.assertionCoordinate[0])), uintField(uint64(row.assertionCoordinate[1])), uintField(uint64(row.assertionCoordinate[2])), uintField(uint64(row.assertionCoordinate[3])), bytesField(row.typeFunctionVariadic), uintField(uint64(len(row.aliasParams))))
		for _, child := range row.aliasParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceExtends))))
		for _, child := range row.interfaceExtends {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceMemberTypes))))
		for _, child := range row.interfaceMemberTypes {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionTypeParams))))
		for _, child := range row.typeFunctionTypeParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionParams))))
		for _, child := range row.typeFunctionParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionReturns))))
		for _, child := range row.typeFunctionReturns {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.fieldKeys))))
		for index, key := range row.fieldKeys {
			fields = append(fields, uintField(uint64(key)))
			text := ""
			if index < len(row.fieldTexts) {
				text = row.fieldTexts[index]
			}
			optional := false
			if index < len(row.fieldOptional) {
				optional = row.fieldOptional[index]
			}
			readonly := false
			if index < len(row.fieldReadonly) {
				readonly = row.fieldReadonly[index]
			}
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes}, boolField(optional), boolField(readonly))
		}
		fields = append(fields, uintField(uint64(len(row.keys))))
		for _, key := range row.keys {
			fields = append(fields, uintField(uint64(key)))
		}
		for index := range row.keys {
			text := ""
			if index < len(row.texts) {
				text = row.texts[index]
			}
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes})
			optional := false
			if index < len(row.optional) {
				optional = row.optional[index]
			}
			memberKind := uint8(0)
			if index < len(row.memberKinds) {
				memberKind = row.memberKinds[index]
			}
			fields = append(fields, boolField(optional), uintField(uint64(memberKind)))
		}
		fields = append(fields, uintField(uint64(len(row.segments))))
		for _, segment := range row.segments {
			fields = append(fields, uintField(uint64(segment)))
		}
		fields = append(fields, boolField(row.returnsKnown))
		fields = append(fields, uintField(uint64(len(row.sourceKeys))))
		for _, key := range row.sourceKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.canonicalKeys))))
		for _, key := range row.canonicalKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.children))))
		for _, child := range row.children {
			fields = append(fields, bytesField(child))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticExpressions))))
	for _, row := range artifact.staticExpressions {
		fields = append(fields, bytesField(row.id), bytesField(row.reference), bytesField(row.owner))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticInputs))))
	for _, row := range artifact.staticInputs {
		exact := row.literal
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), uintField(uint64(row.operandKind)), bytesField(row.expression), bytesField(row.source), bytesField(row.target), bytesField(row.operand), bytesField(row.frontier), bytesField(row.operandReference), bytesField(row.operandSubject), bytesField(row.operandBody), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.cursor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.environment))))
	for _, edge := range artifact.environment {
		fields = append(fields,
			bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), bytesField(edge.route),
			uintField(uint64(edge.arm)), bytesField(edge.guard), bytesField(edge.decision), bytesField(edge.condition), boolField(edge.guarded), boolField(edge.truth),
			bytesField(edge.component), bytesField(edge.mu), boolField(edge.hasMu),
			bytesField(edge.reset), boolField(edge.hasReset), uintField(uint64(len(edge.resets))),
		)
		for _, reset := range edge.resets {
			fields = append(fields, bytesField(reset))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.localTransfers))))
	for _, edge := range artifact.localTransfers {
		fields = append(fields, bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), boolField(edge.full), uintField(uint64(len(edge.roles))))
		for _, role := range edge.roles {
			fields = append(fields, uintField(uint64(role)))
		}
	}
	for roleIndex := 0; roleIndex < MountedRuleRoleCount(); roleIndex++ {
		role, roleOK := MountedRuleRoleAt(roleIndex)
		if !roleOK {
			continue
		}
		rows := artifact.ruleOccurrences[role]
		fields = append(fields, uintField(uint64(role)), uintField(uint64(len(rows))))
		for _, row := range rows {
			fields = append(fields,
				uintField(uint64(row.occurrence)), bytesField(row.point), bytesField(row.input),
				uintField(uint64(row.stage)), uintField(uint64(row.inputKind)), bytesField(row.route),
			)
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.regions))))
	for _, region := range artifact.regions {
		fields = append(fields,
			bytesField(region.id), bytesField(region.head), bytesField(region.sourceHead), bytesField(region.parent), boolField(region.cyclic),
			uintField(uint64(len(region.members))),
		)
		for _, member := range region.members {
			fields = append(fields, bytesField(member))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.events))))
	for _, event := range artifact.events {
		fields = append(fields, uintField(uint64(event.kind)), bytesField(event.region), bytesField(event.point))
	}
	return digest(artifactIDDomain, artifactFormat, fields...)
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
