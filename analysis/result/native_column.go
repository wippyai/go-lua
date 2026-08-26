package result

import (
	"bytes"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/sendsafety"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The publication's own column vocabularies. Each is declared on the
// structural surface under the category named below, and these ordinals are
// the positions of its members, so a renderer resolves the member at the
// ordinal and reads its declared spelling rather than holding a switch of its
// own.

// NativeScalarRepresentation is the carrier a published exact scalar is
// proved under. Its ordinals are keyspace.LiteralKind's own numbering with Lua
// nil appended: nil retains its own identity and has no literal kind.
type NativeScalarRepresentation uint8

const (
	NativeScalarRepresentationInvalid NativeScalarRepresentation = iota
	NativeScalarRepresentationBoolean
	NativeScalarRepresentationInteger
	NativeScalarRepresentationFloat
	NativeScalarRepresentationString
	NativeScalarRepresentationNil
)

func (representation NativeScalarRepresentation) Available() bool {
	return representation >= NativeScalarRepresentationBoolean && representation <= NativeScalarRepresentationNil
}

// Ordinal is this member's position in structure.CategoryNativeScalarRepresentation.
func (representation NativeScalarRepresentation) Ordinal() uint16 {
	if !representation.Available() {
		return 0
	}
	return uint16(representation)
}

// NativeDivisorProperty is the divisor proof a published arithmetic column
// carries. Its first two ordinals are
// programschema.ArithmeticDivisorProperty's own numbering of its proved
// members; not-applicable is the operator-level answer for a division that
// carries no integer divisor obligation, and the absent property publishes no
// column at all.
type NativeDivisorProperty uint8

const (
	NativeDivisorPropertyInvalid NativeDivisorProperty = iota
	NativeDivisorPropertyNonzero
	NativeDivisorPropertyNonzeroNotMinusOne
	NativeDivisorPropertyNotApplicable
)

func (property NativeDivisorProperty) Available() bool {
	return property >= NativeDivisorPropertyNonzero && property <= NativeDivisorPropertyNotApplicable
}

// Ordinal is this member's position in structure.CategoryNativeDivisorProperty.
func (property NativeDivisorProperty) Ordinal() uint16 {
	if !property.Available() {
		return 0
	}
	return uint16(property)
}

// NativeTruthinessClass is the verdict a branch condition's truth fold
// reaches over its whole evidence set. Unobserved is the incomplete fold: a
// condition some point of its evidence set published no truth for is not the
// same answer as a condition proved to take both truths, and a consumer must
// never have to tell them apart by a missing column.
type NativeTruthinessClass uint8

const (
	NativeTruthinessClassInvalid NativeTruthinessClass = iota
	NativeTruthinessClassAlwaysTruthy
	NativeTruthinessClassAlwaysFalsy
	NativeTruthinessClassDynamic
	NativeTruthinessClassUnobserved
)

func (class NativeTruthinessClass) Available() bool {
	return class >= NativeTruthinessClassAlwaysTruthy && class <= NativeTruthinessClassUnobserved
}

// Ordinal is this member's position in structure.CategoryNativeTruthinessClass.
func (class NativeTruthinessClass) Ordinal() uint16 {
	if !class.Available() {
		return 0
	}
	return uint16(class)
}

// NativeBranchPartition is the branch geometry the truth fold licenses.
type NativeBranchPartition uint8

const (
	NativeBranchPartitionInvalid NativeBranchPartition = iota
	NativeBranchPartitionAlwaysTaken
	NativeBranchPartitionAlwaysNotTaken
	NativeBranchPartitionDynamic
	NativeBranchPartitionUnobserved
)

func (partition NativeBranchPartition) Available() bool {
	return partition >= NativeBranchPartitionAlwaysTaken && partition <= NativeBranchPartitionUnobserved
}

// Ordinal is this member's position in structure.CategoryNativeBranchPartition.
func (partition NativeBranchPartition) Ordinal() uint16 {
	if !partition.Available() {
		return 0
	}
	return uint16(partition)
}

// NativeBranchArm is one arm of a Lua conditional. A proved partition names
// the arm it proves dead.
type NativeBranchArm uint8

const (
	NativeBranchArmInvalid NativeBranchArm = iota
	NativeBranchArmThen
	NativeBranchArmElse
)

func (arm NativeBranchArm) Available() bool {
	return arm >= NativeBranchArmThen && arm <= NativeBranchArmElse
}

// Ordinal is this member's position in structure.CategoryNativeBranchArm.
func (arm NativeBranchArm) Ordinal() uint16 {
	if !arm.Available() {
		return 0
	}
	return uint16(arm)
}

// nativeBranchVerdict derives the two published branch columns from the folded
// truth and the completeness of the fold. It is the one place the derivation
// is stated: the truthiness class and the partition can never disagree,
// because a single answer produces both.
func nativeBranchVerdict(truth valuedomain.Truth, complete bool) (NativeTruthinessClass, NativeBranchPartition, NativeBranchArm, bool) {
	if !complete {
		return NativeTruthinessClassUnobserved, NativeBranchPartitionUnobserved, NativeBranchArmInvalid, false
	}
	switch truth {
	case valuedomain.TruthTrue:
		return NativeTruthinessClassAlwaysTruthy, NativeBranchPartitionAlwaysTaken, NativeBranchArmElse, true
	case valuedomain.TruthFalse:
		return NativeTruthinessClassAlwaysFalsy, NativeBranchPartitionAlwaysNotTaken, NativeBranchArmThen, true
	default:
		return NativeTruthinessClassDynamic, NativeBranchPartitionDynamic, NativeBranchArmInvalid, false
	}
}

// nativeScalarRepresentationOf projects a keyspace literal kind onto the
// published scalar carrier. The ordinals are pinned, so the projection is the
// ordinal itself.
func nativeScalarRepresentationOf(kind keyspace.LiteralKind) (NativeScalarRepresentation, bool) {
	representation := NativeScalarRepresentation(kind)
	if kind < keyspace.LiteralBool || kind > keyspace.LiteralString || !representation.Available() {
		return NativeScalarRepresentationInvalid, false
	}
	return representation, true
}

// nativeDivisorPropertyOf projects a Program-issued divisor proof onto the
// published property. The absent proof publishes no column, which is not a
// failure.
func nativeDivisorPropertyOf(property programschema.ArithmeticDivisorProperty) (NativeDivisorProperty, bool) {
	if property == programschema.ArithmeticDivisorNone {
		return NativeDivisorPropertyInvalid, true
	}
	published := NativeDivisorProperty(property)
	return published, published.Available() && property.Valid()
}

// nativePublicationContent is one native row's typed published content. Every
// published distinction is a column here; nothing about a row is carried as a
// rendered string, so a row's identity is its content and a consumer reads a
// fact instead of parsing one back out of a sentence.
type nativePublicationContent struct {
	// exact records that the published carrier is the value's exact carrier
	// rather than a widened one.
	exact bool
	// literal is the proved constant, kind and exact bits. Inf, NaN, and
	// signed zero are ordinary members: the bits are the fact.
	literal keyspace.LiteralValue
	// scalar is the carrier of a proved exact scalar, including Lua nil, which
	// carries no literal.
	scalar NativeScalarRepresentation
	// representation, left, right, and operand are the proved numeric
	// carriers of an arithmetic result and its operands.
	representation, left, right, operand programschema.NumericRepresentation
	// binary and unary are the proved operator. A row publishes at most one.
	binary flowkind.BinaryOp
	unary  flowkind.UnaryOp
	// overflow is the arithmetic discipline the operator evaluates under.
	overflow valuedomain.NumericOverflow
	// divisor is the divisor proof an integer division carries.
	divisor NativeDivisorProperty
	// truthiness, partition, deadArm, and deadArmReachable are the branch
	// condition's verdict over its whole evidence set.
	truthiness       NativeTruthinessClass
	partition        NativeBranchPartition
	deadArm          NativeBranchArm
	deadArmReachable bool
	// sendSafety is the proved allocation-level transfer strategy. VerdictNone
	// means the send-safety column is absent; no default verdict is invented.
	sendSafety sendsafety.Verdict
	// points is the evidence set the row was folded over, in ascending
	// identity order. It is never a single representative of a larger set.
	points []identity.ContentID
}

func (content nativePublicationContent) literalAvailable() bool {
	return content.literal.Kind >= keyspace.LiteralBool && content.literal.Kind <= keyspace.LiteralString
}

// valid states the content law of each published family: which columns the
// family publishes, and that no row carries a column its family does not
// declare. It replaces the rendered value's only former check, that the
// sentence was not empty.
func (content nativePublicationContent) valid(family nativePublicationFamily) bool {
	if len(content.points) == 0 {
		return false
	}
	for index, point := range content.points {
		if !point.Available() || index > 0 && !contentIDLess(content.points[index-1], point) {
			return false
		}
	}
	if content.deadArm.Available() != (content.partition == NativeBranchPartitionAlwaysTaken || content.partition == NativeBranchPartitionAlwaysNotTaken) {
		return false
	}
	if !content.deadArm.Available() && content.deadArmReachable {
		return false
	}
	if content.binary != 0 && content.unary != 0 {
		return false
	}
	// Send safety is a column owned exclusively by its family. In particular,
	// an unknown nonzero enum value must not become an absent column on another
	// family merely because it is not currently in the verdict catalog.
	if family != nativePublicationFamilySendSafety && content.sendSafety != sendsafety.VerdictNone {
		return false
	}
	if content.scalar.Available() && content.representation != programschema.NumericRepresentationInvalid {
		return false
	}
	// A published literal is the constant of a published carrier, and Lua nil
	// carries none. A carrier without a literal is the carrier fact on its own.
	if content.literalAvailable() && (!content.scalar.Available() || content.scalar == NativeScalarRepresentationNil) {
		return false
	}
	switch family {
	case nativePublicationFamilyConstantValue:
		return content.scalar.Available() && content.literalAvailable() != (content.scalar == NativeScalarRepresentationNil) && !content.exact && content.numericColumnsAbsent() && content.operatorColumnsAbsent() && content.branchColumnsAbsent()
	case nativePublicationFamilyRepresentation:
		if content.scalar.Available() {
			return content.exact && !content.literalAvailable() && content.numericColumnsAbsent() && content.operatorColumnsAbsent() && content.branchColumnsAbsent()
		}
		return content.representation.Valid() && content.operatorPublished() && content.divisor == NativeDivisorPropertyInvalid && content.branchColumnsAbsent()
	case nativePublicationFamilyScalarOperator:
		return content.representation.Valid() && content.left.Valid() && content.right.Valid() && content.operand == programschema.NumericRepresentationInvalid &&
			flowkind.IsBinaryArithmetic(content.binary) && content.overflow.Valid() && !content.exact && !content.scalar.Available() && content.branchColumnsAbsent()
	case nativePublicationFamilyDivisorProperty:
		return content.divisor.Available() && flowkind.IsBinaryArithmetic(content.binary) && content.overflow == valuedomain.NumericOverflowInvalid &&
			!content.exact && !content.scalar.Available() && content.numericColumnsAbsent() && content.branchColumnsAbsent()
	case nativePublicationFamilyTruthinessClass:
		return content.truthiness.Available() && content.partition == NativeBranchPartitionInvalid && !content.deadArm.Available() &&
			content.scalarColumnsAbsent() && content.numericColumnsAbsent() && content.operatorColumnsAbsent()
	case nativePublicationFamilyBranchPartition:
		return content.partition.Available() && content.truthiness == NativeTruthinessClassInvalid &&
			content.scalarColumnsAbsent() && content.numericColumnsAbsent() && content.operatorColumnsAbsent()
	case nativePublicationFamilySendSafety:
		return content.sendSafety.Available() && content.exact == false && content.literal == (keyspace.LiteralValue{}) &&
			content.scalarColumnsAbsent() && content.numericColumnsAbsent() && content.operatorColumnsAbsent() && content.branchColumnsAbsent()
	default:
		return false
	}
}

// operatorPublished states the representation family's two operator shapes: a
// binary arithmetic result over two operand carriers, or a unary result over
// one.
func (content nativePublicationContent) operatorPublished() bool {
	if flowkind.IsBinaryArithmetic(content.binary) {
		return content.left.Valid() && content.right.Valid() && content.operand == programschema.NumericRepresentationInvalid && content.overflow.Valid()
	}
	return content.unary == flowkind.UnaryNeg && content.operand.Valid() &&
		content.left == programschema.NumericRepresentationInvalid && content.right == programschema.NumericRepresentationInvalid && content.overflow.Valid()
}

func (content nativePublicationContent) scalarColumnsAbsent() bool {
	return !content.scalar.Available() && !content.literalAvailable() && !content.exact
}

func (content nativePublicationContent) numericColumnsAbsent() bool {
	return content.representation == programschema.NumericRepresentationInvalid && content.left == programschema.NumericRepresentationInvalid &&
		content.right == programschema.NumericRepresentationInvalid && content.operand == programschema.NumericRepresentationInvalid
}

func (content nativePublicationContent) operatorColumnsAbsent() bool {
	return content.binary == 0 && content.unary == 0 && content.overflow == valuedomain.NumericOverflowInvalid && content.divisor == NativeDivisorPropertyInvalid
}

func (content nativePublicationContent) branchColumnsAbsent() bool {
	return content.truthiness == NativeTruthinessClassInvalid && content.partition == NativeBranchPartitionInvalid &&
		!content.deadArm.Available() && !content.deadArmReachable
}

// column resolves one vocabulary-valued column to the ordinal of the member
// the row publishes there, or reports that the row publishes no such column.
func (content nativePublicationContent) column(column NativePublicationColumn) (uint16, bool) {
	switch column {
	case NativePublicationColumnScalarRepresentation:
		return content.scalar.Ordinal(), content.scalar.Available()
	case NativePublicationColumnRepresentation:
		return uint16(content.representation), content.representation.Valid()
	case NativePublicationColumnLeft:
		return uint16(content.left), content.left.Valid()
	case NativePublicationColumnRight:
		return uint16(content.right), content.right.Valid()
	case NativePublicationColumnOperand:
		return uint16(content.operand), content.operand.Valid()
	case NativePublicationColumnBinaryOperator:
		return uint16(content.binary), flowkind.IsBinaryArithmetic(content.binary)
	case NativePublicationColumnUnaryOperator:
		return uint16(content.unary), content.unary != 0
	case NativePublicationColumnOverflow:
		return uint16(content.overflow), content.overflow.Valid()
	case NativePublicationColumnDivisor:
		return content.divisor.Ordinal(), content.divisor.Available()
	case NativePublicationColumnTruthiness:
		return content.truthiness.Ordinal(), content.truthiness.Available()
	case NativePublicationColumnPartition:
		return content.partition.Ordinal(), content.partition.Available()
	case NativePublicationColumnDeadArm:
		return content.deadArm.Ordinal(), content.deadArm.Available()
	case NativePublicationColumnSendSafety:
		return content.sendSafety.Ordinal(), content.sendSafety.Available()
	default:
		return 0, false
	}
}

// contentParts writes the typed content as the ordered, framed preimage of the
// row's identity. Every column is its own frame, so two rows are the same row
// exactly when their columns agree, independently of how any of them renders.
func (content nativePublicationContent) contentParts() [][]byte {
	var columns [17]byte
	columns[0] = boolByte(content.exact)
	columns[1] = uint8(content.literal.Kind)
	columns[2] = boolByte(content.literal.Bool)
	columns[3] = uint8(content.scalar)
	columns[4] = uint8(content.representation)
	columns[5] = uint8(content.left)
	columns[6] = uint8(content.right)
	columns[7] = uint8(content.operand)
	columns[8] = uint8(content.binary)
	columns[9] = uint8(content.unary)
	columns[10] = uint8(content.overflow)
	columns[11] = uint8(content.divisor)
	columns[12] = uint8(content.truthiness)
	columns[13] = uint8(content.partition)
	columns[14] = uint8(content.deadArm)
	columns[15] = boolByte(content.deadArmReachable)
	columns[16] = uint8(content.sendSafety)
	var payload [16]byte
	binary.BigEndian.PutUint64(payload[0:8], uint64(content.literal.Integer))
	binary.BigEndian.PutUint64(payload[8:16], content.literal.FloatBits)
	parts := make([][]byte, 0, 4+len(content.points))
	parts = append(parts, columns[:], payload[:], []byte(content.literal.String))
	for index := range content.points {
		parts = append(parts, content.points[index][:])
	}
	return parts
}

func contentIDLess(left, right identity.ContentID) bool {
	return bytes.Compare(left[:], right[:]) < 0
}

// nativeEvidencePoints is the row's evidence set: the points whose published
// observations the row was folded over, ordered by identity and free of
// repeats.
func nativeEvidencePoints(points ...identity.ContentID) ([]identity.ContentID, bool) {
	if len(points) == 0 {
		return nil, false
	}
	ordered := make([]identity.ContentID, 0, len(points))
	for _, point := range points {
		if !point.Available() {
			return nil, false
		}
		ordered = append(ordered, point)
	}
	identity.SortContentIDs(ordered)
	deduplicated := ordered[:1]
	for _, point := range ordered[1:] {
		if point != deduplicated[len(deduplicated)-1] {
			deduplicated = append(deduplicated, point)
		}
	}
	return deduplicated, true
}

// Category is the declared vocabulary a published column's member belongs to.
// A renderer resolves the member at the column's ordinal in this category and
// reads its declared spelling.
func (column NativePublicationColumn) Category() structure.Category {
	switch column {
	case NativePublicationColumnScalarRepresentation:
		return structure.CategoryNativeScalarRepresentation
	case NativePublicationColumnRepresentation, NativePublicationColumnLeft, NativePublicationColumnRight, NativePublicationColumnOperand:
		return structure.CategoryNativeNumericRepresentation
	case NativePublicationColumnBinaryOperator:
		return structure.CategoryNativeArithmeticOperator
	case NativePublicationColumnUnaryOperator:
		return structure.CategoryNativeUnaryOperator
	case NativePublicationColumnOverflow:
		return structure.CategoryNativeNumericOverflow
	case NativePublicationColumnDivisor:
		return structure.CategoryNativeDivisorProperty
	case NativePublicationColumnTruthiness:
		return structure.CategoryNativeTruthinessClass
	case NativePublicationColumnPartition:
		return structure.CategoryNativeBranchPartition
	case NativePublicationColumnDeadArm:
		return structure.CategoryNativeBranchArm
	case NativePublicationColumnSendSafety:
		return structure.CategoryNativeSendSafety
	default:
		return structure.CategoryInvalid
	}
}

// NativePublicationColumn is the closed set of vocabulary-valued columns a
// native row publishes. It is what a renderer iterates: each member names the
// declared vocabulary its value belongs to, so rendering a row is resolving
// each present column's member and reading its declared spelling.
type NativePublicationColumn uint8

const (
	NativePublicationColumnInvalid NativePublicationColumn = iota
	NativePublicationColumnScalarRepresentation
	NativePublicationColumnRepresentation
	NativePublicationColumnLeft
	NativePublicationColumnRight
	NativePublicationColumnOperand
	NativePublicationColumnBinaryOperator
	NativePublicationColumnUnaryOperator
	NativePublicationColumnOverflow
	NativePublicationColumnDivisor
	NativePublicationColumnTruthiness
	NativePublicationColumnPartition
	NativePublicationColumnDeadArm
	NativePublicationColumnSendSafety
	nativePublicationColumnLimit
)

func (column NativePublicationColumn) Available() bool {
	return column > NativePublicationColumnInvalid && column < nativePublicationColumnLimit
}
