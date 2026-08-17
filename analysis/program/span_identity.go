package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// EvaluationSpan returns the exact existing Entry/Finish geometry for term.
// The returned spanID is the Span.ContextID equation; entry and finish are
// the existing endpoint Terms, not synthesized coordinates.
func (program *Program) EvaluationSpan(term keyspace.Term) (spanID identity.ContentID, entry, finish keyspace.Term, ok bool) {
	if !program.scalarIdentityAvailable() || term == 0 {
		return identity.ContentID{}, 0, 0, false
	}
	span, spanOK := program.Span(term)
	if !spanOK {
		return identity.ContentID{}, 0, 0, false
	}
	entrySite, entryOK := span.Entry()
	finishSite, finishOK := span.Finish()
	entry, entryTermOK := entrySite.Term()
	finish, finishTermOK := finishSite.Term()
	if !entryOK || !finishOK || !entryTermOK || !finishTermOK || entry == 0 || finish == 0 {
		return identity.ContentID{}, 0, 0, false
	}
	spanID = span.ContextID()
	return spanID, entry, finish, spanID.Available()
}

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
	return evaluationSpanID(span.program, span.authored, span.entry.ContextID(), span.finish.ContextID())
}

func evaluationSpanID(program *Program, authored keyspace.Term, entry, finish identity.ContentID) identity.ContentID {
	if program == nil || authored == 0 || !entry.Available() || !finish.Available() {
		return identity.ContentID{}
	}
	return programRoleID("program/transformer/span", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Uint(uint64(keyspace.TermFamily(authored))) == nil &&
			writer.Uint(uint64(keyspace.TermOrdinal(authored))) == nil &&
			writer.Bytes(entry[:]) == nil && writer.Bytes(finish[:]) == nil
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
