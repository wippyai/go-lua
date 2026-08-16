package boundary

import (
	"errors"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
)

// sealMountedCalls performs the sole raw Program-to-Boundary join while the
// Boundary owner is sealing. The retained rows contain exact Project/Program
// proofs and existing Boundary Value ordinals; hot queries never reopen Flow.
func sealMountedCalls(authority *authority) error {
	if authority == nil || authority.project == nil || authority.valueTable == nil {
		return errors.New("link/boundary: unavailable mounted Call inputs")
	}
	applications := authority.project.Applications()
	rows := make([]mountedCallRow, applications.Count())
	calls := applications.Calls()
	semantic := make(map[mountedCallSemanticKey]uint32, calls.Count())
	mounts := authority.project.Mounts()
	for index := 0; index < calls.Count(); index++ {
		mounted, mountedOK := calls.MountedAt(index)
		application, applicationOK := mounted.Application()
		applicationIndex, applicationIndexOK := applications.Index(application)
		occurrence, occurrenceOK := mounted.Occurrence()
		shard, shardOK := mounted.Mount()
		mountIndex, mountIndexOK := mounts.Index(shard)
		module, moduleOK := authority.project.ModuleKey(shard)
		p, programOK := mounts.Program(shard)
		if !mountedOK || !applicationOK || !applicationIndexOK || applicationIndex < 0 || applicationIndex >= len(rows) || rows[applicationIndex].ready || !occurrenceOK || !shardOK || !mountIndexOK || !moduleOK || !module.Available() || !programOK || p == nil {
			return errors.New("link/boundary: malformed mounted Call proof")
		}
		input := p.TransformerInput()
		calleeProof, calleeProofOK := occurrence.Callee()
		calleeSpan, calleeSpanOK := calleeProof.Span()
		actualsProof, actualsProofOK := occurrence.Actuals()
		actualsSpan, actualsSpanOK := actualsProof.Span()
		resultSpan, resultSpanOK := occurrence.Span()
		form, formOK := occurrence.Form()
		values, valuesOK := occurrence.Values()
		callee, calleeOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), calleeSpan)
		actuals, actualsOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), actualsSpan)
		result, resultOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), resultSpan)
		if !input.OwnsCallOccurrence(occurrence) || !calleeProofOK || !input.OwnsCallOperand(calleeProof) || !calleeSpanOK || !input.OwnsSpan(calleeSpan) || !actualsProofOK || !input.OwnsCallOperand(actualsProof) || !actualsSpanOK || !input.OwnsSpan(actualsSpan) || !resultSpanOK || !input.OwnsSpan(resultSpan) || !formOK || !valuesOK || !input.OwnsCallValues(values) || !calleeOK || !actualsOK || !resultOK {
			return errors.New("link/boundary: malformed mounted Call operands")
		}
		row := mountedCallRow{ready: true, module: module, mounted: mounted, occurrence: occurrence, values: values, form: form, callee: callee, actuals: actuals, result: result, arguments: make([]uint32, values.Count())}
		switch form {
		case flow.CallFormPlain:
			if receiver, receiverOK := occurrence.Receiver(); receiverOK || receiver.Available() {
				return errors.New("link/boundary: plain mounted Call has receiver")
			}
		case flow.CallFormMethod:
			receiverProof, receiverProofOK := occurrence.Receiver()
			receiverSpan, receiverSpanOK := receiverProof.Span()
			receiver, receiverOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), receiverSpan)
			if !receiverProofOK || !input.OwnsCallOperand(receiverProof) || !receiverSpanOK || !input.OwnsSpan(receiverSpan) || !receiverOK {
				return errors.New("link/boundary: method mounted Call lacks receiver")
			}
			row.receiver, row.hasReceiver = receiver, true
		default:
			return errors.New("link/boundary: invalid mounted Call form")
		}
		for argumentIndex := 0; argumentIndex < values.Count(); argumentIndex++ {
			argument, argumentOK := values.At(argumentIndex)
			argumentSpan, argumentSpanOK := argument.Span()
			ordinal, ordinalOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), argumentSpan)
			if !argumentOK || !input.OwnsCallArgument(argument) || !argumentSpanOK || !input.OwnsSpan(argumentSpan) || !ordinalOK {
				return errors.New("link/boundary: malformed mounted Call argument")
			}
			row.arguments[argumentIndex] = ordinal
		}
		if producer, open := values.Tail(); open {
			tailSpan, tailSpanOK := producer.Span()
			tailOrdinal, tailOrdinalOK := boundaryValueForProgramSpan(authority.valueTable, uint32(mountIndex+1), tailSpan)
			tailContext := tailSpan.ContextID()
			if !tailSpanOK || !tailOrdinalOK || !input.OwnsTailProducer(producer) || !input.OwnsSpan(tailSpan) || !tailContext.Available() {
				return errors.New("link/boundary: malformed mounted Call actual tail")
			}
			row.actualTail, row.tailSpan, row.tailContext, row.hasTail = tailOrdinal, tailSpan, tailContext, true
		}
		key := mountedCallSemanticKey{module: module, call: occurrence.ContextID()}
		if _, duplicate := semantic[key]; duplicate {
			return errors.New("link/boundary: duplicate mounted semantic Call")
		}
		semantic[key] = uint32(applicationIndex)
		rows[applicationIndex] = row
	}
	authority.mountedCalls = &mountedCallTable{rows: rows, semantic: semantic}
	return nil
}

func boundaryValueForProgramSpan(table *valueTable, mount uint32, span program.Span) (uint32, bool) {
	if table == nil || table.spans == nil || mount == 0 || !span.Available() {
		return 0, false
	}
	context := span.ContextID()
	ordinal, ok := table.spans[valueSpanKey{mount: mount, context: context}]
	if !ok || uint64(ordinal) >= uint64(len(table.rows)) {
		return 0, false
	}
	row := table.rows[ordinal]
	return ordinal, context.Available() && row.shard == mount
}
