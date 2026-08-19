package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) copyAllocations() CompileFailure {
	if compiler == nil || len(compiler.allocationRows) != len(compiler.heapAllocations) {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	for index, allocation := range compiler.allocationRows {
		entryPoints, finishPoints := compiler.pointIDs(allocation.entry), compiler.pointIDs(allocation.finish)
		if !allocation.occurrence.Available() || !allocation.template.Available() || len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.appendOccurrence(programschema.OccurrenceAllocation, allocation.template, identity.ContentID{}, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{allocation.template, allocation.occurrence}, uint64(allocation.form)) ||
			!compiler.recordOccurrenceSpan(programschema.OccurrenceAllocation, allocation.template, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		for fieldIndex, field := range allocation.fields {
			values := field.valuesRow
			inputs := []identity.ContentID{allocation.template}
			if values.Available() {
				inputs = append(inputs, values.ID())
				for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
					member, memberOK := values.MemberAt(memberIndex)
					if !memberOK {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
					}
					inputs = append(inputs, member.ID())
				}
			}
			if field.term == 0 || !values.Available() || !compiler.appendOccurrence(programschema.OccurrenceAllocationField, field.id, identity.ContentID{}, nil, inputs, uint64(fieldIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyCalls() CompileFailure {
	for index := 0; index < compiler.input.Flow().Authored().Calls().Count(); index++ {
		call, ok := compiler.callConstruction(index)
		if !ok {
			// The authored denominator remains canonical; a missing direct join
			// is a non-executable candidate and is not compacted into a new
			// proof table.
			continue
		}
		inputs := []identity.ContentID{call.callee.id, call.actuals.id, call.values, call.formal, call.types}
		if call.receiver.id.Available() {
			inputs = append(inputs, call.receiver.id)
		}
		entryPoints, finishPoints := compiler.pointIDs(call.entry), compiler.pointIDs(call.finish)
		disposition := uint64(1)
		if call.executable {
			disposition = uint64(2)
		}
		if len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.appendOccurrence(programschema.OccurrenceCall, call.id, call.bodyPath, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), inputs, disposition) ||
			!compiler.recordOccurrenceSpan(programschema.OccurrenceCall, call.id, entryPoints, finishPoints) ||
			!compiler.appendOccurrence(programschema.OccurrenceCallActivation, call.id, call.bodyPath, append([]identity.ContentID(nil), finishPoints...), inputs, disposition) ||
			!compiler.recordOccurrenceSpan(programschema.OccurrenceCallActivation, call.id, nil, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for argIndex, argument := range call.arguments {
			if !compiler.appendOccurrence(programschema.OccurrenceCallArgument, argument.id, call.bodyPath, nil, []identity.ContentID{call.id}, uint64(argIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argIndex, CompileReasonOccurrenceCall)
			}
		}
		for typeIndex, argument := range call.typeArguments {
			if !compiler.appendOccurrence(programschema.OccurrenceCallTypeArgument, argument.id, call.bodyPath, nil, []identity.ContentID{call.id}, uint64(typeIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
		}
		if call.boundary.id.Available() {
			if !compiler.appendOccurrence(programschema.OccurrenceCallBoundary, call.boundary.id, call.bodyPath, nil, []identity.ContentID{call.id}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			for armIndex, arm := range call.boundary.arms {
				if !compiler.appendOccurrence(programschema.OccurrenceCallArm, arm.id, call.bodyPath, arm.points, []identity.ContentID{call.boundary.id, arm.route, arm.target}, uint64(armIndex)) {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, armIndex, CompileReasonOccurrenceCall)
				}
			}
		}
	}
	return CompileFailure{}
}
