package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// occurrenceCausalIndex is the one Program-owned neutral join used by the
// guarded occurrence derivations. The environment rows already carry the
// parent-issued route geometry; this index only resolves their existing
// condition/claim and body-entry identities once. Consumers never reopen
// Flow or reconstruct a route proof.
type occurrenceCausalIndex struct {
	nilSources         map[identity.ContentID]struct{}
	storageOrigins     map[identity.ContentID]identity.ContentID
	binaries           []OccurrenceRow
	binaryByID         map[identity.ContentID]OccurrenceRow
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
		binaryByID:         make(map[identity.ContentID]OccurrenceRow),
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
		if call.Form() != CallFormPlain || call.ArgumentCount() != 1 {
			continue
		}
		if _, hasReceiver := call.ReceiverID(); hasReceiver {
			continue
		}
		if _, hasTail := call.TailID(); hasTail {
			continue
		}
		if uint64(call.argumentStart) >= uint64(len(compiler.callArguments)) {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		argument := compiler.callArguments[call.argumentStart]
		if !argument.Available() || argument.call != call.id || argument.position != 0 {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, 0, CompileReasonOccurrenceCall)
		}
		candidate := strictRuntimeCall{id: call.id, span: call.span, subject: argument.member}
		if prior, duplicate := index.callBySpan[candidate.span]; duplicate && prior != candidate {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		index.callBySpan[candidate.span] = candidate
	}
	for bodyIndex, body := range compiler.bodies {
		if !body.Available() {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for pointIndex := 0; pointIndex < body.EntryPointCount(); pointIndex++ {
			point, pointOK := body.EntryPointAt(pointIndex)
			if !pointOK {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, pointIndex, CompileReasonOccurrenceAttachment)
			}
			if prior, duplicate := index.bodyByEntry[point]; duplicate && prior != body.ID() {
				index.ambiguousBodyEntry[point] = struct{}{}
			} else {
				index.bodyByEntry[point] = body.ID()
			}
		}
	}
	for occurrenceIndex, row := range compiler.occurrences {
		if !row.Available() {
			return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case OccurrenceValueSource:
			if row.Code() == 1 {
				span, spanOK := row.ValueSourceSpanID()
				if !spanOK {
					return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceValueSourceAppend)
				}
				index.nilSources[span] = struct{}{}
			}
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			if prior, duplicate := index.storageOrigins[span]; duplicate && prior != cell {
				return occurrenceCausalIndex{}, compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			index.storageOrigins[span] = cell
		case OccurrenceBinaryEquality:
			index.binaries = append(index.binaries, row)
			index.binaryByID[row.ID()] = row
		case OccurrenceValueClaim:
			operand, operandOK := row.InputAt(0)
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
		if !compiler.appendOccurrence(OccurrenceValues, values.ID(), values.BodyPathID(), points, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
			member, ok := values.MemberAt(memberIndex)
			memberTerm, memberTermOK := authoredValues.Member(term, memberIndex)
			memberID, memberIDOK := compiler.input.ValueSubjectID(memberTerm)
			if !ok || !memberTermOK || !memberIDOK ||
				!compiler.appendOccurrence(OccurrenceValuesMember, member.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID(), memberID}, uint64(memberIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, memberIndex, CompileReasonOccurrenceValues)
			}
		}
		if tail, ok := values.Tail(); ok && !compiler.appendOccurrence(OccurrenceValuesTail, tail.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID()}, uint64(tail.Kind())) {
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
	for _, binary := range index.binaries {
		left, right, op, equalityOK := binary.BinaryEquality()
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
			appended := compiler.appendOccurrence(OccurrenceOperationPredicateRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code)
			if !appended ||
				!compiler.occurrences[len(compiler.occurrences)-1].Available() ||
				!compiler.recordOccurrencePredecessor(OccurrenceOperationPredicateRefinement, id, routeID, []identity.ContentID{selected.To()}) {
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
	for binaryIndex, binary := range index.binaries {
		left, right, op, equalityOK := binary.BinaryEquality()
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
			if !compiler.appendOccurrence(OccurrenceBinaryPresenceRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.occurrences[len(compiler.occurrences)-1].Available() ||
				!compiler.recordOccurrencePredecessor(OccurrenceBinaryPresenceRefinement, id, routeID, []identity.ContentID{selected.To()}) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}
