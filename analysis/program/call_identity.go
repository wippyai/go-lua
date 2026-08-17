package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// CallIDAt returns the canonical scalar identity of one authored Program
// call.  The query reads only the sealed Flow, Static, and Program-owned
// Values columns; it does not retain an operand or Values proof. In
// particular, the final call identity commits the Values
// occurrence/root ID, not the derived CallValues identity.
//
// Call rows remain in Flow's authored denominator.  A malformed row or an
// unavailable child join fails closed rather than creating a second local
// call table.
func (program *Program) CallIDAt(index int) (identity.ContentID, bool) {
	if program == nil || index < 0 {
		return identity.ContentID{}, false
	}
	flowView := program.Flow()
	calls := flowView.Authored().Calls()
	call, callOK := calls.At(index)
	owner, callee, receiver, actuals, rowOK := calls.Get(call)
	if !callOK || !rowOK || !callIdentityValidCall(call) || !validCallTermOwner(owner) || !callIdentityValidOperand(callee) ||
		(receiver != 0 && !callIdentityValidOperand(receiver)) || keyspace.TermFamily(actuals) != keyspace.FamilyValues || keyspace.TermOrdinal(actuals) == 0 {
		return identity.ContentID{}, false
	}

	bodyBoundary, bodyOK := flowView.FunctionBoundaries().ForBody(owner)
	if !bodyOK || !bodyBoundary.Available() {
		return identity.ContentID{}, false
	}
	bodyID := bodyBoundary.ContextID()
	if !bodyID.Available() {
		return identity.ContentID{}, false
	}
	spanID, spanOK := program.callSpanID(call)
	if !spanOK {
		return identity.ContentID{}, false
	}
	calleeID, calleeOK := program.callOperandID(call, callee, callOperandRoleCallee)
	actualsID, actualsOK := program.callOperandID(call, actuals, callOperandRoleActuals)
	receiverID := identity.ContentID{}
	receiverOK := true
	form := flow.CallFormPlain
	if receiver != 0 {
		form = flow.CallFormMethod
		receiverID, receiverOK = program.callOperandID(call, receiver, callOperandRoleReceiver)
	}
	typesID, typesOK := program.callTypeArgumentsID(call)
	valuesID, valuesOK := program.ValuesOccurrenceID(actuals)
	if !calleeOK || !actualsOK || !receiverOK || !typesOK || !valuesOK {
		return identity.ContentID{}, false
	}

	callID := programRoleID("program/transformer/call-occurrence", program.ContentID(), func(writer *framing.Writer) bool {
		return callIdentityWriteTerm(writer, call) && writer.Uint(uint64(form)) == nil && writer.Bytes(bodyID[:]) == nil &&
			writer.Bytes(spanID[:]) == nil && writer.Bytes(calleeID[:]) == nil && writer.Bytes(receiverID[:]) == nil &&
			writer.Bytes(actualsID[:]) == nil && writer.Bytes(typesID[:]) == nil && writer.Bytes(valuesID[:]) == nil
	})
	return callID, callID.Available()
}

// CallCalleeIDAt returns the canonical scalar identity of the callee operand.
func (program *Program) CallCalleeIDAt(index int) (identity.ContentID, bool) {
	call, _, callee, _, _, ok := program.callIdentityRow(index)
	if !ok {
		return identity.ContentID{}, false
	}
	return program.callOperandID(call, callee, callOperandRoleCallee)
}

// CallReceiverIDAt returns the canonical scalar identity of the optional
// method receiver. Plain calls deliberately return false.
func (program *Program) CallReceiverIDAt(index int) (identity.ContentID, bool) {
	call, _, _, receiver, _, ok := program.callIdentityRow(index)
	if !ok || receiver == 0 {
		return identity.ContentID{}, false
	}
	return program.callOperandID(call, receiver, callOperandRoleReceiver)
}

// CallActualsIDAt returns the canonical scalar identity of the actual-values
// operand, distinct from both the Values row and CallValues identities.
func (program *Program) CallActualsIDAt(index int) (identity.ContentID, bool) {
	call, _, _, _, actuals, ok := program.callIdentityRow(index)
	if !ok {
		return identity.ContentID{}, false
	}
	return program.callOperandID(call, actuals, callOperandRoleActuals)
}

