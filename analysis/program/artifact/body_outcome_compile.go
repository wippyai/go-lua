package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func outcomeKind(kind flowkind.OutcomeKind) (programschema.OutcomeKind, bool) {
	switch kind {
	case flowkind.OutcomeNormal:
		return programschema.OutcomeNormal, true
	case flowkind.OutcomeReturn:
		return programschema.OutcomeReturn, true
	case flowkind.OutcomeThrow:
		return programschema.OutcomeThrow, true
	case flowkind.OutcomeBreak:
		return programschema.OutcomeBreak, true
	case flowkind.OutcomeGoto:
		return programschema.OutcomeGoto, true
	case flowkind.OutcomeYield:
		return programschema.OutcomeYield, true
	case flowkind.OutcomeCancel:
		return programschema.OutcomeCancel, true
	default:
		return programschema.OutcomeInvalid, false
	}
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }

func (compiler *compiler) copyBodiesAndOutcomesFailure() CompileFailure {
	bodyCount := compiler.input.BodyCount()
	if bodyCount <= 0 || !fitsUint32(bodyCount) {
		return compileFailure(CompileStageBodyOutcomes, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	valueIDs := make(map[identity.ContentID]struct{}, len(compiler.values))
	for _, row := range compiler.values {
		if !row.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, -1, -1, CompileReasonReturnValueReference)
		}
		valueIDs[row.id] = struct{}{}
	}
	bodies := make([]programschema.Body, bodyCount)
	bodyEntries := make([]programschema.BodyEntry, 0, bodyCount)
	bodyRoots := make([]programschema.BodyRoot, 0, bodyCount)
	outcomes := make([]programschema.Outcome, 0)
	returnValues := make([]programschema.OutcomeReturnValue, 0)
	outcomePoints := make([]programschema.OutcomePoint, 0)
	flowView := compiler.input.Flow()
	boundaries := flowView.FunctionBoundaries()
	bodyReturns := flowView.BodyReturns()
	outcomesView := flowView.Outcomes()
	sites := flowView.Causal().Sites()
	seenBodies := make(map[identity.ContentID]struct{}, bodyCount)
	seenBodyContexts := make(map[identity.ContentID]struct{}, bodyCount)
	seenOutcomes := make(map[identity.ContentID]int)
	programID := compiler.key.ProgramID()

	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, ok := compiler.input.BodyAt(bodyIndex)
		if !ok || !body.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if !compiler.input.OwnsBody(body) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyForeign)
		}
		bodyID := body.PathID()
		if !bodyID.Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyIdentity)
		}
		if _, duplicate := seenBodies[bodyID]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		context := body.ContextID()
		bodyBoundary, bodyBoundaryOK := boundaries.ResolveBodyContextID(context)
		bodyTerm, bodyTermOK := bodyBoundary.Body()
		if !bodyBoundaryOK || !bodyBoundary.Available() || !bodyTermOK || bodyBoundary.ContextID() != context || !boundaries.OwnsBody(bodyBoundary) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		entry, entryOK := body.EntrySite()
		entryID := entry.PathID()
		entryPoints := compiler.pointIDs(entry)
		if !context.Available() || !entryOK || !entryID.Available() || !compiler.input.OwnsSite(entry) || len(entryPoints) == 0 {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenBodyContexts[context]; duplicate {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		functionID, formalID := identity.ContentID{}, identity.ContentID{}
		callable := false
		if function, functionOK := body.Function(); functionOK {
			formal, formalOK := flowView.CallBodyTarget(function)
			var functionIDOK bool
			functionID, functionIDOK = artifactFunctionID(programID, flowView, function)
			if formalOK {
				formalID, formalOK = formal.ID()
			}
			if !formalOK || !functionIDOK || !functionID.Available() || !formalID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			callable = true
		}
		if !fitsUint32(len(bodyEntries)) || !fitsUint32(len(entryPoints)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		entryOffset := uint32(len(bodyEntries))
		for pointIndex, point := range entryPoints {
			row, rowOK := programschema.NewBodyEntry(bodyID, point)
			if !rowOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			bodyEntries = append(bodyEntries, row)
		}
		executableRoots := flowView.Executable()
		rootCount, rootsOK := executableRoots.RootCount(bodyTerm)
		if !rootsOK || !fitsUint32(rootCount) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		if !fitsUint32(len(bodyRoots)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		rootOffset := uint32(len(bodyRoots))
		seenRoots := make(map[identity.ContentID]struct{}, rootCount)
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			rootID, rootFamily, rootOK := executableRoots.RootAt(bodyTerm, rootIndex)
			if !rootOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			row, rowOK := programschema.NewBodyRoot(bodyID, rootID, uint8(rootFamily))
			if !rowOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenRoots[row.ID()]; duplicate {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyDuplicate)
			}
			seenRoots[row.ID()] = struct{}{}
			bodyRoots = append(bodyRoots, row)
		}
		seenBodies[bodyID] = struct{}{}
		seenBodyContexts[context] = struct{}{}
		if !fitsUint32(len(outcomes)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		start := uint32(len(outcomes))

		returned, hasReturn := bodyReturns.ForBody(bodyBoundary)
		returnedID := identity.ContentID{}
		if hasReturn {
			returnSite, siteOK := returned.Outcome()
			returnedTerm, termOK := returnSite.Term()
			returnExit, _, returnOK := bodyBoundary.OutcomeForTerm(returnedTerm)
			if !siteOK || !sites.Owns(returnSite) || !termOK || !returnOK || returnExit.Outcome != returnedTerm || returnExit.Kind != flowkind.OutcomeReturn || returnExit.Target != 0 {
				hasReturn = false
			} else {
				returnedID, _ = flowView.SemanticTermPath(returnedTerm)
			}
		}
		matchedReturn := false
		outcomeCount := bodyBoundary.OutcomeCount()
		if outcomeCount <= 0 || !fitsUint32(outcomeCount) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		for outcomeIndex := 0; outcomeIndex < outcomeCount; outcomeIndex++ {
			exit, outcomeOK := bodyBoundary.OutcomeAt(outcomeIndex)
			if !outcomeOK || exit.Outcome == 0 {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeUnavailable)
			}
			ownerBoundary, ownerOK := boundaries.ForOutcome(exit.Outcome)
			ownerBody, ownerBodyOK := ownerBoundary.Body()
			ownerExit, ownerOrdinal, ownerExitOK := ownerBoundary.OutcomeForTerm(exit.Outcome)
			metadata, metadataOK := outcomesView.Get(exit.Outcome)
			if !ownerOK || !ownerBoundary.Available() || !boundaries.OwnsBody(ownerBoundary) || !ownerBodyOK || ownerBody != bodyTerm || !ownerExitOK || ownerOrdinal != outcomeIndex || ownerExit.Outcome != exit.Outcome || !metadataOK || metadata.Body != bodyTerm || metadata.Kind != exit.Kind || metadata.Target != exit.Target {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeForeign)
			}
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK && (!site.Available() || !sites.Owns(site)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeUnavailable)
			}
			outcomeID, outcomePathOK := flowView.SemanticTermPath(exit.Outcome)
			if !outcomePathOK || !outcomeID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeIdentity)
			}
			if _, duplicate := seenOutcomes[outcomeID]; duplicate {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeDuplicate)
			}
			kind, converted := outcomeKind(exit.Kind)
			if !converted {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeKind)
			}
			target := identity.ContentID{}
			hasTarget := false
			switch kind {
			case programschema.OutcomeBreak:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLoop {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				hasTarget = true
			case programschema.OutcomeGoto:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLabel {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				hasTarget = true
			}

			propagationID := identity.ContentID{}
			if nextTerm, propagated := outcomesView.Propagation(exit.Outcome); propagated {
				nextBoundary, nextOK := boundaries.ForOutcome(nextTerm)
				nextBody, nextBodyOK := nextBoundary.Body()
				nextExit, _, nextExitOK := nextBoundary.OutcomeForTerm(nextTerm)
				nextMetadata, nextMetadataOK := outcomesView.Get(nextTerm)
				nextPath, nextPathOK := flowView.SemanticTermPath(nextTerm)
				nextSite, nextSiteOK := sites.ForTerm(nextTerm)
				if nextSiteOK && (!nextSite.Available() || !sites.Owns(nextSite)) {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
				if !nextOK || !nextBoundary.Available() || !boundaries.OwnsBody(nextBoundary) || !nextBodyOK || nextBody == 0 || !nextExitOK || nextExit.Outcome != nextTerm || !nextMetadataOK || nextMetadata.Body != nextBody || nextExit.Kind != exit.Kind || nextExit.Target != exit.Target || !nextPathOK || !nextPath.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
				propagationID = nextPath
				if propagationID == outcomeID {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomePropagation)
				}
			}
			points := []identity.ContentID(nil)
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK {
				if !site.Available() || !sites.Owns(site) {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeAttachment)
				}
				points = compiler.pointIDs(site)
			}

			if !fitsUint32(len(outcomePoints)) || !fitsUint32(len(points)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			pointOffset := uint32(len(outcomePoints))
			for pointIndex, point := range points {
				child, childOK := programschema.NewOutcomePoint(outcomeID, point)
				if !childOK {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, pointIndex, CompileReasonOutcomeAttachment)
				}
				outcomePoints = append(outcomePoints, child)
			}

			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			returnStart := uint32(len(returnValues))
			if hasReturn && outcomeID == returnedID {
				if matchedReturn || kind != programschema.OutcomeReturn {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeReturn)
				}
				matchedReturn = true
				for returnIndex := 0; returnIndex < returned.ValuesCount(); returnIndex++ {
					valueSite, valueOK := returned.ValueAt(returnIndex)
					valueTerm, termOK := valueSite.Term()
					valueRow, rowOK := compiler.valueRowForTerm(valueTerm)
					valuesID := valueRow.ID()
					if !valueOK || !valueSite.Available() || !sites.Owns(valueSite) || !termOK || !rowOK || !valuesID.Available() {
						return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, returnIndex, CompileReasonReturnValueUnavailable)
					}
					if _, exists := valueIDs[valuesID]; !exists {
						return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, returnIndex, CompileReasonReturnValueReference)
					}
					child, childOK := programschema.NewOutcomeReturnValue(outcomeID, valuesID)
					if !childOK {
						return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, returnIndex, CompileReasonReturnValueUnavailable)
					}
					returnValues = append(returnValues, child)
				}
			}
			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			row, rowOK := programschema.NewOutcome(
				outcomeID, bodyID, target, propagationID, kind,
				returnStart, uint32(len(returnValues))-returnStart,
				pointOffset, uint32(len(outcomePoints))-pointOffset,
				hasTarget, propagationID.Available(),
			)
			if !rowOK {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeShape)
			}
			seenOutcomes[outcomeID] = len(outcomes)
			outcomes = append(outcomes, row)
		}
		if hasReturn != matchedReturn || !fitsUint32(len(outcomes)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeReturn)
		}
		bodyRow, bodyRowOK := programschema.NewBody(
			bodyID, context, entryID, functionID, formalID,
			entryOffset, uint32(len(bodyEntries))-entryOffset,
			rootOffset, uint32(len(bodyRoots))-rootOffset,
			start, uint32(len(outcomes))-start,
			callable,
		)
		if !bodyRowOK {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		bodies[bodyIndex] = bodyRow
	}

	for index, row := range outcomes {
		propagation, propagated := row.PropagationID()
		if !propagated {
			continue
		}
		nextIndex, exists := seenOutcomes[propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := outcomes[nextIndex]
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	compiler.bodies, compiler.bodyEntries, compiler.bodyRoots = bodies, bodyEntries, bodyRoots
	compiler.outcomes, compiler.outcomeReturnValues, compiler.outcomePoints = outcomes, returnValues, outcomePoints
	return CompileFailure{}
}
