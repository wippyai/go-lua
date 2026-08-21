package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
)

func (validator *validator) validateSealIndexes(state *validationState) bool {
	if validator == nil || state == nil {
		return false
	}
	program := validator.program
	// Calls and their ordered child columns are one sealed cold publication.
	// Validate contiguous ranges and owner joins at the publication boundary.
	callCount, callsPublished := programschema.CallFamily().Count(&validator.frozen, validator.catalog)
	operandCount, operandsPublished := programschema.CallOperandFamily().Count(&validator.frozen, validator.catalog)
	argumentCount, argumentsPublished := programschema.CallArgumentFamily().Count(&validator.frozen, validator.catalog)
	typeArgumentCount, typeArgumentsPublished := programschema.CallTypeArgumentFamily().Count(&validator.frozen, validator.catalog)
	if !callsPublished || !operandsPublished || !argumentsPublished || !typeArgumentsPublished {
		return false
	}
	seenCalls := make(map[identity.ContentID]struct{}, callCount)
	state.callRows = make(map[identity.ContentID]struct{}, callCount)
	state.callRowsByID = make(map[identity.ContentID]programschema.Call, callCount)
	seenCallOperands := make(map[identity.ContentID]struct{}, operandCount)
	seenCallArguments := make(map[identity.ContentID]struct{}, argumentCount)
	seenCallTypeArguments := make(map[identity.ContentID]struct{}, typeArgumentCount)
	operandCursor, argumentCursor, typeArgumentCursor := 0, 0, 0
	for index := 0; index < callCount; index++ {
		row, held := programschema.CallFamily().At(&validator.frozen, validator.catalog, index)
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
			child, childHeld := programschema.CallOperandFamily().At(&validator.frozen, validator.catalog, int(operandStart+childIndex))
			if !childHeld || !child.Available() || child.CallID() != row.ID() {
				return false
			}
			if _, duplicate := seenCallOperands[child.ID()]; duplicate {
				return false
			}
			seenCallOperands[child.ID()] = struct{}{}
		}
		for childIndex := uint32(0); childIndex < argumentWidth; childIndex++ {
			child, childHeld := programschema.CallArgumentFamily().At(&validator.frozen, validator.catalog, int(argumentStart+childIndex))
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
			child, childHeld := programschema.CallTypeArgumentFamily().At(&validator.frozen, validator.catalog, int(typeArgumentStart+childIndex))
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
		state.callRows[row.ID()] = struct{}{}
		state.callRowsByID[row.ID()] = row
		operandCursor, argumentCursor, typeArgumentCursor = int(operandStart+operandWidth), int(argumentStart+argumentWidth), int(typeArgumentStart+typeArgumentWidth)
	}
	if operandCursor != operandCount || argumentCursor != argumentCount || typeArgumentCursor != typeArgumentCount {
		return false
	}
	callResultCount, callResultsPublished := programschema.CallResultFamily().Count(&validator.frozen, validator.catalog)
	if !callResultsPublished {
		return false
	}
	seenCallResults := make(map[identity.ContentID]struct{}, callResultCount)
	for index := 0; index < callResultCount; index++ {
		row, held := programschema.CallResultFamily().At(&validator.frozen, validator.catalog, index)
		valueID, hasValue := row.ValueID()
		tailID, hasTail := row.ValuesTailID()
		position, hasPosition := row.Position()
		multiplicity := row.Multiplicity()
		count, hasCount := row.ResultCount()
		open, openOK := row.ResultsOpen()
		if !held || !row.Available() || hasValue == hasTail || hasPosition != (row.Form() == programschema.CallResultValue) || !multiplicity.Valid() || !openOK || open != (multiplicity == programschema.CallResultMultiplicityOpen) || hasCount != (multiplicity == programschema.CallResultMultiplicityExact) {
			return false
		}
		callRow, known := state.callRowsByID[row.CallID()]
		if !known {
			return false
		}
		switch row.Form() {
		case programschema.CallResultValue:
			valuesRow, valuesKnown := state.valueRowsByID[row.ValuesID()]
			if !valuesKnown {
				return false
			}
			offset, width, spanOK := valuesRow.MemberSpan()
			if !spanOK {
				return false
			}
			if !hasValue || hasTail || !hasPosition || uint64(position) >= uint64(width) {
				return false
			}
			member, memberHeld := programschema.ValuesMemberFamily().At(&validator.frozen, validator.catalog, int(offset+position))
			if !memberHeld || member.ID() != valueID {
				return false
			}
		case programschema.CallResultValues:
			valuesRow, valuesKnown := state.valueRowsByID[row.ValuesID()]
			if !valuesKnown {
				return false
			}
			if hasValue || !hasTail {
				return false
			}
			if multiplicity == programschema.CallResultMultiplicityExact && count == 0 {
				return false
			}
			tail, tailOK := valuesRow.Tail()
			if !tailOK || tail.Kind() != programschema.ValuesTailCall || tail.ID() != tailID {
				return false
			}
		case programschema.CallResultDirectValue:
			if row.ValuesID().Available() || !hasValue || hasTail || hasPosition || valueID != callRow.SpanID() {
				return false
			}
		default:
			return false
		}
		if _, duplicate := seenCallResults[row.CallID()]; duplicate {
			return false
		}
		seenCallResults[row.CallID()] = struct{}{}
	}
	// CallResultSlot is a child plane of CallResult. Parent spans are
	// contiguous in publication order, and each (CallID,ordinal) coordinate is
	// unique even when two consumers would otherwise carry the same source
	// identity. A Values tail is never copied into a scalar slot ValueID: when
	// a bounded tail resolves to a consumer Cell, that existing Cell identity
	// may be carried; structural/lens consumers may remain absent.
	callResultSlotCount, callResultSlotsPublished := programschema.CallResultSlotFamily().Count(&validator.frozen, validator.catalog)
	if !callResultSlotsPublished {
		return false
	}
	seenCallResultSlotIDs := make(map[identity.ContentID]struct{}, callResultSlotCount)
	type callResultSlotKey struct {
		call    identity.ContentID
		ordinal uint32
	}
	seenCallResultSlotKeys := make(map[callResultSlotKey]struct{}, callResultSlotCount)
	slotCursor := uint32(0)
	for index := 0; index < callResultCount; index++ {
		parent, held := programschema.CallResultFamily().At(&validator.frozen, validator.catalog, index)
		offset, width, spanOK := parent.SlotSpan()
		if !held || !parent.Available() || !spanOK || width != 0 && offset != slotCursor || uint64(offset)+uint64(width) > uint64(callResultSlotCount) {
			return false
		}
		for childIndex := uint32(0); childIndex < width; childIndex++ {
			slot, slotHeld := programschema.CallResultSlotFamily().At(&validator.frozen, validator.catalog, int(offset+childIndex))
			ordinal, ordinalOK := slot.Ordinal()
			if !slotHeld || !slot.Available() || !ordinalOK || ordinal != childIndex || slot.CallID() != parent.CallID() {
				return false
			}
			if _, duplicate := seenCallResultSlotIDs[slot.ID()]; duplicate {
				return false
			}
			key := callResultSlotKey{call: slot.CallID(), ordinal: ordinal}
			if _, duplicate := seenCallResultSlotKeys[key]; duplicate {
				return false
			}
			seenCallResultSlotIDs[slot.ID()] = struct{}{}
			seenCallResultSlotKeys[key] = struct{}{}
			if !parent.AdmitsResult(ordinal) {
				return false
			}
			tailID, hasTail := parent.ValuesTailID()
			switch parent.Form() {
			case programschema.CallResultValue:
				if ordinal != 0 || slot.SourceKind() != programschema.CallResultSlotSourceValue {
					return false
				}
				valueID, hasValue := slot.ValueID()
				fixedID, fixedOK := parent.ValueID()
				if !hasValue || !fixedOK || valueID != fixedID {
					return false
				}
			case programschema.CallResultValues:
				if slot.SourceKind() != programschema.CallResultSlotSourceValuesTail || !hasTail {
					return false
				}
				if valueID, hasValue := slot.ValueID(); hasValue && valueID == tailID {
					return false
				}
			case programschema.CallResultDirectValue:
				valueID, hasValue := slot.ValueID()
				fixedID, fixedOK := parent.ValueID()
				if ordinal != 0 || slot.SourceKind() != programschema.CallResultSlotSourceCallValue ||
					slot.ConsumerKind() != programschema.CallResultSlotConsumerStructural || !hasValue || !fixedOK || valueID != fixedID {
					return false
				}
			default:
				return false
			}
		}
		slotCursor += width
	}
	if slotCursor != uint32(callResultSlotCount) {
		return false
	}
	functionCount, functionsPublished := programschema.FunctionBoundaryFamily().Count(&validator.frozen, validator.catalog)
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
	targetCount, targetsPublished := calltarget.Family().Count(&validator.frozen, validator.catalog)
	if !targetsPublished {
		return false
	}
	seenCallAllocations := make(map[identity.ContentID]struct{}, targetCount)
	seenCallBodies := make(map[identity.ContentID]struct{}, targetCount)
	bodyCount, bodiesPublished := programschema.BodyFamily().Count(&validator.frozen, validator.catalog)
	if !bodiesPublished {
		return false
	}
	bodyByContext := make(map[identity.ContentID]programschema.Body, bodyCount)
	for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
		body, held := programschema.BodyFamily().At(&validator.frozen, validator.catalog, bodyIndex)
		if !held {
			return false
		}
		if _, duplicate := bodyByContext[body.ContextID()]; duplicate {
			return false
		}
		bodyByContext[body.ContextID()] = body
	}
	for index := 0; index < targetCount; index++ {
		target, held := calltarget.Family().At(&validator.frozen, validator.catalog, index)
		body, bodyOK := bodyByContext[target.ContextID()]
		function, functionOK := body.FunctionContextID()
		formal, formalOK := body.CallFormalID()
		if !held || !target.Available() || !bodyOK || !body.Callable() || body.ID() != target.BodyID() ||
			body.ContextID() != target.ContextID() || !functionOK || function != target.FunctionID() || !formalOK || formal != target.FormalID() {
			return false
		}
		if _, duplicate := seenCallAllocations[target.AllocationID()]; duplicate {
			return false
		}
		if _, duplicate := seenCallBodies[target.ContextID()]; duplicate {
			return false
		}
		seenCallAllocations[target.AllocationID()], seenCallBodies[target.ContextID()] = struct{}{}, struct{}{}
	}
	outcomeCount, outcomesPublished := programschema.OutcomeFamily().Count(&validator.frozen, validator.catalog)
	if !outcomesPublished || uint64(state.outcomeCursor) != uint64(outcomeCount) {
		return false
	}
	valuesCount, valuesPublished := programschema.ValuesFamily().Count(&validator.frozen, validator.catalog)
	if !valuesPublished {
		return false
	}
	for index := 0; index < valuesCount; index++ {
		row, held := programschema.ValuesFamily().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		if _, exists := state.bodyRows[row.BodyPathID()]; !exists {
			return false
		}
	}
	typeValueCount, typeValuesPublished := programschema.StaticTypeValueFamily().Count(&validator.frozen, validator.catalog)
	if !typeValuesPublished {
		return false
	}
	seenStaticValues := make(map[identity.ContentID]struct{}, typeValueCount)
	for index := 0; index < typeValueCount; index++ {
		row, held := programschema.StaticTypeValueFamily().At(&validator.frozen, validator.catalog, index)
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
	typeNodeCount, typeNodesPublished := staticnode.StaticTypeNodeFamily().Count(&validator.frozen, validator.catalog)
	if !typeNodesPublished {
		return false
	}
	seenStaticNodes := make(map[identity.ContentID]struct{}, typeNodeCount)
	staticView, staticViewOK := validator.static, validator.static.Available()
	if !staticViewOK {
		return false
	}
	for index := 0; index < typeNodeCount; index++ {
		row, held := staticnode.StaticTypeNodeFamily().At(&validator.frozen, validator.catalog, index)
		// A TypeRefUnresolved is a complete Static leaf: Static sealed its
		// targetless disposition and ProgramArtifact retained its exact lexical
		// proof as a DiagnosticObservation. All other references must retain
		// their resolved/canonical target edge.
		_, operandAvailable := row.OperandID()
		if !held || !row.Available() || row.Kind() == staticnode.StaticNodeTypeOf && !operandAvailable {
			return false
		}
		if _, duplicate := seenStaticNodes[row.ID()]; duplicate {
			return false
		}
		seenStaticNodes[row.ID()] = struct{}{}
	}

	for functionIndex := 0; functionIndex < functionCount; functionIndex++ {
		function, functionHeld := programschema.FunctionBoundaryFamily().At(&validator.frozen, validator.catalog, functionIndex)
		formalOffset, formalWidth, formalSpanOK := function.FormalSpan()
		if !functionHeld || !formalSpanOK {
			return false
		}
		for formalIndex := uint32(0); formalIndex < formalWidth; formalIndex++ {
			formal, formalHeld := programschema.FunctionFormalFamily().At(&validator.frozen, validator.catalog, int(formalOffset+formalIndex))
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
		row, held := staticnode.StaticTypeNodeFamily().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		children, childrenOK := staticView.StaticTypeNodeChildren(index, row, true)
		if !childrenOK {
			return false
		}
		for _, child := range children {
			if _, exists := seenStaticNodes[child]; !exists {
				return false
			}
		}

	}
	expressionCount, expressionsPublished := programschema.StaticExpressionFamily().Count(&validator.frozen, validator.catalog)
	if !expressionsPublished {
		return false
	}
	seenStaticExpressions := make(map[identity.ContentID]struct{}, expressionCount)
	for index := 0; index < expressionCount; index++ {
		row, held := programschema.StaticExpressionFamily().At(&validator.frozen, validator.catalog, index)
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
	inputCount, inputsPublished := programschema.StaticInputFamily().Count(&validator.frozen, validator.catalog)
	if !inputsPublished {
		return false
	}
	seenStaticInputs := make(map[identity.ContentID]struct{}, inputCount)
	for index := 0; index < inputCount; index++ {
		row, held := programschema.StaticInputFamily().At(&validator.frozen, validator.catalog, index)
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
