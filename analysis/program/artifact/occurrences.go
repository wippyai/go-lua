package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
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
	OccurrenceBody
	OccurrenceOutcome
	OccurrenceReturnValue
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
)

func (kind OccurrenceKind) valid() bool {
	return kind >= OccurrencePointAttachment && kind <= OccurrenceReturnBoundary
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
		if !binaryArithmeticOperator(op) || len(row.inputs) != 2 {
			return false
		}
	}
	if row.kind == OccurrenceBinaryOrder {
		op := flowkind.BinaryOp(row.code)
		if !binaryOrderOperator(op) || len(row.inputs) != 2 {
			return false
		}
	}
	if row.kind == OccurrenceBinaryPresenceRefinement &&
		(!row.body.Available() || len(row.points) != 1 || len(row.inputs) != 4 || row.code > 1) {
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

func binaryArithmeticOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryAdd && op <= flowkind.BinaryPow
}

// BinaryOrder returns the authored ordered semantic operands and relational
// operator of one retained primitive order computation.
func (row OccurrenceRow) BinaryOrder() (left, right identity.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryOrder || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}

func binaryOrderOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryLess && op <= flowkind.BinaryGreaterEqual
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

func (compiler *compiler) pointIDs(site flow.Site) []identity.ContentID {
	if compiler == nil || !site.Available() || !compiler.input.OwnsSite(site) || compiler.pointIDsBySite == nil {
		return nil
	}
	points, known := compiler.pointIDsBySite[site.ContextID()]
	if !known {
		return nil
	}
	return points
}

// indexPointAttachmentsFailure copies the immutable Site-to-LocalWTO point
// column once from canonical Flow schedule data. All occurrence families
// subsequently reuse this artifact-owned point order; no root row or
// transformer attachment query remains live after this stage.
func (compiler *compiler) indexPointAttachmentsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAttachment)
	}
	if compiler.pointIDsBySite == nil {
		compiler.pointIDsBySite = make(map[identity.ContentID][]identity.ContentID)
	}
	for site := range compiler.pointIDsBySite {
		delete(compiler.pointIDsBySite, site)
	}
	compiler.pointAttachments = compiler.pointAttachments[:0]

	wto := compiler.input.Flow().Local().WTO()
	seenPoints := make(map[identity.ContentID]struct{})
	seenAttachments := make(map[struct {
		site  identity.ContentID
		point identity.ContentID
	}]struct{})
	for eventIndex := 0; eventIndex < wto.EventCount(); eventIndex++ {
		event, eventOK := wto.EventAt(eventIndex)
		if !eventOK || !event.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		if event.Kind() != flow.WTOEventPoint {
			continue
		}
		point, pointOK := event.Point()
		if !pointOK || !point.Available() || !point.PathID().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		if _, duplicate := seenPoints[point.PathID()]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		seenPoints[point.PathID()] = struct{}{}
		for siteIndex := 0; siteIndex < point.SiteCount(); siteIndex++ {
			site, siteOK := point.SiteAt(siteIndex)
			if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) || !site.ContextID().Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			key := struct {
				site  identity.ContentID
				point identity.ContentID
			}{site: site.ContextID(), point: point.PathID()}
			if _, duplicate := seenAttachments[key]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			if uint64(len(compiler.pointAttachments)) > uint64(^uint32(0)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			seenAttachments[key] = struct{}{}
			row := PointAttachmentRow{site: key.site, point: key.point}
			if !row.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			compiler.pointAttachments = append(compiler.pointAttachments, row)
			compiler.pointIDsBySite[key.site] = append(compiler.pointIDsBySite[key.site], key.point)
		}
	}
	// Freeze each site index at its exact length before any later compiler
	// stage can append a temporary concatenation to the returned slice.
	for site, points := range compiler.pointIDsBySite {
		frozen := make([]identity.ContentID, len(points))
		copy(frozen, points)
		compiler.pointIDsBySite[site] = frozen
	}
	return CompileFailure{}
}

func (compiler *compiler) appendOccurrence(kind OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64) bool {
	// A Rule occurrence is attached to a semantic phase, not once per route
	// through which that phase was reached.  Source spans can legitimately
	// expose the same phase as both entry and finish (notably at a branch
	// join), so canonicalize that relation before it becomes an immutable
	// artifact row. Inputs retain their parent-issued order; only the point
	// membership relation is a set.
	points = canonicalPoints(points)
	row := OccurrenceRow{kind: kind, id: id, body: body, points: points, inputs: inputs, code: code}
	if !row.Available() {
		return false
	}
	compiler.occurrences = append(compiler.occurrences, row)
	return true
}

