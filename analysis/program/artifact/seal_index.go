package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (artifact *Artifact) validateSealIndexes(state *sealValidationState) bool {
	if artifact == nil || state == nil {
		return false
	}
	program := artifact.Program()
	// Calls and their ordered child columns are one sealed cold publication.
	// Validate contiguous ranges and owner joins at the publication boundary.
	callCount, callsPublished := coldCount(artifact, programschema.CallFamily())
	operandCount, operandsPublished := coldCount(artifact, programschema.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(artifact, programschema.CallArgumentFamily())
	typeArgumentCount, typeArgumentsPublished := coldCount(artifact, programschema.CallTypeArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished || !typeArgumentsPublished {
		return false
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
			return false
		}
		if _, duplicate := seenCalls[row.ID()]; duplicate {
			return false
		}
		if _, bodyOK := state.bodyRows[row.BodyID()]; !bodyOK {
			return false
		}
		if _, valuesOK := state.valueRows[row.ValuesRootID()]; !valuesOK {
			return false
		}
		if _, occurrenceOK := program.OccurrenceForID(programschema.OccurrenceCall, row.ID()); !occurrenceOK {
			return false
		}
		for childIndex := uint32(0); childIndex < operandWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallOperandFamily(), int(operandStart+childIndex))
			if !childHeld || !child.Available() || child.CallID() != row.ID() {
				return false
			}
			if _, duplicate := seenCallOperands[child.ID()]; duplicate {
				return false
			}
			seenCallOperands[child.ID()] = struct{}{}
		}
		for childIndex := uint32(0); childIndex < argumentWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallArgumentFamily(), int(argumentStart+childIndex))
			position := childIndex
			if !childHeld || !child.Available() || child.CallID() != row.ID() || child.ValuesID() != row.ValuesID() || child.Index() != position {
				return false
			}
			if _, duplicate := seenCallArguments[child.ID()]; duplicate {
				return false
			}
			seenCallArguments[child.ID()] = struct{}{}
		}
		for childIndex := uint32(0); childIndex < typeArgumentWidth; childIndex++ {
			child, childHeld := coldRow(artifact, programschema.CallTypeArgumentFamily(), int(typeArgumentStart+childIndex))
			position := childIndex
			if !childHeld || !child.Available() || child.CallID() != row.ID() || child.TypesID() != row.TypeArgumentsID() || child.Index() != position {
				return false
			}
			if _, duplicate := seenCallTypeArguments[child.ID()]; duplicate {
				return false
			}
			seenCallTypeArguments[child.ID()] = struct{}{}
		}
		seenCalls[row.ID()] = struct{}{}
		operandCursor, argumentCursor, typeArgumentCursor = int(operandStart+operandWidth), int(argumentStart+argumentWidth), int(typeArgumentStart+typeArgumentWidth)
	}
	if operandCursor != operandCount || argumentCursor != argumentCount || typeArgumentCursor != typeArgumentCount {
		return false
	}
	functionCount, functionsPublished := coldCount(artifact, programschema.FunctionBoundaryFamily())
	if !functionsPublished || functionCount != state.callableBodies {
		return false
	}
	// Storage binds are one generic occurrence row whose inputs are ordered as
	// ValuesID followed by every destination CellID. This is the canonical
	// storage-cell column consumed by Pack; no Pack-specific refinement may
	// duplicate it.
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		return false
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || (row.Kind() != programschema.OccurrenceStorageBind && row.Kind() != programschema.OccurrenceStorageBindTransfer) {
			continue
		}
		body, bodyOK := row.BodyID()
		if !bodyOK {
			return false
		}
		if _, bodyOK := state.bodyRows[body]; !bodyOK {
			return false
		}
		_, _, inputSpanOK := row.InputSpan()
		if !inputSpanOK {
			return false
		}
		_, inputCount, _ := row.InputSpan()
		switch row.Kind() {
		case programschema.OccurrenceStorageBind:
			if inputCount < 1 {
				return false
			}
			valuesRow, valuesOK := program.OccurrenceInputFor(index, 0)
			if !valuesOK || !valuesRow.Available() {
				return false
			}
			valuesID := valuesRow.InputID()
			if _, valuesKnown := state.valueRows[valuesID]; !valuesKnown {
				return false
			}
			for cellIndex := 1; cellIndex < int(inputCount); cellIndex++ {
				cell, cellOK := program.OccurrenceInputFor(index, cellIndex)
				if !cellOK || !cell.Available() {
					return false
				}
			}
		case programschema.OccurrenceStorageBindTransfer:
			if inputCount != 3 {
				return false
			}
			parentRow, parentOK := program.OccurrenceInputFor(index, 0)
			valueRow, valueOK := program.OccurrenceInputFor(index, 1)
			cellRow, cellOK := program.OccurrenceInputFor(index, 2)
			if !parentOK || !valueOK || !cellOK || !parentRow.Available() || !valueRow.Available() || !cellRow.Available() {
				return false
			}
			if _, bindOK := program.OccurrenceForID(programschema.OccurrenceStorageBind, parentRow.InputID()); !bindOK {
				return false
			}
		}
	}
	targetCount, targetsPublished := programschema.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !targetsPublished {
		return false
	}
	seenCallAllocations := make(map[identity.ContentID]struct{}, targetCount)
	seenCallBodies := make(map[identity.ContentID]struct{}, targetCount)
	bodyCount, bodiesPublished := coldCount(artifact, programschema.BodyFamily())
	if !bodiesPublished {
		return false
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, bodyCount)
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, held := coldRow(artifact, programschema.BodyFamily(), bodyIndex)
		if !held {
			return false
		}
		if _, duplicate := bodyByContext[body.ContextID()]; duplicate {
			return false
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
			return false
		}
		if _, duplicate := seenCallAllocations[target.Allocation]; duplicate {
			return false
		}
		if _, duplicate := seenCallBodies[target.Context]; duplicate {
			return false
		}
		seenCallAllocations[target.Allocation], seenCallBodies[target.Context] = struct{}{}, struct{}{}
	}
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	if !outcomesPublished || uint64(state.outcomeCursor) != uint64(outcomeCount) {
		return false
	}
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return false
	}
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		if !held {
			return false
		}
		if _, exists := state.bodyRows[row.BodyPathID()]; !exists {
			return false
		}
	}
	typeValueCount, typeValuesPublished := coldCount(artifact, programschema.StaticTypeValueFamily())
	if !typeValuesPublished {
		return false
	}
	seenStaticValues := make(map[identity.ContentID]struct{}, typeValueCount)
	for index := 0; index < typeValueCount; index++ {
		row, held := coldRow(artifact, programschema.StaticTypeValueFamily(), index)
		if !held {
			return false
		}
		if _, duplicate := seenStaticValues[row.ID()]; duplicate {
			return false
		}
		seenStaticValues[row.ID()] = struct{}{}
		if _, exists := state.bodyRows[row.BodyPathID()]; !exists {
			return false
		}
	}
	typeNodeCount, typeNodesPublished := coldCount(artifact, programschema.StaticTypeNodeFamily())
	if !typeNodesPublished {
		return false
	}
	seenStaticNodes := make(map[identity.ContentID]struct{}, typeNodeCount)
	for index := 0; index < typeNodeCount; index++ {
		row, held := coldRow(artifact, programschema.StaticTypeNodeFamily(), index)
		// A TypeRefUnresolved is a complete Static leaf: Static sealed its
		// targetless disposition and ProgramArtifact retained its exact lexical
		// proof as a DiagnosticObservation. All other references must retain
		// their resolved/canonical target edge.
		_, operandAvailable := row.OperandID()
		if !held || !row.Available() || row.Kind() == programschema.StaticNodeTypeOf && !operandAvailable {
			return false
		}
		if _, duplicate := seenStaticNodes[row.ID()]; duplicate {
			return false
		}
		seenStaticNodes[row.ID()] = struct{}{}
	}

	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		function, functionHeld := coldRow(artifact, programschema.FunctionBoundaryFamily(), functionIndex)
		formalOffset, formalWidth, formalSpanOK := function.FormalSpan()
		if !functionHeld || !formalSpanOK {
			return false
		}
		for formalIndex := uint32(0); formalIndex < formalWidth; formalIndex++ {
			formal, formalHeld := coldRow(artifact, programschema.FunctionFormalFamily(), int(formalOffset+formalIndex))
			declared, declaredOK := formal.DeclaredStaticTypeID()
			if !formalHeld || !formal.Available() {
				return false
			}
			if declaredOK {
				if _, exists := seenStaticNodes[declared]; !exists {
					return false
				}
			}
		}
	}
	for index := 0; index < typeNodeCount; index++ {
		row, held := coldRow(artifact, programschema.StaticTypeNodeFamily(), index)
		if !held {
			return false
		}
		children, childrenOK := artifact.canonicalStaticNodeChildren(row, true)
		if !childrenOK {
			return false
		}
		for _, child := range children {
			if _, exists := seenStaticNodes[child]; !exists {
				return false
			}
		}

	}
	expressionCount, expressionsPublished := coldCount(artifact, programschema.StaticExpressionFamily())
	if !expressionsPublished {
		return false
	}
	seenStaticExpressions := make(map[identity.ContentID]struct{}, expressionCount)
	for index := 0; index < expressionCount; index++ {
		row, held := coldRow(artifact, programschema.StaticExpressionFamily(), index)
		if !held {
			return false
		}
		if _, duplicate := seenStaticExpressions[row.ID()]; duplicate {
			return false
		}
		seenStaticExpressions[row.ID()] = struct{}{}
		if _, exists := seenStaticNodes[row.ReferenceID()]; !exists {
			return false
		}
	}
	inputCount, inputsPublished := coldCount(artifact, programschema.StaticInputFamily())
	if !inputsPublished {
		return false
	}
	seenStaticInputs := make(map[identity.ContentID]struct{}, inputCount)
	for index := 0; index < inputCount; index++ {
		row, held := coldRow(artifact, programschema.StaticInputFamily(), index)
		if !held || !row.Available() {
			return false
		}
		if _, duplicate := seenStaticInputs[row.ID()]; duplicate {
			return false
		}
		seenStaticInputs[row.ID()] = struct{}{}
		if _, exists := seenStaticExpressions[row.ExpressionID()]; !exists {
			return false
		}
	}

	return true
}
