package cold

import "github.com/wippyai/go-lua/analysis/identity"

// The append-only slots these families occupy. Each is written out against
// the slot before it: an implicit repetition would give two families one slot
// and the publication would refuse to seal.
const (
	slotCall             = slotPointDecision + 1
	slotCallOperand      = slotCall + 1
	slotCallArgument     = slotCallOperand + 1
	slotCallTypeArgument = slotCallArgument + 1
)

// The four call families are the single cold declaration of authored-call
// rows. Child positions are dense ordinals named by Call's spans.
func CallFamily() Family[Call] {
	return Family[Call]{slot: slotCall, name: "call"}
}

func CallOperandFamily() Family[CallOperand] {
	return Family[CallOperand]{slot: slotCallOperand, name: "call-operand"}
}

func CallArgumentFamily() Family[CallArgument] {
	return Family[CallArgument]{slot: slotCallArgument, name: "call-argument"}
}

func CallTypeArgumentFamily() Family[CallTypeArgument] {
	return Family[CallTypeArgument]{slot: slotCallTypeArgument, name: "call-type-argument"}
}

// CallForm is the primitive authored-call shape retained by the cold
// publication. Its ordinals are part of the artifact identity contract.
type CallForm uint8

const (
	CallFormInvalid CallForm = iota
	CallFormPlain
	CallFormMethod
)

func (form CallForm) Valid() bool {
	return form == CallFormPlain || form == CallFormMethod
}

// CallOperandKind is the primitive role of one authored call operand. The
// value is deliberately neutral: Flow owns the authored term and Artifact
// owns only this immutable scalar projection.
type CallOperandKind uint8

const (
	CallOperandInvalid CallOperandKind = iota
	CallOperandCallee
	CallOperandReceiver
	CallOperandActuals
)

func (kind CallOperandKind) Valid() bool {
	return kind >= CallOperandCallee && kind <= CallOperandActuals
}

// CallOperand is one immutable, pointer-free call operand. Its position is
// the ordinal in CallOperandFamily; a call names its child range explicitly.
type CallOperand struct {
	id, call, value, span identity.ContentID
	kind                  CallOperandKind
}

// NewCallOperand copies one compiler construction row into the canonical cold
// vocabulary. No authored term or owner pointer crosses this boundary.
func NewCallOperand(id, call, value, span identity.ContentID, kind CallOperandKind) (CallOperand, bool) {
	row := CallOperand{id: id, call: call, value: value, span: span, kind: kind}
	return row, row.Available()
}

func (row CallOperand) Available() bool {
	return row.id.Available() && row.call.Available() && row.value.Available() && row.span.Available() && row.kind.Valid()
}

func (row CallOperand) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row CallOperand) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row CallOperand) ValueID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.value
}

func (row CallOperand) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}

func (row CallOperand) Kind() CallOperandKind {
	if !row.Available() {
		return CallOperandInvalid
	}
	return row.kind
}

// CallArgument is one immutable ordered actual argument. Its position is the
// ordinal in CallArgumentFamily; the parent Call names the contiguous span.
type CallArgument struct {
	id, call, values, member, span identity.ContentID
	position                       uint32
}

// NewCallArgument copies one compiler construction row into the cold
// publication vocabulary.
func NewCallArgument(id, call, values, member, span identity.ContentID, position uint32) (CallArgument, bool) {
	row := CallArgument{id: id, call: call, values: values, member: member, span: span, position: position}
	return row, row.Available()
}

func (row CallArgument) Available() bool {
	return row.id.Available() && row.call.Available() && row.values.Available() && row.member.Available() && row.span.Available()
}

func (row CallArgument) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row CallArgument) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row CallArgument) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}

func (row CallArgument) MemberID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.member
}

func (row CallArgument) ValueID() identity.ContentID { return row.MemberID() }

func (row CallArgument) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}

func (row CallArgument) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// CallTypeArgument is one immutable ordered static type argument. Its
// position is the ordinal in CallTypeArgumentFamily.
type CallTypeArgument struct {
	id, call, types, reference identity.ContentID
	position                   uint32
}

// NewCallTypeArgument copies one compiler construction row into the cold
// publication vocabulary.
func NewCallTypeArgument(id, call, types, reference identity.ContentID, position uint32) (CallTypeArgument, bool) {
	row := CallTypeArgument{id: id, call: call, types: types, reference: reference, position: position}
	return row, row.Available()
}

