package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// NumericRepresentation is the neutral scalar carrier emitted by the Program
// compiler. Its ordinals are persisted in the summary identity; the compiler
// owns the interpretation of those ordinals.
type NumericRepresentation uint8

const (
	NumericRepresentationInvalid NumericRepresentation = iota
	NumericRepresentationInteger
	NumericRepresentationFloat
	NumericRepresentationNumber
)

func (representation NumericRepresentation) Valid() bool {
	return representation >= NumericRepresentationInteger && representation <= NumericRepresentationNumber
}

// ArithmeticDivisorProperty is the neutral divisor-proof carrier. None means
// that no divisor proof was retained; the compiler owns the operation-specific
// meaning of the nonzero variants.
type ArithmeticDivisorProperty uint8

const (
	ArithmeticDivisorNone ArithmeticDivisorProperty = iota
	ArithmeticDivisorNonzero
	ArithmeticDivisorNonzeroNotMinusOne
)

func (property ArithmeticDivisorProperty) Valid() bool {
	return property <= ArithmeticDivisorNonzeroNotMinusOne
}

// SummaryOperator is an opaque Program operator ordinal. Keeping the carrier
// here avoids making the compiled-program schema depend on the Program
// vocabulary package.
type SummaryOperator uint8

// SummaryLiteral is the neutral exact scalar carrier. Kind retains the
// compiler's literal ordinal; Integer and FloatBits retain the historical
// payload fields exactly, including signed zero and NaN payload bits.
type SummaryLiteral struct {
	Kind      uint8
	Integer   int64
	FloatBits uint64
}

func (literal SummaryLiteral) Valid() bool {
	return literal.Kind != 0
}

// ExactScalarSummaryRole identifies which arithmetic use a scalar belongs to.
// It is a declared proof coordinate, not an authored operand ordinal.
type ExactScalarSummaryRole uint8

const (
	ExactScalarSummaryLeft ExactScalarSummaryRole = iota + 1
	ExactScalarSummaryRight
	ExactScalarSummaryResult
)

func (role ExactScalarSummaryRole) Valid() bool {
	return role >= ExactScalarSummaryLeft && role <= ExactScalarSummaryResult
}

// ExactScalarSummary is one immutable Program-owned exact scalar proof. The
// row is stored in a Frozen column and is shared unchanged by every mount of
// the compiled Program.
type ExactScalarSummary struct {
	id         identity.ContentID
	occurrence identity.ContentID
	subject    identity.ContentID
	body       identity.ContentID
	role       ExactScalarSummaryRole
	literal    SummaryLiteral
}

// NewExactScalarSummary constructs and authenticates one exact scalar proof.
// The digest domain and field order intentionally remain the artifact identity
// contract, so moving the row into the cold column does not churn Program IDs.
func NewExactScalarSummary(occurrence, subject, body identity.ContentID, role ExactScalarSummaryRole, literal SummaryLiteral) (ExactScalarSummary, bool) {
	row := ExactScalarSummary{
		occurrence: occurrence,
		subject:    subject,
		body:       body,
		role:       role,
		literal:    literal,
	}
	row.id = exactScalarSummaryID(occurrence, subject, body, role, literal)
	return row, row.Available()
}

func (row ExactScalarSummary) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.subject.Available() && row.body.Available() && row.role.Valid() && row.literal.Valid() &&
		row.id == exactScalarSummaryID(row.occurrence, row.subject, row.body, row.role, row.literal)
}

func (row ExactScalarSummary) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row ExactScalarSummary) OccurrenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.occurrence
}

func (row ExactScalarSummary) SubjectID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.subject
}

func (row ExactScalarSummary) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row ExactScalarSummary) Role() ExactScalarSummaryRole {
	if !row.Available() {
		return 0
	}
	return row.role
}

func (row ExactScalarSummary) Literal() (SummaryLiteral, bool) {
	return row.literal, row.Available()
}

// ArithmeticSummary is one immutable Program-owned abstract arithmetic proof.
type ArithmeticSummary struct {
	id, occurrence, body identity.ContentID
	op                   SummaryOperator
	left, right, result  NumericRepresentation
	divisor              ArithmeticDivisorProperty
}

// NewArithmeticSummary constructs and authenticates one arithmetic proof.
func NewArithmeticSummary(occurrence, body identity.ContentID, op SummaryOperator, left, right, result NumericRepresentation, divisor ArithmeticDivisorProperty) (ArithmeticSummary, bool) {
	row := ArithmeticSummary{occurrence: occurrence, body: body, op: op, left: left, right: right, result: result, divisor: divisor}
	row.id = arithmeticSummaryID(occurrence, body, op, left, right, result, divisor)
	return row, row.Available()
}