// CallValuesIDAt returns the owner-neutral CallValues identity joining the
// authored call occurrence to its canonical Values row.
func (program *Program) CallValuesIDAt(index int) (identity.ContentID, bool) {
	call, _, _, _, actuals, ok := program.callIdentityRow(index)
	if !ok {
		return identity.ContentID{}, false
	}
	path, pathOK := program.Flow().SemanticTermPath(call)
	valuesID, valuesOK := program.ValuesOccurrenceID(actuals)
	semanticCallID := programSemanticID("program/transformer/call-occurrence-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(path[:]) == nil
	})
	id := programSemanticID("program/transformer/call-values", func(writer *framing.Writer) bool {
		return writer.Bytes(semanticCallID[:]) == nil && writer.Bytes(valuesID[:]) == nil
	})
	return id, pathOK && path.Available() && valuesOK && semanticCallID.Available() && id.Available()
}

// CallArgumentIDAt returns the canonical identity of one fixed actual
// argument. Open tails are represented by ValuesTailID, never as an argument.
func (program *Program) CallArgumentIDAt(index, argument int) (identity.ContentID, bool) {
	_, _, _, _, actuals, ok := program.callIdentityRow(index)
	if !ok || argument < 0 {
		return identity.ContentID{}, false
	}
	valuesID, valuesOK := program.CallValuesIDAt(index)
	memberID, memberOK := program.ValuesMemberID(actuals, argument)
	id := programSemanticID("program/transformer/call-argument", func(writer *framing.Writer) bool {
		return writer.Bytes(valuesID[:]) == nil && writer.Bytes(memberID[:]) == nil
	})
	return id, valuesOK && memberOK && id.Available()
}

// CallTypeArgumentsIDAt returns the canonical scalar identity of one call's
// Static type-argument column. The authored term and Static view remain
// behind this query; only the detached column identity crosses the owner
// boundary.
func (program *Program) CallTypeArgumentsIDAt(index int) (identity.ContentID, bool) {
	call, _, _, _, _, ok := program.callIdentityRow(index)
	if !ok {
		return identity.ContentID{}, false
	}
	return program.callTypeArgumentsID(call)
}

// CallTypeArgumentIDAt returns the canonical scalar identity of one ordered
// Static type argument. The type-reference payload is issued separately by
// StaticTypeReferenceID; this identity remains the exact authored child role.
func (program *Program) CallTypeArgumentIDAt(index, argument int) (identity.ContentID, bool) {
	call, _, _, _, _, ok := program.callIdentityRow(index)
	if !ok || argument < 0 {
		return identity.ContentID{}, false
	}
	contracts := program.Static().Contracts().Calls()
	term, termOK := contracts.TypeArgumentAt(call, argument)
	if !termOK {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/call-type-argument", program.ContentID(), func(writer *framing.Writer) bool {
		return callIdentityWriteTerm(writer, call) && writer.Uint(uint64(argument)) == nil && callIdentityWriteTerm(writer, term)
	})
	return id, id.Available()
}

// CallFormalIDAt returns the canonical owner-neutral call formal identity.
// It is the scalar form of the former transient CallFormal proof: body path,
// call coordinate, call form, fixed actual width, tail disposition, and type
// argument count are all read from the sealed Program owners.
func (program *Program) CallFormalIDAt(index int) (identity.ContentID, bool) {
	call, owner, _, receiver, actuals, ok := program.callIdentityRow(index)
	if !ok {
		return identity.ContentID{}, false
	}
	bodyPath, bodyPathOK := program.Flow().BodyPath(owner)
	callPath, callPathOK := program.Flow().CallPath(call)
	_, width, _, tailKind, valuesOK := program.valuesRow(actuals)
	typeCount, typeCountOK := program.Static().Contracts().Calls().TypeArgumentCount(call)
	if !bodyPathOK || !bodyPath.Available() || !callPathOK || !callPath.Available() || !valuesOK || !typeCountOK || width < 0 ||
		typeCount < 0 || uint64(width) > uint64(^uint32(0)) || uint64(typeCount) > uint64(^uint32(0)) {
		return identity.ContentID{}, false
	}
	form := flow.CallFormPlain
	if receiver != 0 {
		form = flow.CallFormMethod
	}
	roles := uint64(2)
	if form == flow.CallFormMethod {
		roles = 3
	}
	open := tailKind != valuesTailInvalid
	id := programSemanticID("program/call-formal", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(callPath[:]) == nil && writer.Uint(1) == nil && writer.Uint(uint64(form)) == nil &&
			writer.Count(roles) == nil && writer.Uint(callOperandRoleCallee) == nil &&
			(form != flow.CallFormMethod || writer.Uint(callOperandRoleReceiver) == nil) && writer.Uint(callOperandRoleActuals) == nil &&
			writer.Count(uint64(width)) == nil && writer.Bool(open) == nil && writer.Count(uint64(typeCount)) == nil
	})
	return id, id.Available()
}

