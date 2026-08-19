package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// StaticOperandKind is the closed disposition vocabulary for one static operator
// operand. It is owned by Static because the disposition is part of the
// static-input contract; Artifact only transports the value into its row.
type StaticOperandKind uint8

const (
	StaticOperandInvalid StaticOperandKind = iota
	StaticOperandKnown
	StaticOperandRuntimeSubject
	StaticOperandTypeValue
)

// StaticOperand is the sealed scalar behind one TypeOf or annotation input. It
// carries no authored term and no owner capability: all fields are detached
// identities or the exact scalar literal needed by the mounted evaluator.
type StaticOperand struct {
	kind      StaticOperandKind
	id        identity.ContentID
	literal   keyspace.LiteralValue
	reference identity.ContentID
	subject   identity.ContentID
	body      identity.ContentID
}

func (operand StaticOperand) Kind() StaticOperandKind { return operand.kind }
func (operand StaticOperand) ID() identity.ContentID  { return operand.id }
func (operand StaticOperand) Literal() keyspace.LiteralValue {
	return operand.literal
}
func (operand StaticOperand) ReferenceID() identity.ContentID { return operand.reference }
func (operand StaticOperand) SubjectID() identity.ContentID   { return operand.subject }
func (operand StaticOperand) BodyPathID() identity.ContentID  { return operand.body }

// StaticOperandResolver is a one-shot cross-owner read capability. Static owns the
// classification and result representation; the coordinating Program lends
// only existing canonical Source/Flow identities and relations for the leaf
// cases. The resolver is not retained and is not a second operand schema.
type StaticOperandResolver struct {
	Literal        func(keyspace.Term) (identity.ContentID, keyspace.LiteralValue, bool)
	Claim          func(keyspace.Term) (keyspace.Term, bool)
	TypeValue      func(keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool)
	RuntimeSubject func(keyspace.Term) (identity.ContentID, identity.ContentID, identity.ContentID, bool)
}

// StaticOperandAt resolves one exact scalar operand through the canonical owner
// relations lent by resolver. Claim operands are transparent; the three leaf
// callbacks are responsible only for admitting their own existing owner rows
// and identities. A fresh seen set makes malformed claim cycles fail closed.
func (view View) StaticOperandAt(term keyspace.Term, resolver StaticOperandResolver) (StaticOperand, bool) {
	if !view.Available() || term == 0 || resolver.Literal == nil || resolver.Claim == nil ||
		resolver.TypeValue == nil || resolver.RuntimeSubject == nil {
		return StaticOperand{}, false
	}
	return resolveOperand(term, resolver, make(map[keyspace.Term]struct{}))
}

func resolveOperand(term keyspace.Term, resolver StaticOperandResolver, seen map[keyspace.Term]struct{}) (StaticOperand, bool) {
	if term == 0 {
		return StaticOperand{}, false
	}
	if _, duplicate := seen[term]; duplicate {
		return StaticOperand{}, false
	}
	seen[term] = struct{}{}
	if id, literal, ok := resolver.Literal(term); ok && id.Available() {
		return StaticOperand{kind: StaticOperandKnown, id: id, literal: literal}, true
	}
	if operand, ok := resolver.Claim(term); ok && operand != 0 {
		return resolveOperand(operand, resolver, seen)
	}
	if id, reference, body, ok := resolver.TypeValue(term); ok && id.Available() && reference.Available() && body.Available() {
		return StaticOperand{kind: StaticOperandTypeValue, id: id, reference: reference, body: body}, true
	}
	if id, subject, body, ok := resolver.RuntimeSubject(term); ok && id.Available() && subject.Available() && body.Available() {
		return StaticOperand{kind: StaticOperandRuntimeSubject, id: id, subject: subject, body: body}, true
	}
	return StaticOperand{}, false
}
