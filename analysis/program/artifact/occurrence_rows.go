package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// OccurrenceKind is the closed, domain-neutral Program semantic occurrence
// vocabulary. Rows retain parent-issued IDs and ordered operand IDs only; no
// analysis domain, Flow coordinate, or runtime handle crosses this boundary.
type OccurrenceKind uint8

const (
	OccurrenceInvalid OccurrenceKind = iota
	OccurrencePointAttachment
	OccurrenceValues
	OccurrenceValuesMember
	OccurrenceValuesTail
	OccurrenceValueSource
	OccurrenceStorageRead
	OccurrenceStorageBind
	OccurrenceStorageBindTransfer
	OccurrenceStorageAssignment
	OccurrenceStorageWrite
	OccurrenceIndexRead
	OccurrenceIndexWrite
	OccurrenceAllocation
	OccurrenceAllocationField
	OccurrenceCall
	// OccurrenceCallActivation is the closed post-input Call rule geometry.
	// It has the same parent call identity as OccurrenceCall but retains only
	// the exact Finish-point attachment; rules that activate a callee cannot
	// be issued at the pre-evaluation Entry attachment.
	OccurrenceCallActivation
	OccurrenceCallBoundary
	OccurrenceCallArm
	OccurrenceCallArgument
	OccurrenceCallTypeArgument
	// Computation rows retain only parent-issued span identities, exact point
	// attachments, and ordered semantic operands. Value domains interpret the
	// closed codes; no Flow term or Program pointer crosses this boundary.
	OccurrenceUnary
	OccurrenceSelect
	OccurrenceValueClaim
	OccurrenceBinaryArithmetic
	OccurrenceBinaryEquality
	OccurrenceBinaryOrder
	// OccurrenceBinaryPresenceRefinement is one exact guarded arm of a
	// nil-comparison whose non-nil operand is an authored storage Read. The
	// row targets the originating storage Cell, not the temporary comparison
	// result, so later Reads observe the reusable branch fact.
	OccurrenceBinaryPresenceRefinement
	OccurrenceReturnBoundary
	// OccurrenceFormalEntry is one callable formal's unconditional entry
	// contribution. Program owns only its storage identity and entry placement;
	// the consuming domain chooses the abstract value written there.
	// It is appended to preserve every existing occurrence ordinal.
	OccurrenceFormalEntry
	// OccurrenceOperationPredicateRefinement is one exact guarded arm of an
	// operation-result equality.  Program retains the authenticated operation
	// occurrence, the subject and comparison operands, and the parent-issued
	// route certificate; the consuming domain interprets the opaque operation
	// predicate relation.
	// It is appended to preserve every existing occurrence ordinal.
	OccurrenceOperationPredicateRefinement
)

// SpanResultOccurrence reports whether the family's result identity is the
// operator's own program-owned span rather than a semantic occurrence.
func SpanResultOccurrence(kind OccurrenceKind) bool {
	return kind == OccurrenceBinaryArithmetic || kind == OccurrenceBinaryEquality || kind == OccurrenceBinaryOrder
}

func (kind OccurrenceKind) valid() bool {
	return kind >= OccurrencePointAttachment && kind <= OccurrenceOperationPredicateRefinement
}

// OccurrenceRow is one immutable generic operand record. Body and points are
// semantic parent IDs; Inputs preserve the exact parent-issued operand order.
// Code is a closed parent-role discriminator (never an ordinal identity).
type OccurrenceRow struct {
	kind          OccurrenceKind
	id            identity.ContentID
	body          identity.ContentID
	points        []identity.ContentID
	inputs        []identity.ContentID
	code          uint64
	literalFamily keyspace.Family
	literal       keyspace.LiteralValue
	literalOK     bool
}

type occurrenceLookup struct {
	kind OccurrenceKind
	id   identity.ContentID
}

// occurrenceSpanGeometry is compile-only scratch captured while the exact
// Program role proof is live. It is discarded after role-specific placements
// are sealed into the Artifact.
type occurrenceSpanGeometry struct {
	entry  []identity.ContentID
	finish []identity.ContentID
	route  identity.ContentID
}

