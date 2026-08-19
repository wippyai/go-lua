package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// RuleStage is the closed reusable execution cut owned by the Program
// artifact. Base is the parent point. Local is the
// domain-neutral post-occurrence cut used by rules which must read the
// pre-result environment and write a distinct result environment. CallDispatch,
// CallSummary, and CallEffect are the ordered native call lattice; each is a
// distinct synthetic point and no rule is allowed to collapse the lattice
// back onto its base Finish point.
type RuleStage uint8

const (
	RuleStageInvalid RuleStage = iota
	RuleStageBase
	RuleStageLocal
	RuleStageCallDispatch
	RuleStageCallSummary
	RuleStageCallEffect
)

func (stage RuleStage) valid() bool { return stage >= RuleStageBase && stage <= RuleStageCallEffect }

// RuleInputKind preserves the owner-issued placement polarity of a rule input.
// None is lawful only for source rules; Entry, Finish, and the guarded route's
// destination predecessor are exact Program proof roles, never Link inference.
type RuleInputKind uint8

const (
	RuleInputInvalid RuleInputKind = iota
	RuleInputNone
	RuleInputFinish
	RuleInputEntry
	RuleInputPredecessor
)

func (kind RuleInputKind) valid() bool { return kind >= RuleInputNone && kind <= RuleInputPredecessor }

type RuleOccurrence struct {
	key        schema.Key
	occurrence uint32
	point      identity.ContentID
	input      identity.ContentID
	stage      RuleStage
	inputKind  RuleInputKind
	route      identity.ContentID
}

func (row RuleOccurrence) Available() bool {
	if !row.key.Available() || !row.point.Available() || !row.stage.valid() || !row.inputKind.valid() {
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

// RuleOccurrenceRow is the immutable placement joined to its exact semantic
// occurrence. Only stable semantic IDs and point IDs escape.
type RuleOccurrenceRow struct {
	placement RuleOccurrence
	row       OccurrenceRow
}

func (row RuleOccurrenceRow) Available() bool {
	return row.placement.Available() && row.row.Available()
}
func (row RuleOccurrenceRow) Key() schema.Key { return row.placement.Key() }
func (row RuleOccurrenceRow) OccurrenceKind() OccurrenceKind {
	if !row.Available() {
		return OccurrenceInvalid
	}
	return row.row.Kind()
}

// OutputSemanticID returns the exact Program-issued semantic value written by
// a placement when that relation is already retained by its occurrence row.
// It never equates the occurrence identity with its output: storage writes
// and index reads name their destination explicitly in the sealed operand
// vector. Placements whose occurrence family does not write a value return false.
func (row RuleOccurrenceRow) OutputSemanticID() (identity.ContentID, bool) {
	if !row.Available() {
		return identity.ContentID{}, false
	}
	switch row.OccurrenceKind() {
	case OccurrenceValueSource, OccurrenceFormalEntry:
		return row.row.ID(), true
	case OccurrenceStorageRead:
		return row.row.ID(), true
	case OccurrenceStorageBindTransfer, OccurrenceStorageWrite:
		return row.row.InputAt(2)
	case OccurrenceIndexRead:
		return row.row.InputAt(2)
	case OccurrenceBinaryEquality, OccurrenceBinaryArithmetic, OccurrenceBinaryOrder:
		return row.row.ID(), true
	default:
		return identity.ContentID{}, false
	}
}
func (row RuleOccurrenceRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.ID()
}
func (row RuleOccurrenceRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return 1
}
func (row RuleOccurrenceRow) PointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index != 0 {
		return identity.ContentID{}, false
	}
	return row.placement.point, true
}
func (row RuleOccurrenceRow) InputPoint() (identity.ContentID, bool) {
	return row.placement.input, row.Available() && row.placement.inputKind != RuleInputNone
}
func (row RuleOccurrenceRow) InputKind() RuleInputKind {
	if !row.Available() {
		return RuleInputInvalid
	}
	return row.placement.inputKind
}
func (row RuleOccurrenceRow) Stage() RuleStage {
	if !row.Available() {
		return RuleStageInvalid
	}
	return row.placement.stage
}
func (row RuleOccurrenceRow) PredecessorRouteID() (identity.ContentID, bool) {
	return row.placement.route, row.Available() && row.placement.inputKind == RuleInputPredecessor
}

func (artifact *Artifact) OccurrenceCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.occurrences)
}
func (artifact *Artifact) OccurrenceAt(index int) (OccurrenceRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.occurrences) {
		return OccurrenceRow{}, false
	}
	return artifact.occurrences[index], true
}

