package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// RuleRole is the closed global schema role catalog. A false Supported result
// is deliberate: the artifact never invents a relation for a role that the
// current Program proof surface cannot state exactly.
type RuleRole uint8

const (
	RuleRoleInvalid RuleRole = iota
	RuleRoleValueSource
	RuleRolePackSource
	RuleRoleHeapIngress
	RuleRoleValueAllocation
	RuleRoleHeapEmpty
	RuleRoleHeapClosed
	RuleRoleRawGet
	RuleRoleRawSet
	RuleRoleCallDispatch
	RuleRoleEffectSelected
	RuleRoleEffectOpaque
	RuleRoleEffectBody
	RuleRoleCallActivation
	RuleRoleValueBootstrap
	RuleRoleHeapBootstrap
	RuleRoleValueStorageTransfer
	RuleRoleValueBinaryArithmetic
	RuleRoleValueBinaryEquality
	RuleRoleValueBinaryOrder
	RuleRoleValuePresenceRefinement
)

// mountedRuleRoles is the one ordered ProgramArtifact vocabulary for rules
// materialized from reusable mounted artifacts. Link-owned bootstrap rules
// are deliberately excluded: they are admitted through the explicit Link
// table at the analysis binding boundary.
var mountedRuleRoles = [...]RuleRole{
	RuleRoleValueSource,
	RuleRolePackSource,
	RuleRoleHeapIngress,
	RuleRoleValueAllocation,
	RuleRoleHeapEmpty,
	RuleRoleHeapClosed,
	RuleRoleRawGet,
	RuleRoleRawSet,
	RuleRoleCallDispatch,
	RuleRoleEffectSelected,
	RuleRoleEffectOpaque,
	RuleRoleEffectBody,
	RuleRoleCallActivation,
	RuleRoleValueStorageTransfer,
	RuleRoleValueBinaryArithmetic,
	RuleRoleValueBinaryEquality,
	RuleRoleValueBinaryOrder,
	RuleRoleValuePresenceRefinement,
}

// MountedRuleRoleCount reports the closed mounted-rule vocabulary size.
func MountedRuleRoleCount() int { return len(mountedRuleRoles) }

// MountedRuleRoleAt returns the ProgramArtifact-owned mounted role at its
// stable ordinal. The ordinal is the canonical attachment/ingress order.
func MountedRuleRoleAt(index int) (RuleRole, bool) {
	if index < 0 || index >= len(mountedRuleRoles) {
		return RuleRoleInvalid, false
	}
	return mountedRuleRoles[index], true
}

func (role RuleRole) valid() bool {
	return role >= RuleRoleValueSource && role <= RuleRoleValuePresenceRefinement
}

// RuleOutputKind is the domain-neutral Factor lane written by one sealed rule
// placement. It is derived only from the closed RuleRole catalog, so consumers
// can select producer occurrences without importing domain implementations or
// guessing from point/stage geometry.
type RuleOutputKind uint8

const (
	RuleOutputInvalid RuleOutputKind = iota
	RuleOutputValue
	RuleOutputPack
	RuleOutputHeap
	RuleOutputCall
	RuleOutputEffect
)

// RuleOutputKindFor is the artifact format's own role-to-factor projection.
// The analyzer's rule table reads its principal from here rather than
// restating the mapping, so a role can name exactly one owning factor.
func RuleOutputKindFor(role RuleRole) RuleOutputKind { return ruleOutputKind(role) }

func ruleOutputKind(role RuleRole) RuleOutputKind {
	switch role {
	case RuleRoleValueSource, RuleRoleValueAllocation, RuleRoleRawGet, RuleRoleValueBootstrap, RuleRoleValueStorageTransfer, RuleRoleValueBinaryArithmetic, RuleRoleValueBinaryEquality, RuleRoleValueBinaryOrder, RuleRoleValuePresenceRefinement:
		return RuleOutputValue
	case RuleRolePackSource:
		return RuleOutputPack
	case RuleRoleHeapIngress, RuleRoleHeapEmpty, RuleRoleHeapClosed, RuleRoleRawSet, RuleRoleHeapBootstrap:
		return RuleOutputHeap
	case RuleRoleCallDispatch, RuleRoleCallActivation:
		return RuleOutputCall
	case RuleRoleEffectSelected, RuleRoleEffectOpaque, RuleRoleEffectBody:
		return RuleOutputEffect
	default:
		return RuleOutputInvalid
	}
}

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

