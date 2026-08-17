package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// StaticExpressionRow is the closed expression denominator for one mounted
// Program. Its identity and reference are both issued by Program.Static.
type StaticExpressionRow struct{ id, reference, owner identity.ContentID }

func (row StaticExpressionRow) Available() bool {
	return row.id.Available() && row.reference.Available() && row.owner.Available()
}
func (row StaticExpressionRow) ID() identity.ContentID          { return row.id }
func (row StaticExpressionRow) ReferenceID() identity.ContentID { return row.reference }
func (row StaticExpressionRow) Owner() identity.ContentID       { return row.owner }

type StaticInputKind uint8

const (
	StaticInputInvalid StaticInputKind = iota
	StaticInputTypeOf
	StaticInputAnnotation
)

// StaticInputOperandKind is the exact Program-issued operand disposition.
// Invalid is reserved for an unavailable row; the compiler rejects malformed
// authored operands instead of fabricating a fallback judgment.
type StaticInputOperandKind uint8

const (
	StaticInputOperandInvalid StaticInputOperandKind = iota
	StaticInputOperandKnown
	StaticInputOperandRuntimeSubject
	StaticInputOperandTypeValue
)

// StaticInputRow is the closed authored input denominator. The row uses the
// existing Program-issued semantic IDs for its expression and operand.
type StaticInputRow struct {
	id, owner, expression, source, target, operand, frontier identity.ContentID
	operandReference, operandSubject, operandBody            identity.ContentID
	literal                                                  keyspace.LiteralValue
	kind                                                     StaticInputKind
	operandKind                                              StaticInputOperandKind
	cursor                                                   uint32
}

func (row StaticInputRow) Available() bool {
	if !row.id.Available() || !row.owner.Available() || !row.expression.Available() || !row.source.Available() || !row.target.Available() || !row.operand.Available() || !row.frontier.Available() || row.kind == StaticInputInvalid || row.operandKind == StaticInputOperandInvalid {
		return false
	}
	switch row.operandKind {
	case StaticInputOperandKnown:
		return row.operandSubject == (identity.ContentID{}) && row.operandReference == (identity.ContentID{})
	case StaticInputOperandRuntimeSubject:
		return row.operandSubject.Available() && row.operandBody.Available() && row.operandReference == (identity.ContentID{})
	case StaticInputOperandTypeValue:
		return row.operandReference.Available() && row.operandBody.Available() && row.operandSubject == (identity.ContentID{})
	default:
		return false
	}
}
func (row StaticInputRow) ID() identity.ContentID                 { return row.id }
func (row StaticInputRow) Owner() identity.ContentID              { return row.owner }
func (row StaticInputRow) Kind() StaticInputKind                  { return row.kind }
func (row StaticInputRow) ExpressionID() identity.ContentID       { return row.expression }
func (row StaticInputRow) SourceID() identity.ContentID           { return row.source }
func (row StaticInputRow) TargetID() identity.ContentID           { return row.target }
func (row StaticInputRow) OperandID() identity.ContentID          { return row.operand }
func (row StaticInputRow) FrontierID() identity.ContentID         { return row.frontier }
func (row StaticInputRow) Cursor() uint32                         { return row.cursor }
func (row StaticInputRow) OperandKind() StaticInputOperandKind    { return row.operandKind }
func (row StaticInputRow) OperandLiteral() keyspace.LiteralValue  { return row.literal }
func (row StaticInputRow) OperandReferenceID() identity.ContentID { return row.operandReference }
func (row StaticInputRow) OperandSubjectID() identity.ContentID   { return row.operandSubject }
func (row StaticInputRow) OperandBodyPathID() identity.ContentID  { return row.operandBody }