func (row OccurrenceRow) Available() bool {
	if !row.kind.valid() || !row.id.Available() || row.code == ^uint64(0) {
		return false
	}
	for _, point := range row.points {
		if !point.Available() {
			return false
		}
	}
	for _, input := range row.inputs {
		if !input.Available() {
			return false
		}
	}
	if row.literalOK && row.kind != OccurrenceValueSource {
		return false
	}
	if row.literalOK && row.literalFamily == keyspace.FamilyInvalid {
		return false
	}
	if row.kind == OccurrenceValueSource && len(row.inputs) != 1 {
		return false
	}
	if row.kind == OccurrenceBinaryEquality {
		op := flowkind.BinaryOp(row.code & binaryEqualityCodeOpMask)
		hasComparison := row.code&binaryEqualityCodeHasComparison != 0
		invert := row.code&binaryEqualityCodeInvert != 0
		if row.code&^(binaryEqualityCodeOpMask|binaryEqualityCodeHasComparison|binaryEqualityCodeInvert) != 0 ||
			(op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) || invert != (op == flowkind.BinaryNotEqual) ||
			(!hasComparison && len(row.inputs) != 2) || (hasComparison && len(row.inputs) != 5) {
			return false
		}
	}
	if row.kind == OccurrenceBinaryArithmetic {
		op := flowkind.BinaryOp(row.code)
		if !flowkind.IsBinaryArithmetic(op) || len(row.inputs) != 2 {
			return false
		}
	}
	if row.kind == OccurrenceBinaryOrder {
		op := flowkind.BinaryOp(row.code)
		if !flowkind.IsBinaryOrder(op) || len(row.inputs) != 2 {
			return false
		}
	}
	if row.kind == OccurrenceBinaryPresenceRefinement &&
		(!row.body.Available() || len(row.points) != 1 || len(row.inputs) != 4 || row.code > 1) {
		return false
	}
	if row.kind == OccurrenceOperationPredicateRefinement &&
		(!row.body.Available() || len(row.points) != 1 || len(row.inputs) != 4 ||
			row.code&^(operationPredicateCodeOpMask|operationPredicateCodeTruth) != 0 ||
			flowkind.BinaryOp(row.code&operationPredicateCodeOpMask) != flowkind.BinaryEqual &&
				flowkind.BinaryOp(row.code&operationPredicateCodeOpMask) != flowkind.BinaryNotEqual) {
		return false
	}
	if row.kind == OccurrenceStorageRead && len(row.inputs) != 2 {
		return false
	}
	return true
}
func (row OccurrenceRow) Kind() OccurrenceKind {
	if !row.Available() {
		return OccurrenceInvalid
	}
	return row.kind
}
func (row OccurrenceRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row OccurrenceRow) BodyID() (identity.ContentID, bool) {
	return row.body, row.Available() && row.body.Available()
}
func (row OccurrenceRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.points)
}
func (row OccurrenceRow) PointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.points) {
		return identity.ContentID{}, false
	}
	return row.points[index], true
}
func (row OccurrenceRow) InputCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.inputs)
}
func (row OccurrenceRow) InputAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.inputs) {
		return identity.ContentID{}, false
	}
	return row.inputs[index], true
}
func (row OccurrenceRow) Code() uint64 {
	if !row.Available() {
		return 0
	}
	return row.code
}

