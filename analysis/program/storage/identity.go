// Package storage owns the canonical identities of authored storage
// transformations and the live Program join needed to admit one StorageRead.
//
// The scalar codecs below are deliberately kept beside the join that proves
// their inputs.  Consumers carry only the issued identity; they do not retain
// a second storage row or a copy of the preimage.
package storage

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// StorageReadIdentity issues the canonical scalar identity of an authored
// storage Read. Flow/Source callers prove the row, cell, and evaluation
// geometry; this codec owns only the published framed equation.
func StorageReadIdentity(programID, bodyPath, bodyID, readPath, entryID, finishID identity.ContentID) (identity.ContentID, bool) {
	return storageRoleIdentity("program/transformer/storage-read", programID, func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(readPath[:]) == nil && writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	}, bodyPath, bodyID, readPath, entryID, finishID)
}

// StorageBindIdentity issues the canonical scalar identity of an authored
// storage Bind. The width is the Source-owned fixed destination count.
func StorageBindIdentity(programID, bodyPath identity.ContentID, width int, bodyID, entryID, finishID identity.ContentID) (identity.ContentID, bool) {
	if width < 0 {
		return identity.ContentID{}, false
	}
	return storageRoleIdentity("program/transformer/storage-bind", programID, func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Count(uint64(width)) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	}, bodyPath, bodyID, entryID, finishID)
}

// StorageBindTransferIdentity issues one fixed destination transfer identity.
func StorageBindTransferIdentity(programID, bindID identity.ContentID, position int) (identity.ContentID, bool) {
	if position < 0 {
		return identity.ContentID{}, false
	}
	return storageRoleIdentity("program/transformer/storage-bind-transfer", programID, func(writer *framing.Writer) bool {
		return writer.Bytes(bindID[:]) == nil && writer.Uint(uint64(position)) == nil
	}, bindID)
}

// AssignmentPredecessorIdentity issues the scalar identity of a sealed local
// reverse-commit predecessor. The caller proves the causal route and passes
// only the exact finish, route, and predecessor digest fields.
func AssignmentPredecessorIdentity(programID, finishID, route, digest identity.ContentID) (identity.ContentID, bool) {
	return storageRoleIdentity("program/transformer/assignment-predecessor", programID, func(writer *framing.Writer) bool {
		return writer.Bytes(finishID[:]) == nil && writer.Bytes(route[:]) == nil && writer.Bytes(digest[:]) == nil
	}, finishID, route, digest)
}

// StorageWriteTransferIdentity issues one fixed assignment write transfer.
// Admission of the write's Finish endpoint is intentionally external: valid
// authored writes may have no Entry port, so callers must not require a full
// evaluation span before invoking this equation.
func StorageWriteTransferIdentity(programID, assignmentID identity.ContentID, position int, finishID, predecessorID identity.ContentID) (identity.ContentID, bool) {
	if position < 0 {
		return identity.ContentID{}, false
	}
	return storageRoleIdentity("program/transformer/storage-write-transfer", programID, func(writer *framing.Writer) bool {
		return writer.Bytes(assignmentID[:]) == nil && writer.Uint(uint64(position)) == nil &&
			writer.Bytes(finishID[:]) == nil && writer.Bytes(predecessorID[:]) == nil
	}, assignmentID, finishID, predecessorID)
}

// ReadIdentityAt proves and issues the StorageRead at one authored ordinal.
// The Program's own ContentID is the sole authority for the root field; a
// caller cannot substitute an equivalent or foreign program identity.
func ReadIdentityAt(input *program.Program, index int) (identity.ContentID, keyspace.Term, bool) {
	if input == nil || !input.Available() || index < 0 {
		return identity.ContentID{}, 0, false
	}
	view := input.Flow()
	reads := view.Authored().Storage().Reads()
	term, present := reads.At(index)
	owner, source, _, related := reads.Get(term)
	if !present || !related || term == 0 || owner == 0 || source == 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, 0, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(source); !cellOK {
		return identity.ContentID{}, 0, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	readPath, readPathOK := view.SemanticTermPath(term)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !readPathOK || !readPath.Available() || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, 0, false
	}
	id, idOK := StorageReadIdentity(input.ContentID(), bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
	return id, term, idOK && id.Available()
}

func storageRoleIdentity(domain string, programID identity.ContentID, write func(*framing.Writer) bool, fields ...identity.ContentID) (identity.ContentID, bool) {
	if !programID.Available() || write == nil {
		return identity.ContentID{}, false
	}
	for _, field := range fields {
		if !field.Available() {
			return identity.ContentID{}, false
		}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(programID[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}
