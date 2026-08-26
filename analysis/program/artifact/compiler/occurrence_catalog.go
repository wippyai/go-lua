package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/exactscalar"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/valuesource"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// validateOccurrenceCausalInputsFailure validates the canonical rows that
// feed the guarded occurrence derivations. The derivations query those rows
// directly; no compiler-local join or publication index is retained.
func (compiler *compiler) validateOccurrenceCausalInputsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	type plainCallArgument struct {
		call   identity.ContentID
		member identity.ContentID
	}
	// A plain one-argument call is the sole Call shape that supplies an
	// occurrence runtime argument. Keep one authenticated pair per SpanID for
	// this validation pass: equal pairs are repeatable; a distinct pair fails
	// at the later Call row, exactly as the former prior-row scan did.
	plainArgumentsBySpan := make(map[identity.ContentID]plainCallArgument, len(compiler.publication.Calls))
	for callIndex, call := range compiler.publication.Calls {
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
		if !argumentSpanOK || argumentCount != 1 || uint64(argumentOffset) >= uint64(len(compiler.publication.CallArguments)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		argument := compiler.publication.CallArguments[argumentOffset]
		if !argument.Available() || argument.CallID() != call.ID() || argument.Index() != 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, 0, CompileReasonOccurrenceCall)
		}
		span := call.SpanID()
		if prior, duplicate := plainArgumentsBySpan[span]; duplicate && (prior.call != call.ID() || prior.member != argument.MemberID()) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, callIndex, -1, CompileReasonOccurrenceCall)
		}
		plainArgumentsBySpan[span] = plainCallArgument{call: call.ID(), member: argument.MemberID()}
	}
	for bodyIndex, body := range compiler.bodyBoundary.Bodies() {
		if !body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyBoundary.BodyEntries())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, -1, CompileReasonOccurrenceAttachment)
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := compiler.bodyBoundary.BodyEntries()[offset+pointIndex]
			point := entry.PointID()
			if !entry.Available() || entry.BodyID() != body.ID() || !point.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, bodyIndex, int(pointIndex), CompileReasonOccurrenceAttachment)
			}
		}
	}
	for occurrenceIndex, row := range compiler.publication.Occurrences {
		if !programschema.OccurrenceDenseAvailable(row, compiler.publication.OccurrencePoints, compiler.publication.OccurrenceInputs) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case programschema.OccurrenceValueSource:
			if row.Code() == 1 {
				_, spanOK := programschema.OccurrenceValueSourceSpanID(row, compiler.publication.OccurrenceInputs)
				if !spanOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceValueSourceAppend)
				}
			}
		case programschema.OccurrenceStorageRead:
			cell, span, readOK := programschema.OccurrenceStorageReadOperands(row, compiler.publication.OccurrenceInputs)
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
			prior, duplicate, conflict := compiler.occurrenceStorageOriginBefore(occurrenceIndex, span)
			if conflict || duplicate && prior != cell {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, occurrenceIndex, -1, CompileReasonOccurrenceStorageRead)
			}
		case programschema.OccurrenceBinaryEquality:
		case programschema.OccurrenceValueClaim:
			operand, operandOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 0)
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
	for _, row := range compiler.publication.Occurrences {
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
	for _, row := range compiler.publication.Occurrences {
		if row.Kind() != programschema.OccurrenceValueClaim || row.ID() != id {
			continue
		}
		return programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 0)
	}
	return identity.ContentID{}, false
}

