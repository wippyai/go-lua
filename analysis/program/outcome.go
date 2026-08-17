package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Outcome is one existing terminal Body Outcome joined to its Causal Site.
// Kind and Target are the typed Flow metadata; no raw Outcome term is exposed.
type Outcome struct {
	ordinal  int
	body     Body
	site     flow.Site
	kind     kind.OutcomeKind
	target   keyspace.Term
	returned flow.BodyReturn
}

func (body Body) OutcomeCount() int {
	if !body.Available() {
		return 0
	}
	return body.boundary.OutcomeCount()
}

// OutcomeAt returns one Body-bound typed Outcome proof. A Site is optional:
// non-terminal Break/Goto Outcomes remain meaningful Body facts even though
// Causal intentionally has no terminal Site for them.
func (body Body) OutcomeAt(index int) (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	exit, ok := body.boundary.OutcomeAt(index)
	if !ok {
		return Outcome{}, false
	}
	site, _ := body.program.Flow().Causal().Sites().ForTerm(exit.Outcome)
	outcome := Outcome{ordinal: index, body: body, site: site, kind: exit.Kind, target: exit.Target}
	return outcome, outcome.Available()
}

// Outcome resolves one existing Body Outcome through FunctionBoundary's sole
// dense inverse and the existing Causal Site table. It does not scan a Body's
// Outcome range or create another outcome index.
func (input *Program) Outcome(term keyspace.Term) (Outcome, bool) {
	if !input.Available() || term == 0 {
		return Outcome{}, false
	}
	boundary, boundaryOK := input.Flow().FunctionBoundaries().ForOutcome(term)
	bodyTerm, bodyTermOK := boundary.Body()
	body, bodyOK := input.Body(bodyTerm)
	exit, ordinal, exitOK := boundary.OutcomeForTerm(term)
	if !boundaryOK || !bodyTermOK || !bodyOK || !body.boundary.Equal(boundary) || !exitOK || exit.Outcome != term {
		return Outcome{}, false
	}
	site, _ := input.Flow().Causal().Sites().ForTerm(term)
	outcome := Outcome{ordinal: ordinal, body: body, site: site, kind: exit.Kind, target: exit.Target}
	return outcome, outcome.Available()
}

func (outcome Outcome) Available() bool {
	if !outcome.body.Available() {
		return false
	}
	if outcome.ordinal < 0 || outcome.ordinal >= outcome.body.boundary.OutcomeCount() {
		return false
	}
	exit, ok := outcome.body.boundary.OutcomeAt(outcome.ordinal)
	if !ok || exit.Kind != outcome.kind || exit.Target != outcome.target {
		return false
	}
	issuedSite, issuedOK := outcome.body.program.Flow().Causal().Sites().ForTerm(exit.Outcome)
	if issuedOK != outcome.site.Available() {
		return false
	}
	if issuedOK && (!outcome.body.program.OwnsSite(outcome.site) || !outcome.site.Equal(issuedSite)) {
		return false
	}
	if outcome.returned.Available() {
		returnSite, returnOK := outcome.returned.Outcome()
		return returnOK && outcome.kind == kind.OutcomeReturn && outcome.target == 0 && outcome.site.Equal(returnSite)
	}
	return true
}

func (outcome Outcome) Site() (flow.Site, bool) {
	if !outcome.Available() || !outcome.site.Available() || !outcome.body.program.OwnsSite(outcome.site) {
		return flow.Site{}, false
	}
	return outcome.site, true
}

// ContextID is the exact Causal Site identity of this terminal Outcome.
func (outcome Outcome) ContextID() identity.ContentID {
	site, ok := outcome.Site()
	if !ok {
		return identity.ContentID{}
	}
	return site.ContextID()
}

func (outcome Outcome) Equal(other Outcome) bool {
	return outcome.Available() && other.Available() && outcome.ordinal == other.ordinal && outcome.body.Equal(other.body) &&
		outcome.kind == other.kind && outcome.target == other.target && outcome.ContextID() == other.ContextID()
}

// BelongsTo proves the existing Body/Outcome ownership join without exposing
// either raw Flow coordinate. Exact mount-local consumers must additionally
// use Program.OwnsBody and OwnsOutcome.
func (outcome Outcome) BelongsTo(body Body) bool {
	return outcome.Available() && body.Available() && outcome.body.Equal(body)
}

func (outcome Outcome) Kind() (kind.OutcomeKind, bool) {
	if !outcome.Available() {
		return 0, false
	}
	return outcome.kind, true
}

func (outcome Outcome) Target() (keyspace.Term, bool) {
	if !outcome.Available() {
		return 0, false
	}
	return outcome.target, true
}

// Return returns this Body's sole targetless executable OutcomeReturn proof.
// The sealed Flow projection has already validated ReturnExit, Propagation,
// activation, and executable Values ownership; this query only exposes it.
func (body Body) Return() (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	returned, returnedOK := body.program.Flow().BodyReturns().ForBody(body.boundary)
	returnSite, siteOK := returned.Outcome()
	returnTerm, termOK := returnSite.Term()
	if !returnedOK || !siteOK || !termOK {
		return Outcome{}, false
	}
	outcome, outcomeOK := body.program.Outcome(returnTerm)
	if !outcomeOK || !outcome.BelongsTo(body) {
		return Outcome{}, false
	}
	outcome.returned = returned
	return outcome, outcome.Available()
}

// Normal returns this Body's canonical OutcomeNormal proof through Flow's
// existing Body-exit projection and FunctionBoundary's direct Outcome inverse.
// The authored coordinates remain inside the Program-owned query.
func (body Body) Normal() (Outcome, bool) {
	if !body.Available() {
		return Outcome{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	normalTerm, normalOK := body.program.Flow().Outcomes().BodyExit(bodyTerm, kind.OutcomeNormal)
	if !bodyOK || !normalOK {
		return Outcome{}, false
	}
	outcome, outcomeOK := body.program.Outcome(normalTerm)
	outcomeKind, kindOK := outcome.Kind()
	target, targetOK := outcome.Target()
	return outcome, outcomeOK && outcome.BelongsTo(body) && kindOK && outcomeKind == kind.OutcomeNormal && targetOK && target == 0
}

// ReturnValuesCount exposes the owner-issued ordered executable Values range
// only for an Outcome returned by Body.Return.
func (outcome Outcome) ReturnValuesCount() int {
	if !outcome.Available() || !outcome.returned.Available() {
		return 0
	}
	return outcome.returned.ValuesCount()
}

// ReturnValueAt returns an existing Span for one ordered executable Values
// alternative. The raw Values coordinate remains internal to this query.
func (outcome Outcome) ReturnValueAt(index int) (Span, bool) {
	if !outcome.Available() || !outcome.returned.Available() {
		return Span{}, false
	}
	site, siteOK := outcome.returned.ValueAt(index)
	term, termOK := site.Term()
	if !siteOK || !termOK {
		return Span{}, false
	}
	span, spanOK := outcome.body.program.Span(term)
	return span, spanOK
}

// OutcomeSiteAt is the compact Site-only OutcomeAt form.
func (body Body) OutcomeSiteAt(index int) (flow.Site, bool) {
	outcome, ok := body.OutcomeAt(index)
	if !ok {
		return flow.Site{}, false
	}
	return outcome.Site()
}
