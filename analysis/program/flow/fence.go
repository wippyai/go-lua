package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Authored is the authored-relation query surface. It publishes only the
// owner's content identity and authored relations.
type Authored struct{ view authored.View }

func (view Authored) ContentID() identity.ContentID   { return view.view.ContentID() }
func (view Authored) Values() authored.Values         { return view.view.Values() }
func (view Authored) Tables() authored.Tables         { return view.view.Tables() }
func (view Authored) Fields() authored.Fields         { return view.view.Fields() }
func (view Authored) Access() authored.Access         { return view.view.Access() }
func (view Authored) Storage() authored.Storage       { return view.view.Storage() }
func (view Authored) Functions() authored.Functions   { return view.view.Functions() }
func (view Authored) Calls() authored.Calls           { return view.view.Calls() }
func (view Authored) Operators() authored.Operators   { return view.view.Operators() }
func (view Authored) Control() authored.Control       { return view.view.Control() }
func (view Authored) Claims() authored.Claims         { return view.view.Claims() }
func (view Authored) TypeValues() authored.TypeValues { return view.view.TypeValues() }

// Outcome is one sealed Outcome row as published by the Flow assembly.
type Outcome struct {
	Body   keyspace.Term
	Kind   kind.OutcomeKind
	Target keyspace.Term
}

// Outcomes is the published Outcome query surface. It withholds the owner's
// Find capability, which reopens the (Body, Kind, Target) join that assembly
// already resolved into issued Outcome terms.
type Outcomes struct{ result *outcome.Result }

func (view Outcomes) Count() int { return view.result.Count() }

func (view Outcomes) At(index int) (keyspace.Term, bool) { return view.result.At(index) }

func (view Outcomes) Get(term keyspace.Term) (Outcome, bool) {
	body, outcomeKind, target, ok := view.result.Get(term)
	return Outcome{Body: body, Kind: outcomeKind, Target: target}, ok
}

func (view Outcomes) BodyRange(body keyspace.Term) (int, int, bool) {
	return view.result.BodyRange(body)
}

func (view Outcomes) BodyExit(body keyspace.Term, outcomeKind kind.OutcomeKind) (keyspace.Term, bool) {
	return view.result.BodyExit(body, outcomeKind)
}

func (view Outcomes) Propagation(term keyspace.Term) (keyspace.Term, bool) {
	return view.result.Propagation(term)
}

func (view Outcomes) ReturnExit(term keyspace.Term) (keyspace.Term, bool) {
	return view.result.ReturnExit(term)
}

func (view Outcomes) BreakExit(term keyspace.Term) (keyspace.Term, bool) {
	return view.result.BreakExit(term)
}

func (view Outcomes) GotoExit(term keyspace.Term) (keyspace.Term, bool) {
	return view.result.GotoExit(term)
}

// Ports is the published evaluation-port query surface. It withholds the
// sealed term denominator, which would let a consumer reconstruct a term
// directory Flow deliberately does not publish.
type Ports struct{ result *evaluation.Ports }

func (view Ports) Entry(term keyspace.Term) (keyspace.Term, bool) { return view.result.Entry(term) }

func (view Ports) Finish(term keyspace.Term) (keyspace.Term, bool) { return view.result.Finish(term) }

// termCount returns the canonical denominator bound carried by Flow's sealed
// evaluation ports. It remains private to Flow's owner-local transitive
// queries; callers cannot use it to reconstruct a term directory.
func (view Ports) termCount() uint32 { return view.result.TermCount() }
