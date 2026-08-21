package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
)

// OccurrenceKind is the closed, domain-neutral Program semantic occurrence
// vocabulary. The compiler emits one parent row and dense point/input child
// rows; no consumer receives a compiler-owned slice or a family index.
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
	OccurrenceCallActivation
	OccurrenceCallBoundary
	OccurrenceCallArm
	OccurrenceCallArgument
	OccurrenceCallTypeArgument
	OccurrenceUnary
	OccurrenceSelect
	OccurrenceValueClaim
	OccurrenceBinaryArithmetic
	OccurrenceBinaryEquality
	OccurrenceBinaryOrder
	OccurrenceBinaryPresenceRefinement
	OccurrenceReturnBoundary
	OccurrenceFormalEntry
	OccurrenceOperationPredicateRefinement
)

func (kind OccurrenceKind) Valid() bool {
	return kind >= OccurrencePointAttachment && kind <= OccurrenceOperationPredicateRefinement
}

// occurrenceOutputOperand names the operand position at which a family whose
// own identity is not the value it establishes carries that value. A storage
// transfer, a storage write and an index read carry the operation under their
// own identity; an allocation carries its reusable template. Each therefore
// names the value it establishes in the sealed operand vector instead.
func occurrenceOutputOperand(kind OccurrenceKind) (int, bool) {
	switch kind {
	case OccurrenceStorageBindTransfer, OccurrenceStorageWrite, OccurrenceIndexRead:
		return 2, true
	case OccurrenceAllocation:
		return occurrenceAllocationOutputOperand, true
	}
	return 0, false
}

// occurrenceAllocationOutputOperand is the position at which an allocation
// names the value it establishes: the owner-issued allocation identity that
// follows its template.
const occurrenceAllocationOutputOperand = 1

// SpanResultOccurrence reports whether the family's result identity is the
// operator's own program-owned span rather than a semantic occurrence.
func SpanResultOccurrence(kind OccurrenceKind) bool {
	return kind == OccurrenceBinaryArithmetic || kind == OccurrenceBinaryEquality || kind == OccurrenceBinaryOrder
}

// RuleStage is the closed reusable execution cut owned by Program. Its
// ordinals are part of the sealed schema vocabulary and are not Link-derived.
type RuleStage uint8

const (
	RuleStageInvalid RuleStage = iota
	RuleStageBase
	RuleStageLocal
	RuleStageCallDispatch
	RuleStageCallSummary
	RuleStageCallEffect
)

func (stage RuleStage) Valid() bool { return stage >= RuleStageBase && stage <= RuleStageCallEffect }

// RuleInputKind preserves the owner-issued placement polarity of a rule input.
type RuleInputKind uint8

const (
	RuleInputInvalid RuleInputKind = iota
	RuleInputNone
	RuleInputFinish
	RuleInputEntry
	RuleInputPredecessor
)

func (kind RuleInputKind) Valid() bool { return kind >= RuleInputNone && kind <= RuleInputPredecessor }

// Occurrence is one immutable parent row. Point and input memberships are
// dense child planes named by half-open spans. The parent does not retain
// slices, maps, or indexes.
type Occurrence struct {
	kind          OccurrenceKind
	id            identity.ContentID
	body          identity.ContentID
	code          uint64
	pointOffset   uint32
	pointCount    uint32
	inputOffset   uint32
	inputCount    uint32
	literalFamily keyspace.Family
	literal       keyspace.LiteralValue
	literalOK     bool
}

func NewOccurrence(kind OccurrenceKind, id, body identity.ContentID, code uint64, pointOffset, pointCount, inputOffset, inputCount uint32, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) (Occurrence, bool) {
	row := Occurrence{kind: kind, id: id, body: body, code: code, pointOffset: pointOffset, pointCount: pointCount, inputOffset: inputOffset, inputCount: inputCount, literalFamily: literalFamily, literal: literal, literalOK: literalOK}
	return row, row.Available()
}

func (row Occurrence) Available() bool {
	if !row.kind.Valid() || !row.id.Available() || row.code == ^uint64(0) {
		return false
	}
	if uint64(row.pointOffset)+uint64(row.pointCount) > uint64(^uint32(0)) || uint64(row.inputOffset)+uint64(row.inputCount) > uint64(^uint32(0)) {
		return false
	}
	if row.literalOK && (row.kind != OccurrenceValueSource || row.literalFamily == keyspace.FamilyInvalid) {
		return false
	}
	if row.kind == OccurrenceValueSource && row.inputCount != 1 {
		return false
	}
	if row.kind == OccurrenceStorageRead && row.inputCount != 2 {
		return false
	}
	if operand, named := occurrenceOutputOperand(row.kind); named && uint64(row.inputCount) <= uint64(operand) {
		return false
	}
	if row.kind == OccurrencePointAttachment && (row.body.Available() || row.pointCount != 1 || row.inputCount != 1 || row.code != 0) {
		return false
	}
	if row.kind == OccurrenceBinaryEquality && row.inputCount != 2 && row.inputCount != 5 {
		return false
	}
	if row.kind == OccurrenceBinaryArithmetic && row.inputCount != 2 {
		return false
	}
	if row.kind == OccurrenceBinaryOrder && row.inputCount != 2 {
		return false
	}
	if row.kind == OccurrenceBinaryPresenceRefinement && (!row.body.Available() || row.pointCount != 1 || row.inputCount != 4) {
		return false
	}
	if row.kind == OccurrenceOperationPredicateRefinement && (!row.body.Available() || row.pointCount != 1 || row.inputCount != 4) {
		return false
	}
	return true
}

