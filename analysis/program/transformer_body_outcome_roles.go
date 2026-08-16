package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// OutcomeTargetKind is the closed semantic destination vocabulary of a Body
// Outcome. Only Break and Goto carry targets; all other Outcomes are
// targetless. The proof never exposes the underlying Loop or Label term.
type OutcomeTargetKind uint8

const (
	OutcomeTargetInvalid OutcomeTargetKind = iota
	OutcomeTargetLoop
	OutcomeTargetLabel
)

// OwnsBodyOutcome authenticates every ordered Body Outcome issued by this
// exact TransformerInput. Unlike OwnsOutcome, it does not require a Causal
// Site: non-terminal or unreachable Outcomes remain complete Program facts.
func (input TransformerInput) OwnsBodyOutcome(outcome Outcome) bool {
	if !input.Available() || outcome.body.input != input || !input.OwnsBody(outcome.body) || !outcome.Available() {
		return false
	}
	exit, ok := outcome.body.boundary.OutcomeAt(outcome.ordinal)
	return ok && exit.Kind == outcome.kind && exit.Target == outcome.target
}

// PathID returns Flow's already-sealed owner-neutral semantic Outcome path.
// The hidden Outcome coordinate is used only inside this exact proof join.
func (outcome Outcome) PathID() identity.ContentID {
	if !outcome.body.input.OwnsBodyOutcome(outcome) {
		return identity.ContentID{}
	}
	exit, ok := outcome.body.boundary.OutcomeAt(outcome.ordinal)
	if !ok {
		return identity.ContentID{}
	}
	path, ok := outcome.body.input.owner.Flow().SemanticTermPath(exit.Outcome)
	if !ok {
		return identity.ContentID{}
	}
	return path
}

// TargetPath returns the exact semantic Loop or Label target for Break or
// Goto. A valid targetless Outcome returns the zero values and false.
func (outcome Outcome) TargetPath() (identity.ContentID, OutcomeTargetKind, bool) {
	if !outcome.body.input.OwnsBodyOutcome(outcome) {
		return identity.ContentID{}, OutcomeTargetInvalid, false
	}
	var targetKind OutcomeTargetKind
	switch outcome.kind {
	case kind.OutcomeBreak:
		if keyspace.TermFamily(outcome.target) != keyspace.FamilyLoop {
			return identity.ContentID{}, OutcomeTargetInvalid, false
		}
		targetKind = OutcomeTargetLoop
	case kind.OutcomeGoto:
		if keyspace.TermFamily(outcome.target) != keyspace.FamilyLabel {
			return identity.ContentID{}, OutcomeTargetInvalid, false
		}
		targetKind = OutcomeTargetLabel
	default:
		return identity.ContentID{}, OutcomeTargetInvalid, false
	}
	path, ok := outcome.body.input.owner.Flow().SemanticTermPath(outcome.target)
	if !ok || !path.Available() {
		return identity.ContentID{}, OutcomeTargetInvalid, false
	}
	return path, targetKind, true
}

// Propagation returns the exact next lexical Outcome already sealed by Flow.
// A terminal Outcome has no successor. The returned proof is issued by the
// same TransformerInput and preserves the parent's kind/target relation.
func (outcome Outcome) Propagation() (Outcome, bool) {
	input := outcome.body.input
	if !input.OwnsBodyOutcome(outcome) {
		return Outcome{}, false
	}
	exit, ok := outcome.body.boundary.OutcomeAt(outcome.ordinal)
	if !ok {
		return Outcome{}, false
	}
	nextTerm, ok := input.owner.Flow().Outcomes().Propagation(exit.Outcome)
	if !ok {
		return Outcome{}, false
	}
	next, ok := input.Outcome(nextTerm)
	if !ok || !input.OwnsBodyOutcome(next) || next.kind != outcome.kind || next.target != outcome.target || next.PathID() == outcome.PathID() {
		return Outcome{}, false
	}
	return next, true
}

// ReturnValueOccurrenceAt joins one ordered executable Return alternative to
// the existing sealed Values catalog. It is available only on the exact
// Outcome proof issued by Body.Return; no Values coordinate escapes.
func (outcome Outcome) ReturnValueOccurrenceAt(index int) (ValuesOccurrence, bool) {
	input := outcome.body.input
	if !input.OwnsBodyOutcome(outcome) || !outcome.returned.Available() || index < 0 {
		return ValuesOccurrence{}, false
	}
	site, siteOK := outcome.returned.ValueAt(index)
	term, termOK := site.Term()
	if !siteOK || !termOK || !input.OwnsSite(site) {
		return ValuesOccurrence{}, false
	}
	values, ok := input.valuesForTerm(term)
	return values, ok && input.OwnsValuesOccurrence(values)
}
