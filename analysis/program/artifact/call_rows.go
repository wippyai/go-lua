package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

// CallOperandKind is the closed role of one authored Call operand.  The
// artifact copy deliberately uses the Artifact vocabulary rather than
// exporting the transformer proof type.
type CallOperandKind uint8

const (
	CallOperandInvalid CallOperandKind = iota
	CallOperandCallee
	CallOperandReceiver
	CallOperandActuals
)

func (kind CallOperandKind) valid() bool {
	return kind >= CallOperandCallee && kind <= CallOperandActuals
}

// CallOperandRow is one immutable, pointer-free Call operand. ID is the
// semantic operand identity issued by Program; ValueID is the corresponding
// reusable value identity when the operand is a Values member/root. SpanID is
// retained so consumers can authenticate the exact mounted Value without
// reopening Program or reconstructing an authored Term.
type CallOperandRow struct {
	id     identity.ContentID
	call   identity.ContentID
	value  identity.ContentID
	span   identity.ContentID
	kind   CallOperandKind
	sealed bool
}

func (row CallOperandRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.value.Available() && row.span.Available() && row.kind.valid()
}
func (row CallOperandRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallOperandRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallOperandRow) ValueID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.value
}
func (row CallOperandRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallOperandRow) Kind() CallOperandKind {
	if !row.Available() {
		return CallOperandInvalid
	}
	return row.kind
}

// CallArgumentRow is one immutable ordered actual argument. ValueID is the
// argument's semantic identity; MemberID and ValuesID retain the exact
// parent/value joins needed by mounted Pack and Link consumers.
type CallArgumentRow struct {
	id       identity.ContentID
	call     identity.ContentID
	values   identity.ContentID
	member   identity.ContentID
	span     identity.ContentID
	position uint32
	sealed   bool
}

func (row CallArgumentRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.values.Available() && row.member.Available() && row.span.Available()
}
func (row CallArgumentRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallArgumentRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallArgumentRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}
func (row CallArgumentRow) MemberID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.member
}
func (row CallArgumentRow) ValueID() identity.ContentID { return row.MemberID() }
func (row CallArgumentRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallArgumentRow) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// CallTypeArgumentRow is one immutable ordered Static type argument. The
// reference identity is the only type payload that crosses the Artifact
// boundary; no Static Term or transformer proof is retained.
type CallTypeArgumentRow struct {
	id        identity.ContentID
	call      identity.ContentID
	types     identity.ContentID
	reference identity.ContentID
	position  uint32
	sealed    bool
}

func (row CallTypeArgumentRow) Available() bool {
	return row.sealed && row.id.Available() && row.call.Available() && row.types.Available() && row.reference.Available()
}
func (row CallTypeArgumentRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallTypeArgumentRow) CallID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.call
}
func (row CallTypeArgumentRow) TypesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}
func (row CallTypeArgumentRow) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}
func (row CallTypeArgumentRow) Index() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}

// CallRow is the complete immutable authored-call row. Its ranges index the
// Artifact-owned operand, argument, and type-argument columns; mounted code
// consumes these scalar IDs and never needs transient Program proofs.
type CallRow struct {
	id         identity.ContentID
	body       identity.ContentID
	span       identity.ContentID
	formal     identity.ContentID
	values     identity.ContentID
	valuesRoot identity.ContentID
	types      identity.ContentID
	callee     identity.ContentID
	actuals    identity.ContentID
	receiver   identity.ContentID
	tail       identity.ContentID
	operandStart,
	operandEnd uint32
	argumentStart,
	argumentEnd uint32
	typeArgumentStart,
	typeArgumentEnd uint32
	form        flow.CallForm
	hasReceiver bool
	hasTail     bool
	sealed      bool
}

func (row CallRow) Available() bool {
	if !row.sealed || !row.id.Available() || !row.body.Available() || !row.span.Available() || !row.formal.Available() ||
		!row.values.Available() || !row.valuesRoot.Available() || !row.types.Available() || !row.callee.Available() || !row.actuals.Available() ||
		(row.form != flow.CallFormPlain && row.form != flow.CallFormMethod) || row.hasReceiver != row.receiver.Available() ||
		(row.form == flow.CallFormMethod) != row.hasReceiver || row.hasTail != row.tail.Available() ||
		row.argumentEnd < row.argumentStart || row.operandEnd < row.operandStart || row.typeArgumentEnd < row.typeArgumentStart {
		return false
	}
	return true
}
func (row CallRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row CallRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row CallRow) SpanID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.span
}
func (row CallRow) FormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

// ValuesID is the CallValues semantic identity used by Pack/Link joins.
func (row CallRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.values
}

// ValuesRootID is the Artifact Values row identity for the actual-values root.
func (row CallRow) ValuesRootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesRoot
}
func (row CallRow) TypeArgumentsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.types
}
func (row CallRow) CalleeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.callee
}
func (row CallRow) ActualsID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.actuals
}
func (row CallRow) Form() flow.CallForm {
	if !row.Available() {
		return 0
	}
	return row.form
}
func (row CallRow) ReceiverID() (identity.ContentID, bool) {
	return row.receiver, row.Available() && row.hasReceiver
}
func (row CallRow) TailID() (identity.ContentID, bool) {
	return row.tail, row.Available() && row.hasTail
}
func (row CallRow) OperandCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.operandEnd - row.operandStart)
}
func (row CallRow) ArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.argumentEnd - row.argumentStart)
}
func (row CallRow) TypeArgumentCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.typeArgumentEnd - row.typeArgumentStart)
}