func (row Occurrence) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row Occurrence) Kind() OccurrenceKind {
	if !row.Available() {
		return OccurrenceInvalid
	}
	return row.kind
}
func (row Occurrence) Code() uint64 {
	if !row.Available() {
		return 0
	}
	return row.code
}
func (row Occurrence) BodyID() (identity.ContentID, bool) {
	return row.body, row.Available() && row.body.Available()
}
func (row Occurrence) PointSpan() (uint32, uint32, bool) {
	return row.pointOffset, row.pointCount, row.Available()
}
func (row Occurrence) InputSpan() (uint32, uint32, bool) {
	return row.inputOffset, row.inputCount, row.Available()
}
func (row Occurrence) Literal() (keyspace.Family, keyspace.LiteralValue, bool) {
	return row.literalFamily, row.literal, row.Available() && row.literalOK
}

// OccurrencePoint is one ordered point attachment owned by an Occurrence row.
type OccurrencePoint struct{ point identity.ContentID }

func NewOccurrencePoint(point identity.ContentID) (OccurrencePoint, bool) {
	row := OccurrencePoint{point: point}
	return row, row.Available()
}
func (row OccurrencePoint) Available() bool { return row.point.Available() }
func (row OccurrencePoint) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}

// OccurrenceInput is one ordered semantic operand owned by an Occurrence row.
type OccurrenceInput struct{ input identity.ContentID }

func NewOccurrenceInput(input identity.ContentID) (OccurrenceInput, bool) {
	row := OccurrenceInput{input: input}
	return row, row.Available()
}
func (row OccurrenceInput) Available() bool { return row.input.Available() }
func (row OccurrenceInput) InputID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.input
}

// RuleOccurrence is one immutable placement joined to an occurrence ordinal.
// The occurrence row remains the sole owner of semantic operands; this row
// carries only placement metadata and the dense parent ordinal.
type RuleOccurrence struct {
	key        schema.Key
	writes     schema.Key
	occurrence uint32
	point      identity.ContentID
	input      identity.ContentID
	stage      RuleStage
	inputKind  RuleInputKind
	route      identity.ContentID
}

func NewRuleOccurrence(key, writes schema.Key, occurrence uint32, point, input identity.ContentID, stage RuleStage, inputKind RuleInputKind, route identity.ContentID) (RuleOccurrence, bool) {
	row := RuleOccurrence{key: key, writes: writes, occurrence: occurrence, point: point, input: input, stage: stage, inputKind: inputKind, route: route}
	return row, row.Available()
}
func (row RuleOccurrence) Available() bool {
	if !row.key.Available() || !row.writes.Available() || !row.point.Available() || !row.stage.Valid() || !row.inputKind.Valid() || row.occurrence == ^uint32(0) {
		return false
	}
	if (row.inputKind == RuleInputNone) == row.input.Available() {
		return false
	}
	if row.inputKind == RuleInputPredecessor {
		return row.route.Available()
	}
	return !row.route.Available()
}
func (row RuleOccurrence) Key() schema.Key {
	if !row.Available() {
		return ""
	}
	return row.key
}

// Writes is the declared axis this placement's rule writes. Several rules are
// placed on one occurrence, and only the ones writing an axis establish that
// axis' result there, so a consumer that reads one axis' result at the points
// producing it separates the placements by this key rather than by the
// occurrence they share.
func (row RuleOccurrence) Writes() schema.Key {
	if !row.Available() {
		return ""
	}
	return row.writes
}
func (row RuleOccurrence) Occurrence() (uint32, bool) { return row.occurrence, row.Available() }
func (row RuleOccurrence) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}
func (row RuleOccurrence) InputPoint() (identity.ContentID, bool) {
	return row.input, row.Available() && row.inputKind != RuleInputNone
}
func (row RuleOccurrence) InputKind() RuleInputKind {
	if !row.Available() {
		return RuleInputInvalid
	}
	return row.inputKind
}
func (row RuleOccurrence) Stage() RuleStage {
	if !row.Available() {
		return RuleStageInvalid
	}
	return row.stage
}
func (row RuleOccurrence) PredecessorRouteID() (identity.ContentID, bool) {
	return row.route, row.Available() && row.inputKind == RuleInputPredecessor
}
