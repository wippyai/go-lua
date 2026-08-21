package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func TestBoundedTailCallAssignmentUsesDistinctValueBeforeStorageCell(t *testing.T) {
	compiled, err := lower.Lower(lower.Source{Name: "artifact-call-result-assignment.lua", Text: []byte(`
local first, second
first, second = require("module")
return second
`)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := compiler{
		input: compiled, key: testCompileKey(t, compiled), occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
		pointIDsBySite: make(map[identity.ContentID][]identity.ContentID),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if failure := transaction.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %+v", failure)
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %+v", failure)
	}
	if failure := transaction.copyCallRowsFailure(); failure.Available() {
		t.Fatalf("copy call rows: %+v", failure)
	}
	assigns := compiled.Flow().Authored().Storage().Assigns()
	if assigns.Count() != 1 {
		t.Fatalf("assignment count = %d, want 1", assigns.Count())
	}
	assignment, assignmentOK := transaction.storageAssignmentAt(0)
	if !assignmentOK {
		t.Fatal("assignment was not admitted")
	}
	if len(assignment.transfers) != 2 {
		t.Fatalf("assignment transfer count = %d, want 2", len(assignment.transfers))
	}
	var tails []programschema.CallResultSlot
	for _, slot := range transaction.callResultSlots {
		if slot.SourceKind() == programschema.CallResultSlotSourceValuesTail && slot.ConsumerKind() == programschema.CallResultSlotConsumerCell {
			tails = append(tails, slot)
		}
	}
	if len(tails) != 2 {
		t.Fatalf("bounded assignment tail slot count = %d, want 2", len(tails))
	}
	seenValues := make(map[identity.ContentID]struct{}, len(tails))
	seenCells := make(map[identity.ContentID]struct{}, len(tails))
	for _, tail := range tails {
		valueID, valueOK := tail.ValueID()
		cellID := tail.ConsumerID()
		position, positionOK := tail.ConsumerPosition()
		if !tail.Available() || !valueOK || valueID == cellID || !positionOK || position > 1 {
			t.Fatalf("tail slot did not separate producer Value %x from consumer Cell %x", valueID[:4], cellID[:4])
		}
		if _, duplicate := seenValues[valueID]; duplicate {
			t.Fatalf("tail slot ValueID %x was reused", valueID[:4])
		}
		if _, duplicate := seenCells[cellID]; duplicate {
			t.Fatalf("tail slot CellID %x was reused", cellID[:4])
		}
		seenValues[valueID], seenCells[cellID] = struct{}{}, struct{}{}
		foundTransfer := false
		for _, transfer := range assignment.transfers {
			if transfer.value == valueID && transfer.cell == cellID && transfer.position == int(position) {
				foundTransfer = true
			}
		}
		if !foundTransfer {
			t.Fatalf("bounded tail slot %x published no explicit Value-to-Cell storage transfer", valueID[:4])
		}
	}

	// The storage write is the post-call transfer edge: its retained finish
	// site must be the assignment write's finish, not the assignment's entry.
	transfer := assignment.transfers[0]
	writeFinishTerm, finishOK := compiled.Flow().Ports().Finish(transfer.term)
	writeFinish, writeFinishOK := compiled.Flow().Causal().Sites().ForTerm(writeFinishTerm)
	transferFinishID := transfer.finish.ContextID()
	calls := compiled.Flow().Authored().Calls()
	callTerm, callOK := calls.At(0)
	_, _, callFinishTerm, callSpanOK := compiled.EvaluationSpan(callTerm)
	callFinish, callFinishOK := compiled.Flow().Causal().Sites().ForTerm(callFinishTerm)
	entryID := assignment.entry.ContextID()
	assignmentFinishID := assignment.finish.ContextID()
	callFinishID := callFinish.ContextID()
	if !finishOK || !writeFinishOK || !callOK || !callSpanOK || !callFinishOK ||
		transferFinishID != writeFinish.ContextID() || transferFinishID != assignmentFinishID ||
		transferFinishID == entryID || transferFinishID == callFinishID {
		t.Fatalf("assignment transfer finish = %x, want post-call write finish distinct from entry", transferFinishID[:4])
	}
}

func TestStorageValueAtRejectsDuplicateCanonicalTailSlots(t *testing.T) {
	transaction := compileStorageTailTransaction(t, `
local first, second
first, second = require("module")
return second
`)
	assigns := transaction.input.Flow().Authored().Storage().Assigns()
	assignment, assignmentOK := assigns.At(0)
	if !assignmentOK {
		t.Fatal("assignment was not authored")
	}
	_, values, valuesOK := assigns.Get(assignment)
	if !valuesOK {
		t.Fatal("assignment Values parent was not authored")
	}
	var resultIndex = -1
	for index, result := range transaction.callResults {
		if result.Form() == programschema.CallResultValues {
			resultIndex = index
			break
		}
	}
	if resultIndex < 0 {
		t.Fatal("bounded tail CallResult was not compiled")
	}
	result := transaction.callResults[resultIndex]
	offset, count, spanOK := result.SlotSpan()
	if !spanOK || count < 2 || uint64(offset)+uint64(count) > uint64(len(transaction.callResultSlots)) {
		t.Fatalf("bounded tail slot span = (%d,%d,%v), want at least two canonical slots", offset, count, spanOK)
	}
	// Reusing a canonical slot at another ordinal preserves the parent span
	// but creates two matches for Values position zero. Resolution must reject
	// that ambiguity rather than selecting an arbitrary producer value.
	transaction.callResultSlots[offset+1] = transaction.callResultSlots[offset]
	valueID, valueAvailable := transaction.storageValueAt(values, 0)
	if valueAvailable || valueID.Available() {
		t.Fatalf("duplicate canonical tail slot resolved to %x", valueID[:4])
	}
}

func TestStorageValueAtDoesNotTransferOpenTail(t *testing.T) {
	transaction := compileStorageTailTransaction(t, `
return require("module")
`)
	var openValues keyspace.Term
	for index := 0; index < transaction.input.Flow().Authored().Values().Count(); index++ {
		term, termOK := transaction.input.Flow().Authored().Values().At(index)
		if !termOK {
			continue
		}
		for _, result := range transaction.callResults {
			open, openOK := result.ResultsOpen()
			if !openOK || !open || result.ValuesID() != transaction.values[index].ID() {
				continue
			}
			_, count, spanOK := result.SlotSpan()
			if !spanOK || count != 0 {
				t.Fatalf("open tail slot span count = %d (%v), want zero", count, spanOK)
			}
			openValues = term
		}
	}
	if openValues == 0 {
		t.Fatal("open tail Values parent was not compiled")
	}
	valueID, valueAvailable := transaction.storageValueAt(openValues, 0)
	if valueAvailable || valueID.Available() {
		t.Fatalf("open tail produced storage value %x", valueID[:4])
	}
}

func compileStorageTailTransaction(t *testing.T, source string) *compiler {
	t.Helper()
	compiled, err := lower.Lower(lower.Source{Name: "storage-tail-law.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	transaction := &compiler{
		input: compiled, key: testCompileKey(t, compiled), occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry),
		pointIDsBySite: make(map[identity.ContentID][]identity.ContentID),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %+v", failure)
	}
	if failure := transaction.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %+v", failure)
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %+v", failure)
	}
	if failure := transaction.copyCallRowsFailure(); failure.Available() {
		t.Fatalf("copy call rows: %+v", failure)
	}
	return transaction
}
