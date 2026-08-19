package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// occurrenceCausalIndex is the one Program-owned neutral join used by the
// guarded occurrence derivations. The environment rows already carry the
// parent-issued route geometry; this index only resolves their existing
// condition/claim and body-entry identities once. Consumers never reopen
// Flow or reconstruct a route proof.
type occurrenceCausalIndex struct {
	nilSources         map[identity.ContentID]struct{}
	storageOrigins     map[identity.ContentID]identity.ContentID
	binaries           []uint32
	binaryByID         map[identity.ContentID]uint32
	claims             map[identity.ContentID]identity.ContentID
	branchEdges        map[identity.ContentID][]EnvironmentEdge
	bodyByEntry        map[identity.ContentID]identity.ContentID
	ambiguousBodyEntry map[identity.ContentID]struct{}
	callBySpan         map[identity.ContentID]strictRuntimeCall
}

type strictRuntimeCall struct {
	id      identity.ContentID
	span    identity.ContentID
	subject identity.ContentID
}

func (compiler *compiler) occurrenceCausalIndexFailure() (occurrenceCausalIndex, CompileFailure) {
	if compiler == nil {
		return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	index := occurrenceCausalIndex{
		nilSources:         make(map[identity.ContentID]struct{}),
		storageOrigins:     make(map[identity.ContentID]identity.ContentID),
		binaryByID:         make(map[identity.ContentID]uint32),
		claims:             make(map[identity.ContentID]identity.ContentID),
		branchEdges:        make(map[identity.ContentID][]EnvironmentEdge),
		bodyByEntry:        make(map[identity.ContentID]identity.ContentID, len(compiler.bodies)),
		ambiguousBodyEntry: make(map[identity.ContentID]struct{}),
		callBySpan:         make(map[identity.ContentID]strictRuntimeCall),
	}
	for callIndex, call := range compiler.calls {
		if !call.Available() {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
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
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		argument := compiler.callArguments[argumentOffset]
		if !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, 0, CompileReasonOccurrenceCall)
		}
		candidate := strictRuntimeCall{id: call.ID(), span: call.SpanID(), subject: argument.MemberID()}
		if prior, duplicate := index.callBySpan[candidate.span]; duplicate && prior != candidate {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		index.callBySpan[candidate.span] = candidate
	}
	for bodyIndex, body := range compiler.bodies {
		if !body.Available() {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyEntries)) {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceAttachment)
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := compiler.bodyEntries[offset+pointIndex]
			point := entry.PointID()
			if !entry.Available() || entry.BodyID() != body.ID() || !point.Available() {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, int(pointIndex), CompileReasonOccurrenceAttachment)
			}
			if prior, duplicate := index.bodyByEntry[point]; duplicate && prior != body.ID() {
				index.ambiguousBodyEntry[point] = struct{}{}
			} else {
				index.bodyByEntry[point] = body.ID()
			}
		}
	}
	for occurrenceIndex, row := range compiler.occurrences {
		if !occurrenceDenseAvailable(row, compiler.occurrencePoints, compiler.occurrenceInputs) {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case programschema.OccurrenceValueSource:
			if row.Code() == 1 {
				span, spanOK := occurrenceValueSourceSpanID(row, compiler.occurrenceInputs)
				if !spanOK {
					return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceValueSourceAppend)
				}
				index.nilSources[span] = struct{}{}
			}
		case programschema.OccurrenceStorageRead:
			cell, span, readOK := occurrenceStorageRead(row, compiler.occurrenceInputs)
			if !readOK {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			if prior, duplicate := index.storageOrigins[span]; duplicate && prior != cell {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			index.storageOrigins[span] = cell
		case programschema.OccurrenceBinaryEquality:
			index.binaries = append(index.binaries, uint32(occurrenceIndex))
			index.binaryByID[row.ID()] = uint32(occurrenceIndex)
		case programschema.OccurrenceValueClaim:
			operand, operandOK := occurrenceInputID(row, compiler.occurrenceInputs, 0)
			if !operandOK {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			if prior, duplicate := index.claims[row.ID()]; duplicate && prior != operand {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			index.claims[row.ID()] = operand
		}
	}
	for edgeIndex, edge := range compiler.environment {
		condition, conditionOK := edge.ConditionValueSpanID()
		if !edge.Available() {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		if !conditionOK {
			continue
		}
		seen := make(map[identity.ContentID]struct{})
		for condition.Available() {
			if _, cycle := seen[condition]; cycle {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			seen[condition] = struct{}{}
			if _, binaryOK := index.binaryByID[condition]; binaryOK {
				index.branchEdges[condition] = append(index.branchEdges[condition], edge)
				break
			}
			next, claimOK := index.claims[condition]
			if !claimOK {
				break
			}
			condition = next
		}
	}
	return index, CompileFailure{}
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
			memberID, memberIDOK := compiler.input.ValueSubjectID(memberTerm)
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
	causalIndex, failure := compiler.occurrenceCausalIndexFailure()
	if failure.Available() {
		return failure
	}
	if failure := compiler.deriveBinaryPresenceRefinementsFailure(causalIndex); failure.Available() {
		return failure
	}
	if failure := compiler.deriveOperationPredicateRefinementsFailure(causalIndex); failure.Available() {
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

func (compiler *compiler) deriveOperationPredicateRefinementsFailure(index occurrenceCausalIndex) CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for _, binaryOrdinal := range index.binaries {
		if uint64(binaryOrdinal) >= uint64(len(compiler.occurrences)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(binaryOrdinal), -1, CompileReasonOccurrenceUnavailable)
		}
		binary := compiler.occurrences[binaryOrdinal]
		left, right, op, equalityOK := occurrenceBinaryEquality(binary, compiler.occurrenceInputs)
		if !equalityOK {
			continue
		}
		call, leftCall := index.callBySpan[left]
		operand := right
		if !leftCall {
			call, leftCall = index.callBySpan[right]
			operand = left
		}
		if !leftCall || !call.subject.Available() || !operand.Available() {
			continue
		}
		for armIndex, selected := range index.branchEdges[binary.ID()] {
			truth, truthOK := selected.Truth()
			bodyID, bodyOK := index.bodyByEntry[selected.To()]
			_, ambiguousBody := index.ambiguousBodyEntry[selected.To()]
			routeID := selected.RouteID()
			if !truthOK || !bodyOK || ambiguousBody || !routeID.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, armIndex, CompileReasonOccurrenceUnavailable)
			}
			id := digest("analysis/program-artifact/operation-predicate-refinement", artifactFormat,
				bytesField(call.id), bytesField(binary.ID()), bytesField(selected.ID()), bytesField(call.subject), bytesField(operand), boolField(truth))
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, armIndex, CompileReasonOccurrenceUnavailable)
			}
			code := uint64(op) | operationPredicateCodeTruth
			if !truth {
				code = uint64(op)
			}
			inputs := []identity.ContentID{call.id, call.subject, operand, routeID}
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
func (compiler *compiler) deriveBinaryPresenceRefinementsFailure(index occurrenceCausalIndex) CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for binaryIndex, binaryOrdinal := range index.binaries {
		if uint64(binaryOrdinal) >= uint64(len(compiler.occurrences)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, int(binaryOrdinal), -1, CompileReasonOccurrenceUnavailable)
		}
		binary := compiler.occurrences[binaryOrdinal]
		left, right, op, equalityOK := occurrenceBinaryEquality(binary, compiler.occurrenceInputs)
		if !equalityOK {
			continue
		}
		_, leftNil := index.nilSources[left]
		_, rightNil := index.nilSources[right]
		var operand, target identity.ContentID
		switch {
		case leftNil && !rightNil:
			operand, target = right, index.storageOrigins[right]
		case rightNil && !leftNil:
			operand, target = left, index.storageOrigins[left]
		default:
			continue
		}
		// A comparison of a temporary or structural value remains valid but has
		// no persistent storage coordinate to narrow for later Reads.
		if !target.Available() {
			continue
		}

		for armIndex, selected := range index.branchEdges[binary.ID()] {
			truth, truthOK := selected.Truth()
			bodyID, bodyOK := index.bodyByEntry[selected.To()]
			_, ambiguousBody := index.ambiguousBodyEntry[selected.To()]
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