func (compiler *compiler) occurrenceClaimOperandBefore(limit int, id identity.ContentID) (identity.ContentID, bool, bool) {
	if compiler == nil || !id.Available() || limit < 0 {
		return identity.ContentID{}, false, false
	}
	if limit > len(compiler.publication.Occurrences) {
		limit = len(compiler.publication.Occurrences)
	}
	var operand identity.ContentID
	found := false
	for index := 0; index < limit; index++ {
		row := compiler.publication.Occurrences[index]
		if row.Kind() != programschema.OccurrenceValueClaim || row.ID() != id {
			continue
		}
		candidate, ok := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 0)
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
	if limit > len(compiler.publication.Occurrences) {
		limit = len(compiler.publication.Occurrences)
	}
	var cell identity.ContentID
	found := false
	for index := 0; index < limit; index++ {
		row := compiler.publication.Occurrences[index]
		if row.Kind() != programschema.OccurrenceStorageRead {
			continue
		}
		candidate, rowSpan, rowOK := programschema.OccurrenceStorageReadOperands(row, compiler.publication.OccurrenceInputs)
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
	for _, row := range compiler.publication.Occurrences {
		if row.Kind() != programschema.OccurrenceStorageRead {
			continue
		}
		cell, rowSpan, rowOK := programschema.OccurrenceStorageReadOperands(row, compiler.publication.OccurrenceInputs)
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
	for _, row := range compiler.publication.Occurrences {
		if row.Kind() != programschema.OccurrenceValueSource || row.Code() != 1 {
			continue
		}
		rowSpan, rowOK := programschema.OccurrenceValueSourceSpanID(row, compiler.publication.OccurrenceInputs)
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
	for _, call := range compiler.publication.Calls {
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
		if !argumentSpanOK || argumentCount != 1 || uint64(argumentOffset) >= uint64(len(compiler.publication.CallArguments)) {
			continue
		}
		argument := compiler.publication.CallArguments[argumentOffset]
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
	for _, body := range compiler.bodyBoundary.Bodies() {
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyBoundary.BodyEntries())) {
			continue
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := compiler.bodyBoundary.BodyEntries()[offset+pointIndex]
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

func (compiler *compiler) occurrenceBranchEdges(condition identity.ContentID) []environmentEdgeDraft {
	if compiler == nil || !condition.Available() {
		return nil
	}
	var edges []environmentEdgeDraft
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
	for valuesIndex, values := range compiler.publication.Values {
		term, termOK := authoredValues.At(valuesIndex)
		if !termOK || !values.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		var pointPaths causal.SitePointPaths
		if span, spanOK := compiler.input.Span(term); spanOK {
			finish, finishOK := span.Finish()
			rootSpanID, rootSpanOK := values.RootSpanID()
			if !finishOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsSite(finish) ||
				!rootSpanOK || rootSpanID != span.ContextID() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
			}
			pointPaths = compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		}
		// A Values row has exactly one occurrence identity. The path-bearing
		// append already publishes that row; do not first append an empty
		// geometry row, because the issuance owner rejects duplicate geometry.
		var appended bool
		if pointPaths.Available() {
			appended = compiler.appendOccurrencePaths(programschema.OccurrenceValues, values.ID(), values.BodyPathID(), causal.SitePointPaths{}, pointPaths, nil, 0)
		} else {
			appended = compiler.appendOccurrence(programschema.OccurrenceValues, values.ID(), values.BodyPathID(), nil, nil, 0)
		}
		if !appended {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
			member, ok := compiler.valueMemberAt(values, memberIndex)
			memberTerm, memberTermOK := authoredValues.Member(term, memberIndex)
			memberID, memberIDOK := valuesource.SubjectSpan(compiler.input, memberTerm)
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
	if compiler.exactScalar != nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	bundle, fault := exactscalar.Compile(exactscalar.Input{
		Occurrences:        compiler.publication.Occurrences,
		OccurrencePoints:   compiler.publication.OccurrencePoints,
		OccurrenceInputs:   compiler.publication.OccurrenceInputs,
		FunctionBoundaries: compiler.bodyBoundary.FunctionBoundaries(),
		FunctionFormals:    compiler.bodyBoundary.FunctionFormals(),
		FunctionVarargs:    compiler.bodyBoundary.FunctionVarargs(),
		FunctionCaptures:   compiler.bodyBoundary.FunctionCaptures(),
	})
	if fault.Available() {
		return CompileFailure{construction: fault}
	}
	compiler.exactScalar = bundle
	return CompileFailure{}
}

// deriveOperationPredicateRefinementsFailure compiles the neutral guarded
// operation-predicate geometry from existing Call, BinaryEquality, and
// environmentEdgeDraft rows. It authenticates only the parent-issued operation and
// route identities; the mounted target behavior and predicate meaning remain
// opaque to Program and are interpreted by the consuming Value domain.

func (compiler *compiler) deriveOperationPredicateRefinementsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for binaryIndex, binary := range compiler.publication.Occurrences {
		if binary.Kind() != programschema.OccurrenceBinaryEquality {
			continue
		}
		left, right, op, equalityOK := programschema.OccurrenceBinaryEqualityOperands(binary, compiler.publication.OccurrenceInputs)
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
			id := artifactdigest.Digest("analysis/program-artifact/operation-predicate-refinement", artifactFormat(),
				artifactdigest.ContentID(callID), artifactdigest.ContentID(binary.ID()), artifactdigest.ContentID(selected.ID()), artifactdigest.ContentID(subject), artifactdigest.ContentID(operand), artifactdigest.Bool(truth))
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, armIndex, CompileReasonOccurrenceUnavailable)
			}
			code, codeOK := programschema.OccurrenceOperationPredicateCode(op, truth)
			inputs := []identity.ContentID{callID, subject, operand, routeID}
			appended := codeOK && compiler.appendOccurrence(programschema.OccurrenceOperationPredicateRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code)
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
	for binaryIndex, binary := range compiler.publication.Occurrences {
		if binary.Kind() != programschema.OccurrenceBinaryEquality {
			continue
		}
		left, right, op, equalityOK := programschema.OccurrenceBinaryEqualityOperands(binary, compiler.publication.OccurrenceInputs)
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
			id := artifactdigest.Digest("analysis/program-artifact/binary-presence-refinement", artifactFormat(),
				artifactdigest.ContentID(binary.ID()), artifactdigest.ContentID(selected.ID()), artifactdigest.ContentID(target), artifactdigest.Bool(present))
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
