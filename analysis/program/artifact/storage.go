package artifact

// This file is the construction-only Storage column join.  It deliberately
// reads Flow.Authored and Source's bind widths directly; no Program storage
// occurrence/assignment row is retained by Artifact.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

type storageReadCompileRow struct {
	term, source  keyspace.Term
	body, context identity.ContentID
	span          program.Span
	entry, finish causal.Site
	id, cell      identity.ContentID
}

type storageBindCompileRow struct {
	term, values  keyspace.Term
	body, context identity.ContentID
	span          program.Span
	entry, finish causal.Site
	width         int
	id            identity.ContentID
	cells         []identity.ContentID
	transfers     []storageBindTransferCompileRow
}

type storageBindTransferCompileRow struct {
	position    int
	value, cell identity.ContentID
	id          identity.ContentID
}

type storageAssignmentCompileRow struct {
	term, values  keyspace.Term
	body, context identity.ContentID
	span          program.Span
	entry, finish causal.Site
	width         int
	id            identity.ContentID
	transfers     []storageWriteCompileRow
}

type storageWriteCompileRow struct {
	position    int
	term        keyspace.Term
	value, cell identity.ContentID
	finish      causal.Site
	predecessor identity.ContentID
	route       identity.ContentID
	id          identity.ContentID
	eligible    bool
}

func (compiler *compiler) storageReadAt(index int) (storageReadCompileRow, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return storageReadCompileRow{}, false
	}
	view := compiler.input.Flow()
	reads := view.Authored().Storage().Reads()
	term, ok := reads.At(index)
	owner, source, _, relationOK := reads.Get(term)
	if !ok || !relationOK || !view.Executable().Contains(term) {
		return storageReadCompileRow{}, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(source); !cellOK {
		return storageReadCompileRow{}, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	readPath, readPathOK := view.SemanticTermPath(term)
	span, spanOK := compiler.input.Span(term)
	entry, entryOK := span.Entry()
	finish, finishOK := span.Finish()
	body, bodyIdentityOK := compiler.input.Body(owner)
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		programID = compiler.input.ContentID()
	}
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !readPathOK || !readPath.Available() || !spanOK || !entryOK || !finishOK || !bodyIdentityOK || body.ContextID() != bodyID {
		return storageReadCompileRow{}, false
	}
	cellID, _ := artifactStorageCellID(programID, view, source)
	readID, readOK := programschema.StorageReadIdentity(programID, bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
	if !readOK {
		return storageReadCompileRow{}, false
	}
	row := storageReadCompileRow{term: term, source: source, body: bodyPath, context: body.ContextID(), span: span, entry: entry, finish: finish, id: readID, cell: cellID}
	return row, readID.Available() && cellID.Available() && bodyPath.Available()
}

func (compiler *compiler) storageBindAt(index int) (storageBindCompileRow, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return storageBindCompileRow{}, false
	}
	input, view := compiler.input, compiler.input.Flow()
	binds := view.Authored().Storage().Binds()
	term, ok := binds.At(index)
	owner, values, relationOK := binds.Get(term)
	width, widthOK := input.Source().Binds().Len(term)
	if !ok || !relationOK || !widthOK || width < 0 || !view.Executable().Contains(term) {
		return storageBindCompileRow{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return storageBindCompileRow{}, false
	}
	bodyPath, bodyOK := view.BodyPath(owner)
	span, spanOK := input.Span(term)
	entry, entryOK := span.Entry()
	finish, finishOK := span.Finish()
	body, bodyIdentityOK := input.Body(owner)
	row := storageBindCompileRow{term: term, values: values, body: bodyPath, context: body.ContextID(), span: span, entry: entry, finish: finish, width: width}
	if !bodyOK || !bodyPath.Available() || !spanOK || !entryOK || !finishOK || !bodyIdentityOK {
		return storageBindCompileRow{}, false
	}
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		programID = input.ContentID()
	}
	var rowIDOK bool
	row.id, rowIDOK = storageBindIdentityAt(input, programID, index)
	if !rowIDOK || !row.id.Available() {
		return storageBindCompileRow{}, false
	}
	row.cells = make([]identity.ContentID, width)
	for position := range row.cells {
		cellTerm, cellOK := input.Source().Binds().At(term, position)
		if !cellOK {
			return storageBindCompileRow{}, false
		}
		row.cells[position], _ = artifactStorageCellID(programID, view, cellTerm)
		if !row.cells[position].Available() {
			return storageBindCompileRow{}, false
		}
		_, fixed := view.Authored().Values().Member(values, position)
		if !fixed {
			continue
		}
		valueRow, valueRowOK := compiler.valueRowForTerm(values)
		member, memberOK := compiler.valueMemberAt(valueRow, position)
		if !valueRowOK || !memberOK {
			return storageBindCompileRow{}, false
		}
		transferID, transferOK := programschema.StorageBindTransferIdentity(programID, row.id, position)
		if !transferOK || !transferID.Available() {
			return storageBindCompileRow{}, false
		}
		row.transfers = append(row.transfers, storageBindTransferCompileRow{position: position, value: member.ID(), cell: row.cells[position], id: transferID})
	}
	return row, row.id.Available()
}

