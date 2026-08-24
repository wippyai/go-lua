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
	// OccurrenceBinaryConcat is the structural occurrence of a `..` term. The
	// operation carries no representation lattice, so it has no binary
	// primitive projection; the row names the operand pair and the evaluation
	// span under which the concatenation is reached.
	OccurrenceBinaryConcat
	// OccurrenceSubjectLiveness is the Program owner's executable view of one
	// lifecycle.SubjectLiveness row. Identity remains the lifecycle row ID;
	// its sole finish Point and Call input are owner-issued references, not a
	// consumer-side reconstruction.
	OccurrenceSubjectLiveness
)

func (kind OccurrenceKind) Valid() bool {
	return kind >= OccurrencePointAttachment && kind <= OccurrenceSubjectLiveness
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
	if row.kind == OccurrenceBinaryConcat && (!row.body.Available() || row.inputCount != 2) {
		return false
	}
	if row.kind == OccurrenceSubjectLiveness && (row.body.Available() || row.pointCount != 1 || row.inputCount != 1 || row.code != 0) {
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
	// inputs is the ordered point-role vector issued by the sealed Program
	// form. It is fixed-width rather than a retained slice so the parent row
	// remains an immutable value; issuance instructions have the same six
	// operand bound. inputCount distinguishes an empty vector from an
	// unavailable slot.
	inputs     [6]identity.ContentID
	inputCount uint8
	stage      schema.Key
	inputSpec  schema.Key
	route      RuleOccurrenceRoute
	native     bool
	source     RuleOccurrenceSource
}

// RuleOccurrenceSource is the candidate row issuance resolved for one placed
// rule: the Program row space the rule's candidates live in and the dense
// ordinal of its own row there. Ordinal zero is a real row, so the space is
// what states presence. The pair is one value because an ordinal without its
// space addresses nothing.
type RuleOccurrenceSource struct {
	Space   schema.Key
	Ordinal uint32
}

// Available reports whether a source was resolved.
func (source RuleOccurrenceSource) Available() bool { return source.Space.Available() }

// RuleOccurrenceRoute is the exact authored control-flow predecessor proof
// for a routed rule placement. Point is the route landing; ID identifies the
// sealed environment edge. It is distinct from the rule's data inputs.
type RuleOccurrenceRoute struct {
	Point identity.ContentID
	ID    identity.ContentID
}

// Available reports whether both halves of the routed predecessor proof are
// present. The zero value states that the placement is not route-bound.
func (route RuleOccurrenceRoute) Available() bool {
	return route.Point.Available() && route.ID.Available()
}

// NewRuleOccurrenceWithInputs seals one ordered point-role vector. The vector
// is copied into the row's fixed dense storage; callers cannot mutate the
// published placement after construction.
func NewRuleOccurrenceWithInputs(key, writes schema.Key, occurrence uint32, point identity.ContentID, inputs []identity.ContentID, stage, inputSpec schema.Key, route RuleOccurrenceRoute, native bool, source RuleOccurrenceSource) (RuleOccurrence, bool) {
	row := RuleOccurrence{key: key, writes: writes, occurrence: occurrence, point: point, stage: stage, inputSpec: inputSpec, route: route, native: native, source: source}
	if len(inputs) > len(row.inputs) {
		return RuleOccurrence{}, false
	}
	row.inputCount = uint8(len(inputs))
	for index, input := range inputs {
		if !input.Available() {
			return RuleOccurrence{}, false
		}
		row.inputs[index] = input
	}
	return row, row.Available()
}
func (row RuleOccurrence) Available() bool {
	if !row.key.Available() || !row.writes.Available() || !row.point.Available() || !row.stage.Available() || !row.inputSpec.Available() || row.occurrence == ^uint32(0) {
		return false
	}
	if row.inputCount > uint8(len(row.inputs)) {
		return false
	}
	for index := uint8(0); index < row.inputCount; index++ {
		if !row.inputs[index].Available() {
			return false
		}
	}
	if row.inputCount == 0 {
		return !row.route.Point.Available() && !row.route.ID.Available() && !row.native
	}
	if row.route.Point.Available() != row.route.ID.Available() {
		return false
	}
	return true
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

// InputPointCount is the sealed width of this placement's ordered point-role
// vector. It returns zero for an unavailable row.
func (row RuleOccurrence) InputPointCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.inputCount)
}

// InputPointAt resolves one exact point role by ordinal. No scan or fallback
// is permitted: an out-of-range ordinal is unavailable.
func (row RuleOccurrence) InputPointAt(index int) (identity.ContentID, bool) {
	if index < 0 || !row.Available() || index >= int(row.inputCount) {
		return identity.ContentID{}, false
	}
	return row.inputs[index], true
}

func (row RuleOccurrence) InputSpec() schema.Key {
	if !row.Available() {
		return ""
	}
	return row.inputSpec
}
func (row RuleOccurrence) Stage() schema.Key {
	if !row.Available() {
		return ""
	}
	return row.stage
}
func (row RuleOccurrence) Native() (bool, bool) { return row.native, row.Available() }

// Source is the candidate row issuance resolved for this placement, and false
// when the rule draws its candidates from somewhere other than a Program row
// space.
func (row RuleOccurrence) Source() (RuleOccurrenceSource, bool) {
	return row.source, row.Available() && row.source.Available()
}
func (row RuleOccurrence) PredecessorRoute() (RuleOccurrenceRoute, bool) {
	return row.route, row.Available() && row.route.Available()
}
