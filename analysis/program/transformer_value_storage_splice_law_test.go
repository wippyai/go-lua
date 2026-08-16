package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// TestTransformerValueStorageRolesRejectEquivalentReplaySubproofSplices is an
// in-package hostile law because the role fields are deliberately opaque. A
// replay has equal semantic identities but different hot proof ownership; no
// composite role may accept one replay child under a local parent.
func TestTransformerValueStorageRolesRejectEquivalentReplaySubproofSplices(t *testing.T) {
	left := transformerStorageSpliceProgram(t, "transformer-storage-splice.lua")
	replay := transformerStorageSpliceProgram(t, "transformer-storage-splice.lua")
	input, replayInput := left.TransformerInput(), replay.TransformerInput()

	leftSource, leftSourceOK := input.NilSourceAt(0)
	replaySource, replaySourceOK := replayInput.NilSourceAt(0)
	if !leftSourceOK || !replaySourceOK || leftSource.ContextID() != replaySource.ContextID() {
		t.Fatal("fixture did not publish replay-equivalent Nil source roles")
	}
	splicedSource := leftSource
	splicedSource.anchor = replaySource.anchor
	if splicedSource.Available() || input.OwnsValueSourceOccurrence(splicedSource) {
		t.Fatal("ValueSourceOccurrence accepted an equivalent replay anchor")
	}
	splicedSource = leftSource
	splicedSource.body = replaySource.body
	if splicedSource.Available() || input.OwnsValueSourceOccurrence(splicedSource) {
		t.Fatal("ValueSourceOccurrence accepted an equivalent replay Body")
	}

	leftAssignment, leftAssignmentOK := input.StorageAssignmentAt(0)
	replayAssignment, replayAssignmentOK := replayInput.StorageAssignmentAt(0)
	leftWrite, leftWriteOK := leftAssignment.TransferAt(0)
	replayWrite, replayWriteOK := replayAssignment.TransferAt(0)
	leftPredecessor, leftPredecessorOK := leftWrite.Predecessor()
	replayPredecessor, replayPredecessorOK := replayWrite.Predecessor()
	if !leftAssignmentOK || !replayAssignmentOK || !leftWriteOK || !replayWriteOK || !leftPredecessorOK || !replayPredecessorOK ||
		leftWrite.ContextID() != replayWrite.ContextID() || leftPredecessor.ContextID() != replayPredecessor.ContextID() {
		t.Fatal("fixture did not publish replay-equivalent assignment Write roles")
	}

	splicedWrite := leftWrite
	splicedWrite.predecessor = replayPredecessor
	if splicedWrite.Available() || input.OwnsStorageWriteOccurrence(splicedWrite) {
		t.Fatal("StorageWriteOccurrence accepted an equivalent replay predecessor")
	}
	splicedWrite = leftWrite
	splicedWrite.finish = replayWrite.finish
	if splicedWrite.Available() || input.OwnsStorageWriteOccurrence(splicedWrite) {
		t.Fatal("StorageWriteOccurrence accepted an equivalent replay Finish site")
	}
	splicedAssignment := leftAssignment
	splicedAssignment.body = replayAssignment.body
	if splicedAssignment.Available() || input.OwnsStorageAssignment(splicedAssignment) {
		t.Fatal("StorageAssignment accepted an equivalent replay Body")
	}
}

func transformerStorageSpliceProgram(t *testing.T, name string) *Program {
	t.Helper()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	bindValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	assignValue := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyNil] = 2
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyBind] = 1
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyWrite] = 1

	sourceDraft, err := source.Build(source.Input{
		Name:     name,
		Families: rootFamilySpans(name, counts),
		Nil:      []source.NilLiteral{{Owner: body}, {Owner: body}},
		Bodies:   []source.BodySource{{Body: body, Terms: []keyspace.Term{bind, assign}}},
		Binds:    []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
	})
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	staticDraft, err := static.Build(static.Input{Counts: counts})
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalizer.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("imports.Finalizer: %v", err)
	}
	flowDraft, err := flow.Build(flow.Input{
		Counts: counts,
		Values: flow.ValuesInput{
			Rows: []flow.Value{
				{Owner: body, Fixed: flow.Range{End: 1}},
				{Owner: body, Fixed: flow.Range{Start: 1, End: 2}},
			},
			Terms: []keyspace.Term{bindValue, assignValue},
		},
		Storage: flow.StorageInput{
			Cells:   []flow.Cell{{Kind: flow.CellLocal, Body: body}},
			Binds:   []flow.Bind{{Owner: body, Values: bindValues}},
			Assigns: []flow.Assign{{Owner: body, Values: assignValues}},
			Writes:  []flow.Write{{Assign: assign, Target: cell}},
		},
	})
	if err != nil {
		_ = moduleFinalizer.Abort()
		_ = staticFinalizer.Abort()
		_ = sourceFinalizer.Abort()
		t.Fatalf("flow.Build: %v", err)
	}
	assembly, err := flow.Assemble(sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft, body)
	if err != nil {
		t.Fatalf("flow.Assemble: %v", err)
	}
	published, err := Publish(assembly)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published
}
