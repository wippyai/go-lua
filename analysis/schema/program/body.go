package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// Body, Outcome, and their ordered child planes extend the append-only cold
// catalog. A parent owns each child by both an explicit parent identity and a
// half-open span, so attachment and order are facts of the publication.
const (
	slotBody               = slotWTOEvent + 1
	slotBodyEntry          = slotBody + 1
	slotBodyRoot           = slotBodyEntry + 1
	slotOutcome            = slotBodyRoot + 1
	slotOutcomeReturnValue = slotOutcome + 1
	slotOutcomePoint       = slotOutcomeReturnValue + 1
)

func BodyFamily() Family[Body] { return Family[Body]{slot: slotBody, name: "body"} }
func BodyEntryFamily() Family[BodyEntry] {
	return Family[BodyEntry]{slot: slotBodyEntry, name: "body-entry"}
}
func BodyRootFamily() Family[BodyRoot] {
	return Family[BodyRoot]{slot: slotBodyRoot, name: "body-root"}
}
func OutcomeFamily() Family[Outcome] { return Family[Outcome]{slot: slotOutcome, name: "outcome"} }
func OutcomeReturnValueFamily() Family[OutcomeReturnValue] {
	return Family[OutcomeReturnValue]{slot: slotOutcomeReturnValue, name: "outcome-return-value"}
}
func OutcomePointFamily() Family[OutcomePoint] {
	return Family[OutcomePoint]{slot: slotOutcomePoint, name: "outcome-point"}
}

// BodyEntry is one ordered point membership of a Body's entry site.
type BodyEntry struct{ body, point identity.ContentID }

func NewBodyEntry(body, point identity.ContentID) (BodyEntry, bool) {
	row := BodyEntry{body: body, point: point}
	return row, row.Available()
}
func (row BodyEntry) Available() bool { return row.body.Available() && row.point.Available() }
func (row BodyEntry) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row BodyEntry) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}
func (row BodyEntry) ID() identity.ContentID { return row.PointID() }

// BodyRoot is one ordered executable-root identity and its neutral Program
// family ordinal. The ordinal is intentionally a byte: cold does not import
// or reinterpret the Program vocabulary it preserves.
type BodyRoot struct {
	body   identity.ContentID
	id     identity.ContentID
	family uint8
}

func NewBodyRoot(body, id identity.ContentID, family uint8) (BodyRoot, bool) {
	row := BodyRoot{body: body, id: id, family: family}
	return row, row.Available()
}
func (row BodyRoot) Available() bool {
	return row.body.Available() && row.id.Available() && row.family != 0
}
func (row BodyRoot) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row BodyRoot) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row BodyRoot) Family() uint8 {
	if !row.Available() {
		return 0
	}
	return row.family
}

// Body is one immutable lexical body boundary. Its three spans address the
// entry, root, and outcome families; none of those ordered children is
// retained behind a slice header.
type Body struct {
	id, context, entry, function, formal identity.ContentID
	entryOffset, entryCount              uint32
	rootOffset, rootCount                uint32
	outcomeOffset, outcomeCount          uint32
	callable                             bool
}

func NewBody(
	id, context, entry, function, formal identity.ContentID,
	entryOffset, entryCount, rootOffset, rootCount, outcomeOffset, outcomeCount uint32,
	callable bool,
) (Body, bool) {
	row := Body{
		id: id, context: context, entry: entry, function: function, formal: formal,
		entryOffset: entryOffset, entryCount: entryCount, rootOffset: rootOffset, rootCount: rootCount,
		outcomeOffset: outcomeOffset, outcomeCount: outcomeCount, callable: callable,
	}
	return row, row.Available()
}
func (row Body) Available() bool {
	return row.id.Available() && row.context.Available() && row.entry.Available() &&
		row.entryCount != 0 && row.outcomeCount != 0 &&
		((row.callable && row.function.Available() && row.formal.Available()) ||
			(!row.callable && !row.function.Available() && !row.formal.Available())) &&
		uint64(row.entryOffset)+uint64(row.entryCount) <= uint64(^uint32(0)) &&
		uint64(row.rootOffset)+uint64(row.rootCount) <= uint64(^uint32(0)) &&
		uint64(row.outcomeOffset)+uint64(row.outcomeCount) <= uint64(^uint32(0))
}
func (row Body) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row Body) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}
func (row Body) EntryID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.entry
}
func (row Body) Callable() bool                                { return row.Available() && row.callable }
func (row Body) FunctionContextID() (identity.ContentID, bool) { return row.function, row.Callable() }
func (row Body) CallFormalID() (identity.ContentID, bool)      { return row.formal, row.Callable() }
func (row Body) EntrySpan() (uint32, uint32, bool) {
	return row.entryOffset, row.entryCount, row.Available()
}
func (row Body) RootSpan() (uint32, uint32, bool) {
	return row.rootOffset, row.rootCount, row.Available()
}
func (row Body) OutcomeSpan() (uint32, uint32, bool) {
	return row.outcomeOffset, row.outcomeCount, row.Available()
}
func (row Body) EntryCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.entryCount)
}
func (row Body) RootCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.rootCount)
}
func (row Body) OutcomeCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.outcomeCount)
}

