package compiler

import "github.com/wippyai/go-lua/analysis/schema/program"

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/valuesource"
)

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
			points := compiler.input.Flow().LocalWTO().PointPathsForSite(source.finish)
			spanID, spanOK := source.spanID, source.spanID.Available()
			if points.Count() == 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourcePoints)
			}
			if !spanOK || !compiler.appendOccurrencePathsPayload(programschema.OccurrenceValueSource, source.id, source.body, causal.SitePointPaths{}, points, []identity.ContentID{spanID}, family.code, source.literalFamily, source.literal, source.literalOK) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			if family.code != 6 && !source.literalOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
		}
	}
	return CompileFailure{}
}

// copyFormalEntrySources publishes each callable formal at the exact entry
// points and storage coordinate already issued by Program. The row contains
// no abstract Value policy; its subscribing domain owns that interpretation.
func (compiler *compiler) copyFormalEntrySources() CompileFailure {
	for boundaryIndex, boundary := range compiler.bodyBoundary.FunctionBoundaries() {
		if !boundary.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, -1, CompileReasonOccurrenceValueSourceAppend)
		}
		var points []identity.ContentID
		for _, body := range compiler.bodyBoundary.Bodies() {
			if body.ID() != boundary.BodyID() {
				continue
			}
			offset, count, spanOK := body.EntrySpan()
			if !spanOK || uint64(offset)+uint64(count) > uint64(len(compiler.bodyBoundary.BodyEntries())) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, -1, CompileReasonOccurrenceValueSourcePoints)
			}
			for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
				entry := compiler.bodyBoundary.BodyEntries()[offset+pointIndex]
				point := entry.PointID()
				if !entry.Available() || entry.BodyID() != body.ID() || !point.Available() {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, int(pointIndex), CompileReasonOccurrenceValueSourcePoints)
				}
				points = append(points, point)
			}
			break
		}
		if len(points) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, boundaryIndex, -1, CompileReasonOccurrenceValueSourcePoints)
		}
		for formalIndex := 0; formalIndex < boundary.FormalCount(); formalIndex++ {
			formal, ok := compiler.bodyBoundary.FunctionFormalAt(boundary, formalIndex)
			if !ok || !compiler.appendOccurrence(
				programschema.OccurrenceFormalEntry,
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
		leftID, leftOK := valuesource.SubjectSpan(compiler.input, operation.Left)
		rightID, rightOK := valuesource.SubjectSpan(compiler.input, operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !flowkind.IsBinaryArithmetic(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		if !compiler.appendOccurrencePaths(programschema.OccurrenceBinaryArithmetic, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{leftID, rightID}, uint64(operation.Op)) {
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
		leftID, leftOK := valuesource.SubjectSpan(compiler.input, operation.Left)
		rightID, rightOK := valuesource.SubjectSpan(compiler.input, operation.Right)
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 {
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
		code, codeOK := programschema.OccurrenceBinaryEqualityCode(operation.Op, hasComparison, invert)
		if !codeOK || !compiler.appendOccurrencePaths(programschema.OccurrenceBinaryEquality, span.ContextID(), body.PathID(), entryPoints, finishPoints, inputs, code) {
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
		leftID, leftOK := valuesource.SubjectSpan(compiler.input, operation.Left)
		rightID, rightOK := valuesource.SubjectSpan(compiler.input, operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !flowkind.IsBinaryOrder(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		if !compiler.appendOccurrencePaths(programschema.OccurrenceBinaryOrder, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{leftID, rightID}, uint64(operation.Op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	// Concat is an intentional non-member of the binary primitive projection:
	// the operation has no arithmetic representation lattice to project. Its
	// occurrence is therefore issued from the executable candidate bucket, so
	// every `..` term still has one artifact owner for its operand pair and
	// its evaluation span. The bucket is already proof-filtered, so a row it
	// names that cannot be sealed is corruption rather than a dead candidate.
	concat := flowView.Candidates().Concat()
	binaries := flowView.Authored().Operators().Binaries()
	for index := 0; index < concat.Count(); index++ {
		term, termOK := concat.At(index)
		_, op, left, right, rowOK := binaries.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftID, leftOK := valuesource.SubjectSpan(compiler.input, left)
		rightID, rightOK := valuesource.SubjectSpan(compiler.input, right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !rowOK || op != flowkind.BinaryConcat || !executable.Contains(term) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		if !compiler.appendOccurrencePaths(programschema.OccurrenceBinaryConcat, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{leftID, rightID}, uint64(op)) {
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
		operandID, operandOK := valuesource.SubjectSpan(compiler.input, operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 ||
			!compiler.appendOccurrencePaths(programschema.OccurrenceUnary, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{operandID}, uint64(op)) {
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
		leftID, leftOK := valuesource.SubjectSpan(compiler.input, left)
		rightID, rightOK := valuesource.SubjectSpan(compiler.input, right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !compiler.appendOccurrencePaths(programschema.OccurrenceSelect, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{leftID, rightID}, uint64(op)) {
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
		operandID, operandOK := valuesource.SubjectSpan(compiler.input, operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !compiler.appendOccurrencePaths(programschema.OccurrenceValueClaim, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{operandID}, uint64(claimKind)) {
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		if !compiler.appendOccurrencePaths(programschema.OccurrenceReturnBoundary, span.ContextID(), body.PathID(), entryPoints, finishPoints, []identity.ContentID{valuesID}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}

// copyGlobalEntryAdmissions publishes each Host global cell that no code in
// this program writes at the entry of every body that reads it.
//
// A body entered by an unknown caller still knows what such a cell holds:
// nothing can have changed it, so its value is the same at every entry, and
// the row says exactly that. A cell this program does write is deliberately
// absent - its value at an arbitrary entry is the join over those writes,
// which this row cannot state, and admitting the unwritten value in its place
// would be one path's value standing in for all of them.
//
// The row spans bodies rather than naming one, because the fact it admits is
// the same for all of them; its identity is the cell, and its points are the
// entries of the bodies that read it, so a cell nothing reads costs nothing.
func (compiler *compiler) copyGlobalEntryAdmissions() CompileFailure {
	view := compiler.input.Flow()
	cells := view.Authored().Storage().Cells()
	programID := compiler.input.ContentID()
	readers, written := compiler.storageCellAccess()
	if len(readers) == 0 {
		return CompileFailure{}
	}
	entries, entriesOK := compiler.bodyEntryPoints()
	if !entriesOK {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceValueSourcePoints)
	}
	for index := 0; index < cells.Count(); index++ {
		term, termOK := cells.At(index)
		if !termOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
		}
		kind, _, _, cellOK := cells.Get(term)
		if !cellOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
		}
		if kind != authored.CellGlobal {
			continue
		}
		cellID, idOK := rowidentity.StorageCellID(programID, view, term)
		if !idOK || !cellID.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
		}
		bodies, mentioned := readers[cellID]
		if !mentioned {
			continue
		}
		if _, mutated := written[cellID]; mutated {
			continue
		}
		var points []identity.ContentID
		for _, body := range bodies {
			points = append(points, entries[body]...)
		}
		if len(points) == 0 {
			continue
		}
		if !compiler.appendOccurrence(
			programschema.OccurrenceGlobalEntry,
			cellID,
			identity.ContentID{},
			canonicalPoints(points),
			[]identity.ContentID{cellID},
			0,
		) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
		}
	}
	return CompileFailure{}
}

// bodyEntryPoints answers each callable body's own entry points, which is
// where a fact admitted for that body is stated.
func (compiler *compiler) bodyEntryPoints() (map[identity.ContentID][]identity.ContentID, bool) {
	entries := make(map[identity.ContentID][]identity.ContentID)
	all := compiler.bodyBoundary.BodyEntries()
	for _, body := range compiler.bodyBoundary.Bodies() {
		offset, count, spanOK := body.EntrySpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(all)) {
			return nil, false
		}
		for pointIndex := uint32(0); pointIndex < count; pointIndex++ {
			entry := all[offset+pointIndex]
			point := entry.PointID()
			if !entry.Available() || entry.BodyID() != body.ID() || !point.Available() {
				return nil, false
			}
			entries[body.ID()] = append(entries[body.ID()], point)
		}
	}
	return entries, true
}

// storageCellAccess answers which storage cells this program's own code reads
// and which it writes, from the rows that state those accesses rather than by
// rediscovering them from authored syntax.
//
// Both sets bound the admission: a cell nothing reads needs no entry fact, and
// a cell anything writes may not be given one.
func (compiler *compiler) storageCellAccess() (map[identity.ContentID][]identity.ContentID, map[identity.ContentID]struct{}) {
	readers := make(map[identity.ContentID][]identity.ContentID)
	written := make(map[identity.ContentID]struct{})
	seen := make(map[[2]identity.ContentID]struct{})
	for _, row := range compiler.publication.Occurrences {
		switch row.Kind() {
		case programschema.OccurrenceStorageRead:
			cell, cellOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 0)
			body, bodyOK := row.BodyID()
			if !cellOK || !bodyOK || !cell.Available() || !body.Available() {
				continue
			}
			key := [2]identity.ContentID{cell, body}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			readers[cell] = append(readers[cell], body)
		case programschema.OccurrenceStorageWrite, programschema.OccurrenceStorageBindTransfer:
			cell, cellOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 2)
			if cellOK && cell.Available() {
				written[cell] = struct{}{}
			}
		}
	}
	return readers, written
}
