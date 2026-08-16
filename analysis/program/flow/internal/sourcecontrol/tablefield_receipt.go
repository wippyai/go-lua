package sourcecontrol

import (
	"errors"
	"math"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TableFieldThrowEligibility is an opaque causal parent proof. It is issued
// only after Causal has classified the FieldKey/invalid-FieldExact case and
// consumed exactly once by SourceControl's TableField route issuer.
type TableFieldThrowEligibility struct {
	state *tableFieldEligibilityState
	owner *Result
	field identity.ContentID
	body  identity.ContentID
	exit  identity.ContentID
}

type tableFieldEligibilityState struct {
	mu   sync.Mutex
	used bool
}

// IssueTableFieldThrowEligibility is the sole exact-key classifier. A nil
// receipt with nil error means an ordinary available FieldExact key; a live
// receipt proves precisely a FieldKey or unavailable FieldExact Throw arm.
func (r *Result) IssueTableFieldThrowEligibility(sourceView source.View, flow authored.View, outcomes *outcome.Result, field, owner keyspace.Term) (*TableFieldThrowEligibility, error) {
	if !r.matchesOperationInputs(sourceView, flow, outcomes) || keyspace.TermFamily(field) != keyspace.FamilyTableField || keyspace.TermFamily(owner) != keyspace.FamilyBody {
		return nil, errors.New("program/flow/sourcecontrol: TableField eligibility owner is unavailable")
	}
	table, key, _, fieldKind, fieldOK := flow.Fields().Get(field)
	tableOwner, ownerOK := flow.Tables().Get(table)
	if !fieldOK || !ownerOK || tableOwner != owner {
		return nil, errors.New("program/flow/sourcecontrol: TableField eligibility relation is unavailable")
	}
	switch fieldKind {
	case kind.FieldList, kind.FieldName:
		// List/name fields have no evaluated key and therefore no Throw arm.
		return nil, nil
	case kind.FieldExact:
		if tableFieldExactAvailable(sourceView, flow, key) {
			return nil, nil
		}
	case kind.FieldKey:
		// Every evaluated key carries the typed Throw arm below.
	default:
		return nil, errors.New("program/flow/sourcecontrol: TableField eligibility kind is unavailable")
	}
	exit, exitOK := outcomes.BodyExit(owner, kind.OutcomeThrow)
	if !exitOK {
		return nil, errors.New("program/flow/sourcecontrol: TableField Throw exit is unavailable")
	}
	return &TableFieldThrowEligibility{state: &tableFieldEligibilityState{}, owner: r, field: routeTermID(field), body: routeTermID(owner), exit: routeTermID(exit)}, nil
}

func (proof *TableFieldThrowEligibility) consume(graph *Result, field, exit, owner keyspace.Term) bool {
	if proof == nil || proof.state == nil {
		return false
	}
	proof.state.mu.Lock()
	defer proof.state.mu.Unlock()
	if proof.state.used {
		return false
	}
	proof.state.used = true
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