func (compiler *compiler) recordOccurrenceSpan(kind OccurrenceKind, id identity.ContentID, entry, finish []identity.ContentID) bool {
	if compiler == nil || compiler.occurrenceSpans == nil || !kind.valid() || !id.Available() || len(finish) == 0 {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	if _, duplicate := compiler.occurrenceSpans[key]; duplicate {
		return false
	}
	entry = canonicalPoints(entry)
	finish = canonicalPoints(finish)
	for _, point := range append(append([]identity.ContentID(nil), entry...), finish...) {
		if !point.Available() {
			return false
		}
	}
	compiler.occurrenceSpans[key] = occurrenceSpanGeometry{entry: append([]identity.ContentID(nil), entry...), finish: append([]identity.ContentID(nil), finish...)}
	return true
}

func (compiler *compiler) recordOccurrencePredecessor(kind OccurrenceKind, id, route identity.ContentID, finish []identity.ContentID) bool {
	if !compiler.recordOccurrenceSpan(kind, id, nil, finish) || !route.Available() {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	geometry := compiler.occurrenceSpans[key]
	geometry.route = route
	compiler.occurrenceSpans[key] = geometry
	return true
}

func canonicalPoints(points []identity.ContentID) []identity.ContentID {
	if len(points) < 2 {
		return points
	}
	seen := make(map[identity.ContentID]struct{}, len(points))
	canonical := make([]identity.ContentID, 0, len(points))
	for _, point := range points {
		if _, duplicate := seen[point]; duplicate {
			continue
		}
		seen[point] = struct{}{}
		canonical = append(canonical, point)
	}
	return canonical
}

func (compiler *compiler) copyOccurrenceCatalogFailure() CompileFailure {
	// Existing Values/Body/Outcome planes are copied first, then restated as
	// generic rows so all later role derivations share exactly one catalog.
	authoredValues := compiler.input.Flow().Authored().Values()
	for valuesIndex, values := range compiler.values {
		term, termOK := authoredValues.At(valuesIndex)
		if !termOK || !values.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		var points []identity.ContentID
		if span, spanOK := compiler.input.Span(term); spanOK {
			finish, finishOK := span.Finish()
			rootSpanID, rootSpanOK := values.RootSpanID()
			if !finishOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsSite(finish) ||
				!rootSpanOK || rootSpanID != span.ContextID() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
			}
			points = compiler.pointIDs(finish)
		}
		if !compiler.appendOccurrence(OccurrenceValues, values.ID(), values.BodyPathID(), points, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
			member, ok := values.MemberAt(memberIndex)
			memberTerm, memberTermOK := authoredValues.Member(term, memberIndex)
			memberSpan, memberSpanOK := compiler.input.Span(memberTerm)
			if !ok || !memberTermOK || !memberSpanOK || !compiler.input.OwnsSpan(memberSpan) ||
				!compiler.appendOccurrence(OccurrenceValuesMember, member.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID(), memberSpan.ContextID()}, uint64(memberIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, memberIndex, CompileReasonOccurrenceValues)
			}
		}
		if tail, ok := values.Tail(); ok && !compiler.appendOccurrence(OccurrenceValuesTail, tail.ID(), values.BodyPathID(), nil, []identity.ContentID{values.ID()}, uint64(tail.Kind())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
	}
	for _, body := range compiler.bodies {
		if !compiler.appendOccurrence(OccurrenceBody, body.ID(), body.ID(), nil, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for outcomeIndex, outcome := range compiler.outcomes {
		if !compiler.appendOccurrence(OccurrenceOutcome, outcome.ID(), outcome.BodyID(), nil, nil, uint64(outcome.Kind())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, outcomeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		for valueIndex := 0; valueIndex < outcome.ReturnValueCount(); valueIndex++ {
			value, ok := compiler.returnValueAt(outcome, valueIndex)
			id := digest("analysis/program-artifact/return-value-occurrence", artifactFormat, bytesField(outcome.ID()), bytesField(value.ID()), uintField(uint64(valueIndex)))
			if !ok || !compiler.appendOccurrence(OccurrenceReturnValue, id, outcome.BodyID(), nil, []identity.ContentID{outcome.ID(), value.ID()}, uint64(valueIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, outcomeIndex, valueIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	if failure := compiler.copyPointAttachments(); failure.Available() {
		return failure
	}
	if failure := compiler.copyValueSources(); failure.Available() {
		return failure
	}
	if failure := compiler.copyComputations(); failure.Available() {
		return failure
	}
	if failure := compiler.copyStorage(); failure.Available() {
		return failure
	}
	if failure := compiler.copyIndexAccess(); failure.Available() {
		return failure
	}
	if failure := compiler.copyAllocations(); failure.Available() {
		return failure
	}
	if failure := compiler.copyCalls(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveBinaryPresenceRefinementsFailure(); failure.Available() {
		return failure
	}
	if failure := compiler.deriveExactScalarSummariesFailure(); failure.Available() {
		return failure
	}
	return CompileFailure{}
}

// deriveBinaryPresenceRefinementsFailure compiles the reusable nilability
// transfer already proved by Program's BinaryPrimitive, storage, and causal
// route rows. The join runs once while the Program artifact is built:
// Link and Runtime receive only the resulting scalar rows and never reopen or
// rescan Program/Flow.
func (compiler *compiler) deriveBinaryPresenceRefinementsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	nilSources := make(map[identity.ContentID]struct{})
	storageOrigins := make(map[identity.ContentID]identity.ContentID)
	var binaries []OccurrenceRow
	binaryByID := make(map[identity.ContentID]OccurrenceRow)
	claims := make(map[identity.ContentID]identity.ContentID)
	for index, row := range compiler.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		switch row.Kind() {
		case OccurrenceValueSource:
			if row.Code() == 1 {
				span, spanOK := row.ValueSourceSpanID()
				if !spanOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceValueSourceAppend)
				}
				nilSources[span] = struct{}{}
			}
		case OccurrenceStorageRead:
			cell, span, readOK := row.StorageRead()
			if !readOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			if prior, duplicate := storageOrigins[span]; duplicate && prior != cell {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
			}
			storageOrigins[span] = cell
		case OccurrenceBinaryEquality:
			binaries = append(binaries, row)
			binaryByID[row.ID()] = row
		case OccurrenceValueClaim:
			operand, operandOK := row.InputAt(0)
			if !operandOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if prior, duplicate := claims[row.ID()]; duplicate && prior != operand {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			claims[row.ID()] = operand
		}
	}

	bodyByEntry := make(map[identity.ContentID]identity.ContentID, len(compiler.bodies))
	ambiguousBodyEntry := make(map[identity.ContentID]struct{})
	for index, body := range compiler.bodies {
		if !body.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for pointIndex := 0; pointIndex < body.EntryPointCount(); pointIndex++ {
			point, pointOK := body.EntryPointAt(pointIndex)
			if !pointOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, pointIndex, CompileReasonOccurrenceAttachment)
			}
			if prior, duplicate := bodyByEntry[point]; duplicate && prior != body.ID() {
				ambiguousBodyEntry[point] = struct{}{}
			} else {
				bodyByEntry[point] = body.ID()
			}
		}
	}

	branchEdges := make(map[identity.ContentID][]EnvironmentEdge)
	for edgeIndex, edge := range compiler.environment {
		condition, conditionOK := edge.ConditionValueSpanID()
		if !edge.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		if !conditionOK {
			continue
		}
		seen := make(map[identity.ContentID]struct{})
		for condition.Available() {
			if _, cycle := seen[condition]; cycle {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
			}
			seen[condition] = struct{}{}
			if _, binaryOK := binaryByID[condition]; binaryOK {
				branchEdges[condition] = append(branchEdges[condition], edge)
				break
			}
			next, claimOK := claims[condition]
			if !claimOK {
				break
			}
			condition = next
		}
	}

	for binaryIndex, binary := range binaries {
		left, right, op, equalityOK := binary.BinaryEquality()
		if !equalityOK {
			continue
		}
		_, leftNil := nilSources[left]
		_, rightNil := nilSources[right]
		operand, target := identity.ContentID{}, identity.ContentID{}
		switch {
		case leftNil && !rightNil:
			operand, target = right, storageOrigins[right]
		case rightNil && !leftNil:
			operand, target = left, storageOrigins[left]
		default:
			continue
		}
		// A comparison of a temporary or structural value remains valid but has
		// no persistent storage coordinate to narrow for later Reads.
		if !target.Available() {
			continue
		}

		for armIndex, selected := range branchEdges[binary.ID()] {
			truth, truthOK := selected.Truth()
			bodyID, bodyOK := bodyByEntry[selected.To()]
			_, ambiguousBody := ambiguousBodyEntry[selected.To()]
			if !truthOK || !bodyOK || ambiguousBody {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			present := truth == (op == flowkind.BinaryNotEqual)
			id := digest("analysis/program-artifact/binary-presence-refinement", artifactFormat,
				bytesField(binary.ID()), bytesField(selected.ID()), bytesField(target), boolField(present))
			routeID := selected.RouteID()
			inputs := []identity.ContentID{binary.ID(), target, operand, routeID}
			code := uint64(0)
			if present {
				code = 1
			}
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.appendOccurrence(OccurrenceBinaryPresenceRefinement, id, bodyID, []identity.ContentID{selected.To()}, inputs, code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.occurrences[len(compiler.occurrences)-1].Available() ||
				!compiler.recordOccurrencePredecessor(OccurrenceBinaryPresenceRefinement, id, routeID, []identity.ContentID{selected.To()}) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) returnValueAt(outcome OutcomeRow, index int) (ReturnValue, bool) {
	position := uint64(outcome.returnStart) + uint64(index)
	if index < 0 || position >= uint64(len(compiler.returnValues)) {
		return ReturnValue{}, false
	}
	return compiler.returnValues[position], true
}

func (compiler *compiler) copyPointAttachments() CompileFailure {
	for index, row := range compiler.pointAttachments {
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		id := digest("analysis/program-artifact/point-attachment", artifactFormat, bytesField(row.site), bytesField(row.point))
		if !compiler.appendOccurrence(OccurrencePointAttachment, id, identity.ContentID{}, []identity.ContentID{row.point}, []identity.ContentID{row.site}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyValueSources() CompileFailure {
	input, view := compiler.input, compiler.input.Flow()
	rows := []struct {
		count int
		code  uint64
	}{
		{input.Source().Literals().Nils().Count(), 1}, {input.Source().Literals().Bools().Count(), 2},
		{input.Source().Literals().Integers().Count(), 3}, {input.Source().Literals().Floats().Count(), 4},
		{input.Source().Literals().Strings().Count(), 5}, {view.Authored().TypeValues().Count(), 6},
	}
	for _, family := range rows {
		for index := 0; index < family.count; index++ {
			source, ok := compiler.valueSourceAt(family.code, index)
			if !ok && family.code == 6 {
				// TypeValue's authored denominator includes dead candidates;
				// only an executable proof becomes a ValueSource rule row.
				continue
			}
			if !ok {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceProof)
			}
			points := compiler.pointIDs(source.finish)
			span, spanOK := source.span, source.span.Available()
			if len(points) == 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourcePoints)
			}
			if !spanOK || !compiler.appendOccurrence(OccurrenceValueSource, source.id, source.body, points, []identity.ContentID{span.ContextID()}, family.code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			if family.code != 6 && !source.literalOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			row := &compiler.occurrences[len(compiler.occurrences)-1]
			row.literalFamily, row.literal, row.literalOK = source.literalFamily, source.literal, source.literalOK
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyComputations() CompileFailure {
	flowView := compiler.input.Flow()
	executable := flowView.Executable()
	primitives := flowView.BinaryPrimitives()

	arithmetic := primitives.Arithmetic()
	for index := 0; index < arithmetic.Count(); index++ {
		term, termOK := arithmetic.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftSpan, leftOK := compiler.input.Span(operation.Left)
		rightSpan, rightOK := compiler.input.Span(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !binaryArithmeticOperator(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK || !compiler.input.OwnsSpan(leftSpan) || !compiler.input.OwnsSpan(rightSpan) ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryArithmetic, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !compiler.appendOccurrence(OccurrenceBinaryArithmetic, span.ContextID(), body.PathID(), points, []identity.ContentID{leftSpan.ContextID(), rightSpan.ContextID()}, uint64(operation.Op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	equality := primitives.Equality()
	for index := 0; index < equality.Count(); index++ {
		term, termOK := equality.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftSpan, leftOK := compiler.input.Span(operation.Left)
		rightSpan, rightOK := compiler.input.Span(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK ||
			(operation.Op != flowkind.BinaryEqual && operation.Op != flowkind.BinaryNotEqual) || !spanOK || !bodyOK ||
			!compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) || !leftOK || !rightOK ||
			!compiler.input.OwnsSpan(leftSpan) || !compiler.input.OwnsSpan(rightSpan) || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			// BinaryEquality's dense primitive bucket is an executable
			// denominator. A missing row is corruption, never a dead authored hole.
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryEquality, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		inputs := []identity.ContentID{leftSpan.ContextID(), rightSpan.ContextID()}
		hasComparison, invert := false, operation.Op == flowkind.BinaryNotEqual
		if comparison, comparisonOK := primitive.Comparison(); comparisonOK && comparison.Left == operation.Left && comparison.Right == operation.Right && comparison.Invert == (operation.Op == flowkind.BinaryNotEqual) {
			branch, branchOK := flowView.SemanticTermPath(comparison.Branch)
			whenTrue, trueOK := compiler.input.ContainingBody(comparison.TrueBody)
			whenFalse, falseOK := compiler.input.ContainingBody(comparison.FalseBody)
			if branchOK && branch.Available() && trueOK && falseOK && compiler.input.OwnsBody(whenTrue) && compiler.input.OwnsBody(whenFalse) {
				inputs = append(inputs, branch, whenTrue.PathID(), whenFalse.PathID())
				hasComparison, invert = true, comparison.Invert
			}
		}
		code, codeOK := binaryEqualityCode(operation.Op, hasComparison, invert)
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !codeOK || !compiler.appendOccurrence(OccurrenceBinaryEquality, span.ContextID(), body.PathID(), points, inputs, code) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	order := primitives.Order()
	for index := 0; index < order.Count(); index++ {
		term, termOK := order.At(index)
		primitive, primitiveOK := primitives.Primitive(term)
		source, sourceOK := primitive.Source()
		operation, operationOK := primitive.Operation()
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftSpan, leftOK := compiler.input.Span(operation.Left)
		rightSpan, rightOK := compiler.input.Span(operation.Right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !binaryOrderOperator(operation.Op) ||
			!spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK || !compiler.input.OwnsSpan(leftSpan) || !compiler.input.OwnsSpan(rightSpan) ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryOrder, span.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if !compiler.appendOccurrence(OccurrenceBinaryOrder, span.ContextID(), body.PathID(), points, []identity.ContentID{leftSpan.ContextID(), rightSpan.ContextID()}, uint64(operation.Op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	unaries := flowView.Authored().Operators().Unaries()
	for index := 0; index < unaries.Count(); index++ {
		term, termOK := unaries.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, op, operand, rowOK := unaries.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		operandSpan, operandOK := compiler.input.Span(operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !compiler.input.OwnsSpan(operandSpan) || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		points := append(append([]identity.ContentID(nil), entryPoints...), finishPoints...)
		if len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.recordOccurrenceSpan(OccurrenceUnary, span.ContextID(), entryPoints, finishPoints) ||
			!compiler.appendOccurrence(OccurrenceUnary, span.ContextID(), body.PathID(), points, []identity.ContentID{operandSpan.ContextID()}, uint64(op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	selects := flowView.Authored().Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, termOK := selects.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, op, left, right, rowOK := selects.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		leftSpan, leftOK := compiler.input.Span(left)
		rightSpan, rightOK := compiler.input.Span(right)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!leftOK || !rightOK || !compiler.input.OwnsSpan(leftSpan) || !compiler.input.OwnsSpan(rightSpan) ||
			!entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceSelect, span.ContextID(), body.PathID(), points, []identity.ContentID{leftSpan.ContextID(), rightSpan.ContextID()}, uint64(op)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	claims := flowView.Authored().Claims()
	for index := 0; index < claims.Count(); index++ {
		term, termOK := claims.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, operand, claimKind, rowOK := claims.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		operandSpan, operandOK := compiler.input.Span(operand)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !rowOK || !spanOK || !bodyOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) ||
			!operandOK || !compiler.input.OwnsSpan(operandSpan) || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceValueClaim, span.ContextID(), body.PathID(), points, []identity.ContentID{operandSpan.ContextID()}, uint64(claimKind)) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}

	returns := flowView.Authored().Control().Returns()
	for index := 0; index < returns.Count(); index++ {
		term, termOK := returns.At(index)
		if !termOK || !executable.Contains(term) {
			continue
		}
		_, valuesTerm, rowOK := returns.Get(term)
		span, spanOK := compiler.input.Span(term)
		body, bodyOK := compiler.input.ContainingBody(term)
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		values, valuesOK := compiler.valueRowForTerm(valuesTerm)
		valuesID := values.ID()
		if keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || !rowOK || !spanOK || !bodyOK ||
			!compiler.input.OwnsSpan(span) || !compiler.input.OwnsBody(body) || !entryOK || !finishOK ||
			!compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) || !valuesOK || !values.Available() || !valuesID.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceReturnBoundary, span.ContextID(), body.PathID(), points, []identity.ContentID{valuesID}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyStorage() CompileFailure {
	reads := compiler.input.Flow().Authored().Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		row, ok := compiler.storageReadAt(index)
		if !ok {
			// Flow preserves authored denominator positions while withholding
			// dead/non-executable occurrences. Such a position has no artifact
			// row.
			continue
		}
		entryPoints, finishPoints := compiler.pointIDs(row.entry), compiler.pointIDs(row.finish)
		spanID := row.span.ContextID()
		// A one-input rule cannot select an Entry attachment from the
		// parent's deliberately multi-valued Site relation. Refuse such a
		// Flow until it publishes an explicit occurrence-to-point pairing;
		// never zip or cross-product attachments here.
		if len(entryPoints) != 1 || !spanID.Available() ||
			!compiler.appendOccurrence(OccurrenceStorageRead, row.id, row.body, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{row.cell, spanID}, 0) ||
			!compiler.recordOccurrenceSpan(OccurrenceStorageRead, row.id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	binds := compiler.input.Flow().Authored().Storage().Binds()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := compiler.storageBindAt(index)
		if !ok {
			continue
		}
		values, valuesOK := compiler.valueRowForTerm(bind.values)
		entryPoints, finishPoints := compiler.pointIDs(bind.entry), compiler.pointIDs(bind.finish)
		bindInputs := make([]identity.ContentID, 1, 1+len(bind.cells))
		bindInputs[0] = values.ID()
		// The generic storage-bind occurrence owns the complete destination
		// Cell column. Pack consumes this canonical row directly; it must not
		// receive a second bind/Cell row plane.
		bindInputs = append(bindInputs, bind.cells...)
		if !valuesOK || !values.Available() || !compiler.appendOccurrence(OccurrenceStorageBind, bind.id, bind.body, append(entryPoints, finishPoints...), bindInputs, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		for _, transfer := range bind.transfers {
			transferEntryPoints, transferFinishPoints := compiler.pointIDs(bind.entry), compiler.pointIDs(bind.finish)
			// As with a read, this one-input transfer rule requires one
			// unambiguous Entry attachment.
			if len(transferEntryPoints) != 1 ||
				!compiler.appendOccurrence(OccurrenceStorageBindTransfer, transfer.id, bind.body, transferFinishPoints, []identity.ContentID{bind.id, transfer.value, transfer.cell}, uint64(transfer.position)) ||
				!compiler.recordOccurrenceSpan(OccurrenceStorageBindTransfer, transfer.id, transferEntryPoints, transferFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, transfer.position, CompileReasonOccurrenceStorageBind)
			}
		}
	}
	assigns := compiler.input.Flow().Authored().Storage().Assigns()
	for index := 0; index < assigns.Count(); index++ {
		assignment, ok := compiler.storageAssignmentAt(index)
		if !ok {
			continue
		}
		values, valuesOK := compiler.valueRowForTerm(assignment.values)
		entryPoints, finishPoints := compiler.pointIDs(assignment.entry), compiler.pointIDs(assignment.finish)
		if !valuesOK || !values.Available() || !compiler.appendOccurrence(OccurrenceStorageAssignment, assignment.id, assignment.body, append(entryPoints, finishPoints...), []identity.ContentID{values.ID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageAssignment)
		}
		for _, write := range assignment.transfers {
			writeFinishPoints := compiler.pointIDs(write.finish)
			if !compiler.appendOccurrence(OccurrenceStorageWrite, write.id, assignment.body, writeFinishPoints, []identity.ContentID{assignment.id, write.value, write.cell, write.predecessor, write.route}, uint64(write.position)) ||
				!compiler.recordOccurrencePredecessor(OccurrenceStorageWrite, write.id, write.route, writeFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, write.position, CompileReasonOccurrenceStorageAssignment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyIndexAccess() CompileFailure {
	geometry := compiler.input.Flow().AccessGeometry()
	reads, writes := geometry.IndexAccesses().Reads(), geometry.IndexAccesses().Writes()
	for index := 0; index < reads.Count(); index++ {
		read, ok := compiler.indexReadAt(index)
		if !ok {
			// AccessGeometry preserves candidate ordinals whose executable
			// Span proof can be absent. Only a complete artifact row is
			// executable.
			continue
		}
		entry, entryOK := read.resultSpan.Entry()
		finish, finishOK := read.resultSpan.Finish()
		if !entryOK || !finishOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.appendOccurrence(OccurrenceIndexRead, read.id, identity.ContentID{}, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{read.baseID, read.lensID, read.resultID}, 0) ||
			!compiler.recordOccurrenceSpan(OccurrenceIndexRead, read.id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	for index := 0; index < writes.Count(); index++ {
		write, ok := compiler.indexWriteAt(index)
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexCandidate)
		}
		finishPoints := compiler.pointIDs(write.finish)
		if len(finishPoints) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !compiler.appendOccurrence(OccurrenceIndexWrite, write.id, identity.ContentID{}, finishPoints, []identity.ContentID{write.baseID, write.lensID, write.valuesID, write.predecessorID, write.route}, 0) ||
			!compiler.recordOccurrencePredecessor(OccurrenceIndexWrite, write.id, write.route, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	return CompileFailure{}
}

// copyHeapGeometryFailure captures Heap's complete cold source denominator
// while the Program proof is live.  Link later substitutes these scalar rows
// through its own mounted Values/Keys authority; Heap never needs to reopen a
// Program after artifact compilation.
func (compiler *compiler) copyHeapGeometryFailure() CompileFailure {
	geometry := compiler.input.Flow().AccessGeometry()
	reads, writes := geometry.IndexAccesses().Reads(), geometry.IndexAccesses().Writes()
	compiler.heapIndexes = make([]HeapIndexRow, 0, reads.Count()+writes.Count())
	for index := 0; index < reads.Count(); index++ {
		occurrence, occurrenceOK := compiler.indexReadAt(index)
		row := HeapIndexRow{id: occurrence.id, read: true, baseSpan: occurrence.baseSpan.ContextID(), resultSpan: occurrence.resultSpan.ContextID(), position: -1}
		if occurrence.exact {
			row.lensKind = 1
			row.exactKey = occurrence.exactKey
		} else {
			if occurrence.dynamicKeySpan.Available() {
				row.lensKind, row.keySpan = 2, occurrence.dynamicKeySpan.ContextID()
			}
		}
		if !occurrenceOK || !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	for index := 0; index < writes.Count(); index++ {
		occurrence, occurrenceOK := compiler.indexWriteAt(index)
		valueRow, valueRowOK := compiler.valueRowForTerm(occurrence.values)
		valueSpan, valueSpanOK := valueRow.RootSpanID()
		row := HeapIndexRow{id: occurrence.id, baseSpan: occurrence.baseSpan.ContextID(), valuesSpan: valueSpan, valuesID: valueRow.ID(), position: occurrence.position}
		if occurrence.exact {
			row.lensKind = 1
			row.exactKey = occurrence.exactKey
		} else {
			if occurrence.dynamicKeySpan.Available() {
				row.lensKind, row.keySpan = 2, occurrence.dynamicKeySpan.ContextID()
			}
		}
		if !occurrenceOK || !valueRowOK || !valueSpanOK || !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	return CompileFailure{}
}

func (compiler *compiler) copyAllocations() CompileFailure {
	if compiler == nil || len(compiler.allocationRows) != len(compiler.heapAllocations) {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	for index, allocation := range compiler.allocationRows {
		entryPoints, finishPoints := compiler.pointIDs(allocation.entry), compiler.pointIDs(allocation.finish)
		if !allocation.occurrence.Available() || !allocation.template.Available() || len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.appendOccurrence(OccurrenceAllocation, allocation.template, identity.ContentID{}, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), []identity.ContentID{allocation.template, allocation.occurrence.ID()}, uint64(allocation.form)) ||
			!compiler.recordOccurrenceSpan(OccurrenceAllocation, allocation.template, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		for fieldIndex, field := range allocation.fields {
			values := field.valuesRow
			inputs := []identity.ContentID{allocation.template}
			if values.Available() {
				inputs = append(inputs, values.ID())
				for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
					member, memberOK := values.MemberAt(memberIndex)
					if !memberOK {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
					}
					inputs = append(inputs, member.ID())
				}
			}
			if !field.field.Available() || !values.Available() || !compiler.appendOccurrence(OccurrenceAllocationField, field.id, identity.ContentID{}, nil, inputs, uint64(fieldIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyCalls() CompileFailure {
	for index := 0; index < compiler.input.Flow().Authored().Calls().Count(); index++ {
		call, ok := compiler.callConstruction(index)
		if !ok {
			// The authored denominator remains canonical; a missing direct join
			// is a non-executable candidate and is not compacted into a new
			// proof table.
			continue
		}
		inputs := []identity.ContentID{call.callee.id, call.actuals.id, call.values, call.formal, call.types}
		if call.receiver.id.Available() {
			inputs = append(inputs, call.receiver.id)
		}
		entryPoints, finishPoints := compiler.pointIDs(call.entry), compiler.pointIDs(call.finish)
		disposition := uint64(1)
		if call.executable {
			disposition = uint64(2)
		}
		if len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.appendOccurrence(OccurrenceCall, call.id, call.bodyPath, append(append([]identity.ContentID(nil), entryPoints...), finishPoints...), inputs, disposition) ||
			!compiler.recordOccurrenceSpan(OccurrenceCall, call.id, entryPoints, finishPoints) ||
			!compiler.appendOccurrence(OccurrenceCallActivation, call.id, call.bodyPath, append([]identity.ContentID(nil), finishPoints...), inputs, disposition) ||
			!compiler.recordOccurrenceSpan(OccurrenceCallActivation, call.id, nil, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for argIndex, argument := range call.arguments {
			if !compiler.appendOccurrence(OccurrenceCallArgument, argument.id, call.bodyPath, nil, []identity.ContentID{call.id}, uint64(argIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argIndex, CompileReasonOccurrenceCall)
			}
		}
		for typeIndex, argument := range call.typeArguments {
			if !compiler.appendOccurrence(OccurrenceCallTypeArgument, argument.id, call.bodyPath, nil, []identity.ContentID{call.id}, uint64(typeIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
		}
		if call.boundary.id.Available() {
			if !compiler.appendOccurrence(OccurrenceCallBoundary, call.boundary.id, call.bodyPath, nil, []identity.ContentID{call.id}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			for armIndex, arm := range call.boundary.arms {
				if !compiler.appendOccurrence(OccurrenceCallArm, arm.id, call.bodyPath, arm.points, []identity.ContentID{call.boundary.id, arm.route, arm.target}, uint64(armIndex)) {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, armIndex, CompileReasonOccurrenceCall)
				}
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	compiler.ruleOccurrences = make(map[RuleRole][]RuleOccurrence)
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			compiler.ruleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		ordinal := uint32(index)
		geometry := compiler.occurrenceSpans[occurrenceLookup{kind: row.kind, id: row.id}]
		finish := geometry.finish
		if len(finish) == 0 {
			finish = row.points
		}
		appendBase := func(role RuleRole, inputKind RuleInputKind, input []identity.ContentID) bool {
			if len(finish) == 0 || inputKind != RuleInputNone && len(input) != 1 {
				return false
			}
			for _, point := range finish {
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: point, stage: RuleStageBase, inputKind: inputKind}
				if inputKind != RuleInputNone {
					placement.input = input[0]
				}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendLocal := func(role RuleRole, inputKind RuleInputKind, inputs []identity.ContentID) bool {
			if len(finish) == 0 || inputKind == RuleInputNone || inputKind == RuleInputPredecessor || inputKind == RuleInputEntry && len(inputs) != 1 {
				return false
			}
			for _, base := range finish {
				stage, stageOK := compiler.localStage(base)
				if !stageOK {
					return false
				}
				input := base
				if inputKind == RuleInputEntry {
					input = inputs[0]
				}
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: input, stage: RuleStageLocal, inputKind: inputKind}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendComputation := func(role RuleRole) bool {
			if len(finish) == 0 || len(row.inputs) < 2 {
				return false
			}
			for _, base := range finish {
				stage, stageOK := compiler.localComputationStage(base, role, row.id, row.inputs[0], row.inputs[1])
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: base, stage: RuleStageLocal, inputKind: RuleInputFinish}
				if !stageOK || !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendLocalPredecessor := func(role RuleRole) bool {
			if !geometry.route.Available() {
				return false
			}
			if _, duplicate := compiler.environmentRouteDuplicates[geometry.route]; duplicate {
				return false
			}
			predecessor, found := compiler.environmentByRoute[geometry.route]
			if !found || !predecessor.Available() {
				return false
			}
			finishMember := false
			for _, point := range finish {
				if point == predecessor.to {
					finishMember = true
					break
				}
			}
			stage, stageOK := compiler.localStage(predecessor.to)
			placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: predecessor.from, stage: RuleStageLocal, inputKind: RuleInputPredecessor, route: geometry.route}
			if !finishMember || !stageOK || !placement.Available() {
				return false
			}
			compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			return true
		}
		appendCallStage := func(role RuleRole, stage RuleStage) bool {
			if len(finish) == 0 || stage < RuleStageCallDispatch || stage > RuleStageCallEffect {
				return false
			}
			for _, base := range finish {
				stages, stagesOK := compiler.callStage(base)
				if !stagesOK {
					return false
				}
				point, input := stages.dispatch, base
				switch stage {
				case RuleStageCallSummary:
					point, input = stages.summary, stages.dispatch
				case RuleStageCallEffect:
					point, input = stages.effect, stages.summary
				}
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: point, input: input, stage: stage, inputKind: RuleInputFinish}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		ok := true
		switch row.kind {
		case OccurrenceValueSource:
			ok = appendBase(RuleRoleValueSource, RuleInputNone, nil)
		case OccurrenceValues:
			if len(row.points) != 0 {
				ok = appendBase(RuleRolePackSource, RuleInputNone, nil)
			}
		case OccurrenceStorageRead, OccurrenceStorageBindTransfer:
			// Storage reads and fixed bind transfers read the exact pre-result
			// Entry environment, then write at a separate post-Finish Local cut.
			// The entry witness is issued by Program and retained only in the
			// sealed artifact geometry; Link never reconstructs it.
			ok = appendLocal(RuleRoleValueStorageTransfer, RuleInputEntry, geometry.entry)
		case OccurrenceStorageWrite:
			// A storage write reads its exact reverse assignment-commit
			// predecessor, including the parent route's guard/reset proof, and
			// writes at the Local cut after that route's Finish attachment.
			ok = appendLocalPredecessor(RuleRoleValueStorageTransfer)
		case OccurrenceIndexRead:
			ok = appendLocal(RuleRoleRawGet, RuleInputEntry, geometry.entry)
		case OccurrenceIndexWrite:
			ok = appendLocalPredecessor(RuleRoleRawSet)
		case OccurrenceBinaryEquality:
			// A computation consumes the environment after its operands have
			// finished. ProgramArtifact gives every primitive its own stable local
			// cut; installLocalStagesFailure orders those cuts by exact semantic
			// operand dependencies before any Link mount exists.
			ok = appendComputation(RuleRoleValueBinaryEquality)
		case OccurrenceBinaryArithmetic:
			ok = appendComputation(RuleRoleValueBinaryArithmetic)
		case OccurrenceBinaryOrder:
			ok = appendComputation(RuleRoleValueBinaryOrder)
		case OccurrenceBinaryPresenceRefinement:
			// The generic refinement consumes its exact guarded predecessor and
			// writes at the ordinary local cut.  It never guesses a later
			// consumer through a shared base point.
			ok = appendLocalPredecessor(RuleRoleValuePresenceRefinement)
		case OccurrenceCall:
			ok = appendCallStage(RuleRoleCallDispatch, RuleStageCallDispatch) &&
				appendBase(RuleRolePackSource, RuleInputNone, nil) &&
				appendCallStage(RuleRoleEffectSelected, RuleStageCallEffect) &&
				appendCallStage(RuleRoleEffectOpaque, RuleStageCallEffect) &&
				appendCallStage(RuleRoleEffectBody, RuleStageCallEffect)
		case OccurrenceCallActivation:
			ok = appendCallStage(RuleRoleCallActivation, RuleStageCallSummary)
		case OccurrenceAllocation:
			ok = appendBase(RuleRoleHeapIngress, RuleInputNone, nil) &&
				appendLocal(RuleRoleValueAllocation, RuleInputEntry, geometry.entry)
			if ok && row.code == uint64(flow.AllocationFormEmpty) {
				ok = appendLocal(RuleRoleHeapEmpty, RuleInputFinish, finish)
			}
			if ok && row.code == uint64(flow.AllocationFormClosed) {
				ok = appendLocal(RuleRoleHeapClosed, RuleInputFinish, finish)
			}
		}
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}
