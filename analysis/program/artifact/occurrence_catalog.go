package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// validateOccurrenceCausalInputsFailure validates the canonical rows that
// feed the guarded occurrence derivations. The derivations query those rows
// directly; no compiler-local join or publication index is retained.
func (compiler *compiler) validateOccurrenceCausalInputsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for callIndex, call := range compiler.calls {
		if !call.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		if call.Form() != programschema.CallFormPlain || call.ArgumentCount() != 1 {
			continue
		}
		if _, hasReceiver := call.ReceiverID(); hasReceiver {
			continue
		}
		if _, hasTail := call.TailID(); hasTail {
			continue
		}
		argumentOffset, argumentCount, argumentSpanOK := call.ArgumentSpan()
		if !argumentSpanOK || argumentCount != 1 || uint64(argumentOffset) >= uint64(len(compiler.callArguments)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		argument := compiler.callArguments[argumentOffset]
		if !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, 0, CompileReasonOccurrenceCall)
		}
		for priorIndex := 0; priorIndex < callIndex; priorIndex++ {
			prior := compiler.calls[priorIndex]
			if !prior.Available() || prior.Form() != programschema.CallFormPlain || prior.ArgumentCount() != 1 || prior.SpanID() != call.SpanID() {
				continue
			}
			if _, hasReceiver := prior.ReceiverID(); hasReceiver {
				continue
			}
			if _, hasTail := prior.TailID(); hasTail {
				continue
			}
			priorOffset, priorCount, priorSpanOK := prior.ArgumentSpan()
			if !priorSpanOK || priorCount != 1 || uint64(priorOffset) >= uint64(len(compiler.callArguments)) {
				continue
			}
			priorArgument := compiler.callArguments[priorOffset]
			if priorArgument.Available() && priorArgument.CallID() == prior.ID() && priorArgument.Index() == 0 &&
				(prior.ID() != call.ID() || priorArgument.MemberID() != argument.MemberID()) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
			}
		}
	}
	for bodyIndex, body := range compiler.bodies {
		if !body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyEntries)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceAttachment)
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := compiler.bodyEntries[offset+pointIndex]
			point := entry.PointID()
			if !entry.Available() || entry.BodyID() != body.ID() || !point.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, int(pointIndex), CompileReasonOccurrenceAttachment)
			}
		}
	}
	for occurrenceIndex, row := range compiler.occurrences {
		if !occurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case programschema.OccurrenceValueSource:
			if row.Code() == 1 {
				_, spanOK := occurrenceValueSourceSpanID(row, compiler.occurrenceInputs)
				if !spanOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceValueSourceAppend)
				}
			}
		case programschema.OccurrenceStorageRead:
			cell, span, readOK := occurrenceStorageRead(row, compiler.occurrenceInputs)
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			prior, duplicate, conflict := compiler.occurrenceStorageOriginBefore(occurrenceIndex, span)
			if conflict || duplicate && prior != cell {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
		case programschema.OccurrenceBinaryEquality:
		case programschema.OccurrenceValueClaim:
			operand, operandOK := occurrenceInputID(row, compiler.occurrenceInputs, 0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			prior, duplicate, conflict := compiler.occurrenceClaimOperandBefore(occurrenceIndex, row.ID())
			if conflict || duplicate && prior != operand {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	for edgeIndex, edge := range compiler.environment {
		condition, conditionOK := edge.ConditionValueSpanID()
		if !edge.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		if !conditionOK {
			continue
		}
		seen := make([]identity.ContentID, 0, 4)
		for condition.Available() {
			if containsOccurrenceID(seen, condition) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			seen = append(seen, condition)
			if compiler.occurrenceBinaryID(condition) {
				break
			}
			next, claimOK := compiler.occurrenceClaimOperand(condition)
			if !claimOK {
				break
			}
			condition = next
		}
	}
	return CompileFailure{}
}

func containsOccurrenceID(ids []identity.ContentID, target identity.ContentID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func (compiler *compiler) occurrenceBinaryID(id identity.ContentID) bool {
	if compiler == nil || !id.Available() {
		return false
	}
	for _, row := range compiler.occurrences {
		if row.Kind() == programschema.OccurrenceBinaryEquality && row.ID() == id {
			return true
		}
	}
	return false
}

func (compiler *compiler) occurrenceClaimOperand(id identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || !id.Available() {
		return identity.ContentID{}, false
	}
	for _, row := range compiler.occurrences {
		if row.Kind() != programschema.OccurrenceValueClaim || row.ID() != id {
			continue
		}
		return occurrenceInputID(row, compiler.occurrenceInputs, 0)
	}
	return identity.ContentID{}, false
}

func (compiler *compiler) occurrenceClaimOperandBefore(limit int, id identity.ContentID) (identity.ContentID, bool, bool) {
	if compiler == nil || !id.Available() || limit < 0 {
		return identity.ContentID{}, false, false
	}
	if limit > len(compiler.occurrences) {
		limit = len(compiler.occurrences)
	}
	var operand identity.ContentID
	found := false
	for index := 0; index < limit; index++ {
		row := compiler.occurrences[index]
		if row.Kind() != programschema.OccurrenceValueClaim || row.ID() != id {
			continue
		}
		candidate, ok := occurrenceInputID(row, compiler.occurrenceInputs, 0)
		if !ok {
			continue
		}
		if found && operand != candidate {
			return operand, true, true
		}
		operand, found = candidate, true
	}
	return operand, found, false
}

func (compiler *compiler) occurrenceStorageOriginBefore(limit int, span identity.ContentID) (identity.ContentID, bool, bool) {
	if compiler == nil || !span.Available() || limit < 0 {
		return identity.ContentID{}, false, false
	}
	if limit > len(compiler.occurrences) {
		limit = len(compiler.occurrences)
	}
	var cell identity.ContentID
	found := false
	for index := 0; index < limit; index++ {
		row := compiler.occurrences[index]
		if row.Kind() != programschema.OccurrenceStorageRead {
			continue
		}
		candidate, rowSpan, rowOK := occurrenceStorageRead(row, compiler.occurrenceInputs)
		if rowOK && rowSpan == span {
			if found && cell != candidate {
				return cell, true, true
			}
			cell, found = candidate, true
		}
	}
	return cell, found, false
}

func (compiler *compiler) occurrenceStorageOrigin(span identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || !span.Available() {
		return identity.ContentID{}, false
	}
	for _, row := range compiler.occurrences {
		if row.Kind() != programschema.OccurrenceStorageRead {
			continue
		}
		cell, rowSpan, rowOK := occurrenceStorageRead(row, compiler.occurrenceInputs)
		if rowOK && rowSpan == span {
			return cell, true
		}
	}
	return identity.ContentID{}, false
}

func (compiler *compiler) occurrenceNilSource(span identity.ContentID) bool {
	if compiler == nil || !span.Available() {
		return false
	}
	for _, row := range compiler.occurrences {
		if row.Kind() != programschema.OccurrenceValueSource || row.Code() != 1 {
			continue
		}
		rowSpan, rowOK := occurrenceValueSourceSpanID(row, compiler.occurrenceInputs)
		if rowOK && rowSpan == span {
			return true
		}
	}
	return false
}

func (compiler *compiler) occurrenceRuntimeCallForSpan(span identity.ContentID) (identity.ContentID, identity.ContentID, bool) {
	if compiler == nil || !span.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	for _, call := range compiler.calls {
		if !call.Available() || call.Form() != programschema.CallFormPlain || call.ArgumentCount() != 1 || call.SpanID() != span {
			continue
		}
		if _, hasReceiver := call.ReceiverID(); hasReceiver {
			continue
		}
		if _, hasTail := call.TailID(); hasTail {
			continue
		}
		argumentOffset, argumentCount, argumentSpanOK := call.ArgumentSpan()
		if !argumentSpanOK || argumentCount != 1 || uint64(argumentOffset) >= uint64(len(compiler.callArguments)) {
			continue
		}
		argument := compiler.callArguments[argumentOffset]
		if argument.Available() && argument.CallID() == call.ID() && argument.Index() == 0 {
			return call.ID(), argument.MemberID(), true
		}
	}
	return identity.ContentID{}, identity.ContentID{}, false
}

func (compiler *compiler) occurrenceBodyForPoint(point identity.ContentID) (identity.ContentID, bool, bool) {
	if compiler == nil || !point.Available() {
		return identity.ContentID{}, false, false
	}
	var bodyID identity.ContentID
	found := false
	ambiguous := false
	for _, body := range compiler.bodies {
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyEntries)) {
			continue
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := compiler.bodyEntries[offset+pointIndex]
			if !entry.Available() || entry.PointID() != point {
				continue
			}
			if !found {
				bodyID, found = body.ID(), true
				continue
			}
			if bodyID != body.ID() {
				ambiguous = true
			}
		}
	}
	return bodyID, ambiguous, found
}

func (compiler *compiler) occurrenceBranchEdges(condition identity.ContentID) []EnvironmentEdge {
	if compiler == nil || !condition.Available() {
		return nil
	}
	var edges []EnvironmentEdge
	for _, edge := range compiler.environment {
		current, conditionOK := edge.ConditionValueSpanID()
		if !conditionOK {
			continue
		}
		seen := make([]identity.ContentID, 0, 4)
		for current.Available() {
			if containsOccurrenceID(seen, current) {
				break
			}
			seen = append(seen, current)
			if current == condition {
				edges = append(edges, edge)
				break
			}
			next, claimOK := compiler.occurrenceClaimOperand(current)
			if !claimOK {
				break
			}
			current = next
		}
	}
	return edges
}

func (compiler *compiler) copyOccurrenceCatalogFailure() CompileFailure {
	// Existing Values/Body/Outcome planes are copied first, then restated as
	// generic rows so all later role derivations share exactly one catalog.
	authoredValues := compiler.input.Flow().Authored().Values()
	for valuesIndex, values := range compiler.values {
		term, termOK := authoredValues.At(valuesIndex)
		if !termOK || !values.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		var points []identity.ContentID
		if span, spanOK := compiler.input.Span(term); spanOK {
			finish, finishOK := span.Finish()
			rootSpanID, rootSpanOK := values.RootSpanID()
			if !finishOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsSite(finish) ||
				!rootSpanOK || rootSpanID != span.ContextID() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
			}
			points = compiler.pointIDs(finish)
		}
		if !compiler.appendOccurrence(programschema.OccurrenceValues, values.ID(), values.BodyPathID(), points, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
			member, ok := values.MemberAt(memberIndex)
			memberTerm, memberTermOK := authoredValues.Member(term, memberIndex)
			memberID, memberIDOK := compiler.valueSubjectID(memberTerm)
			if !ok || !memberTermOK || !memberIDOK ||
				!compiler.appendOccurrence(programschema.OccurrenceValuesMember, member.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID(), memberID}, uint64(memberIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, memberIndex, CompileReasonOccurrenceValues)
			}
		}
		if tail, ok := values.Tail(); ok && !compiler.appendOccurrence(programschema.OccurrenceValuesTail, tail.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID()}, uint64(tail.Kind())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
	}
	if failure := compiler.copyValueSources(); failure.Available() {
		return failure
	}
	if failure := compiler.copyFormalEntrySources(); failure.Available() {
		return failure
	}
	if failure := compiler.copyComputations(); failure.Available() {
		return failure
	}
	if failure := compiler.copyStorage(); failure.Available() {
		return failure
	}
	if failure := compiler.copyIndexAccess(); failure.Available() {
		return failure
	}
	if failure := compiler.copyAllocations(); failure.Available() {
		return failure
	}
	if failure := compiler.copyCalls(); failure.Available() {
		return failure
	}
	if failure := compiler.validateOccurrenceCausalInputsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveBinaryPresenceRefinementsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveOperationPredicateRefinementsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveExactScalarSummariesFailure(); failure.Available() {
		return failure
	}
	return CompileFailure{}
}

