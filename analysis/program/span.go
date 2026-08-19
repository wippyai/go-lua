package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// Span is an opaque owner-fenced join of one authored occurrence to its
// existing Entry and Finish Sites. It is transient, not a generic Port or a
// retained projection.
type Span struct {
	program  *Program
	authored keyspace.Term
	entry    flow.Site
	finish   flow.Site
	context  identity.ContentID
}

func (program *Program) Span(term keyspace.Term) (Span, bool) {
	if !program.Available() {
		return Span{}, false
	}
	ports, sites := program.Flow().Ports(), program.Flow().Causal().Sites()
	entry, entryOK := ports.Entry(term)
	finishSite, finishOK := program.finishSite(term)
	if !entryOK || !finishOK {
		return Span{}, false
	}
	entrySite, entrySiteOK := sites.ForTerm(entry)
	if !entrySiteOK {
		// A contextual value can be fused into its consumer and therefore have
		// no standalone causal vertex. Its exact evaluation span collapses to
		// the canonical finish Site; this is runtime geometry, not a fabricated
		// source position.
		entrySite = finishSite
	}
	span := Span{program: program, authored: term, entry: entrySite, finish: finishSite}
	if !span.availableGeometry() {
		return Span{}, false
	}
	span.context = spanContextID(span)
	return span, span.Available()
}

// Available proves that this is still the exact published Program join.
// Equivalent artifact replay follows flow.Site.Equal semantics: matching
// sealed-quartet Sites remain valid; foreign/mutated handles fail closed.
func (span Span) Available() bool {
	return span.context.Available() && span.availableGeometry()
}

func (span Span) availableGeometry() bool {
	if !span.program.Available() || span.authored == 0 || !span.entry.Available() || !span.finish.Available() {
		return false
	}
	ports, sites := span.program.Flow().Ports(), span.program.Flow().Causal().Sites()
	entry, entryOK := ports.Entry(span.authored)
	wantFinish, finishOK := span.program.finishSite(span.authored)
	wantEntry, wantEntryOK := sites.ForTerm(entry)
	if !wantEntryOK {
		wantEntry, wantEntryOK = wantFinish, finishOK
	}
	return entryOK && finishOK && wantEntryOK &&
		span.entry.Equal(wantEntry) && span.finish.Equal(wantFinish)
}

// finishSite resolves the authored Finish chain to its first canonical causal
// Site. Contextual value evaluation may pass through positionless literal
// terms, which are valid intermediate ports but never causal vertices. The
// Program owner performs this normalization once; consumers never reconstruct
// geometry from the chain.
func (program *Program) finishSite(term keyspace.Term) (flow.Site, bool) {
	if !program.Available() || term == 0 {
		return flow.Site{}, false
	}
	ports, sites := program.Flow().Ports(), program.Flow().Causal().Sites()
	limit := uint64(program.Source().Identity().TermCount())
	for step := uint64(0); step <= limit; step++ {
		finish, ok := ports.Finish(term)
		if !ok || finish == 0 {
			return flow.Site{}, false
		}
		if site, siteOK := sites.ForTerm(finish); siteOK && site.Available() {
			return site, true
		}
		if finish == term {
			return flow.Site{}, false
		}
		term = finish
	}
	return flow.Site{}, false
}

// FinishSite returns the first canonical causal Site reached by an authored
// Finish chain. Positionless value terms may occur inside that chain but never
// escape through this query.
func (program *Program) FinishSite(term keyspace.Term) (flow.Site, bool) {
	return program.finishSite(term)
}

func (span Span) Entry() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.entry, true
}

func (span Span) Finish() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.finish, true
}

// Equal follows the published Site replay policy: equivalent sealed Programs
// compare by their exact-quartet Site identities, not by a Program pointer.
func (span Span) Equal(other Span) bool {
	return span.Available() && other.Available() && span.authored == other.authored &&
		span.context == other.context && span.entry.Equal(other.entry) && span.finish.Equal(other.finish)
}

// EvaluationSpan returns the exact existing Entry/Finish geometry for term.
// The returned spanID is the Span.ContextID equation; entry and finish are
// the existing endpoint Terms, not synthesized coordinates.
func (program *Program) EvaluationSpan(term keyspace.Term) (spanID identity.ContentID, entry, finish keyspace.Term, ok bool) {
	if !program.Available() || term == 0 {
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
