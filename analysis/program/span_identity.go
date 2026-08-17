package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ContextID is the identity of one existing authored evaluation span. The
// authored coordinate stays private; equivalent replay has the same ID while
// OwnsSpan remains an exact hot-owner predicate.
func (span Span) ContextID() identity.ContentID {
	if !span.Available() {
		return identity.ContentID{}
	}
	return span.context
}

func spanContextID(span Span) identity.ContentID {
	if !span.availableGeometry() {
		return identity.ContentID{}
	}
	entryID, finishID := span.entry.ContextID(), span.finish.ContextID()
	return programRoleID("program/transformer/span", span.program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(span.authored))) == nil &&
			writer.Uint(uint64(keyspace.TermOrdinal(span.authored))) == nil &&
			writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
	})
}

func (program *Program) OwnsSpan(span Span) bool {
	if !program.Available() || span.program != program || !span.Available() {
		return false
	}
	entry, entryOK := span.Entry()
	finish, finishOK := span.Finish()
	return entryOK && finishOK && program.OwnsSite(entry) && program.OwnsSite(finish)
}