func (row CallTypeArgument) Available() bool {
	return row.id.Available() && row.call.Available() && row.types.Available() && row.reference.Available()
}

func (row CallTypeArgument) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row CallTypeArgument) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}

func (row CallTypeArgument) TypesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}

func (row CallTypeArgument) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}

func (row CallTypeArgument) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// Call is the complete immutable authored-call row. Its child ranges address
// the three dense call families by ordinal; no child slice or slice header is
// retained in the sealed publication.
type Call struct {
	id, body, span, formal, values, valuesRoot, types identity.ContentID
	callee, actuals, receiver, tail, target           identity.ContentID
	operandStart, operandEnd                          uint32
	argumentStart, argumentEnd                        uint32
	typeArgumentStart, typeArgumentEnd                uint32
	form                                              CallForm
	hasReceiver, hasTail                              bool
}

// NewCall copies one compiler construction row into the canonical cold call
// vocabulary. Optional receiver, tail, and direct-target identities are
// carried exactly as emitted; the booleans authenticate the optional shapes.
func NewCall(
	id, body, span, formal, values, valuesRoot, types identity.ContentID,
	callee, actuals, receiver, tail, target identity.ContentID,
	form CallForm,
	operandStart, operandEnd, argumentStart, argumentEnd, typeArgumentStart, typeArgumentEnd uint32,
	hasReceiver, hasTail bool,
) (Call, bool) {
	row := Call{
		id: id, body: body, span: span, formal: formal, values: values, valuesRoot: valuesRoot, types: types,
		callee: callee, actuals: actuals, receiver: receiver, tail: tail, target: target,
		form: form, operandStart: operandStart, operandEnd: operandEnd,
		argumentStart: argumentStart, argumentEnd: argumentEnd,
		typeArgumentStart: typeArgumentStart, typeArgumentEnd: typeArgumentEnd,
		hasReceiver: hasReceiver, hasTail: hasTail,
	}
	return row, row.Available()
}

func (row Call) Available() bool {
	return row.id.Available() && row.body.Available() && row.span.Available() && row.formal.Available() &&
		row.values.Available() && row.valuesRoot.Available() && row.types.Available() &&
		row.callee.Available() && row.actuals.Available() && row.form.Valid() &&
		row.hasReceiver == row.receiver.Available() && (row.form == CallFormMethod) == row.hasReceiver &&
		row.hasTail == row.tail.Available() && row.operandEnd >= row.operandStart &&
		row.argumentEnd >= row.argumentStart && row.typeArgumentEnd >= row.typeArgumentStart
}

func (row Call) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Call) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row Call) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}

func (row Call) FormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

func (row Call) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}

func (row Call) ValuesRootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesRoot
}

func (row Call) TypeArgumentsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}

func (row Call) CalleeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.callee
}

func (row Call) ActualsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.actuals
}

func (row Call) Form() CallForm {
	if !row.Available() {
		return CallFormInvalid
	}
	return row.form
}

func (row Call) DirectTargetBody() (identity.ContentID, bool) {
	return row.target, row.Available() && row.target.Available()
}

func (row Call) ReceiverID() (identity.ContentID, bool) {
	return row.receiver, row.Available() && row.hasReceiver
}

func (row Call) TailID() (identity.ContentID, bool) {
	return row.tail, row.Available() && row.hasTail
}

func (row Call) OperandSpan() (offset, count uint32, ok bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.operandStart, row.operandEnd - row.operandStart, row.Available()
}

func (row Call) ArgumentSpan() (offset, count uint32, ok bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.argumentStart, row.argumentEnd - row.argumentStart, row.Available()
}

func (row Call) TypeArgumentSpan() (offset, count uint32, ok bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.typeArgumentStart, row.typeArgumentEnd - row.typeArgumentStart, row.Available()
}

func (row Call) OperandCount() int {
	_, count, ok := row.OperandSpan()
	if !ok {
		return 0
	}
	return int(count)
}

func (row Call) ArgumentCount() int {
	_, count, ok := row.ArgumentSpan()
	if !ok {
		return 0
	}
	return int(count)
}

func (row Call) TypeArgumentCount() int {
	_, count, ok := row.TypeArgumentSpan()
	if !ok {
		return 0
	}
	return int(count)
}