func (row ArithmeticSummary) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.body.Available() && row.op != 0 && row.left.Valid() && row.right.Valid() && row.result.Valid() && row.divisor.Valid() &&
		row.id == arithmeticSummaryID(row.occurrence, row.body, row.op, row.left, row.right, row.result, row.divisor)
}

func (row ArithmeticSummary) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row ArithmeticSummary) OccurrenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.occurrence
}

func (row ArithmeticSummary) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row ArithmeticSummary) Operator() SummaryOperator {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row ArithmeticSummary) Representations() (left, right, result NumericRepresentation, ok bool) {
	if !row.Available() {
		return 0, 0, 0, false
	}
	return row.left, row.right, row.result, true
}

func (row ArithmeticSummary) DivisorProperty() ArithmeticDivisorProperty {
	if !row.Available() {
		return ArithmeticDivisorNone
	}
	return row.divisor
}

// UnarySummary is one immutable Program-owned unary numeric proof at an exact
// output point.
type UnarySummary struct {
	id, occurrence, body, point identity.ContentID
	op                          SummaryOperator
	operand, result             NumericRepresentation
}

// NewUnarySummary constructs and authenticates one unary proof.
func NewUnarySummary(occurrence, body, point identity.ContentID, op SummaryOperator, operand, result NumericRepresentation) (UnarySummary, bool) {
	row := UnarySummary{occurrence: occurrence, body: body, point: point, op: op, operand: operand, result: result}
	row.id = unarySummaryID(occurrence, body, point, op, operand, result)
	return row, row.Available()
}

func (row UnarySummary) Available() bool {
	return row.id.Available() && row.occurrence.Available() && row.body.Available() && row.point.Available() && row.op != 0 && row.operand.Valid() && row.result.Valid() &&
		row.id == unarySummaryID(row.occurrence, row.body, row.point, row.op, row.operand, row.result)
}

func (row UnarySummary) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row UnarySummary) OccurrenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.occurrence
}

func (row UnarySummary) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row UnarySummary) OutputPointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}

func (row UnarySummary) Operator() SummaryOperator {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row UnarySummary) Representations() (operand, result NumericRepresentation, ok bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.operand, row.result, true
}

const (
	// summaryFormat is the retained semantic identity version for all three
	// frozen summary columns. It is deliberately the sole summary authority;
	// it is not a second copy of the Artifact package format version.
	summaryFormat = uint64(33)
)

func exactScalarSummaryID(occurrence, subject, body identity.ContentID, role ExactScalarSummaryRole, literal SummaryLiteral) identity.ContentID {
	if !occurrence.Available() || !subject.Available() || !body.Available() || !role.Valid() || !literal.Valid() {
		return identity.ContentID{}
	}
	var writer canonical.DigestWriter
	if writer.Reset("analysis/program-artifact/exact-scalar-summary", summaryFormat) != nil ||
		writer.Bytes(occurrence[:]) != nil || writer.Bytes(subject[:]) != nil || writer.Bytes(body[:]) != nil ||
		writer.Uint(uint64(role)) != nil || writer.Uint(uint64(literal.Kind)) != nil ||
		writer.Uint(uint64(literal.Integer)) != nil || writer.Uint(literal.FloatBits) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(writer.Sum())
}

func arithmeticSummaryID(occurrence, body identity.ContentID, op SummaryOperator, left, right, result NumericRepresentation, divisor ArithmeticDivisorProperty) identity.ContentID {
	if !occurrence.Available() || !body.Available() || op == 0 || !left.Valid() || !right.Valid() || !result.Valid() || !divisor.Valid() {
		return identity.ContentID{}
	}
	var writer canonical.DigestWriter
	if writer.Reset("analysis/program-artifact/arithmetic-summary", summaryFormat) != nil ||
		writer.Bytes(occurrence[:]) != nil || writer.Bytes(body[:]) != nil || writer.Uint(uint64(op)) != nil ||
		writer.Uint(uint64(left)) != nil || writer.Uint(uint64(right)) != nil || writer.Uint(uint64(result)) != nil ||
		writer.Uint(uint64(divisor)) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(writer.Sum())
}

func unarySummaryID(occurrence, body, point identity.ContentID, op SummaryOperator, operand, result NumericRepresentation) identity.ContentID {
	if !occurrence.Available() || !body.Available() || !point.Available() || op == 0 || !operand.Valid() || !result.Valid() {
		return identity.ContentID{}
	}
	var writer canonical.DigestWriter
	if writer.Reset("analysis/program-artifact/unary-summary", summaryFormat) != nil ||
		writer.Bytes(occurrence[:]) != nil || writer.Bytes(body[:]) != nil || writer.Bytes(point[:]) != nil ||
		writer.Uint(uint64(op)) != nil || writer.Uint(uint64(operand)) != nil || writer.Uint(uint64(result)) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	return identity.ContentID(writer.Sum())
}
