package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/accessgeometry"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) copyCallRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	calls := compiler.input.Flow().Authored().Calls().Count()
	compiler.calls = make([]programschema.Call, 0, calls)
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
		compiler.calls = append(compiler.calls, row)
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
