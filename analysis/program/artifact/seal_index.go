package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/analysis/schema/cold"
)

func (artifact *Artifact) validateSealIndexes(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	// Calls and their ordered child columns are a single Artifact-owned plane.
	// Validate contiguous ranges and owner joins here.
	seenCalls := make(map[identity.ContentID]struct{}, len(artifact.calls))
	seenCallOperands := make(map[identity.ContentID]struct{}, len(artifact.callOperands))
	seenCallArguments := make(map[identity.ContentID]struct{}, len(artifact.callArguments))
	seenCallTypeArguments := make(map[identity.ContentID]struct{}, len(artifact.callTypeArguments))
	operandCursor, argumentCursor, typeArgumentCursor := 0, 0, 0
	for index, row := range artifact.calls {
		if !row.Available() || uint64(row.operandStart) != uint64(operandCursor) || uint64(row.argumentStart) != uint64(argumentCursor) || uint64(row.typeArgumentStart) != uint64(typeArgumentCursor) || uint64(row.operandEnd) > uint64(len(artifact.callOperands)) || uint64(row.argumentEnd) > uint64(len(artifact.callArguments)) || uint64(row.typeArgumentEnd) > uint64(len(artifact.callTypeArguments)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, duplicate := seenCalls[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, bodyOK := state.bodyRows[row.body]; !bodyOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if _, valuesOK := state.valueRows[row.valuesRoot]; !valuesOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		occurrenceIndex, occurrenceOK := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceCall, id: row.id}]
		if !occurrenceOK || uint64(occurrenceIndex) >= uint64(len(artifact.occurrences)) || !artifact.occurrences[occurrenceIndex].Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for childIndex := int(row.operandStart); childIndex < int(row.operandEnd); childIndex++ {
			child := artifact.callOperands[childIndex]
			if !child.Available() || child.call != row.id {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex-int(row.operandStart), CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallOperands[child.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex-int(row.operandStart), CompileReasonOccurrenceCall)
			}
			seenCallOperands[child.id] = struct{}{}
		}
		for childIndex := int(row.argumentStart); childIndex < int(row.argumentEnd); childIndex++ {
			child := artifact.callArguments[childIndex]
			position := childIndex - int(row.argumentStart)
			if !child.Available() || child.call != row.id || child.values != row.values || child.position != uint32(position) {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallArguments[child.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
			}
			seenCallArguments[child.id] = struct{}{}
		}
		for childIndex := int(row.typeArgumentStart); childIndex < int(row.typeArgumentEnd); childIndex++ {
			child := artifact.callTypeArguments[childIndex]
			position := childIndex - int(row.typeArgumentStart)
			if !child.Available() || child.call != row.id || child.types != row.types || child.position != uint32(position) {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
			}
			if _, duplicate := seenCallTypeArguments[child.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, position, CompileReasonOccurrenceCall)
			}
			seenCallTypeArguments[child.id] = struct{}{}
		}
		seenCalls[row.id] = struct{}{}
		operandCursor, argumentCursor, typeArgumentCursor = int(row.operandEnd), int(row.argumentEnd), int(row.typeArgumentEnd)
	}
	if operandCursor != len(artifact.callOperands) || argumentCursor != len(artifact.callArguments) || typeArgumentCursor != len(artifact.callTypeArguments) {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	if len(artifact.functionBoundaries) != state.callableBodies {
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
	targetCount, targetsPublished := cold.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !targetsPublished {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyUnavailable)
	}
	seenCallAllocations := make(map[identity.ContentID]struct{}, targetCount)
	seenCallBodies := make(map[identity.ContentID]struct{}, targetCount)
	bodyByContext := make(map[identity.ContentID]BodyRow, len(artifact.bodies))
	for _, body := range artifact.bodies {
		if _, duplicate := bodyByContext[body.context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyDuplicate)
		}
		bodyByContext[body.context] = body
	}
	for index := 0; index < targetCount; index++ {
		target, held := cold.CallTargetFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		body, bodyOK := bodyByContext[target.Context]
		if !held || !target.Available() || !bodyOK || !body.Callable() || body.ID() != target.Body ||
			body.context != target.Context || body.function != target.Function || body.formal != target.Formal {
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
	if state.outcomeCursor != uint32(len(artifact.outcomes)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	valuesCount, valuesPublished := coldCount(artifact, cold.ValuesFamily())
	if !valuesPublished {
		return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, cold.ValuesFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, exists := state.bodyRows[row.BodyPathID()]; !exists {
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

	return CompileFailure{}
}
