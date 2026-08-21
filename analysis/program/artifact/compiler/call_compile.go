package compiler

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) copyCallRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	calls := compiler.input.Flow().Authored().Calls().Count()
	compiler.calls = make([]programschema.Call, 0, calls)
	compiler.callResults = compiler.callResults[:0]
	compiler.callResultSlots = compiler.callResultSlots[:0]
	callIDs := make([]identity.ContentID, calls+1)
	callSpans := make([]identity.ContentID, calls+1)
	compiler.callOperands = compiler.callOperands[:0]
	compiler.callArguments = compiler.callArguments[:0]
	compiler.callTypeArguments = compiler.callTypeArguments[:0]
	for index := 0; index < calls; index++ {
		call, ok := compiler.callConstruction(index)
		if !ok {
			continue
		}
		form, formOK := coldCallForm(call.form)
		if !formOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		if !fitsUint32(len(compiler.callOperands)) || !fitsUint32(len(compiler.callArguments)) || !fitsUint32(len(compiler.callTypeArguments)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		operandStart := uint32(len(compiler.callOperands))
		argumentStart := uint32(len(compiler.callArguments))
		typeArgumentStart := uint32(len(compiler.callTypeArguments))
		appendOperand := func(operand callOperandConstruction) bool {
			value, valueOK := programschema.NewCallOperand(operand.id, call.id, operand.id, operand.span, operand.kind)
			if !valueOK {
				return false
			}
			compiler.callOperands = append(compiler.callOperands, value)
			return true
		}
		if !appendOperand(call.callee) || !appendOperand(call.actuals) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		receiver := identity.ContentID{}
		hasReceiver := false
		if call.receiver.id.Available() {
			if !appendOperand(call.receiver) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			receiver, hasReceiver = call.receiver.id, true
		}
		for argumentIndex, argument := range call.arguments {
			if !fitsUint32(argumentIndex) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			argumentRow, argumentOK := programschema.NewCallArgument(argument.id, call.id, call.values, argument.member, argument.span, uint32(argumentIndex))
			if !argumentOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			compiler.callArguments = append(compiler.callArguments, argumentRow)
		}
		for typeIndex, argument := range call.typeArguments {
			if !fitsUint32(typeIndex) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
			argumentRow, argumentOK := programschema.NewCallTypeArgument(argument.id, call.id, call.types, argument.reference, uint32(typeIndex))
			if !argumentOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
			compiler.callTypeArguments = append(compiler.callTypeArguments, argumentRow)
		}
		if !fitsUint32(len(compiler.callOperands)) || !fitsUint32(len(compiler.callArguments)) || !fitsUint32(len(compiler.callTypeArguments)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		operandEnd := uint32(len(compiler.callOperands))
		argumentEnd := uint32(len(compiler.callArguments))
		typeArgumentEnd := uint32(len(compiler.callTypeArguments))
		tail := identity.ContentID{}
		hasTail := false
		if call.tail.Available() {
			tail, hasTail = call.tail, true
		}
		row, rowOK := programschema.NewCall(
			call.id, call.bodyPath, call.span, call.formal, call.values, call.valuesRoot, call.types,
			call.callee.id, call.actuals.id, receiver, tail, call.targetBody, form,
			operandStart, operandEnd, argumentStart, argumentEnd, typeArgumentStart, typeArgumentEnd,
			hasReceiver, hasTail,
		)
		if !rowOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		ordinal := keyspace.TermOrdinal(call.term)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(callIDs)) || callIDs[ordinal].Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		callIDs[ordinal] = call.id
		callSpans[ordinal] = call.span
		compiler.calls = append(compiler.calls, row)
	}
	// Flow already owns the complete ordered Call-result geometry walk. Join it
	// once to the dense Call IDs compiled above instead of rebuilding a second
	// term-to-geometry index in the Artifact compiler.
	type resultAt struct {
		ordinal                uint32
		geometry               flow.CallResultGeometry
		directConsumer         keyspace.Term
		directConsumerPosition uint32
	}
	slotsByCall := make(map[keyspace.Term][]flow.CallResultSlotGeometry)
	if !compiler.input.Flow().VisitCallResultSlotGeometry(func(geometry flow.CallResultSlotGeometry) bool {
		slotsByCall[geometry.Call] = append(slotsByCall[geometry.Call], geometry)
		return true
	}) {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	results := make([]resultAt, 0)
	geometryOK := compiler.input.Flow().VisitCallResultGeometry(func(geometry flow.CallResultGeometry) bool {
		ordinal := keyspace.TermOrdinal(geometry.Call)
		if keyspace.TermFamily(geometry.Call) != keyspace.FamilyCall || ordinal == 0 || uint64(ordinal) >= uint64(len(callIDs)) ||
			!callIDs[ordinal].Available() {
			return false
		}
		results = append(results, resultAt{ordinal: ordinal, geometry: geometry})
		return true
	})
	if !geometryOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	directOK := compiler.input.Flow().VisitDirectScalarCallResultGeometry(func(geometry flow.DirectScalarCallResultGeometry) bool {
		ordinal := keyspace.TermOrdinal(geometry.Call)
		if !geometry.Available() || ordinal == 0 || uint64(ordinal) >= uint64(len(callIDs)) ||
			!callIDs[ordinal].Available() || !callSpans[ordinal].Available() {
			return false
		}
		results = append(results, resultAt{
			ordinal: ordinal,
			geometry: flow.CallResultGeometry{
				Call: geometry.Call, Value: callSpans[ordinal], Form: programschema.CallResultDirectValue,
				Multiplicity: programschema.CallResultMultiplicityExact, Count: 1,
			},
			directConsumer:         geometry.Consumer,
			directConsumerPosition: geometry.ConsumerPosition,
		})
		return true
	})
	if !directOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	sort.Slice(results, func(left, right int) bool { return results[left].ordinal < results[right].ordinal })
	programID := compiler.key.ProgramID()
	for index, result := range results {
		if index != 0 && results[index-1].ordinal == result.ordinal {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, -1, CompileReasonOccurrenceCall)
		}
		geometry := result.geometry
		slots := slotsByCall[geometry.Call]
		wantSlots := uint32(0)
		if geometry.Multiplicity == programschema.CallResultMultiplicityExact {
			wantSlots = geometry.Count
		}
		if (geometry.Form != programschema.CallResultDirectValue && uint64(len(slots)) != uint64(wantSlots)) || !fitsUint32(len(compiler.callResultSlots)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, -1, CompileReasonOccurrenceCall)
		}
		slotOffset := uint32(len(compiler.callResultSlots))
		if geometry.Multiplicity == programschema.CallResultMultiplicityOpen {
			// Open tails have no finite child span. Their canonical zero-width
			// span is independent of the dense slots emitted for other Calls.
			slotOffset = 0
		}
		parent, parentOK := programschema.NewCallResultWithMultiplicityAndSlots(
			callIDs[result.ordinal], geometry.Values, geometry.Value, geometry.Tail, geometry.Position,
			geometry.Form, geometry.Multiplicity, geometry.Count, slotOffset, wantSlots,
		)
		if !parentOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, -1, CompileReasonOccurrenceCall)
		}
		if geometry.Form == programschema.CallResultDirectValue {
			consumerID, consumerOK := compiler.input.Flow().SemanticTermPath(result.directConsumer)
			slot, slotOK := programschema.NewDerivedCallResultSlot(
				callIDs[result.ordinal], 0, programschema.CallResultSlotSourceCallValue,
				programschema.CallResultSlotConsumerStructural, consumerID, result.directConsumerPosition, geometry.Value,
			)
			if !consumerOK || !consumerID.Available() || !slotOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, 0, CompileReasonOccurrenceCall)
			}
			compiler.callResultSlots = append(compiler.callResultSlots, slot)
			compiler.callResults = append(compiler.callResults, parent)
			continue
		}
		for slotIndex, slotGeometry := range slots {
			if slotGeometry.Ordinal != uint32(slotIndex) || slotGeometry.Values != geometry.Values {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, slotIndex, CompileReasonOccurrenceCall)
			}
			consumerID, valueID := identity.ContentID{}, identity.ContentID{}
			switch slotGeometry.SourceKind {
			case programschema.CallResultSlotSourceValue:
				consumerID, valueID = slotGeometry.Source, slotGeometry.Source
			case programschema.CallResultSlotSourceValuesTail:
				switch slotGeometry.ConsumerKind {
				case programschema.CallResultSlotConsumerCell:
					consumerID, _ = rowidentity.StorageCellID(programID, compiler.input.Flow(), slotGeometry.Consumer)
					valueID, _ = programschema.CallResultSlotSyntheticValueIdentity(
						callIDs[result.ordinal], slotGeometry.Ordinal, slotGeometry.ConsumerKind,
						consumerID, slotGeometry.Position,
					)
				case programschema.CallResultSlotConsumerLens, programschema.CallResultSlotConsumerStructural:
					consumerID, _ = compiler.input.Flow().SemanticTermPath(slotGeometry.Consumer)
				default:
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, slotIndex, CompileReasonOccurrenceCall)
				}
			default:
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, slotIndex, CompileReasonOccurrenceCall)
			}
			slot, slotOK := programschema.NewDerivedCallResultSlot(
				callIDs[result.ordinal], slotGeometry.Ordinal, slotGeometry.SourceKind,
				slotGeometry.ConsumerKind, consumerID, slotGeometry.Position, valueID,
			)
			if !slotOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(result.ordinal)-1, slotIndex, CompileReasonOccurrenceCall)
			}
			compiler.callResultSlots = append(compiler.callResultSlots, slot)
		}
		compiler.callResults = append(compiler.callResults, parent)
	}
	return CompileFailure{}
}

func coldCallForm(form accessgeometry.CallForm) (programschema.CallForm, bool) {
	switch form {
	case accessgeometry.CallFormPlain:
		return programschema.CallFormPlain, true
	case accessgeometry.CallFormMethod:
		return programschema.CallFormMethod, true
	default:
		return programschema.CallFormInvalid, false
	}
}
