package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

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
	for _, body := range compiler.bodies {
		if !compiler.appendOccurrence(OccurrenceBody, body.ID(), body.ID(), nil, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for outcomeIndex, outcome := range compiler.outcomes {
		if !compiler.appendOccurrence(OccurrenceOutcome, outcome.ID(), outcome.BodyID(), nil, nil, uint64(outcome.Kind())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, outcomeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for valueIndex := 0; valueIndex < outcome.ReturnValueCount(); valueIndex++ {
			value, ok := compiler.returnValueAt(outcome, valueIndex)
			id := digest("analysis/program-artifact/return-value-occurrence", artifactFormat, bytesField(outcome.ID()), bytesField(value.ID()), uintField(uint64(valueIndex)))
			if !ok || !compiler.appendOccurrence(OccurrenceReturnValue, id, outcome.BodyID(), nil, []identity.ContentID{outcome.ID(), value.ID()}, uint64(valueIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, outcomeIndex, valueIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	if failure := compiler.copyPointAttachments(); failure.Available() {
		return failure
	}
	if failure := compiler.copyValueSources(); failure.Available() {
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
	if failure := compiler.deriveBinaryPresenceRefinementsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveExactScalarSummariesFailure(); failure.Available() {
		return failure
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
	nilSources := make(map[identity.ContentID]struct{})
	storageOrigins := make(map[identity.ContentID]identity.ContentID)
	var binaries []OccurrenceRow
	binaryByID := make(map[identity.ContentID]OccurrenceRow)
	claims := make(map[identity.ContentID]identity.ContentID)
	for index, row := range compiler.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case OccurrenceValueSource:
			if row.Code() == 1 {
				span, spanOK := row.ValueSourceSpanID()
				if !spanOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
				}
				nilSources[span] = struct{}{}
			}
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			if prior, duplicate := storageOrigins[span]; duplicate && prior != cell {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			storageOrigins[span] = cell
		case OccurrenceBinaryEquality:
			binaries = append(binaries, row)
			binaryByID[row.ID()] = row
		case OccurrenceValueClaim:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if prior, duplicate := claims[row.ID()]; duplicate && prior != operand {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			claims[row.ID()] = operand
		}
	}

	bodyByEntry := make(map[identity.ContentID]identity.ContentID, len(compiler.bodies))
	ambiguousBodyEntry := make(map[identity.ContentID]struct{})
	for index, body := range compiler.bodies {
		if !body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for pointIndex := 0; pointIndex < body.EntryPointCount(); pointIndex++ {
			point, pointOK := body.EntryPointAt(pointIndex)
			if !pointOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, pointIndex, CompileReasonOccurrenceAttachment)
			}
			if prior, duplicate := bodyByEntry[point]; duplicate && prior != body.ID() {
				ambiguousBodyEntry[point] = struct{}{}
			} else {
				bodyByEntry[point] = body.ID()
			}
		}
	}

	branchEdges := make(map[identity.ContentID][]EnvironmentEdge)
	for edgeIndex, edge := range compiler.environment {
		condition, conditionOK := edge.ConditionValueSpanID()
		if !edge.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		if !conditionOK {
			continue
		}
		seen := make(map[identity.ContentID]struct{})
		for condition.Available() {
			if _, cycle := seen[condition]; cycle {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			seen[condition] = struct{}{}
			if _, binaryOK := binaryByID[condition]; binaryOK {
				branchEdges[condition] = append(branchEdges[condition], edge)
				break
			}
			next, claimOK := claims[condition]
			if !claimOK {
				break
			}
			condition = next
		}
	}

	for binaryIndex, binary := range binaries {
		left, right, op, equalityOK := binary.BinaryEquality()
		if !equalityOK {
			continue
		}
		_, leftNil := nilSources[left]
		_, rightNil := nilSources[right]
		operand, target := identity.ContentID{}, identity.ContentID{}
		switch {
		case leftNil && !rightNil:
			operand, target = right, storageOrigins[right]
		case rightNil && !leftNil:
			operand, target = left, storageOrigins[left]
		default:
			continue
		}
		// A comparison of a temporary or structural value remains valid but has
		// no persistent storage coordinate to narrow for later Reads.
		if !target.Available() {
			continue
		}

		for armIndex, selected := range branchEdges[binary.ID()] {
			truth, truthOK := selected.Truth()
			bodyID, bodyOK := bodyByEntry[selected.To()]
			_, ambiguousBody := ambiguousBodyEntry[selected.To()]
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
