package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// The append-only slots these families occupy. They are derived from the last
// slot declared before them so a family added later cannot reuse a slot a
// consumer already addresses.
const (
	slotStaticTypeValue  = slotEnvironmentReset + 1
	slotStaticExpression = slotStaticTypeValue + 1
	slotStaticInput      = slotFunctionCapture + 1
)

var (
	staticTypeValueFamily  = Family[StaticTypeValue]{slot: slotStaticTypeValue, name: "static-type-value"}
	staticExpressionFamily = Family[StaticExpression]{slot: slotStaticExpression, name: "static-expression"}
	staticInputFamily      = Family[StaticInput]{slot: slotStaticInput, name: "static-input"}
)

func StaticTypeValueFamily() Family[StaticTypeValue] { return staticTypeValueFamily }

func StaticExpressionFamily() Family[StaticExpression] { return staticExpressionFamily }

func StaticInputFamily() Family[StaticInput] { return staticInputFamily }

// StaticTypeValue is one authored type-value binding. The row is flat: every
// field is an identity the compiler issued while the Static proof was live,
// plus the authored name the binding is known by.
type StaticTypeValue struct {
	id        identity.ContentID
	body      identity.ContentID
	reference identity.ContentID
	root      identity.ContentID
	name      string
}

// NewStaticTypeValue copies one canonical StaticTypeValueRow.
func NewStaticTypeValue(id, body, reference, root identity.ContentID, name string) (StaticTypeValue, bool) {
	row := StaticTypeValue{id: id, body: body, reference: reference, root: root, name: name}
	return row, row.Available()
}

func (row StaticTypeValue) Available() bool {
	return row.id.Available() && row.body.Available() && row.reference.Available() && row.root.Available() && row.name != ""
}

func (row StaticTypeValue) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StaticTypeValue) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row StaticTypeValue) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}

func (row StaticTypeValue) RootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.root
}

func (row StaticTypeValue) Name() string {
	if !row.Available() {
		return ""
	}
	return row.name
}

// StaticExpression is one authored type expression: the expression identity,
// the static node it references, and the owner that authored it.
type StaticExpression struct {
	id        identity.ContentID
	reference identity.ContentID
	owner     identity.ContentID
}

// NewStaticExpression copies one canonical StaticExpressionRow.
func NewStaticExpression(id, reference, owner identity.ContentID) (StaticExpression, bool) {
	row := StaticExpression{id: id, reference: reference, owner: owner}
	return row, row.Available()
}

func (row StaticExpression) Available() bool {
	return row.id.Available() && row.reference.Available() && row.owner.Available()
}

func (row StaticExpression) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StaticExpression) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}

func (row StaticExpression) Owner() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.owner
}

// StaticInputKind identifies the authored static operation that produced one
// input row. Its ordinal is part of the artifact identity and is preserved in
// the cold publication without reopening the authored Static graph.
type StaticInputKind uint8

const (
	StaticInputInvalid StaticInputKind = iota
	StaticInputTypeOf
	StaticInputAnnotation
)

// StaticInput is one authored static input. The row is flat and pointer-free:
// every identity was issued while the compiler's Static proof was live, and
// the operand disposition is the existing Static query vocabulary.
type StaticInput struct {
	id, owner, expression, source, target, operand, frontier identity.ContentID
	operandReference, operandSubject, operandBody            identity.ContentID
	literal                                                  keyspace.LiteralValue
	kind                                                     StaticInputKind
	operandKind                                              uint8
	cursor                                                   uint32
}

// NewStaticInput copies one compiler input into the canonical Program schema.
// The constructor authenticates the operand shape before the row can enter a
// cold family, so a malformed disposition cannot become a published absence.
func NewStaticInput(
	id, owner, expression, source, target, operand, frontier identity.ContentID,
	operandReference, operandSubject, operandBody identity.ContentID,
	literal keyspace.LiteralValue, kind StaticInputKind, operandKind uint8,
	cursor uint32,
) (StaticInput, bool) {
	row := StaticInput{
		id: id, owner: owner, expression: expression, source: source, target: target,
		operand: operand, frontier: frontier, operandReference: operandReference,
		operandSubject: operandSubject, operandBody: operandBody, literal: literal,
		kind: kind, operandKind: operandKind, cursor: cursor,
	}
	return row, row.Available()
}

func (row StaticInput) Available() bool {
	if !row.id.Available() || !row.owner.Available() || !row.expression.Available() || !row.source.Available() || !row.target.Available() || !row.operand.Available() || !row.frontier.Available() || row.kind == StaticInputInvalid || row.operandKind == 0 {
		return false
	}
	switch row.operandKind {
	case 1: // StaticOperandKnown
		return row.operandSubject == (identity.ContentID{}) && row.operandReference == (identity.ContentID{})
	case 2: // StaticOperandRuntimeSubject
		return row.operandSubject.Available() && row.operandBody.Available() && row.operandReference == (identity.ContentID{})
	case 3: // StaticOperandTypeValue
		return row.operandReference.Available() && row.operandBody.Available() && row.operandSubject == (identity.ContentID{})
	default:
		return false
	}
}

func (row StaticInput) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StaticInput) Owner() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.owner
}

func (row StaticInput) Kind() StaticInputKind {
	if !row.Available() {
		return StaticInputInvalid
	}
	return row.kind
}

func (row StaticInput) ExpressionID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.expression
}

func (row StaticInput) SourceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.source
}

func (row StaticInput) TargetID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.target
}

func (row StaticInput) OperandID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.operand
}

func (row StaticInput) FrontierID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.frontier
}

func (row StaticInput) Cursor() uint32 {
	if !row.Available() {
		return 0
	}
	return row.cursor
}

func (row StaticInput) OperandKind() uint8 {
	if !row.Available() {
		return 0
	}
	return row.operandKind
}

func (row StaticInput) OperandLiteral() keyspace.LiteralValue {
	if !row.Available() {
		return keyspace.LiteralValue{}
	}
	return row.literal
}

func (row StaticInput) OperandReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.operandReference
}

func (row StaticInput) OperandSubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.operandSubject
}

func (row StaticInput) OperandBodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.operandBody
}
