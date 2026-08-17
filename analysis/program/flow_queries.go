package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Available reports whether all four immutable Program owners and their
// provenance fence are sealed. Program itself is the construction input;
// there is no second transport or proof object around it.
func (program *Program) Available() bool {
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil ||
		!program.id.Available() {
		return false
	}
	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.Cold().ContentID()
	moduleID := program.module.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return false
	}
	provenance := program.flow.View().Provenance()
	return provenance.Source == sourceID && provenance.Flow == flowID &&
		provenance.Static == staticID && provenance.Module == moduleID
}

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

// EvaluationFinish returns the first canonical causal Site reached by an
// authored Finish chain. Positionless value terms may occur inside that chain
// but never escape through this query.
func (program *Program) EvaluationFinish(term keyspace.Term) (flow.Site, bool) {
	return program.finishSite(term)
}

func (span Span) Entry() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.entry, true
}

// Authored is retained only for the construction path that still needs the
// authored coordinate while its artifact column is assembled.
func (span Span) Authored() (keyspace.Term, bool) {
	if !span.Available() {
		return 0, false
	}
	return span.authored, true
}

func (span Span) Finish() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.finish, true
}

// TailReturn returns the exact terminal Outcome owned by this Call span's
// already-sealed causal boundary. It never exposes the Flow Outcome term or
// scans authored Return rows; the boundary and Body outcome range are the
// sole proof chain.
func (span Span) TailReturn() (Outcome, bool) {
	if !span.Available() {
		return Outcome{}, false
	}
	boundary, boundaryOK := span.program.Flow().Causal().Boundaries().For(span.authored)
	if !boundaryOK || boundary.Call != span.authored || boundary.TailReturn == 0 {
		return Outcome{}, false
	}
	outcome, ok := span.program.Outcome(boundary.TailReturn)
	body, bodyOK := span.program.ContainingBody(span.authored)
	outcomeKind, kindOK := outcome.Kind()
	target, targetOK := outcome.Target()
	return outcome, ok && bodyOK && span.program.OwnsBody(body) && outcome.body.program == span.program && outcome.Available() &&
		outcome.BelongsTo(body) && kindOK && outcomeKind == kind.OutcomeReturn && targetOK && target == 0
}

// Equal follows the published Site replay policy: equivalent sealed Programs
// compare by their exact-quartet Site identities, not by a Program pointer.
func (span Span) Equal(other Span) bool {
	return span.Available() && other.Available() && span.authored == other.authored &&
		span.context == other.context && span.entry.Equal(other.entry) && span.finish.Equal(other.finish)
}
