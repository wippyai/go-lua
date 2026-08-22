package bodyboundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }

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

func valueRowForTerm(values []programschema.Values, term keyspace.Term) (programschema.Values, bool) {
	if keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(values) {
		return programschema.Values{}, false
	}
	row := values[index]
	return row, row.Available()
}

func pointIDs(index map[identity.ContentID][]identity.ContentID, site causal.Site) []identity.ContentID {
	if !site.Available() || index == nil {
		return nil
	}
	points := index[site.ContextID()]
	return points
}

// Build emits the complete Body/Outcome plane and then the callable boundary
// planes from the same canonical Flow snapshot. It intentionally performs no
// parent mutation; the Bundle is the sole construction owner until transfer.
func Build(input Input) (*Bundle, programconstruction.Fault) {
	if input.Program == nil || !input.Program.Available() || !input.ProgramID.Available() {
		return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, -1, -1)
	}
	bodyCount := input.Program.BodyCount()
	if bodyCount <= 0 || !fitsUint32(bodyCount) {
		return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, -1, -1)
	}
	valueIDs := make(map[identity.ContentID]struct{}, len(input.Values))
	for _, row := range input.Values {
		if !row.Available() {
			return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueReturnValueReference, -1, -1)
		}
		valueIDs[row.ID()] = struct{}{}
	}
	bodies := make([]programschema.Body, bodyCount)
	bodyEntries := make([]programschema.BodyEntry, 0, bodyCount)
	bodyRoots := make([]programschema.BodyRoot, 0, bodyCount)
	outcomes := make([]programschema.Outcome, 0)
	returnValues := make([]programschema.OutcomeReturnValue, 0)
	outcomePoints := make([]programschema.OutcomePoint, 0)
	flowView := input.Program.Flow()
	boundaries := flowView.FunctionBoundaries()
	bodyReturns := flowView.ReturnProjection()
	outcomesView := flowView.Outcomes()
	sites := flowView.Causal().Sites()
	seenBodies := make(map[identity.ContentID]struct{}, bodyCount)
	seenBodyContexts := make(map[identity.ContentID]struct{}, bodyCount)
	seenOutcomes := make(map[identity.ContentID]int)

	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, ok := input.Program.BodyAt(bodyIndex)
		if !ok || !body.Available() {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		if !input.Program.OwnsBody(body) {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyForeign, bodyIndex, -1)
		}
		bodyID := body.PathID()
		if !bodyID.Available() {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyIdentity, bodyIndex, -1)
		}
		if _, duplicate := seenBodies[bodyID]; duplicate {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyDuplicate, bodyIndex, -1)
		}
		context := body.ContextID()
		bodyBoundary, bodyBoundaryOK := boundaries.ResolveBodyContextID(context)
		bodyTerm, bodyTermOK := bodyBoundary.Body()
		if !bodyBoundaryOK || !bodyBoundary.Available() || !bodyTermOK || bodyBoundary.ContextID() != context || !boundaries.OwnsBody(bodyBoundary) {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		entry, entryOK := body.EntrySite()
		entryID := entry.PathID()
		entryPoints := pointIDs(input.PointIDsBySite, entry)
		if !context.Available() || !entryOK || !entryID.Available() || !input.Program.OwnsSite(entry) || len(entryPoints) == 0 {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		if _, duplicate := seenBodyContexts[context]; duplicate {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyDuplicate, bodyIndex, -1)
		}
		functionID, formalID := identity.ContentID{}, identity.ContentID{}
		callable := false
		if function, functionOK := body.Function(); functionOK {
			var formalOK bool
			formalID, formalOK = programschema.CallFormalIdentity(context)
			var functionIDOK bool
			functionID, functionIDOK = input.Program.FunctionID(function)
			if !formalOK || !functionIDOK || !functionID.Available() || !formalID.Available() {
				return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
			}
			callable = true
		}
		if !fitsUint32(len(bodyEntries)) || !fitsUint32(len(entryPoints)) {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyRange, bodyIndex, -1)
		}
		entryOffset := uint32(len(bodyEntries))
		for pointIndex, point := range entryPoints {
			row, rowOK := programschema.NewBodyEntry(bodyID, point)
			if !rowOK {
				return nil, programconstruction.New(programcatalog.BodyEntry(), programconstruction.IssueBodyUnavailable, bodyIndex, pointIndex)
			}
			bodyEntries = append(bodyEntries, row)
		}
		executableRoots := flowView.Executable()
		rootCount, rootsOK := executableRoots.RootCount(bodyTerm)
		if !rootsOK || !fitsUint32(rootCount) {
			return nil, programconstruction.New(programcatalog.BodyRoot(), programconstruction.IssueBodyRange, bodyIndex, -1)
		}
		if !fitsUint32(len(bodyRoots)) {
			return nil, programconstruction.New(programcatalog.BodyRoot(), programconstruction.IssueBodyRange, bodyIndex, -1)
		}
		rootOffset := uint32(len(bodyRoots))
		seenRoots := make(map[identity.ContentID]struct{}, rootCount)
		for rootIndex := 0; rootIndex < rootCount; rootIndex++ {
			rootID, rootFamily, rootOK := executableRoots.RootAt(bodyTerm, rootIndex)
			if !rootOK {
				return nil, programconstruction.New(programcatalog.BodyRoot(), programconstruction.IssueBodyUnavailable, bodyIndex, rootIndex)
			}
			row, rowOK := programschema.NewBodyRoot(bodyID, rootID, uint8(rootFamily))
			if !rowOK {
				return nil, programconstruction.New(programcatalog.BodyRoot(), programconstruction.IssueBodyUnavailable, bodyIndex, rootIndex)
			}
			if _, duplicate := seenRoots[row.ID()]; duplicate {
				return nil, programconstruction.New(programcatalog.BodyRoot(), programconstruction.IssueBodyDuplicate, bodyIndex, rootIndex)
			}
			seenRoots[row.ID()] = struct{}{}
			bodyRoots = append(bodyRoots, row)
		}
		seenBodies[bodyID] = struct{}{}
		seenBodyContexts[context] = struct{}{}
		if !fitsUint32(len(outcomes)) {
			return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeRange, bodyIndex, -1)
		}
		start := uint32(len(outcomes))

		returnedTerm, returnedValueCount, hasReturn := bodyReturns.ForBody(bodyTerm)
		returnedID := identity.ContentID{}
		if hasReturn {
			returnSite, siteOK := sites.ForTerm(returnedTerm)
			ownedTerm, termOK := returnSite.Term()
			returnExit, _, returnOK := bodyBoundary.OutcomeForTerm(returnedTerm)
			if !siteOK || !sites.Owns(returnSite) || !termOK || ownedTerm != returnedTerm || !returnOK || returnExit.Outcome != returnedTerm || returnExit.Kind != flowkind.OutcomeReturn || returnExit.Target != 0 {
				hasReturn = false
			} else {
				returnedID, _ = flowView.SemanticTermPath(returnedTerm)
			}
		}
		matchedReturn := false
		outcomeCount := bodyBoundary.OutcomeCount()
		if outcomeCount <= 0 || !fitsUint32(outcomeCount) {
			return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeRange, bodyIndex, -1)
		}
		for outcomeIndex := 0; outcomeIndex < outcomeCount; outcomeIndex++ {
			exit, outcomeOK := bodyBoundary.OutcomeAt(outcomeIndex)
			if !outcomeOK || exit.Outcome == 0 {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeUnavailable, bodyIndex, outcomeIndex)
			}
			ownerBoundary, ownerOK := boundaries.ForOutcome(exit.Outcome)
			ownerBody, ownerBodyOK := ownerBoundary.Body()
			ownerExit, ownerOrdinal, ownerExitOK := ownerBoundary.OutcomeForTerm(exit.Outcome)
			metadataBody, metadataKind, metadataTarget, metadataOK := outcomesView.Get(exit.Outcome)
			if !ownerOK || !ownerBoundary.Available() || !boundaries.OwnsBody(ownerBoundary) || !ownerBodyOK || ownerBody != bodyTerm || !ownerExitOK || ownerOrdinal != outcomeIndex || ownerExit.Outcome != exit.Outcome || !metadataOK || metadataBody != bodyTerm || metadataKind != exit.Kind || metadataTarget != exit.Target {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeForeign, bodyIndex, outcomeIndex)
			}
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK && (!site.Available() || !sites.Owns(site)) {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeUnavailable, bodyIndex, outcomeIndex)
			}
			outcomeID, outcomePathOK := flowView.SemanticTermPath(exit.Outcome)
			if !outcomePathOK || !outcomeID.Available() {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeIdentity, bodyIndex, outcomeIndex)
			}
			if _, duplicate := seenOutcomes[outcomeID]; duplicate {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeDuplicate, bodyIndex, outcomeIndex)
			}
			kind, converted := outcomeKind(exit.Kind)
			if !converted {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeKind, bodyIndex, outcomeIndex)
			}
			target := identity.ContentID{}
			hasTarget := false
			switch kind {
			case programschema.OutcomeBreak:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLoop {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeTarget, bodyIndex, outcomeIndex)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeTarget, bodyIndex, outcomeIndex)
				}
				hasTarget = true
			case programschema.OutcomeGoto:
				if keyspace.TermFamily(exit.Target) != keyspace.FamilyLabel {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeTarget, bodyIndex, outcomeIndex)
				}
				var targetOK bool
				target, targetOK = flowView.SemanticTermPath(exit.Target)
				if !targetOK || !target.Available() {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeTarget, bodyIndex, outcomeIndex)
				}
				hasTarget = true
			}

			propagationID := identity.ContentID{}
			if nextTerm, propagated := outcomesView.Propagation(exit.Outcome); propagated {
				nextBoundary, nextOK := boundaries.ForOutcome(nextTerm)
				nextBody, nextBodyOK := nextBoundary.Body()
				nextExit, _, nextExitOK := nextBoundary.OutcomeForTerm(nextTerm)
				nextMetadataBody, _, _, nextMetadataOK := outcomesView.Get(nextTerm)
				nextPath, nextPathOK := flowView.SemanticTermPath(nextTerm)
				nextSite, nextSiteOK := sites.ForTerm(nextTerm)
				if nextSiteOK && (!nextSite.Available() || !sites.Owns(nextSite)) {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomePropagation, bodyIndex, outcomeIndex)
				}
				if !nextOK || !nextBoundary.Available() || !boundaries.OwnsBody(nextBoundary) || !nextBodyOK || nextBody == 0 || !nextExitOK || nextExit.Outcome != nextTerm || !nextMetadataOK || nextMetadataBody != nextBody || nextExit.Kind != exit.Kind || nextExit.Target != exit.Target || !nextPathOK || !nextPath.Available() {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomePropagation, bodyIndex, outcomeIndex)
				}
				propagationID = nextPath
				if propagationID == outcomeID {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomePropagation, bodyIndex, outcomeIndex)
				}
			}
			points := []identity.ContentID(nil)
			if site, siteOK := sites.ForTerm(exit.Outcome); siteOK {
				if !site.Available() || !sites.Owns(site) {
					return nil, programconstruction.New(programcatalog.OutcomePoint(), programconstruction.IssueOutcomeAttachment, bodyIndex, outcomeIndex)
				}
				points = pointIDs(input.PointIDsBySite, site)
			}
			if !fitsUint32(len(outcomePoints)) || !fitsUint32(len(points)) {
				return nil, programconstruction.New(programcatalog.OutcomePoint(), programconstruction.IssueOutcomeRange, bodyIndex, outcomeIndex)
			}
			pointOffset := uint32(len(outcomePoints))
			for pointIndex, point := range points {
				child, childOK := programschema.NewOutcomePoint(outcomeID, point)
				if !childOK {
					return nil, programconstruction.New(programcatalog.OutcomePoint(), programconstruction.IssueOutcomeAttachment, bodyIndex, pointIndex)
				}
				outcomePoints = append(outcomePoints, child)
			}

			if !fitsUint32(len(returnValues)) {
				return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueOutcomeRange, bodyIndex, outcomeIndex)
			}
			returnStart := uint32(len(returnValues))
			if hasReturn && outcomeID == returnedID {
				if matchedReturn || kind != programschema.OutcomeReturn {
					return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeReturn, bodyIndex, outcomeIndex)
				}
				matchedReturn = true
				for returnIndex := 0; returnIndex < returnedValueCount; returnIndex++ {
					projectedTerm, projected := bodyReturns.ValueAt(bodyTerm, returnIndex)
					valueSite, valueOK := sites.ForTerm(projectedTerm)
					valueTerm, termOK := valueSite.Term()
					valueRow, rowOK := valueRowForTerm(input.Values, valueTerm)
					valuesID := valueRow.ID()
					if !projected || !valueOK || !valueSite.Available() || !sites.Owns(valueSite) || !termOK || valueTerm != projectedTerm || !rowOK || !valuesID.Available() {
						return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueReturnValueUnavailable, bodyIndex, returnIndex)
					}
					if _, exists := valueIDs[valuesID]; !exists {
						return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueReturnValueReference, bodyIndex, returnIndex)
					}
					child, childOK := programschema.NewOutcomeReturnValue(outcomeID, valuesID)
					if !childOK {
						return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueReturnValueUnavailable, bodyIndex, returnIndex)
					}
					returnValues = append(returnValues, child)
				}
			}
			if !fitsUint32(len(returnValues)) {
				return nil, programconstruction.New(programcatalog.OutcomeReturnValue(), programconstruction.IssueOutcomeRange, bodyIndex, outcomeIndex)
			}
			row, rowOK := programschema.NewOutcome(
				outcomeID, bodyID, target, propagationID, kind,
				returnStart, uint32(len(returnValues))-returnStart,
				pointOffset, uint32(len(outcomePoints))-pointOffset,
				hasTarget, propagationID.Available(),
			)
			if !rowOK {
				return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeShape, bodyIndex, outcomeIndex)
			}
			seenOutcomes[outcomeID] = len(outcomes)
			outcomes = append(outcomes, row)
		}
		if hasReturn != matchedReturn || !fitsUint32(len(outcomes)) {
			return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeReturn, bodyIndex, -1)
		}
		bodyRow, bodyRowOK := programschema.NewBody(
			bodyID, context, entryID, functionID, formalID,
			entryOffset, uint32(len(bodyEntries))-entryOffset,
			rootOffset, uint32(len(bodyRoots))-rootOffset,
			start, uint32(len(outcomes))-start,
			callable,
		)
		if !bodyRowOK {
			return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
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
			return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomeReference, index, -1)
		}
		next := outcomes[nextIndex]
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return nil, programconstruction.New(programcatalog.Outcome(), programconstruction.IssueOutcomePropagation, index, -1)
		}
	}
	rootBoundary, rootOK := boundaries.Root()
	rootBody, rootBodyOK := rootBoundary.Body()
	rootBodyOrdinal := keyspace.TermOrdinal(rootBody)
	if !rootOK || !rootBoundary.Available() || !rootBodyOK || keyspace.TermFamily(rootBody) != keyspace.FamilyBody || rootBodyOrdinal == 0 ||
		uint64(rootBodyOrdinal) > uint64(len(bodies)) || !bodies[rootBodyOrdinal-1].Available() || bodies[rootBodyOrdinal-1].Callable() {
		return nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, -1, -1)
	}

	functionBoundaries, functionFormals, functionVarargs, functionCaptures, functionIDs, functionBoundaryByBody, fault := buildFunctionBoundaries(input, bodies, outcomes)
	if fault.Available() {
		return nil, fault
	}
	return &Bundle{
		bodies: bodies, bodyEntries: bodyEntries, bodyRoots: bodyRoots,
		entryBodyID: bodies[rootBodyOrdinal-1].ID(),
		outcomes:    outcomes, outcomeReturnValues: returnValues, outcomePoints: outcomePoints,
		functionBoundaries: functionBoundaries, functionFormals: functionFormals, functionVarargs: functionVarargs,
		functionCaptures: functionCaptures, functionIDsByTerm: functionIDs, functionBoundaryByBody: functionBoundaryByBody,
	}, programconstruction.Fault{}
}