func (row OccurrenceRow) Literal() (keyspace.Family, keyspace.LiteralValue, bool) {
	if !row.Available() || row.kind != OccurrenceValueSource || !row.literalOK {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	return row.literalFamily, row.literal, true
}

// ValueSourceSpanID is the exact evaluation Span of a literal/TypeValue
// source. The occurrence role ID remains its rule-output identity; expression
// operands name the Span, so preserving both avoids any downstream raw-Term
// reconstruction or guessed equality between the two namespaces.
func (row OccurrenceRow) ValueSourceSpanID() (identity.ContentID, bool) {
	if !row.Available() || row.kind != OccurrenceValueSource || len(row.inputs) != 1 {
		return identity.ContentID{}, false
	}
	return row.inputs[0], true
}

const (
	binaryEqualityCodeOpMask        = uint64(0xff)
	binaryEqualityCodeHasComparison = uint64(1 << 8)
	binaryEqualityCodeInvert        = uint64(1 << 9)
)

func binaryEqualityCode(op flowkind.BinaryOp, hasComparison, invert bool) (uint64, bool) {
	if (op != flowkind.BinaryEqual && op != flowkind.BinaryNotEqual) || invert != (op == flowkind.BinaryNotEqual) {
		return 0, false
	}
	code := uint64(op)
	if hasComparison {
		code |= binaryEqualityCodeHasComparison
	}
	if invert {
		code |= binaryEqualityCodeInvert
	}
	return code, true
}

// BinaryEquality returns the ordered semantic operands and operator of one
// retained primitive equality computation. It never exposes authored Terms.
func (row OccurrenceRow) BinaryEquality() (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryEquality || len(row.inputs) < 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code & binaryEqualityCodeOpMask), true
}

// BinaryArithmetic returns the authored ordered semantic operands and closed
// primitive arithmetic operator.  The row is reusable Program geometry and
// exposes no authored Term.
func (row OccurrenceRow) BinaryArithmetic() (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryArithmetic || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}

// BinaryOrder returns the authored ordered semantic operands and relational
// operator of one retained primitive order computation.
func (row OccurrenceRow) BinaryOrder() (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryOrder || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}

// BinaryComparison returns the optional exact Branch and two causal body
// identities retained beside a Binary equality occurrence.
func (row OccurrenceRow) BinaryComparison() (branch, whenTrue, whenFalse identity.ContentID, invert bool, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryEquality || row.code&binaryEqualityCodeHasComparison == 0 || len(row.inputs) != 5 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false, false
	}
	return row.inputs[2], row.inputs[3], row.inputs[4], row.code&binaryEqualityCodeInvert != 0, true
}

// BinaryPresenceRefinement returns the exact reusable proof constituents for
// one nil-comparison arm. Source is the Binary occurrence, Target is the
// storage Cell being narrowed, Operand is the comparison operand whose
// StorageRead proved that origin, and Route is the exact guarded environment
// edge entering this arm. Present is the arm's closed nilability conclusion.
func (row OccurrenceRow) BinaryPresenceRefinement() (source, target, operand, route identity.ContentID, present bool, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryPresenceRefinement || len(row.inputs) != 4 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false, false
	}
	return row.inputs[0], row.inputs[1], row.inputs[2], row.inputs[3], row.code == 1, true
}

const (
	operationPredicateCodeOpMask = uint64(0xff)
	operationPredicateCodeTruth  = uint64(1 << 8)
)

// OperationPredicateRefinement returns one neutral guarded operation
// predicate proof. Source is the existing operation occurrence, Target is
// the subject Value semantic identity, Operand is the comparison Value
// identity, and Route is the exact parent-issued environment edge. Truth is
// the edge polarity; the equality operator is retained in the closed code.
func (row OccurrenceRow) OperationPredicateRefinement() (source, target, operand, route identity.ContentID, op flowkind.BinaryOp, truth bool, ok bool) {
	if !row.Available() || row.kind != OccurrenceOperationPredicateRefinement || len(row.inputs) != 4 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, 0, false, false
	}
	return row.inputs[0], row.inputs[1], row.inputs[2], row.inputs[3], flowkind.BinaryOp(row.code & operationPredicateCodeOpMask), row.code&operationPredicateCodeTruth != 0, true
}

// StorageRead returns the existing Cell and exact expression Span identities
// retained while Program owned both proofs. The occurrence role ID remains
// distinct: computation operands name spans, so the span is the only sound
// reusable join between a Binary operand and its storage origin.
func (row OccurrenceRow) StorageRead() (cell, span identity.ContentID, ok bool) {
	if !row.Available() || row.kind != OccurrenceStorageRead || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return row.inputs[0], row.inputs[1], true
}