func (compiler *compiler) storageAssignmentAt(index int) (storageAssignmentCompileRow, bool) {
	if compiler == nil || !compiler.input.Available() || index < 0 {
		return storageAssignmentCompileRow{}, false
	}
	input, view := compiler.input, compiler.input.Flow()
	assigns := view.Authored().Storage().Assigns()
	term, ok := assigns.At(index)
	owner, values, relationOK := assigns.Get(term)
	width, widthOK := assigns.WriteCount(term)
	if !ok || !relationOK || !widthOK || width <= 0 || !view.Executable().Contains(term) {
		return storageAssignmentCompileRow{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return storageAssignmentCompileRow{}, false
	}
	bodyPath, bodyOK := view.BodyPath(owner)
	span, spanOK := input.Span(term)
	entry, entryOK := span.Entry()
	finish, finishOK := span.Finish()
	body, bodyIdentityOK := input.Body(owner)
	_, assignmentPathOK := view.StorageAssignmentPath(term)
	if !bodyOK || !bodyPath.Available() || !spanOK || !entryOK || !finishOK || !bodyIdentityOK || !assignmentPathOK {
		return storageAssignmentCompileRow{}, false
	}
	row := storageAssignmentCompileRow{term: term, values: values, body: bodyPath, context: body.ContextID(), span: span, entry: entry, finish: finish, width: width}
	rowID, rowIDOK := view.StorageAssignmentID(term)
	row.id = rowID
	if !rowIDOK || !row.id.Available() {
		return storageAssignmentCompileRow{}, false
	}
	programID := compiler.key.ProgramID()
	if !programID.Available() {
		programID = input.ContentID()
	}
	writes := view.Authored().Storage().Writes()
	exact, dynamic := view.Authored().Access().Exact(), view.Authored().Access().Dynamic()
	for position := 0; position < width; position++ {
		writeTerm, writeOK := assigns.WriteAt(term, position)
		actualAssignment, target, relationOK := writes.Get(writeTerm)
		_, fixed := view.Authored().Values().Member(values, position)
		if !writeOK || !relationOK || actualAssignment != term || !fixed {
			continue
		}
		cellKind, cellBody, cellKey, cellOK := view.Authored().Storage().Cells().Get(target)
		if !cellOK || (cellKind == authored.CellLocal && cellBody == 0 && cellKey == 0) {
			// Index writes share the authored assignment width but are owned by
			// the index-access column. Storage's transfer lane retains only
			// writes whose target is an existing storage Cell.
			continue
		}
		valueRow, valueRowOK := compiler.valueRowForTerm(values)
		member, memberOK := compiler.valueMemberAt(valueRow, position)
		if !valueRowOK || !memberOK {
			return storageAssignmentCompileRow{}, false
		}
		finishTerm, finishSpanOK := view.Ports().Finish(writeTerm)
		writeFinish, writeFinishOK := view.Causal().Sites().ForTerm(finishTerm)
		predecessorID, route, predecessorOK := compiler.assignmentPredecessorIdentity(programID, writeTerm)
		if !finishSpanOK || !writeFinishOK || !predecessorOK || !route.Available() {
			return storageAssignmentCompileRow{}, false
		}
		cellID, _ := artifactStorageCellID(programID, view, target)
		_, _, _, _, exactOK := exact.Get(target)
		_, _, _, dynamicOK := dynamic.Get(target)
		writeID, writeOK := programschema.StorageWriteTransferIdentity(programID, row.id, position, writeFinish.ContextID(), predecessorID)
		row.transfers = append(row.transfers, storageWriteCompileRow{position: position, term: writeTerm, value: member.ID(), cell: cellID, finish: writeFinish, predecessor: predecessorID, route: route, id: writeID, eligible: exactOK || dynamicOK})
		if !cellID.Available() || !predecessorID.Available() || !writeOK || !writeID.Available() {
			return storageAssignmentCompileRow{}, false
		}
	}
	return row, true
}

// storageBindIdentityAt is the Artifact construction admission for one
// authored Bind identity. It consumes canonical Flow/Source geometry and
// delegates the byte-level equation to schema/program; no Program row or
// identity method is retained.
func storageBindIdentityAt(input *program.Program, programID identity.ContentID, index int) (identity.ContentID, bool) {
	if input == nil || !input.Available() || !programID.Available() || index < 0 {
		return identity.ContentID{}, false
	}
	view := input.Flow()
	binds := view.Authored().Storage().Binds()
	term, present := binds.At(index)
	owner, values, related := binds.Get(term)
	width, widthOK := input.Source().Binds().Len(term)
	if !present || !related || !widthOK || width < 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false
	}
	return programschema.StorageBindIdentity(programID, bodyPath, width, bodyID, entry.ContextID(), finish.ContextID())
}

// assignmentPredecessorIdentity is shared by Storage's write construction and
// the Artifact index-write construction. Finish admission is deliberately
// independent of Entry admission for assignment writes.
func (compiler *compiler) assignmentPredecessorIdentity(programID identity.ContentID, write keyspace.Term) (identity.ContentID, identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !programID.Available() || write == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	view := compiler.input.Flow()
	finishTerm, finishOK := view.Ports().Finish(write)
	finish, finishSiteOK := view.Causal().Sites().ForTerm(finishTerm)
	successor, successorOK := view.Causal().Successors().AssignmentPredecessor(write)
	identityProof, identityOK := successor.Identity()
	route, routeOK := successor.SemanticID()
	if !finishOK || !finishSiteOK || !successorOK || !identityOK || !routeOK || !finish.Available() || !route.Available() ||
		successor.To != finishTerm || successor.Arm != causal.BoundaryLocal || identityProof.To != finishTerm ||
		identityProof.Arm != causal.BoundaryLocal || identityProof.Provenance() != view.Provenance() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	id, idOK := programschema.AssignmentPredecessorIdentity(programID, finish.ContextID(), route, identityProof.Digest)
	return id, route, idOK && id.Available()
}