// RuleInputKind preserves the owner-issued Span polarity of a rule input.
// None is lawful only for source rules; Entry and Finish are exact Program
// proof roles, never Link-side point inference.
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
	role       RuleRole
	occurrence uint32
	point      identity.ContentID
	input      identity.ContentID
	stage      RuleStage
	inputKind  RuleInputKind
	route      identity.ContentID
}

func (row RuleOccurrence) Available() bool {
	if !row.role.valid() || !row.point.Available() || !row.stage.valid() || !row.inputKind.valid() {
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
func (row RuleOccurrence) Role() RuleRole {
	if !row.Available() {
		return RuleRoleInvalid
	}
	return row.role
}

// RuleOccurrenceRow is the immutable role-specific placement joined to its
// exact semantic occurrence. Only stable semantic IDs and point IDs escape.
type RuleOccurrenceRow struct {
	placement RuleOccurrence
	row       OccurrenceRow
}

func (row RuleOccurrenceRow) Available() bool {
	return row.placement.Available() && row.row.Available()
}
func (row RuleOccurrenceRow) Role() RuleRole { return row.placement.Role() }
func (row RuleOccurrenceRow) OutputKind() RuleOutputKind {
	if !row.Available() {
		return RuleOutputInvalid
	}
	return ruleOutputKind(row.placement.Role())
}

// OutputSemanticID returns the exact Program-issued semantic value written by
// a placement when that relation is already retained by its occurrence row.
// It never equates the occurrence identity with its output: storage writes
// and index reads name their destination explicitly in the sealed operand
// vector. Roles whose output belongs to another owner return false.
func (row RuleOccurrenceRow) OutputSemanticID() (identity.ContentID, bool) {
	if !row.Available() || row.OutputKind() != RuleOutputValue {
		return identity.ContentID{}, false
	}
	switch row.Role() {
	case RuleRoleValueSource:
		return row.row.ID(), true
	case RuleRoleValueStorageTransfer:
		switch row.row.Kind() {
		case OccurrenceStorageRead:
			return row.row.ID(), true
		case OccurrenceStorageBindTransfer, OccurrenceStorageWrite:
			return row.row.InputAt(2)
		default:
			return identity.ContentID{}, false
		}
	case RuleRoleRawGet:
		if row.row.Kind() != OccurrenceIndexRead {
			return identity.ContentID{}, false
		}
		return row.row.InputAt(2)
	case RuleRoleValueBinaryEquality:
		if row.row.Kind() != OccurrenceBinaryEquality {
			return identity.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValueBinaryArithmetic:
		if row.row.Kind() != OccurrenceBinaryArithmetic {
			return identity.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValueBinaryOrder:
		if row.row.Kind() != OccurrenceBinaryOrder {
			return identity.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValuePresenceRefinement:
		if row.row.Kind() != OccurrenceBinaryPresenceRefinement {
			return identity.ContentID{}, false
		}
		_, target, _, _, _, ok := row.row.BinaryPresenceRefinement()
		return target, ok
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
func (artifact *Artifact) RuleRoleSupported(role RuleRole) bool {
	return artifact.Available() && ruleRoleSupported(role)
}

// ruleRoleSupported is the closed format capability used while sealing an
// Artifact. It deliberately has no Artifact availability dependency: the
// public projection above adds that lifecycle fence after seal succeeds.
func ruleRoleSupported(role RuleRole) bool {
	for _, candidate := range mountedRuleRoles {
		if role == candidate {
			return true
		}
	}
	return false
}
func (artifact *Artifact) RuleOccurrenceCount(role RuleRole) int {
	if !artifact.Available() || !artifact.RuleRoleSupported(role) {
		return 0
	}
	return len(artifact.ruleOccurrences[role])
}
func (artifact *Artifact) RuleOccurrenceAt(role RuleRole, index int) (RuleOccurrenceRow, bool) {
	if !artifact.Available() || !artifact.RuleRoleSupported(role) || index < 0 {
		return RuleOccurrenceRow{}, false
	}
	rows := artifact.ruleOccurrences[role]
	if index >= len(rows) || int(rows[index].occurrence) >= len(artifact.occurrences) {
		return RuleOccurrenceRow{}, false
	}
	row := RuleOccurrenceRow{placement: rows[index], row: artifact.occurrences[rows[index].occurrence]}
	return row, row.Available()
}
