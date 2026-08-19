package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (artifact *Artifact) validateSealIndexes(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	// Calls and their ordered child columns are one sealed cold publication.
	// Validate contiguous ranges and owner joins at the publication boundary.
	callCount, callsPublished := coldCount(artifact, programschema.CallFamily())
	operandCount, operandsPublished := coldCount(artifact, programschema.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(artifact, programschema.CallArgumentFamily())
	typeArgumentCount, typeArgumentsPublished := coldCount(artifact, programschema.CallTypeArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished || !typeArgumentsPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	seenCalls := make(map[identity.ContentID]struct{}, callCount)
	seenCallOperands := make(map[identity.ContentID]struct{}, operandCount)
	seenCallArguments := make(map[identity.ContentID]struct{}, argumentCount)
	seenCallTypeArguments := make(map[identity.ContentID]struct{}, typeArgumentCount)
	operandCursor, argumentCursor, typeArgumentCursor := 0, 0, 0
	for index := 0; index < callCount; index++ {
		row, held := coldRow(artifact, programschema.CallFamily(), index)
		operandStart, operandWidth, operandSpanOK := row.OperandSpan()
		argumentStart, argumentWidth, argumentSpanOK := row.ArgumentSpan()
		typeArgumentStart, typeArgumentWidth, typeArgumentSpanOK := row.TypeArgumentSpan()
		if !held || !row.Available() || !operandSpanOK || !argumentSpanOK || !typeArgumentSpanOK || uint64(operandStart) != uint64(operandCursor) || uint64(argumentStart) != uint64(argumentCursor) || uint64(typeArgumentStart) != uint64(typeArgumentCursor) || uint64(operandStart)+uint64(operandWidth) > uint64(operandCount) || uint64(argumentStart)+uint64(argumentWidth) > uint64(argumentCount) || uint64(typeArgumentStart)+uint64(typeArgumentWidth) > uint64(typeArgumentCount) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, duplicate := seenCalls[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, bodyOK := state.bodyRows[row.BodyID()]; !bodyOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, valuesOK := state.valueRows[row.ValuesRootID()]; !valuesOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		occurrenceIndex, occurrenceOK := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceCall, id: row.ID()}]
		if !occurrenceOK || uint64(occurrenceIndex) >= uint64(len(artifact.occurrences)) || !artifact.occurrences[occurrenceIndex].Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for childIndex := uint32(0); childIndex < operandWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallOperandFamily(), int(operandStart+childIndex))
			if !childHeld || !child.Available() || child.CallID() != row.ID() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(childIndex), CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallOperands[child.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(childIndex), CompileReasonOccurrenceCall)
			}
			seenCallOperands[child.ID()] = struct{}{}
		}
		for childIndex := uint32(0); childIndex < argumentWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallArgumentFamily(), int(argumentStart+childIndex))
			position := childIndex
			if !childHeld || !child.Available() || child.CallID() != row.ID() || child.ValuesID() != row.ValuesID() || child.Index() != position {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallArguments[child.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceCall)
			}
			seenCallArguments[child.ID()] = struct{}{}
		}
		for childIndex := uint32(0); childIndex < typeArgumentWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallTypeArgumentFamily(), int(typeArgumentStart+childIndex))
			position := childIndex
			if !childHeld || !child.Available() || child.CallID() != row.ID() || child.TypesID() != row.TypeArgumentsID() || child.Index() != position {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallTypeArguments[child.ID()]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceCall)
			}
			seenCallTypeArguments[child.ID()] = struct{}{}
		}
		seenCalls[row.ID()] = struct{}{}
		operandCursor, argumentCursor, typeArgumentCursor = int(operandStart+operandWidth), int(argumentStart+argumentWidth), int(typeArgumentStart+typeArgumentWidth)
	}
	if operandCursor != operandCount || argumentCursor != argumentCount || typeArgumentCursor != typeArgumentCount {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	functionCount, functionsPublished := coldCount(artifact, programschema.FunctionBoundaryFamily())
	if !functionsPublished || functionCount != state.callableBodies {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceStorage)
	}
	// Storage binds are one generic occurrence row whose inputs are ordered as
	// ValuesID followed by every destination CellID. This is the canonical
	// storage-cell column consumed by Pack; no Pack-specific refinement may
	// duplicate it.
	for index, row := range artifact.occurrences {
		if row.kind != OccurrenceStorageBind && row.kind != OccurrenceStorageBindTransfer {
			continue
		}
		if _, bodyOK := state.bodyRows[row.body]; !bodyOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		switch row.kind {
		case OccurrenceStorageBind:
			if row.InputCount() < 1 {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			valuesID, valuesOK := row.InputAt(0)
			if !valuesOK {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			if _, valuesKnown := state.valueRows[valuesID]; !valuesKnown {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			for cellIndex := 1; cellIndex < row.InputCount(); cellIndex++ {
				cell, cellOK := row.InputAt(cellIndex)
				if !cellOK || !cell.Available() {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, cellIndex, CompileReasonOccurrenceStorageBind)
				}
			}
		case OccurrenceStorageBindTransfer:
			if row.InputCount() != 3 {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			parent, parentOK := row.InputAt(0)
			value, valueOK := row.InputAt(1)
			cell, cellOK := row.InputAt(2)
			if !parentOK || !valueOK || !cellOK || !parent.Available() || !value.Available() || !cell.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
			if _, bindOK := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceStorageBind, id: parent}]; !bindOK {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
			}
		}
	}
	targetCount, targetsPublished := programschema.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !targetsPublished {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	seenCallAllocations := make(map[identity.ContentID]struct{}, targetCount)
	seenCallBodies := make(map[identity.ContentID]struct{}, targetCount)
	bodyCount, bodiesPublished := coldCount(artifact, programschema.BodyFamily())
	if !bodiesPublished {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, bodyCount)
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, held := coldRow(artifact, programschema.BodyFamily(), bodyIndex)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := bodyByContext[body.ContextID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyDuplicate)
		}
		bodyByContext[body.ContextID()] = body
	}
	for index := 0; index < targetCount; index++ {
		target, held := programschema.CallTargetFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		body, bodyOK := bodyByContext[target.Context]
		function, functionOK := body.FunctionContextID()
		formal, formalOK := body.CallFormalID()
		if !held || !target.Available() || !bodyOK || !body.Callable() || body.ID() != target.Body ||
			body.ContextID() != target.Context || !functionOK || function != target.Function || !formalOK || formal != target.Formal {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenCallAllocations[target.Allocation]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenCallBodies[target.Context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenCallAllocations[target.Allocation], seenCallBodies[target.Context] = struct{}{}, struct{}{}
	}
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	if !outcomesPublished || uint64(state.outcomeCursor) != uint64(outcomeCount) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, exists := state.bodyRows[row.BodyPathID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
	}
	typeValueCount, typeValuesPublished := coldCount(artifact, programschema.StaticTypeValueFamily())
	if !typeValuesPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	seenStaticValues := make(map[identity.ContentID]struct{}, typeValueCount)
	for index := 0; index < typeValueCount; index++ {
		row, held := artifact.staticTypeValueRowAt(index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticValues[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticValues[row.id] = struct{}{}
		if _, exists := state.bodyRows[row.body]; !exists {
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
			row.Kind() == StaticNodeReference && row.Resolution() == uint8(staticrefs.Unresolved)
		if !row.Available() || row.ChildCount() == 0 && !zeroChildAllowed || row.Kind() == StaticNodeTypeOf && !row.operand.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticNodes[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticNodes[row.id] = struct{}{}
	}
	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		function, functionHeld := coldRow(artifact, programschema.FunctionBoundaryFamily(), functionIndex)
		formalOffset, formalWidth, formalSpanOK := function.FormalSpan()
		if !functionHeld || !formalSpanOK {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		for formalIndex := uint32(0); formalIndex < formalWidth; formalIndex++ {
			formal, formalHeld := coldRow(artifact, programschema.FunctionFormalFamily(), int(formalOffset+formalIndex))
			declared, declaredOK := formal.DeclaredStaticTypeID()
			if !formalHeld || !formal.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(formalIndex), CompileReasonBodyUnavailable)
			}
			if declaredOK {
				if _, exists := seenStaticNodes[declared]; !exists {
					return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, int(formalIndex), CompileReasonBodyUnavailable)
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
	expressionCount, expressionsPublished := coldCount(artifact, programschema.StaticExpressionFamily())
	if !expressionsPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	seenStaticExpressions := make(map[identity.ContentID]struct{}, expressionCount)
	for index := 0; index < expressionCount; index++ {
		row, held := artifact.staticExpressionRowAt(index)
		if !held {
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
	inputCount, inputsPublished := coldCount(artifact, programschema.StaticInputFamily())
	if !inputsPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	seenStaticInputs := make(map[identity.ContentID]struct{}, inputCount)
	for index := 0; index < inputCount; index++ {
		row, held := coldRow(artifact, programschema.StaticInputFamily(), index)
		if !held || !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticInputs[row.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticInputs[row.ID()] = struct{}{}
		if _, exists := seenStaticExpressions[row.ExpressionID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	return CompileFailure{}
}