func (program *Program) callIdentityRow(index int) (call, owner, callee, receiver, actuals keyspace.Term, ok bool) {
	if program == nil || index < 0 {
		return 0, 0, 0, 0, 0, false
	}
	calls := program.Flow().Authored().Calls()
	call, present := calls.At(index)
	owner, callee, receiver, actuals, related := calls.Get(call)
	return call, owner, callee, receiver, actuals, present && related && callIdentityValidCall(call) && validCallTermOwner(owner) &&
		callIdentityValidOperand(callee) && (receiver == 0 || callIdentityValidOperand(receiver)) &&
		keyspace.TermFamily(actuals) == keyspace.FamilyValues && keyspace.TermOrdinal(actuals) != 0
}

func validCallTermOwner(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0
}

func callIdentityValidCall(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCall && keyspace.TermOrdinal(term) != 0
}

func callIdentityValidOperand(term keyspace.Term) bool {
	return keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func callIdentityWriteTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && callIdentityValidOperand(term) &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}

const (
	callOperandRoleInvalid uint64 = iota
	callOperandRoleCallee
	callOperandRoleReceiver
	callOperandRoleActuals
)

func (program *Program) callSpanID(term keyspace.Term) (identity.ContentID, bool) {
	if program == nil || !callIdentityValidCall(term) {
		return identity.ContentID{}, false
	}
	return program.occurrenceSpanID(term)
}

// occurrenceSpanID is the evaluation span identity of one authored occurrence.
// Span is the single owner of that geometry: it resolves the authored Finish
// continuation to its first causal Site and collapses a fused entry onto that
// Site. Reading the port planes here instead would be a second span authority
// and would reject every occurrence whose continuation passes through a term
// that holds no causal vertex.
func (program *Program) occurrenceSpanID(term keyspace.Term) (identity.ContentID, bool) {
	if program == nil || term == 0 || keyspace.TermFamily(term) == keyspace.FamilyInvalid || keyspace.TermOrdinal(term) == 0 {
		return identity.ContentID{}, false
	}
	span, spanOK := program.Span(term)
	if !spanOK {
		return identity.ContentID{}, false
	}
	spanID := span.ContextID()
	return spanID, spanID.Available()
}

func (program *Program) callOperandID(call, term keyspace.Term, kind uint64) (identity.ContentID, bool) {
	if program == nil || !callIdentityValidCall(call) || !callIdentityValidOperand(term) ||
		(kind != callOperandRoleCallee && kind != callOperandRoleReceiver && kind != callOperandRoleActuals) {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/call-operand", program.ContentID(), func(writer *framing.Writer) bool {
		return writer.Uint(kind) == nil && callIdentityWriteTerm(writer, call) && callIdentityWriteTerm(writer, term)
	})
	return id, id.Available()
}

func (program *Program) callTypeArgumentsID(call keyspace.Term) (identity.ContentID, bool) {
	if program == nil || !callIdentityValidCall(call) {
		return identity.ContentID{}, false
	}
	contracts := program.Static().Contracts().Calls()
	count, countOK := contracts.TypeArgumentCount(call)
	typeArgumentID, idOK := contracts.TypeArgumentID(call)
	if !countOK || !idOK || count < 0 {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/call-type-arguments", program.ContentID(), func(writer *framing.Writer) bool {
		return callIdentityWriteTerm(writer, call) && writer.Count(uint64(count)) == nil && writer.Bytes(typeArgumentID[:]) == nil
	})
	return id, id.Available()
}
