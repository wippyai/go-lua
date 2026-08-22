package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(row.entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(row.finish)
		spanID := row.span.ContextID()
		// A one-input rule cannot select an Entry attachment from the
		// parent's deliberately multi-valued Site relation. Refuse such a
		// Flow until it publishes an explicit occurrence-to-point pairing;
		// never zip or cross-product attachments here.
		if entryPoints.Count() != 1 || finishPoints.Count() == 0 || !spanID.Available() ||
			!compiler.appendOccurrencePaths(programschema.OccurrenceStorageRead, row.id, row.body, entryPoints, finishPoints, []identity.ContentID{row.cell, spanID}, 0) ||
			!compiler.recordOccurrencePaths(programschema.OccurrenceStorageRead, row.id, entryPoints, finishPoints) {
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(bind.entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(bind.finish)
		bindInputs := make([]identity.ContentID, 1, 1+len(bind.cells))
		bindInputs[0] = values.ID()
		// The generic storage-bind occurrence owns the complete destination
		// Cell column. Pack consumes this canonical row directly; it must not
		// receive a second bind/Cell row plane.
		bindInputs = append(bindInputs, bind.cells...)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !valuesOK || !values.Available() || !compiler.appendOccurrencePaths(programschema.OccurrenceStorageBind, bind.id, bind.body, entryPoints, finishPoints, bindInputs, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		for _, transfer := range bind.transfers {
			transferEntryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(bind.entry)
			transferFinishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(bind.finish)
			// As with a read, this one-input transfer rule requires one
			// unambiguous Entry attachment.
			if transferEntryPoints.Count() != 1 || transferFinishPoints.Count() == 0 ||
				!compiler.appendOccurrencePaths(programschema.OccurrenceStorageBindTransfer, transfer.id, bind.body, causal.SitePointPaths{}, transferFinishPoints, []identity.ContentID{bind.id, transfer.value, transfer.cell}, uint64(transfer.position)) ||
				!compiler.recordOccurrencePaths(programschema.OccurrenceStorageBindTransfer, transfer.id, transferEntryPoints, transferFinishPoints) {
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
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(assignment.entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(assignment.finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !valuesOK || !values.Available() || !compiler.appendOccurrencePaths(programschema.OccurrenceStorageAssignment, assignment.id, assignment.body, entryPoints, finishPoints, []identity.ContentID{values.ID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageAssignment)
		}
		for _, write := range assignment.transfers {
			writeFinishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(write.finish)
			if writeFinishPoints.Count() == 0 || !compiler.appendOccurrencePaths(programschema.OccurrenceStorageWrite, write.id, assignment.body, causal.SitePointPaths{}, writeFinishPoints, []identity.ContentID{assignment.id, write.value, write.cell, write.predecessor, write.route}, uint64(write.position)) ||
				!compiler.recordOccurrencePredecessorPaths(programschema.OccurrenceStorageWrite, write.id, write.route, writeFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, write.position, CompileReasonOccurrenceStorageAssignment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyIndexAccess() CompileFailure {
	geometry := compiler.input.Flow().AccessGeometry()
	reads, writes := geometry.IndexAccesses().Reads(), geometry.IndexAccesses().Writes()
	compiler.publication.HeapIndexes = make([]heapindex.Index, 0, reads.Count()+writes.Count())
	for index := 0; index < reads.Count(); index++ {
		read, occurrenceOK := compiler.indexReadAt(index)
		lensKind, exactKey := uint8(0), uint64(0)
		keySpan := identity.ContentID{}
		if read.exact {
			lensKind, exactKey = heapindex.LensExact, uint64(read.exactKey)
		} else if read.dynamicKeySpan.Available() {
			lensKind, keySpan = heapindex.LensDynamic, read.dynamicKeySpan.ContextID()
		}
		heapRow, heapRowOK := heapindex.NewIndex(read.id, true, read.baseSpan.ContextID(), read.resultSpan.ContextID(), keySpan, lensKind, exactKey, identity.ContentID{}, identity.ContentID{}, -1)
		if !occurrenceOK || !heapRowOK {
			// Heap's source denominator is strict: an authored index candidate
			// without complete geometry refuses the compilation. Do not inherit
			// copyIndexAccess's former candidate-skipping behavior.
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.publication.HeapIndexes = append(compiler.publication.HeapIndexes, heapRow)
		entry, entryOK := read.resultSpan.Entry()
		finish, finishOK := read.resultSpan.Finish()
		if !entryOK || !finishOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		entryPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(entry)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(finish)
		if entryPoints.Count() == 0 || finishPoints.Count() == 0 || !compiler.appendOccurrencePaths(programschema.OccurrenceIndexRead, read.id, identity.ContentID{}, entryPoints, finishPoints, []identity.ContentID{read.baseID, read.lensID, read.resultID}, 0) ||
			!compiler.recordOccurrencePaths(programschema.OccurrenceIndexRead, read.id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	for index := 0; index < writes.Count(); index++ {
		write, occurrenceOK := compiler.indexWriteAt(index)
		valueRow, valueRowOK := compiler.valueRowForTerm(write.values)
		valueSpan, valueSpanOK := valueRow.RootSpanID()
		lensKind, exactKey := uint8(0), uint64(0)
		keySpan := identity.ContentID{}
		if write.exact {
			lensKind, exactKey = heapindex.LensExact, uint64(write.exactKey)
		} else if write.dynamicKeySpan.Available() {
			lensKind, keySpan = heapindex.LensDynamic, write.dynamicKeySpan.ContextID()
		}
		heapRow, heapRowOK := heapindex.NewIndex(write.id, false, write.baseSpan.ContextID(), identity.ContentID{}, keySpan, lensKind, exactKey, valueSpan, valueRow.ID(), write.position)
		if !occurrenceOK || !valueRowOK || !valueSpanOK || !heapRowOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.publication.HeapIndexes = append(compiler.publication.HeapIndexes, heapRow)
		finishPoints := compiler.input.Flow().LocalWTO().PointPathsForSite(write.finish)
		if finishPoints.Count() == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !compiler.appendOccurrencePaths(programschema.OccurrenceIndexWrite, write.id, identity.ContentID{}, causal.SitePointPaths{}, finishPoints, []identity.ContentID{write.baseID, write.lensID, write.valuesID, write.predecessorID, write.route}, 0) ||
			!compiler.recordOccurrencePredecessorPaths(programschema.OccurrenceIndexWrite, write.id, write.route, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	return CompileFailure{}
}