// OccurrenceKindCount returns the exact sealed denominator for one generic
// occurrence family. The index is built with the Artifact seal; consumers do
// not construct parallel family indexes over the raw occurrence stream.
func (artifact *Artifact) OccurrenceKindCount(kind OccurrenceKind) int {
	if artifact == nil || !artifact.Available() || !kind.valid() || artifact.occurrenceByKind == nil {
		return 0
	}
	return len(artifact.occurrenceByKind[kind])
}

// OccurrenceKindAt returns one row from the sealed family index while keeping
// the canonical Artifact occurrence row as the sole data owner.
func (artifact *Artifact) OccurrenceKindAt(kind OccurrenceKind, index int) (OccurrenceRow, bool) {
	if artifact == nil || !artifact.Available() || !kind.valid() || index < 0 || artifact.occurrenceByKind == nil {
		return OccurrenceRow{}, false
	}
	indexes := artifact.occurrenceByKind[kind]
	if index >= len(indexes) || uint64(indexes[index]) >= uint64(len(artifact.occurrences)) {
		return OccurrenceRow{}, false
	}
	row := artifact.occurrences[indexes[index]]
	return row, row.Available() && row.kind == kind
}

// OccurrenceForID is the immutable artifact-local inverse for one typed
// semantic occurrence. The kind is part of the key because IDs are only
// required to be unique within their closed occurrence family.
func (artifact *Artifact) OccurrenceForID(kind OccurrenceKind, id identity.ContentID) (OccurrenceRow, bool) {
	if artifact == nil || !artifact.Available() || !kind.valid() || !id.Available() || artifact.occurrenceByID == nil {
		return OccurrenceRow{}, false
	}
	index, ok := artifact.occurrenceByID[occurrenceLookup{kind: kind, id: id}]
	if !ok || uint64(index) >= uint64(len(artifact.occurrences)) {
		return OccurrenceRow{}, false
	}
	row := artifact.occurrences[index]
	return row, row.Available() && row.kind == kind && row.id == id
}

// TransferOccurrenceForID resolves either existing storage-transfer family
// by its artifact semantic occurrence ID in O(1).
func (artifact *Artifact) TransferOccurrenceForID(id identity.ContentID) (OccurrenceRow, bool) {
	if row, ok := artifact.OccurrenceForID(OccurrenceStorageBindTransfer, id); ok {
		return row, true
	}
	return artifact.OccurrenceForID(OccurrenceStorageWrite, id)
}
func (artifact *Artifact) placementAt(index int) (RuleOccurrenceRow, bool) {
	if artifact == nil || index < 0 || index >= len(artifact.ruleOccurrences) {
		return RuleOccurrenceRow{}, false
	}
	placement := artifact.ruleOccurrences[index]
	if int(placement.occurrence) >= len(artifact.occurrences) {
		return RuleOccurrenceRow{}, false
	}
	row := RuleOccurrenceRow{placement: placement, row: artifact.occurrences[placement.occurrence]}
	return row, row.Available()
}

// RulePlacementCount is the number of issued placements in issuance order.
func (artifact *Artifact) RulePlacementCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.ruleOccurrences)
}

// RulePlacementAt returns one issued placement in issuance order.
func (artifact *Artifact) RulePlacementAt(index int) (RuleOccurrenceRow, bool) {
	if !artifact.Available() {
		return RuleOccurrenceRow{}, false
	}
	return artifact.placementAt(index)
}

// RulePlacementCountForKey is the number of issued placements declared under key.
func (artifact *Artifact) RulePlacementCountForKey(key schema.Key) int {
	if !artifact.Available() || !key.Available() {
		return 0
	}
	count := 0
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, ok := artifact.RulePlacementAt(index)
		if ok && row.Key() == key {
			count++
		}
	}
	return count
}

// RulePlacementForKeyAt returns one issued placement declared under key.
func (artifact *Artifact) RulePlacementForKeyAt(key schema.Key, index int) (RuleOccurrenceRow, bool) {
	if !artifact.Available() || !key.Available() || index < 0 {
		return RuleOccurrenceRow{}, false
	}
	for placement := 0; placement < artifact.RulePlacementCount(); placement++ {
		row, ok := artifact.RulePlacementAt(placement)
		if !ok || row.Key() != key {
			continue
		}
		if index == 0 {
			return row, true
		}
		index--
	}
	return RuleOccurrenceRow{}, false
}