// deriveOperationPredicateRefinementsFailure compiles the neutral guarded
// operation-predicate geometry from existing Call, BinaryEquality, and
// EnvironmentEdge rows. It authenticates only the parent-issued operation and
// route identities; the mounted target behavior and predicate meaning remain
// opaque to Program and are interpreted by the consuming Value domain.

func (compiler *compiler) deriveOperationPredicateRefinementsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for binaryIndex, binary := range compiler.occurrences {
		if binary.Kind() != programschema.OccurrenceBinaryEquality {
			continue
		}
		left, right, op, equalityOK := occurrenceBinaryEquality(binary, compiler.occurrenceInputs)
		if !equalityOK {
			continue
		}
		callID, subject, leftCall := compiler.occurrenceRuntimeCallForSpan(left)
		operand := right
		if !leftCall {
			callID, subject, leftCall = compiler.occurrenceRuntimeCallForSpan(right)
			operand = left
		}
		if !leftCall || !subject.Available() || !operand.Available() {
			continue
		}
		for armIndex, selected := range compiler.occurrenceBranchEdges(binary.ID()) {
			truth, truthOK := selected.Truth()
			bodyID, ambiguousBody, bodyOK := compiler.occurrenceBodyForPoint(selected.To())
			routeID := selected.RouteID()
			if !truthOK || !bodyOK || ambiguousBody || !routeID.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			id := digest("analysis/program-artifact/operation-predicate-refinement", artifactFormat,
				bytesField(callID), bytesField(binary.ID()), bytesField(selected.ID()), bytesField(subject), bytesField(operand), boolField(truth))
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, armIndex, CompileReasonOccurrenceUnavailable)
			}
			code := uint64(op) | operationPredicateCodeTruth
			if !truth {
				code = uint64(op)
			}
			inputs := []identity.ContentID{callID, subject, operand, routeID}
			appended := compiler.appendOccurrence(programschema.OccurrenceOperationPredicateRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code)
			if !appended ||
				!compiler.recordOccurrencePredecessor(programschema.OccurrenceOperationPredicateRefinement, id, routeID, []identity.ContentID{selected.To()}) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, armIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}

