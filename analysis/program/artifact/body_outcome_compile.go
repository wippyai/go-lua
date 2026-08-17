package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func outcomeKind(kind flowkind.OutcomeKind) (OutcomeKind, bool) {
	switch kind {
	case flowkind.OutcomeNormal:
		return OutcomeNormal, true
	case flowkind.OutcomeReturn:
		return OutcomeReturn, true
	case flowkind.OutcomeThrow:
		return OutcomeThrow, true
	case flowkind.OutcomeBreak:
		return OutcomeBreak, true
	case flowkind.OutcomeGoto:
		return OutcomeGoto, true
	case flowkind.OutcomeYield:
		return OutcomeYield, true
	case flowkind.OutcomeCancel:
		return OutcomeCancel, true
	default:
		return OutcomeInvalid, false
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
	bodies := make([]BodyRow, bodyCount)
	outcomes := make([]OutcomeRow, 0)
	returnValues := make([]ReturnValue, 0)
	flowView := compiler.input.Flow()
	boundaries := flowView.FunctionBoundaries()
	bodyReturns := flowView.BodyReturns()
	outcomesView := flowView.Outcomes()
	sites := flowView.Causal().Sites()
	seenBodies := make(map[identity.ContentID]struct{}, bodyCount)
	seenBodyContexts := make(map[identity.ContentID]struct{}, bodyCount)
	seenOutcomes := make(map[identity.ContentID]int)

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
			formal, formalOK := body.CallTarget()
			var functionIDOK bool
			functionID, functionIDOK = compiler.input.FunctionID(function)
			if formalOK {
				formalID, formalOK = formal.ID()
			}
			if !formalOK || !functionIDOK || !functionID.Available() || !formalID.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
			}
			callable = true
		}
		rootCatalog, rootsOK := body.ExecutableRoots()
		rootCount := rootCatalog.Count()
		if !rootsOK || !compiler.input.OwnsExecutableRoots(rootCatalog) || !fitsUint32(rootCount) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		roots := make([]RootRow, rootCount)
		seenRoots := make(map[identity.ContentID]struct{}, rootCount)
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			root, rootOK := rootCatalog.At(rootIndex)
			if !rootOK || !compiler.input.OwnsExecutableRoot(root) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			row := RootRow{id: root.ID(), family: root.Family()}
			if !row.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyIdentity)
			}
			if _, duplicate := seenRoots[row.id]; duplicate {
				return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyDuplicate)
			}
			seenRoots[row.id] = struct{}{}
			roots[rootIndex] = row
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
			case OutcomeBreak:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLoop {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeTarget)
				}
				hasTarget = true
			case OutcomeGoto:
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

			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			returnStart := uint32(len(returnValues))
			if hasReturn && outcomeID == returnedID {
				if matchedReturn || kind != OutcomeReturn {
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
					returnValues = append(returnValues, ReturnValue{id: valuesID})
				}
			}
			if !fitsUint32(len(returnValues)) {
				return compileFailure(CompileStageBodyOutcomes, CompileRowReturnValue, bodyIndex, outcomeIndex, CompileReasonOutcomeRange)
			}
			row := OutcomeRow{
				id: outcomeID, body: bodyID, target: target, propagation: propagationID, kind: kind,
				hasTarget: hasTarget, hasPropagation: propagationID.Available(),
				returnStart: returnStart, returnEnd: uint32(len(returnValues)), points: points, sealed: true,
			}
			if !row.Available() {
				return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, outcomeIndex, CompileReasonOutcomeShape)
			}
			seenOutcomes[outcomeID] = len(outcomes)
			outcomes = append(outcomes, row)
		}
		if hasReturn != matchedReturn || !fitsUint32(len(outcomes)) {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeReturn)
		}
		bodies[bodyIndex] = BodyRow{id: bodyID, context: context, entry: entryID, function: functionID, formal: formalID, callable: callable, entryPoints: entryPoints, roots: roots, outcomeStart: start, outcomeEnd: uint32(len(outcomes)), sealed: true}
		if !bodies[bodyIndex].Available() {
			return compileFailure(CompileStageBodyOutcomes, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
	}

	for index, row := range outcomes {
		if !row.hasPropagation {
			continue
		}
		nextIndex, exists := seenOutcomes[row.propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := outcomes[nextIndex]
		if next.kind != row.kind || next.hasTarget != row.hasTarget || next.target != row.target {
			return compileFailure(CompileStageBodyOutcomes, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	compiler.bodies, compiler.outcomes, compiler.returnValues = bodies, outcomes, returnValues
	return CompileFailure{}
}
