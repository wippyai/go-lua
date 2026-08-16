package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

// CallOccurrence is the opaque Program proof for one existing authored call.
// Its denominator is Flow.Authored.Calls; Source contributes the containing
// Body and the exact occurrence Span, while Static contributes the authored
// type-argument template. No call table or operand index is retained here.
type CallOccurrence struct {
	input      TransformerInput
	index      int
	call       keyspace.Term
	span       Span
	body       Body
	form       flow.CallForm
	callee     CallOperand
	receiver   CallOperand
	actuals    CallOperand
	types      CallTypeArguments
	values     ValuesOccurrence
	semanticID identity.ContentID
	valuesID   identity.ContentID
	contextID  identity.ContentID
	validated  bool
}

// CallCount forwards Flow's sole authored call denominator, including rows
// that later fail a proof join rather than compacting them into a new table.
func (input TransformerInput) CallCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Calls().Count()
}

// CallAt issues one existing Flow call in canonical authored order.
func (input TransformerInput) CallAt(index int) (CallOccurrence, bool) {
	if !input.Available() || index < 0 || index >= input.CallCount() {
		return CallOccurrence{}, false
	}
	calls := input.owner.Flow().Authored().Calls()
	call, ok := calls.At(index)
	owner, callee, receiver, actuals, rowOK := calls.Get(call)
	if !ok || !rowOK {
		return CallOccurrence{}, false
	}
	body, bodyOK := input.Body(owner)
	span, spanOK := input.Span(call)
	calleeProof, calleeOK := newCallOperand(input, call, callee, CallOperandCallee)
	receiverProof, receiverOK := newCallReceiver(input, call, receiver)
	actualsProof, actualsOK := newCallOperand(input, call, actuals, CallOperandActuals)
	types, typesOK := newCallTypeArguments(input, call)
	values, valuesOK := input.valuesForTerm(actuals)
	callPath, callPathOK := input.owner.Flow().SemanticTermPath(call)
	callSemanticID := transformerSemanticID("program/transformer/call-occurrence-semantic", func(writer *framing.Writer) bool {
		return callPathOK && writer.Bytes(callPath[:]) == nil
	})
	form := flow.CallFormPlain
	if receiver != 0 {
		form = flow.CallFormMethod
	}
	result := CallOccurrence{
		input: input, index: index, call: call, span: span, body: body, form: form,
		callee: calleeProof, receiver: receiverProof, actuals: actualsProof, types: types, values: values, semanticID: callSemanticID,
	}
	result.valuesID = values.ID()
	if !ok || !bodyOK || !spanOK || !calleeOK || !receiverOK || !actualsOK || !typesOK || !valuesOK || !result.validPayload() {
		return CallOccurrence{}, false
	}
	result.validated = true
	result.contextID = result.buildContextID()
	if !result.contextID.Available() {
		return CallOccurrence{}, false
	}
	return result, true
}

func (call CallOccurrence) Available() bool {
	return call.validated && call.input.Available() && call.contextID.Available()
}

