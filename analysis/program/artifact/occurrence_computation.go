package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (compiler *compiler) returnValueAt(outcome OutcomeRow, index int) (ReturnValue, bool) {
	position := uint64(outcome.returnStart) + uint64(index)
	if index < 0 || position >= uint64(len(compiler.returnValues)) {
		return ReturnValue{}, false
	}
	return compiler.returnValues[position], true
}

func (compiler *compiler) copyPointAttachments() CompileFailure {
	for index, row := range compiler.pointAttachments {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		id := digest("analysis/program-artifact/point-attachment", artifactFormat, bytesField(row.site), bytesField(row.point))
		if !compiler.appendOccurrence(OccurrencePointAttachment, id, identity.ContentID{}, []identity.ContentID{row.point}, []identity.ContentID{row.site}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyValueSources() CompileFailure {
	input, view := compiler.input, compiler.input.Flow()
	rows := []struct {
		count int
		code  uint64
	}{
		{input.Source().Literals().Nils().Count(), 1}, {input.Source().Literals().Bools().Count(), 2},
		{input.Source().Literals().Integers().Count(), 3}, {input.Source().Literals().Floats().Count(), 4},
		{input.Source().Literals().Strings().Count(), 5}, {view.Authored().TypeValues().Count(), 6},
	}
	for _, family := range rows {
		for index := 0; index < family.count; index++ {
			source, ok := compiler.valueSourceAt(family.code, index)
			if !ok && family.code == 6 {
				// TypeValue's authored denominator includes dead candidates;
				// only an executable proof becomes a ValueSource rule row.
				continue
			}
			if !ok {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceProof)
			}
			points := compiler.pointIDs(source.finish)
			spanID, spanOK := source.spanID, source.spanID.Available()
			if len(points) == 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourcePoints)
			}
			if !spanOK || !compiler.appendOccurrence(OccurrenceValueSource, source.id, source.body, points, []identity.ContentID{spanID}, family.code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			if family.code != 6 && !source.literalOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			row := &compiler.occurrences[len(compiler.occurrences)-1]
			row.literalFamily, row.literal, row.literalOK = source.literalFamily, source.literal, source.literalOK
		}
	}
	return CompileFailure{}
}

// copyFormalEntrySources publishes each callable formal at the exact entry
// points and storage coordinate already issued by Program. The row contains
// no abstract Value policy; its subscribing domain owns that interpretation.
func (compiler *compiler) copyFormalEntrySources() CompileFailure {
	for boundaryIndex, boundary := range compiler.functionBoundaries {
		if !boundary.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, -1, CompileReasonOccurrenceValueSourceAppend)
		}
		var points []identity.ContentID
		for _, body := range compiler.bodies {
			if body.ID() != boundary.BodyID() {
				continue
			}
			for pointIndex := 0; pointIndex < body.EntryPointCount(); pointIndex++ {
				point, pointOK := body.EntryPointAt(pointIndex)
				if !pointOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, pointIndex, CompileReasonOccurrenceValueSourcePoints)
				}
				points = append(points, point)
			}
			break
		}
		if len(points) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, -1, CompileReasonOccurrenceValueSourcePoints)
		}
		for formalIndex := 0; formalIndex < boundary.FormalCount(); formalIndex++ {
			formal, ok := boundary.FormalAt(formalIndex)
			if !ok || !compiler.appendOccurrence(
				OccurrenceFormalEntry,
				formal.StorageCellID(),
				boundary.BodyID(),
				points,
				[]identity.ContentID{formal.ID()},
				0,
			) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, formalIndex, CompileReasonOccurrenceValueSourceAppend)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyComputations() CompileFailure {
	flowView := compiler.input.Flow()
	executable := flowView.Executable()
	primitives := flowView.BinaryPrimitives()

	arithmetic := primitives.Arithmetic()
	for index := 0; index < arithmetic.Count(); index++ {
		term, termOK := arithmetic.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftID, leftOK := compiler.input.ValueSubjectID(operation.Left)
		rightID, rightOK := compiler.input.ValueSubjectID(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !flowkind.IsBinaryArithmetic(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryArithmetic, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !compiler.appendOccurrence(OccurrenceBinaryArithmetic, span.ContextID(), body.PathID(), points, []identity.ContentID{leftID, rightID}, uint64(operation.Op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	equality := primitives.Equality()
	for index := 0; index < equality.Count(); index++ {
		term, termOK := equality.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftID, leftOK := compiler.input.ValueSubjectID(operation.Left)
		rightID, rightOK := compiler.input.ValueSubjectID(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK ||
			(operation.Op != flowkind.BinaryEqual && operation.Op != flowkind.BinaryNotEqual) || !spanOK || !bodyOK ||
			!compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) || !leftOK || !rightOK || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			// BinaryEquality's dense primitive bucket is an executable
			// denominator. A missing row is corruption, never a dead authored hole.
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryEquality, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		inputs := []identity.ContentID{leftID, rightID}
		hasComparison, invert := false, operation.Op == flowkind.BinaryNotEqual
		if comparison, comparisonOK := primitive.Comparison(); comparisonOK && comparison.Left == operation.Left && comparison.Right == operation.Right && comparison.Invert == (operation.Op == flowkind.BinaryNotEqual) {
			branch, branchOK := flowView.SemanticTermPath(comparison.Branch)
			whenTrue, trueOK := compiler.input.ContainingBody(comparison.TrueBody)
			whenFalse, falseOK := compiler.input.ContainingBody(comparison.FalseBody)
			if branchOK && branch.Available() && trueOK && falseOK && compiler.input.OwnsBody(whenTrue) && compiler.input.OwnsBody(whenFalse) {
				inputs = append(inputs, branch, whenTrue.PathID(), whenFalse.PathID())
				hasComparison, invert = true, comparison.Invert
			}
		}
		code, codeOK := binaryEqualityCode(operation.Op, hasComparison, invert)
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !codeOK || !compiler.appendOccurrence(OccurrenceBinaryEquality, span.ContextID(), body.PathID(), points, inputs, code) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	order := primitives.Order()
	for index := 0; index < order.Count(); index++ {
		term, termOK := order.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftID, leftOK := compiler.input.ValueSubjectID(operation.Left)
		rightID, rightOK := compiler.input.ValueSubjectID(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !flowkind.IsBinaryOrder(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryOrder, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !compiler.appendOccurrence(OccurrenceBinaryOrder, span.ContextID(), body.PathID(), points, []identity.ContentID{leftID, rightID}, uint64(operation.Op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	unaries := flowView.Authored().Operators().Unaries()
	for index := 0; index < unaries.Count(); index++ {
		term, termOK := unaries.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, op, operand, rowOK := unaries.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		operandID, operandOK := compiler.input.ValueSubjectID(operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.recordOccurrenceSpan(OccurrenceUnary, span.ContextID(), entryPoints, finishPoints) ||
			!compiler.appendOccurrence(OccurrenceUnary, span.ContextID(), body.PathID(), points, []identity.ContentID{operandID}, uint64(op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	selects := flowView.Authored().Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, termOK := selects.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, op, left, right, rowOK := selects.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftID, leftOK := compiler.input.ValueSubjectID(left)
		rightID, rightOK := compiler.input.ValueSubjectID(right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceSelect, span.ContextID(), body.PathID(), points, []identity.ContentID{leftID, rightID}, uint64(op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	claims := flowView.Authored().Claims()
	for index := 0; index < claims.Count(); index++ {
		term, termOK := claims.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, operand, claimKind, rowOK := claims.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		operandID, operandOK := compiler.input.ValueSubjectID(operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceValueClaim, span.ContextID(), body.PathID(), points, []identity.ContentID{operandID}, uint64(claimKind)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	returns := flowView.Authored().Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		term, termOK := returns.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, valuesTerm, rowOK := returns.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		values, valuesOK := compiler.valueRowForTerm(valuesTerm)
		valuesID := values.ID()
		if keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || !rowOK || !spanOK || !bodyOK ||
			!compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) || !valuesOK || !values.Available() || !valuesID.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceReturnBoundary, span.ContextID(), body.PathID(), points, []identity.ContentID{valuesID}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}
