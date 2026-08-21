package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
)

func (compiler *compiler) copyStorage() CompileFailure {
	reads := compiler.input.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		row, ok := compiler.storageReadAt(index)
		if !ok {
			// Flow preserves authored denominator positions while withholding
			// dead/non-executable occurrences. Such a position has no artifact
			// row.
			continue
		}
		entryPoints, finishPoints := compiler.pointIDs(row.entry), compiler.pointIDs(row.finish)
		spanID := row.span.ContextID()
		// A one-input rule cannot select an Entry attachment from the
		// parent's deliberately multi-valued Site relation. Refuse such a
		// Flow until it publishes an explicit occurrence-to-point pairing;
		// never zip or cross-product attachments here.
		if len(entryPoints) != 1 || !spanID.Available() ||
			!compiler.appendOccurrence(programschema.OccurrenceStorageRead, row.id, row.body, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{row.cell, spanID}, 0) ||
			!compiler.recordOccurrenceSpan(programschema.OccurrenceStorageRead, row.id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	binds := compiler.input.Flow().Authored().Storage().Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := compiler.storageBindAt(index)
		if !ok {
			continue
		}
		values, valuesOK := compiler.valueRowForTerm(bind.values)
		entryPoints, finishPoints := compiler.pointIDs(bind.entry), compiler.pointIDs(bind.finish)
		bindInputs := make([]identity.ContentID, 1, 1+len(bind.cells))
		bindInputs[0] = values.ID()
		// The generic storage-bind occurrence owns the complete destination
		// Cell column. Pack consumes this canonical row directly; it must not
		// receive a second bind/Cell row plane.
		bindInputs = append(bindInputs, bind.cells...)
		if !valuesOK || !values.Available() || !compiler.appendOccurrence(programschema.OccurrenceStorageBind, bind.id, bind.body, append(entryPoints, finishPoints...), bindInputs, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		for _, transfer := range bind.transfers {
			transferEntryPoints, transferFinishPoints := compiler.pointIDs(bind.entry), compiler.pointIDs(bind.finish)
			// As with a read, this one-input transfer rule requires one
			// unambiguous Entry attachment.
			if len(transferEntryPoints) != 1 ||
				!compiler.appendOccurrence(programschema.OccurrenceStorageBindTransfer, transfer.id, bind.body, transferFinishPoints, []identity.ContentID{bind.id, transfer.value, transfer.cell}, uint64(transfer.position)) ||
				!compiler.recordOccurrenceSpan(programschema.OccurrenceStorageBindTransfer, transfer.id, transferEntryPoints, transferFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, transfer.position, CompileReasonOccurrenceStorageBind)
			}
		}
	}
	assigns := compiler.input.Flow().Authored().Storage().Assigns()
	for index := 0; index < assigns.Count(); index++ {
		assignment, ok := compiler.storageAssignmentAt(index)
		if !ok {
			continue
		}
		values, valuesOK := compiler.valueRowForTerm(assignment.values)
		entryPoints, finishPoints := compiler.pointIDs(assignment.entry), compiler.pointIDs(assignment.finish)
		if !valuesOK || !values.Available() || !compiler.appendOccurrence(programschema.OccurrenceStorageAssignment, assignment.id, assignment.body, append(entryPoints, finishPoints...), []identity.ContentID{values.ID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageAssignment)
		}
		for _, write := range assignment.transfers {
			writeFinishPoints := compiler.pointIDs(write.finish)
			if !compiler.appendOccurrence(programschema.OccurrenceStorageWrite, write.id, assignment.body, writeFinishPoints, []identity.ContentID{assignment.id, write.value, write.cell, write.predecessor, write.route}, uint64(write.position)) ||
				!compiler.recordOccurrencePredecessor(programschema.OccurrenceStorageWrite, write.id, write.route, writeFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, write.position, CompileReasonOccurrenceStorageAssignment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyIndexAccess() CompileFailure {
	geometry := compiler.input.Flow().AccessGeometry()
	reads, writes := geometry.IndexAccesses().Reads(), geometry.IndexAccesses().Writes()
	for index := 0; index < reads.Count(); index++ {
		read, ok := compiler.indexReadAt(index)
		if !ok {
			// AccessGeometry preserves candidate ordinals whose executable
			// Span proof can be absent. Only a complete artifact row is
			// executable.
			continue
		}
		entry, entryOK := read.resultSpan.Entry()
		finish, finishOK := read.resultSpan.Finish()
		if !entryOK || !finishOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.appendOccurrence(programschema.OccurrenceIndexRead, read.id, identity.ContentID{}, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{read.baseID, read.lensID, read.resultID}, 0) ||
			!compiler.recordOccurrenceSpan(programschema.OccurrenceIndexRead, read.id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	for index := 0; index < writes.Count(); index++ {
		write, ok := compiler.indexWriteAt(index)
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexCandidate)
		}
		finishPoints := compiler.pointIDs(write.finish)
		if len(finishPoints) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !compiler.appendOccurrence(programschema.OccurrenceIndexWrite, write.id, identity.ContentID{}, finishPoints, []identity.ContentID{write.baseID, write.lensID, write.valuesID, write.predecessorID, write.route}, 0) ||
			!compiler.recordOccurrencePredecessor(programschema.OccurrenceIndexWrite, write.id, write.route, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	return CompileFailure{}
}

// copyHeapGeometryFailure captures Heap's complete cold source denominator
// while the Program proof is live.  Link later substitutes these scalar rows
// through its own mounted Values/Keys authority; Heap never needs to reopen a
// Program after artifact compilation.
func (compiler *compiler) copyHeapGeometryFailure() CompileFailure {
	geometry := compiler.input.Flow().AccessGeometry()
	reads, writes := geometry.IndexAccesses().Reads(), geometry.IndexAccesses().Writes()
	compiler.heapIndexes = make([]heapindex.Index, 0, reads.Count()+writes.Count())
	for index := 0; index < reads.Count(); index++ {
		occurrence, occurrenceOK := compiler.indexReadAt(index)
		lensKind, exactKey := uint8(0), uint64(0)
		keySpan := identity.ContentID{}
		if occurrence.exact {
			lensKind, exactKey = heapindex.LensExact, uint64(occurrence.exactKey)
		} else if occurrence.dynamicKeySpan.Available() {
			lensKind, keySpan = heapindex.LensDynamic, occurrence.dynamicKeySpan.ContextID()
		}
		row, rowOK := heapindex.NewIndex(occurrence.id, true, occurrence.baseSpan.ContextID(), occurrence.resultSpan.ContextID(), keySpan, lensKind, exactKey, identity.ContentID{}, identity.ContentID{}, -1)
		if !occurrenceOK || !rowOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	for index := 0; index < writes.Count(); index++ {
		occurrence, occurrenceOK := compiler.indexWriteAt(index)
		valueRow, valueRowOK := compiler.valueRowForTerm(occurrence.values)
		valueSpan, valueSpanOK := valueRow.RootSpanID()
		lensKind, exactKey := uint8(0), uint64(0)
		keySpan := identity.ContentID{}
		if occurrence.exact {
			lensKind, exactKey = heapindex.LensExact, uint64(occurrence.exactKey)
		} else if occurrence.dynamicKeySpan.Available() {
			lensKind, keySpan = heapindex.LensDynamic, occurrence.dynamicKeySpan.ContextID()
		}
		row, rowOK := heapindex.NewIndex(occurrence.id, false, occurrence.baseSpan.ContextID(), identity.ContentID{}, keySpan, lensKind, exactKey, valueSpan, valueRow.ID(), occurrence.position)
		if !occurrenceOK || !valueRowOK || !valueSpanOK || !rowOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	return CompileFailure{}
}