func (call CallOccurrence) validPayload() bool {
	if !call.input.Available() || call.index < 0 || !validCallTerm(call.call) || !validCallForm(call.form) || !exactCallSpan(call.input, call.span, call.call) ||
		!call.input.OwnsBody(call.body) || !call.input.OwnsCallOperand(call.callee) || !call.input.OwnsCallOperand(call.actuals) ||
		call.callee.kind != CallOperandCallee || call.actuals.kind != CallOperandActuals ||
		!call.input.OwnsCallTypeArguments(call.types) || call.types.input != call.input || call.types.call != call.call ||
		!call.input.OwnsValuesOccurrence(call.values) || !call.values.matchesTerm(call.actuals.term) ||
		!call.semanticID.Available() || call.valuesID != call.values.ID() {
		return false
	}
	contained, containedOK := call.input.ContainingBody(call.call)
	owner, callee, receiver, actuals, rowOK := call.input.owner.Flow().Authored().Calls().Get(call.call)
	issued, issuedOK := call.input.owner.Flow().Authored().Calls().At(call.index)
	bodyTerm, bodyOK := call.body.boundary.Body()
	if !containedOK || !contained.Equal(call.body) || !rowOK || !issuedOK || issued != call.call || !bodyOK || owner != bodyTerm {
		return false
	}
	if call.form == flow.CallFormPlain {
		if receiver != 0 || call.receiver.term != 0 || call.receiver.Available() {
			return false
		}
	} else if call.form == flow.CallFormMethod {
		if receiver == 0 || !call.input.OwnsCallOperand(call.receiver) || call.receiver.kind != CallOperandReceiver || call.receiver.call != call.call || call.receiver.term != receiver {
			return false
		}
	} else {
		return false
	}
	if call.callee.input != call.input || call.callee.call != call.call || call.callee.term != callee ||
		call.actuals.input != call.input || call.actuals.call != call.call || call.actuals.term != actuals {
		return false
	}
	// An executable call is admitted only when its closed causal boundary was
	// sealed. A source/authored call without that parent receipt is not a valid
	// Program operator occurrence.
	if call.input.owner.Flow().Executable().Contains(call.call) {
		boundary, boundaryOK := call.input.owner.Flow().Causal().Boundaries().For(call.call)
		callBoundary, callBoundaryOK := call.input.CallBoundary(call.span)
		if !boundaryOK || boundary.Call != call.call || !callBoundaryOK || !callBoundary.Available() {
			return false
		}
	}
	return true
}

func (call CallOccurrence) ContextID() identity.ContentID {
	if !call.Available() {
		return identity.ContentID{}
	}
	return call.contextID
}

func (call CallOccurrence) buildContextID() identity.ContentID {
	bodyID, spanID := call.body.ContextID(), call.span.ContextID()
	calleeID, receiverID, actualsID := call.callee.ContextID(), call.receiver.ContextID(), call.actuals.ContextID()
	typesID := call.types.ContextID()
	valuesID := call.values.ID()
	return transformerRoleID("program/transformer/call-occurrence", call.input.programID, func(writer *framing.Writer) bool {
		return writeTransformerTerm(writer, call.call) && writer.Uint(uint64(call.form)) == nil && writer.Bytes(bodyID[:]) == nil && writer.Bytes(spanID[:]) == nil &&
			writer.Bytes(calleeID[:]) == nil && writer.Bytes(receiverID[:]) == nil && writer.Bytes(actualsID[:]) == nil && writer.Bytes(typesID[:]) == nil && writer.Bytes(valuesID[:]) == nil
	})
}

func (call CallOccurrence) Equal(other CallOccurrence) bool {
	left, right := call.ContextID(), other.ContextID()
	return left.Available() && left == right
}

func (call CallOccurrence) Span() (Span, bool) {
	if !call.Available() {
		return Span{}, false
	}
	return call.span, true
}

func (call CallOccurrence) Body() (Body, bool) {
	if !call.Available() {
		return Body{}, false
	}
	return call.body, true
}

func (call CallOccurrence) Callee() (CallOperand, bool) {
	if !call.Available() {
		return CallOperand{}, false
	}
	return call.callee, true
}

func (call CallOccurrence) Form() (flow.CallForm, bool) {
	if !call.Available() {
		return 0, false
	}
	return call.form, true
}

func (call CallOccurrence) Receiver() (CallOperand, bool) {
	if !call.Available() || call.form != flow.CallFormMethod {
		return CallOperand{}, false
	}
	return call.receiver, true
}

func (call CallOccurrence) Actuals() (CallOperand, bool) {
	if !call.Available() {
		return CallOperand{}, false
	}
	return call.actuals, true
}

func (call CallOccurrence) TypeArguments() (CallTypeArguments, bool) {
	if !call.Available() {
		return CallTypeArguments{}, false
	}
	return call.types, true
}

// Executable reports Flow's sealed executable admission for this exact call.
// It does not infer executability from the presence of a source Span.
func (call CallOccurrence) Executable() bool {
	return call.Available() && call.input.owner.Flow().Executable().Contains(call.call)
}