func buildFunctionBoundaries(input Input, bodies []programschema.Body, outcomes []programschema.Outcome) ([]programschema.FunctionBoundary, []programschema.FunctionFormal, []programschema.FunctionVararg, []programschema.FunctionCapture, map[keyspace.Term]identity.ContentID, map[identity.ContentID]programschema.FunctionBoundary, programconstruction.Fault) {
	if len(bodies) == 0 || len(outcomes) == 0 {
		return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyUnavailable, -1, -1)
	}
	flowView := input.Program.Flow()
	boundaries := flowView.FunctionBoundaries()
	staticView := input.Program.Static()
	functionBoundaries := make([]programschema.FunctionBoundary, 0)
	functionFormals := make([]programschema.FunctionFormal, 0)
	functionVarargs := make([]programschema.FunctionVararg, 0)
	functionCaptures := make([]programschema.FunctionCapture, 0)
	functionIDs := make(map[keyspace.Term]identity.ContentID)
	functionBoundaryByBody := make(map[identity.ContentID]programschema.FunctionBoundary)
	for bodyIndex := 0; bodyIndex < input.Program.BodyCount(); bodyIndex++ {
		body, bodyOK := input.Program.BodyAt(bodyIndex)
		if !bodyOK || !input.Program.OwnsBody(body) || bodyIndex >= len(bodies) {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.Body(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		function, callable := body.Function()
		if !callable {
			continue
		}
		copiedBody := bodies[bodyIndex]
		functionID, functionOK := copiedBody.FunctionContextID()
		callFormalID, callFormalIDOK := programschema.CallFormalIdentity(copiedBody.ContextID())
		copiedFormalID, copiedFormalIDOK := copiedBody.CallFormalID()
		if !functionOK || !boundaries.OwnsFunction(function) || !copiedBody.Callable() || copiedBody.OutcomeCount() == 0 || !callFormalIDOK || !copiedFormalIDOK || copiedFormalID != callFormalID {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		if _, duplicate := functionBoundaryByBody[copiedBody.ID()]; duplicate {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyDuplicate, bodyIndex, -1)
		}
		if !fitsUint32(len(functionFormals)) || !fitsUint32(len(functionCaptures)) || !fitsUint32(len(functionVarargs)) {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyRange, bodyIndex, -1)
		}
		formalOffset := uint32(len(functionFormals))
		for position := 0; position < function.FormalCount(); position++ {
			formalID, cellID, formalOK := flowView.FunctionFormalIDs(function, position)
			term, termOK := function.FormalAt(position)
			storageID, storageOK := rowidentity.StorageCellID(input.ProgramID, flowView, term)
			declared := identity.ContentID{}
			if termOK {
				declared, _ = rowidentity.DeclaredStaticTypeID(input.ProgramID, staticView, term)
			}
			if !formalOK || !termOK || !storageOK || !storageID.Available() || uint64(position) > uint64(^uint32(0)) {
				return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionFormal(), programconstruction.IssueBodyUnavailable, bodyIndex, position)
			}
			formal, formalSealed := programschema.NewFunctionFormal(formalID, cellID, storageID, declared, uint32(position))
			if !formalSealed {
				return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionFormal(), programconstruction.IssueBodyUnavailable, bodyIndex, position)
			}
			functionFormals = append(functionFormals, formal)
		}
		varargOffset := uint32(len(functionVarargs))
		varargCount := uint32(0)
		if varargID, cellID, varargOK := flowView.FunctionVarargIDs(function); varargOK {
			vararg, varargSealed := programschema.NewFunctionVararg(varargID, cellID)
			if !varargSealed {
				return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionVararg(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
			}
			functionVarargs = append(functionVarargs, vararg)
			varargCount = 1
		}
		captureOffset := uint32(len(functionCaptures))
		for position := 0; position < function.CaptureCount(); position++ {
			captureID, innerID, outerID, innerStorageID, outerStorageID, innerBodyID, outerBodyID, captureOK := input.Program.FunctionCaptureIDs(function, position)
			if !captureOK || uint64(position) > uint64(^uint32(0)) {
				return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionCapture(), programconstruction.IssueBodyUnavailable, bodyIndex, position)
			}
			capture, captureSealed := programschema.NewFunctionCapture(captureID, innerID, outerID, innerStorageID, outerStorageID, innerBodyID, outerBodyID, uint32(position))
			if !captureSealed {
				return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionCapture(), programconstruction.IssueBodyUnavailable, bodyIndex, position)
			}
			functionCaptures = append(functionCaptures, capture)
		}
		boundary, boundarySealed := programschema.NewFunctionBoundary(
			functionID, copiedBody.ID(), copiedBody.ContextID(), copiedBody.EntryID(), callFormalID,
			formalOffset, uint32(function.FormalCount()), varargOffset, varargCount,
			captureOffset, uint32(function.CaptureCount()),
		)
		if !boundarySealed {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		functionBoundaries = append(functionBoundaries, boundary)
		functionBoundaryByBody[copiedBody.ID()] = boundary
		functionTerm, functionTermOK := function.Function()
		if !functionTermOK || functionTerm == 0 {
			return nil, nil, nil, nil, nil, nil, programconstruction.New(programcatalog.FunctionBoundary(), programconstruction.IssueBodyUnavailable, bodyIndex, -1)
		}
		functionIDs[functionTerm] = functionID
	}
	return functionBoundaries, functionFormals, functionVarargs, functionCaptures, functionIDs, functionBoundaryByBody, programconstruction.Fault{}
}
