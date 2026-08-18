package artifact

import "github.com/wippyai/go-lua/analysis/identity"

func (compiler *compiler) copyCallRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceCall)
	}
	calls := compiler.input.Flow().Authored().Calls().Count()
	compiler.calls = make([]CallRow, 0, calls)
	compiler.callOperands = compiler.callOperands[:0]
	compiler.callArguments = compiler.callArguments[:0]
	compiler.callTypeArguments = compiler.callTypeArguments[:0]
	for index := 0; index < calls; index++ {
		call, ok := compiler.callConstruction(index)
		if !ok {
			continue
		}
		form, formOK := receiptCallForm(call.form)
		if !formOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		row := CallRow{id: call.id, body: call.bodyPath, span: call.span, formal: call.formal, values: call.values, valuesRoot: call.valuesRoot, types: call.types, form: form, target: call.targetBody, tail: identity.ContentID{},
			operandStart: uint32(len(compiler.callOperands)), argumentStart: uint32(len(compiler.callArguments)), typeArgumentStart: uint32(len(compiler.callTypeArguments)), sealed: true}
		if call.tail.Available() {
			row.tail, row.hasTail = call.tail, true
		}
		appendOperand := func(operand callOperandConstruction) bool {
			value := CallOperandRow{id: operand.id, call: call.id, value: operand.id, span: operand.span, kind: operand.kind, sealed: true}
			if !value.Available() {
				return false
			}
			compiler.callOperands = append(compiler.callOperands, value)
			return true
		}
		if !appendOperand(call.callee) || !appendOperand(call.actuals) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		row.callee, row.actuals = call.callee.id, call.actuals.id
		if call.receiver.id.Available() {
			if !appendOperand(call.receiver) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			row.receiver, row.hasReceiver = call.receiver.id, true
		}
		for argumentIndex, argument := range call.arguments {
			argumentRow := CallArgumentRow{id: argument.id, call: call.id, values: call.values, member: argument.member, span: argument.span, position: uint32(argumentIndex), sealed: true}
			if !argumentRow.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argumentIndex, CompileReasonOccurrenceCall)
			}
			compiler.callArguments = append(compiler.callArguments, argumentRow)
		}
		for typeIndex, argument := range call.typeArguments {
			argumentRow := CallTypeArgumentRow{id: argument.id, call: call.id, types: call.types, reference: argument.reference, position: uint32(typeIndex), sealed: true}
			if !argumentRow.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
			compiler.callTypeArguments = append(compiler.callTypeArguments, argumentRow)
		}
		row.operandEnd = uint32(len(compiler.callOperands))
		row.argumentEnd = uint32(len(compiler.callArguments))
		row.typeArgumentEnd = uint32(len(compiler.callTypeArguments))
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		compiler.calls = append(compiler.calls, row)
	}
	return CompileFailure{}
}