// CallDisposition is the closed local Program disposition of an executable
// call. A call may be executable yet have no causal route when Flow sealed no
// boundary for it; that is distinct from an unavailable/non-executable row.
type CallDisposition uint8

const (
	CallDispositionInvalid CallDisposition = iota
	CallDispositionNotExecutable
	CallDispositionExecutableRouted
)

func (call CallOccurrence) Disposition() CallDisposition {
	if !call.Available() {
		return CallDispositionInvalid
	}
	if !call.input.owner.Flow().Executable().Contains(call.call) {
		return CallDispositionNotExecutable
	}
	return CallDispositionExecutableRouted
}

// Boundary returns the exact sealed causal call boundary. No route is
// fabricated when Flow did not publish one.
func (call CallOccurrence) Boundary() (CallBoundary, bool) {
	if !call.Available() {
		return CallBoundary{}, false
	}
	return call.input.CallBoundary(call.span)
}

// StaticFormal is the exact owner-neutral call formal descriptor used by
// later symbolic binding. It is an alias with an explicit contract-oriented
// name; the descriptor still contains no Static/Link/mount data.
func (call CallOccurrence) StaticFormal() (CallFormal, bool) { return call.Formal() }

func (input TransformerInput) OwnsCallOccurrence(call CallOccurrence) bool {
	return input.Available() && call.input == input && call.Available()
}

// CallOperandKind is the closed operand role carried by a CallOccurrence.
type CallOperandKind uint8

const (
	CallOperandInvalid CallOperandKind = iota
	CallOperandCallee
	CallOperandReceiver
	CallOperandActuals
)

// CallOperand is an opaque Flow-owned callee or actual-values occurrence.
// The underlying authored term is deliberately private.
type CallOperand struct {
	input TransformerInput
	call  keyspace.Term
	term  keyspace.Term
	kind  CallOperandKind
}

func (operand CallOperand) Available() bool {
	if !operand.input.Available() || !validCallTerm(operand.call) || !validCallOperandTerm(operand.term) {
		return false
	}
	_, callee, receiver, actuals, ok := operand.input.owner.Flow().Authored().Calls().Get(operand.call)
	if !ok {
		return false
	}
	switch operand.kind {
	case CallOperandCallee:
		return callee == operand.term
	case CallOperandReceiver:
		return receiver == operand.term
	case CallOperandActuals:
		return actuals == operand.term && keyspace.TermFamily(operand.term) == keyspace.FamilyValues
	default:
		return false
	}
}

func (operand CallOperand) Kind() CallOperandKind {
	if !operand.Available() {
		return CallOperandInvalid
	}
	return operand.kind
}

func (operand CallOperand) ContextID() identity.ContentID {
	if !operand.Available() {
		return identity.ContentID{}
	}
	return transformerRoleID("program/transformer/call-operand", operand.input.programID, func(writer *framing.Writer) bool {
		return writer.Uint(uint64(operand.kind)) == nil && writeTransformerTerm(writer, operand.call) && writeTransformerTerm(writer, operand.term)
	})
}

// Span returns the exact Program occurrence span carried by this call
// operand. The authored term remains private; Boundary uses the opaque span
// to bind the operand to its already-sealed Value row once.
func (operand CallOperand) Span() (Span, bool) {
	if !operand.Available() {
		return Span{}, false
	}
	span, ok := operand.input.Span(operand.term)
	return span, ok && operand.input.OwnsSpan(span)
}

func (input TransformerInput) OwnsCallOperand(operand CallOperand) bool {
	return input.Available() && operand.input == input && operand.Available()
}

// CallTypeArguments is Static's exact authored type-argument template for a
// call. It retains only the parent and count; arguments are re-queried at At.
type CallTypeArguments struct {
	input   TransformerInput
	call    keyspace.Term
	count   int
	receipt identity.ContentID
}

func (types CallTypeArguments) Available() bool {
	if !types.input.Available() || !validCallTerm(types.call) || types.count < 0 {
		return false
	}
	contracts := types.input.owner.Static().Contracts().Calls()
	count, ok := contracts.TypeArgumentCount(types.call)
	receipt, receiptOK := contracts.TypeArgumentReceipt(types.call)
	return ok && count == types.count && receiptOK && receipt == types.receipt
}