// deriveBinaryPresenceRefinementsFailure compiles the reusable nilability
// transfer already proved by Program's BinaryPrimitive, storage, and causal
// route rows. The join runs once while the Program artifact is built:
// Link and Runtime receive only the resulting scalar rows and never reopen or
// rescan Program/Flow.
func (compiler *compiler) deriveBinaryPresenceRefinementsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for binaryIndex, binary := range compiler.occurrences {
		if binary.Kind() != programschema.OccurrenceBinaryEquality {
			continue
		}
		left, right, op, equalityOK := occurrenceBinaryEquality(binary, compiler.occurrenceInputs)
		if !equalityOK {
			continue
		}
		leftNil, rightNil := compiler.occurrenceNilSource(left), compiler.occurrenceNilSource(right)
		var operand, target identity.ContentID
		switch {
		case leftNil && !rightNil:
			operand = right
			target, _ = compiler.occurrenceStorageOrigin(right)
		case rightNil && !leftNil:
			operand = left
			target, _ = compiler.occurrenceStorageOrigin(left)
		default:
			continue
		}
		// A comparison of a temporary or structural value remains valid but has
		// no persistent storage coordinate to narrow for later Reads.
		if !target.Available() {
			continue
		}

		for armIndex, selected := range compiler.occurrenceBranchEdges(binary.ID()) {
			truth, truthOK := selected.Truth()
			bodyID, ambiguousBody, bodyOK := compiler.occurrenceBodyForPoint(selected.To())
			if !truthOK || !bodyOK || ambiguousBody {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			present := truth == (op == flowkind.BinaryNotEqual)
			id := digest("analysis/program-artifact/binary-presence-refinement", artifactFormat,
				bytesField(binary.ID()), bytesField(selected.ID()), bytesField(target), boolField(present))
			routeID := selected.RouteID()
			inputs := []identity.ContentID{binary.ID(), target, operand, routeID}
			code := uint64(0)
			if present {
				code = 1
			}
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.appendOccurrence(programschema.OccurrenceBinaryPresenceRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.recordOccurrencePredecessor(programschema.OccurrenceBinaryPresenceRefinement, id, routeID, []identity.ContentID{selected.To()}) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}
