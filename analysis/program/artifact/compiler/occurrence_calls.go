package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) copyAllocations() CompileFailure {
	if compiler == nil || compiler.allocations == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	for index := 0; index < compiler.allocations.Count(); index++ {
		allocation, allocationOK := compiler.allocations.RowAt(index)
		entry, entryOK := allocation.Entry()
		finish, finishOK := allocation.Finish()
		occurrence, occurrenceOK := allocation.Occurrence()
		template, templateOK := allocation.Template()
		form, formOK := allocation.Form()
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !allocationOK || !entryOK || !finishOK || !occurrenceOK || !templateOK || !formOK ||
			!compiler.appendOccurrencePaths(programschema.OccurrenceAllocation, template, identity.ContentID{}, entryPoints, finishPoints, []identity.ContentID{template, occurrence}, uint64(form)) ||
			!compiler.recordOccurrencePaths(programschema.OccurrenceAllocation, template, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		for fieldIndex := 0; fieldIndex < allocation.FieldCount(); fieldIndex++ {
			field, fieldOK := allocation.FieldAt(fieldIndex)
			values, valuesOK := field.Values()
			fieldID, fieldIDOK := field.ID()
			inputs := []identity.ContentID{template}
			if values.Available() {
				inputs = append(inputs, values.ID())
				for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
					member, memberOK := compiler.valueMemberAt(values, memberIndex)
					if !memberOK {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
					}
					inputs = append(inputs, member.ID())
				}
			}
			if !fieldOK || !valuesOK || !fieldIDOK || !values.Available() || !compiler.appendOccurrence(programschema.OccurrenceAllocationField, fieldID, identity.ContentID{}, nil, inputs, uint64(fieldIndex)) {
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(call.entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(call.finish)
		disposition := uint64(1)
		if call.executable {
			disposition = uint64(2)
		}
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 ||
			!compiler.appendOccurrencePaths(programschema.OccurrenceCall, call.id, call.bodyPath, entryPoints, finishPoints, inputs, disposition) ||
			!compiler.recordOccurrencePaths(programschema.OccurrenceCall, call.id, entryPoints, finishPoints) ||
			!compiler.appendOccurrencePaths(programschema.OccurrenceCallActivation, call.id, call.bodyPath, causal.SitePointPaths{}, finishPoints, inputs, disposition) ||
			!compiler.recordOccurrencePaths(programschema.OccurrenceCallActivation, call.id, causal.SitePointPaths{}, finishPoints) {
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
				if !compiler.appendOccurrencePaths(programschema.OccurrenceCallArm, arm.id, call.bodyPath, causal.SitePointPaths{}, arm.points, []identity.ContentID{call.boundary.id, arm.route, arm.target}, uint64(armIndex)) {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, armIndex, CompileReasonOccurrenceCall)
				}
			}
		}
	}
	return CompileFailure{}
}