func (types CallTypeArguments) Count() int {
	if !types.Available() {
		return 0
	}
	return types.count
}

func (types CallTypeArguments) At(index int) (CallTypeArgument, bool) {
	if !types.Available() || index < 0 || index >= types.count {
		return CallTypeArgument{}, false
	}
	term, ok := types.input.owner.Static().Contracts().Calls().TypeArgumentAt(types.call, index)
	argument := CallTypeArgument{input: types.input, call: types.call, index: index, term: term}
	return argument, ok && argument.Available()
}

func (types CallTypeArguments) ContextID() identity.ContentID {
	if !types.Available() {
		return identity.ContentID{}
	}
	return transformerRoleID("program/transformer/call-type-arguments", types.input.programID, func(writer *framing.Writer) bool {
		return writeTransformerTerm(writer, types.call) && writer.Count(uint64(types.count)) == nil && writer.Bytes(types.receipt[:]) == nil
	})
}

func (input TransformerInput) OwnsCallTypeArguments(types CallTypeArguments) bool {
	return input.Available() && types.input == input && types.Available()
}

// CallTypeArgument is an opaque member of a Static type-argument template.
// Its authored type term is private and cannot be used as a raw join key.
type CallTypeArgument struct {
	input TransformerInput
	call  keyspace.Term
	index int
	term  keyspace.Term
}

func (argument CallTypeArgument) Available() bool {
	if !argument.input.Available() || !validCallTerm(argument.call) || argument.index < 0 || !validStaticTypeTerm(argument.term) {
		return false
	}
	term, ok := argument.input.owner.Static().Contracts().Calls().TypeArgumentAt(argument.call, argument.index)
	return ok && term == argument.term
}

func (argument CallTypeArgument) ContextID() identity.ContentID {
	if !argument.Available() {
		return identity.ContentID{}
	}
	return transformerRoleID("program/transformer/call-type-argument", argument.input.programID, func(writer *framing.Writer) bool {
		return writeTransformerTerm(writer, argument.call) && writer.Uint(uint64(argument.index)) == nil && writeTransformerTerm(writer, argument.term)
	})
}

func (input TransformerInput) OwnsCallTypeArgument(argument CallTypeArgument) bool {
	return input.Available() && argument.input == input && argument.Available()
}

func newCallOperand(input TransformerInput, call, term keyspace.Term, kind CallOperandKind) (CallOperand, bool) {
	operand := CallOperand{input: input, call: call, term: term, kind: kind}
	return operand, operand.Available()
}

func newCallReceiver(input TransformerInput, call, term keyspace.Term) (CallOperand, bool) {
	if term == 0 {
		return CallOperand{}, true
	}
	return newCallOperand(input, call, term, CallOperandReceiver)
}

func newCallTypeArguments(input TransformerInput, call keyspace.Term) (CallTypeArguments, bool) {
	contracts := input.owner.Static().Contracts().Calls()
	count, ok := contracts.TypeArgumentCount(call)
	receipt, receiptOK := contracts.TypeArgumentReceipt(call)
	types := CallTypeArguments{input: input, call: call, count: count, receipt: receipt}
	return types, ok && receiptOK && types.Available()
}

func exactCallSpan(input TransformerInput, span Span, call keyspace.Term) bool {
	if !input.Available() || !input.OwnsSpan(span) {
		return false
	}
	want, ok := input.Span(call)
	return ok && span.Equal(want)
}

func validCallTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCall && keyspace.TermOrdinal(term) != 0
}

func validCallForm(form flow.CallForm) bool {
	return form == flow.CallFormPlain || form == flow.CallFormMethod
}

func validCallOperandTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0
}

func validStaticTypeTerm(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	if keyspace.TermOrdinal(term) == 0 {
		return false
	}
	switch family {
	case keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral, keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion, keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray, keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord, keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf, keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional:
		return true
	default:
		return false
	}
}
