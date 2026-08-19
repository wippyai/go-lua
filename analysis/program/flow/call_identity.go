package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// CallValuesID returns the canonical owner-neutral identity joining one
// authored Call occurrence to its authored Values row. Call relation,
// semantic Call path, and Values occurrence are all Flow-owned inputs.
func (view View) CallValuesID(call keyspace.Term) (identity.ContentID, bool) {
	valuesID, _, ok := view.callValuesIdentity(call)
	return valuesID, ok && valuesID.Available()
}

// CallArgumentID returns the canonical identity of one fixed actual argument.
// Open Values tails are represented by ValuesTailID and never by this child.
func (view View) CallArgumentID(call keyspace.Term, argument int) (identity.ContentID, bool) {
	if argument < 0 {
		return identity.ContentID{}, false
	}
	valuesID, actuals, valuesOK := view.callValuesIdentity(call)
	if !valuesOK {
		return identity.ContentID{}, false
	}
	memberID, memberOK := view.ValuesMemberID(actuals, argument)
	if !memberOK {
		return identity.ContentID{}, false
	}
	id := flowSemanticID("program/transformer/call-argument", func(writer *framing.Writer) bool {
		return writer.Bytes(valuesID[:]) == nil && writer.Bytes(memberID[:]) == nil
	})
	return id, id.Available()
}

func (view View) callValuesIdentity(call keyspace.Term) (identity.ContentID, keyspace.Term, bool) {
	actuals, rowOK := view.callValuesRow(call)
	if !rowOK {
		return identity.ContentID{}, 0, false
	}
	path, pathOK := view.SemanticTermPath(call)
	valuesID, valuesOK := view.ValuesOccurrenceID(actuals)
	semanticCallID := flowSemanticID("program/transformer/call-occurrence-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(path[:]) == nil
	})
	id := flowSemanticID("program/transformer/call-values", func(writer *framing.Writer) bool {
		return writer.Bytes(semanticCallID[:]) == nil && writer.Bytes(valuesID[:]) == nil
	})
	return id, actuals, pathOK && path.Available() && valuesOK && semanticCallID.Available() && id.Available()
}

func (view View) callValuesRow(call keyspace.Term) (keyspace.Term, bool) {
	if !view.available() || keyspace.TermFamily(call) != keyspace.FamilyCall || keyspace.TermOrdinal(call) == 0 {
		return 0, false
	}
	owner, callee, receiver, rowActuals, ok := view.Authored().Calls().Get(call)
	if !ok || keyspace.TermFamily(rowActuals) != keyspace.FamilyValues || keyspace.TermOrdinal(rowActuals) == 0 {
		return 0, false
	}
	return rowActuals, keyspace.TermFamily(owner) == keyspace.FamilyBody && keyspace.TermOrdinal(owner) != 0 &&
		keyspace.TermFamily(callee) != keyspace.FamilyInvalid && keyspace.TermOrdinal(callee) != 0 &&
		(receiver == 0 || (keyspace.TermFamily(receiver) != keyspace.FamilyInvalid && keyspace.TermOrdinal(receiver) != 0))
}