// OutcomeKind is the closed neutral terminal vocabulary. Its ordinals are the
// historical Program outcome ordinals and are part of Artifact identity.
type OutcomeKind uint8

const (
	OutcomeInvalid OutcomeKind = iota
	OutcomeNormal
	OutcomeReturn
	OutcomeThrow
	OutcomeBreak
	OutcomeGoto
	OutcomeYield
	OutcomeCancel
)

func (kind OutcomeKind) Valid() bool { return kind >= OutcomeNormal && kind <= OutcomeCancel }

// OutcomeReturnValue retains one Values identity occurrence under a Return
// outcome. Repetition is meaningful and therefore remains positional.
type OutcomeReturnValue struct{ outcome, value identity.ContentID }

func NewOutcomeReturnValue(outcome, value identity.ContentID) (OutcomeReturnValue, bool) {
	row := OutcomeReturnValue{outcome: outcome, value: value}
	return row, row.Available()
}
func (row OutcomeReturnValue) Available() bool {
	return row.outcome.Available() && row.value.Available()
}
func (row OutcomeReturnValue) OutcomeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.outcome
}
func (row OutcomeReturnValue) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.value
}
func (row OutcomeReturnValue) ID() identity.ContentID { return row.ValuesID() }

// OutcomePoint retains one ordered LocalWTO point membership.
type OutcomePoint struct{ outcome, point identity.ContentID }

func NewOutcomePoint(outcome, point identity.ContentID) (OutcomePoint, bool) {
	row := OutcomePoint{outcome: outcome, point: point}
	return row, row.Available()
}
func (row OutcomePoint) Available() bool { return row.outcome.Available() && row.point.Available() }
func (row OutcomePoint) OutcomeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.outcome
}
func (row OutcomePoint) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}
func (row OutcomePoint) ID() identity.ContentID { return row.PointID() }

// Outcome is one immutable Body-owned semantic terminal. Optional target and
// propagation identities retain their exact shape; return values and point
// memberships are spans in their canonical child families.
type Outcome struct {
	id, body, target, propagation                      identity.ContentID
	returnOffset, returnCount, pointOffset, pointCount uint32
	kind                                               OutcomeKind
	hasTarget, hasPropagation                          bool
}

func NewOutcome(
	id, body, target, propagation identity.ContentID, kind OutcomeKind,
	returnOffset, returnCount, pointOffset, pointCount uint32,
	hasTarget, hasPropagation bool,
) (Outcome, bool) {
	row := Outcome{id: id, body: body, target: target, propagation: propagation, kind: kind,
		returnOffset: returnOffset, returnCount: returnCount, pointOffset: pointOffset, pointCount: pointCount,
		hasTarget: hasTarget, hasPropagation: hasPropagation}
	return row, row.Available()
}
func (row Outcome) Available() bool {
	if !row.id.Available() || !row.body.Available() || !row.kind.Valid() ||
		row.hasPropagation != row.propagation.Available() ||
		uint64(row.returnOffset)+uint64(row.returnCount) > uint64(^uint32(0)) ||
		uint64(row.pointOffset)+uint64(row.pointCount) > uint64(^uint32(0)) {
		return false
	}
	if row.kind == OutcomeBreak || row.kind == OutcomeGoto {
		if !row.hasTarget || !row.target.Available() {
			return false
		}
	} else if row.hasTarget || row.target.Available() {
		return false
	}
	return row.returnCount == 0 || row.kind == OutcomeReturn
}
func (row Outcome) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row Outcome) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row Outcome) Kind() OutcomeKind {
	if !row.Available() {
		return OutcomeInvalid
	}
	return row.kind
}
func (row Outcome) TargetID() (identity.ContentID, bool) {
	return row.target, row.Available() && row.hasTarget
}
func (row Outcome) PropagationID() (identity.ContentID, bool) {
	return row.propagation, row.Available() && row.hasPropagation
}
func (row Outcome) ReturnValueSpan() (uint32, uint32, bool) {
	return row.returnOffset, row.returnCount, row.Available()
}
func (row Outcome) PointSpan() (uint32, uint32, bool) {
	return row.pointOffset, row.pointCount, row.Available()
}
func (row Outcome) ReturnValueCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.returnCount)
}
func (row Outcome) PointCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.pointCount)
}
