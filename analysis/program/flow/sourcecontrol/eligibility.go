package sourcecontrol

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TableFieldThrowEligibility is the immutable SourceControl row for a
// FieldKey or unavailable FieldExact throw arm. Causal owns the one-shot
// preparation transaction that admits the row to a route plan.
type TableFieldThrowEligibility struct {
	owner *Result
	field identity.ContentID
	body  identity.ContentID
	exit  identity.ContentID
}

func (proof TableFieldThrowEligibility) Available() bool {
	return proof.owner != nil && proof.owner.available() && proof.field.Available() && proof.body.Available() && proof.exit.Available()
}

// TableFieldThrowEligibility classifies the exact FieldKey/FieldExact arm. A
// zero row with nil error means an ordinary available FieldExact or a field
// with no evaluated key; a populated row names the required Throw exit.
func (r *Result) TableFieldThrowEligibility(sourceView source.View, flow authored.View, outcomes *outcome.Result, field, owner keyspace.Term) (TableFieldThrowEligibility, error) {
	if r == nil || !r.available() {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility graph is unavailable")
	}
	if sourceView.Identity().ContentID() != r.sourceID || !outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility source/outcome owner is unavailable")
	}
	if flow.ContentID() != r.flowID {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility flow owner is unavailable")
	}
	if keyspace.TermFamily(field) != keyspace.FamilyTableField || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility term owner is unavailable")
	}
	table, key, _, fieldKind, fieldOK := flow.Fields().Get(field)
	tableOwner, ownerOK := flow.Tables().Get(table)
	if !fieldOK || !ownerOK || tableOwner != owner {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility relation is unavailable")
	}
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		return TableFieldThrowEligibility{}, nil
	case kind.FieldExact:
		if tableFieldExactAvailable(sourceView, flow, key) {
			return TableFieldThrowEligibility{}, nil
		}
	case kind.FieldKey:
		// Every evaluated key carries the typed Throw arm below.
	default:
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField eligibility kind is unavailable")
	}
	exit, exitOK := outcomes.BodyExit(owner, kind.OutcomeThrow)
	if !exitOK {
		return TableFieldThrowEligibility{}, errors.New("program/flow/sourcecontrol: TableField Throw exit is unavailable")
	}
	return TableFieldThrowEligibility{owner: r, field: routeTermID(field), body: routeTermID(owner), exit: routeTermID(exit)}, nil
}

// ValidFor checks the exact SourceControl owner and route identities without
// introducing a mutable per-row lifecycle.
func (proof TableFieldThrowEligibility) ValidFor(graph *Result, field, exit, owner keyspace.Term) bool {
	return proof.owner != nil && graph == proof.owner && graph.available() && proof.field == routeTermID(field) && proof.body == routeTermID(owner) && proof.exit == routeTermID(exit)
}

func tableFieldExactAvailable(sourceView source.View, flow authored.View, term keyspace.Term) bool {
	literal, ok := tableFieldExactLiteral(sourceView, flow, term)
	if !ok {
		return false
	}
	_, ok = sourceView.Keys().Find(literal)
	return ok
}

func tableFieldExactLiteral(sourceView source.View, flow authored.View, term keyspace.Term) (keyspace.LiteralValue, bool) {
	negated := false
	for keyspace.TermFamily(term) == keyspace.FamilyUnary {
		_, op, operand, ok := flow.Operators().Unaries().Get(term)
		if !ok || op != kind.UnaryNeg {
			return keyspace.LiteralValue{}, false
		}
		negated = !negated
		term = operand
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.LiteralValue{}, false
	}
	var literal keyspace.LiteralValue
	var ok bool
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyBool:
		_, _, value, found := sourceView.Literals().Bools().At(int(ordinal - 1))
		literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}, found
	case keyspace.FamilyInteger:
		_, _, value, found := sourceView.Literals().Integers().At(int(ordinal - 1))
		literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}, found
	case keyspace.FamilyFloat:
		_, _, value, found := sourceView.Literals().Floats().At(int(ordinal - 1))
		literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: value}, found
	case keyspace.FamilyString:
		_, _, value, found := sourceView.Literals().Strings().At(int(ordinal - 1))
		literal, ok = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, found
	}
	if !ok || !negated {
		return literal, ok
	}
	switch literal.Kind {
	case keyspace.LiteralInteger:
		if literal.Integer == -1<<63 {
			return keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(-float64(literal.Integer))}, true
		}
		literal.Integer = -literal.Integer
		return literal, true
	case keyspace.LiteralFloat:
		literal.FloatBits = math.Float64bits(-math.Float64frombits(literal.FloatBits))
		return literal, true
	default:
		return keyspace.LiteralValue{}, false
	}
}
