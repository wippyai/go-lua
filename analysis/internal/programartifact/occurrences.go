package programartifact

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
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
	id            keyspace.ContentID
	body          keyspace.ContentID
	points        []keyspace.ContentID
	inputs        []keyspace.ContentID
	code          uint64
	literalFamily keyspace.Family
	literal       keyspace.LiteralValue
	literalOK     bool
}

type occurrenceLookup struct {
	kind OccurrenceKind
	id   keyspace.ContentID
}

// occurrenceSpanGeometry is compile-only scratch captured while the exact
// Program role proof is live. It is discarded after role-specific placements
// are sealed into the Artifact.
type occurrenceSpanGeometry struct {
	entry  []keyspace.ContentID
	finish []keyspace.ContentID
	route  keyspace.ContentID
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
func (row OccurrenceRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row OccurrenceRow) BodyID() (keyspace.ContentID, bool) {
	return row.body, row.Available() && row.body.Available()
}
func (row OccurrenceRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.points)
}
func (row OccurrenceRow) PointAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.points) {
		return keyspace.ContentID{}, false
	}
	return row.points[index], true
}
func (row OccurrenceRow) InputCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.inputs)
}
func (row OccurrenceRow) InputAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.inputs) {
		return keyspace.ContentID{}, false
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
func (row OccurrenceRow) ValueSourceSpanID() (keyspace.ContentID, bool) {
	if !row.Available() || row.kind != OccurrenceValueSource || len(row.inputs) != 1 {
		return keyspace.ContentID{}, false
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
func (row OccurrenceRow) BinaryEquality() (left, right keyspace.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryEquality || len(row.inputs) < 2 {
		return keyspace.ContentID{}, keyspace.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code & binaryEqualityCodeOpMask), true
}

// BinaryArithmetic returns the authored ordered semantic operands and closed
// primitive arithmetic operator.  The row is reusable Program geometry and
// exposes no authored Term.
func (row OccurrenceRow) BinaryArithmetic() (left, right keyspace.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryArithmetic || len(row.inputs) != 2 {
		return keyspace.ContentID{}, keyspace.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}

func binaryArithmeticOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryAdd && op <= flowkind.BinaryPow
}

// BinaryOrder returns the authored ordered semantic operands and relational
// operator of one retained primitive order computation.
func (row OccurrenceRow) BinaryOrder() (left, right keyspace.ContentID, op flowkind.BinaryOp, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryOrder || len(row.inputs) != 2 {
		return keyspace.ContentID{}, keyspace.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}

func binaryOrderOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryLess && op <= flowkind.BinaryGreaterEqual
}

// BinaryComparison returns the optional exact Branch and two causal body
// identities retained beside a Binary equality occurrence.
func (row OccurrenceRow) BinaryComparison() (branch, whenTrue, whenFalse keyspace.ContentID, invert bool, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryEquality || row.code&binaryEqualityCodeHasComparison == 0 || len(row.inputs) != 5 {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false, false
	}
	return row.inputs[2], row.inputs[3], row.inputs[4], row.code&binaryEqualityCodeInvert != 0, true
}

// BinaryPresenceRefinement returns the exact reusable proof constituents for
// one nil-comparison arm. Source is the Binary occurrence, Target is the
// storage Cell being narrowed, Operand is the comparison operand whose
// StorageRead proved that origin, and Route is the exact guarded environment
// edge entering this arm. Present is the arm's closed nilability conclusion.
func (row OccurrenceRow) BinaryPresenceRefinement() (source, target, operand, route keyspace.ContentID, present bool, ok bool) {
	if !row.Available() || row.kind != OccurrenceBinaryPresenceRefinement || len(row.inputs) != 4 {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false, false
	}
	return row.inputs[0], row.inputs[1], row.inputs[2], row.inputs[3], row.code == 1, true
}

// StorageRead returns the existing Cell and exact expression Span identities
// retained while Program owned both proofs. The occurrence role ID remains
// distinct: computation operands name spans, so the span is the only sound
// reusable join between a Binary operand and its storage origin.
func (row OccurrenceRow) StorageRead() (cell, span keyspace.ContentID, ok bool) {
	if !row.Available() || row.kind != OccurrenceStorageRead || len(row.inputs) != 2 {
		return keyspace.ContentID{}, keyspace.ContentID{}, false
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
	point      keyspace.ContentID
	input      keyspace.ContentID
	stage      RuleStage
	inputKind  RuleInputKind
	route      keyspace.ContentID
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
// vector. Roles whose output belongs to another owner receipt return false.
func (row RuleOccurrenceRow) OutputSemanticID() (keyspace.ContentID, bool) {
	if !row.Available() || row.OutputKind() != RuleOutputValue {
		return keyspace.ContentID{}, false
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
			return keyspace.ContentID{}, false
		}
	case RuleRoleRawGet:
		if row.row.Kind() != OccurrenceIndexRead {
			return keyspace.ContentID{}, false
		}
		return row.row.InputAt(2)
	case RuleRoleValueBinaryEquality:
		if row.row.Kind() != OccurrenceBinaryEquality {
			return keyspace.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValueBinaryArithmetic:
		if row.row.Kind() != OccurrenceBinaryArithmetic {
			return keyspace.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValueBinaryOrder:
		if row.row.Kind() != OccurrenceBinaryOrder {
			return keyspace.ContentID{}, false
		}
		return row.row.ID(), true
	case RuleRoleValuePresenceRefinement:
		if row.row.Kind() != OccurrenceBinaryPresenceRefinement {
			return keyspace.ContentID{}, false
		}
		_, target, _, _, _, ok := row.row.BinaryPresenceRefinement()
		return target, ok
	default:
		return keyspace.ContentID{}, false
	}
}
func (row RuleOccurrenceRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.row.ID()
}
func (row RuleOccurrenceRow) PointCount() int {
	if !row.Available() {
		return 0
	}
	return 1
}
func (row RuleOccurrenceRow) PointAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index != 0 {
		return keyspace.ContentID{}, false
	}
	return row.placement.point, true
}
func (row RuleOccurrenceRow) InputPoint() (keyspace.ContentID, bool) {
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
func (row RuleOccurrenceRow) PredecessorRouteID() (keyspace.ContentID, bool) {
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

// OccurrenceForID is the immutable artifact-local inverse for one typed
// semantic occurrence. The kind is part of the key because IDs are only
// required to be unique within their closed occurrence family.
func (artifact *Artifact) OccurrenceForID(kind OccurrenceKind, id keyspace.ContentID) (OccurrenceRow, bool) {
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
func (artifact *Artifact) TransferOccurrenceForID(id keyspace.ContentID) (OccurrenceRow, bool) {
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

func (compiler *compiler) pointIDs(site flow.Site) []keyspace.ContentID {
	if compiler == nil || !site.Available() || !compiler.input.OwnsSite(site) || compiler.pointIDsBySite == nil {
		return nil
	}
	points, known := compiler.pointIDsBySite[site.ContextID()]
	if !known {
		return nil
	}
	return points
}

// indexPointAttachmentsFailure materializes each sealed Site attachment once.
// All occurrence families subsequently reuse the owner-published point order;
// this removes repeated receipt traversal and per-use point slice allocation
// while retaining the same attachment ownership fence.
func (compiler *compiler) indexPointAttachmentsFailure() CompileFailure {
	if compiler == nil || compiler.pointIDsBySite == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAttachment)
	}
	for siteIndex := 0; siteIndex < compiler.input.CausalSiteCount(); siteIndex++ {
		site, ok := compiler.input.CausalSiteAt(siteIndex)
		if !ok || !compiler.input.OwnsSite(site) || !site.ContextID().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, -1, CompileReasonOccurrenceAttachment)
		}
		attachments := compiler.input.PointAttachments(site)
		points := make([]keyspace.ContentID, attachments.Count())
		for index := range points {
			attachment, attachmentOK := attachments.At(index)
			if !attachmentOK || !compiler.input.OwnsPointAttachment(attachment) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, index, CompileReasonOccurrenceAttachment)
			}
			points[index] = attachment.PointPathID()
			if !points[index].Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, index, CompileReasonOccurrenceAttachment)
			}
		}
		compiler.pointIDsBySite[site.ContextID()] = points
	}
	return CompileFailure{}
}

func (compiler *compiler) appendOccurrence(kind OccurrenceKind, id, body keyspace.ContentID, points, inputs []keyspace.ContentID, code uint64) bool {
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

func (compiler *compiler) recordOccurrenceSpan(kind OccurrenceKind, id keyspace.ContentID, entry, finish []keyspace.ContentID) bool {
	if compiler == nil || compiler.occurrenceSpans == nil || !kind.valid() || !id.Available() || len(finish) == 0 {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	if _, duplicate := compiler.occurrenceSpans[key]; duplicate {
		return false
	}
	entry = canonicalPoints(entry)
	finish = canonicalPoints(finish)
	for _, point := range append(append([]keyspace.ContentID(nil), entry...), finish...) {
		if !point.Available() {
			return false
		}
	}
	compiler.occurrenceSpans[key] = occurrenceSpanGeometry{entry: append([]keyspace.ContentID(nil), entry...), finish: append([]keyspace.ContentID(nil), finish...)}
	return true
}

func (compiler *compiler) recordOccurrencePredecessor(kind OccurrenceKind, id, route keyspace.ContentID, finish []keyspace.ContentID) bool {
	if !compiler.recordOccurrenceSpan(kind, id, nil, finish) || !route.Available() {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	geometry := compiler.occurrenceSpans[key]
	geometry.route = route
	compiler.occurrenceSpans[key] = geometry
	return true
}

func canonicalPoints(points []keyspace.ContentID) []keyspace.ContentID {
	if len(points) < 2 {
		return points
	}
	seen := make(map[keyspace.ContentID]struct{}, len(points))
	canonical := make([]keyspace.ContentID, 0, len(points))
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
	parentValues := compiler.input.Values()
	for valuesIndex, values := range compiler.values {
		proof, proofOK := parentValues.At(valuesIndex)
		if !proofOK || !compiler.input.OwnsValuesOccurrence(proof) || proof.ID() != values.ID() || proof.BodyPathID() != values.BodyPathID() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		var points []keyspace.ContentID
		if span, spanOK := proof.Span(); spanOK {
			finish, finishOK := span.Finish()
			if !finishOK || !compiler.input.OwnsSpan(span) || !compiler.input.OwnsSite(finish) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
			}
			points = compiler.pointIDs(finish)
		}
		if !compiler.appendOccurrence(OccurrenceValues, values.ID(), values.BodyPathID(), points, nil, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, -1, CompileReasonOccurrenceValues)
		}
		for memberIndex := 0; memberIndex < values.MemberCount(); memberIndex++ {
			member, ok := values.MemberAt(memberIndex)
			proofMember, proofMemberOK := proof.At(memberIndex)
			memberSpan, memberSpanOK := proofMember.Span()
			if !ok || !proofMemberOK || !memberSpanOK || !compiler.input.OwnsValuesMember(proofMember) || !compiler.input.OwnsSpan(memberSpan) ||
				proofMember.ID() != member.ID() || !compiler.appendOccurrence(OccurrenceValuesMember, member.ID(), values.BodyPathID(), nil, []keyspace.ContentID{values.ID(), memberSpan.ContextID()}, uint64(memberIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, valuesIndex, memberIndex, CompileReasonOccurrenceValues)
			}
		}
		if tail, ok := values.Tail(); ok && !compiler.appendOccurrence(OccurrenceValuesTail, tail.ID(), values.BodyPathID(), nil, []keyspace.ContentID{values.ID()}, uint64(tail.Kind())) {
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
			if !ok || !compiler.appendOccurrence(OccurrenceReturnValue, id, outcome.BodyID(), nil, []keyspace.ContentID{outcome.ID(), value.ID()}, uint64(valueIndex)) {
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
// route receipts. The join runs once while the Program artifact is built:
// Link and Runtime receive only the resulting scalar rows and never reopen or
// rescan Program/Flow.
func (compiler *compiler) deriveBinaryPresenceRefinementsFailure() CompileFailure {
	if compiler == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	nilSources := make(map[keyspace.ContentID]struct{})
	storageOrigins := make(map[keyspace.ContentID]keyspace.ContentID)
	var binaries []OccurrenceRow
	binaryByID := make(map[keyspace.ContentID]OccurrenceRow)
	claims := make(map[keyspace.ContentID]keyspace.ContentID)
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

	bodyByEntry := make(map[keyspace.ContentID]keyspace.ContentID, len(compiler.bodies))
	ambiguousBodyEntry := make(map[keyspace.ContentID]struct{})
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

	branchEdges := make(map[keyspace.ContentID][]EnvironmentEdge)
	for edgeIndex, edge := range compiler.environment {
		condition, conditionOK := edge.ConditionValueSpanID()
		if !edge.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, edgeIndex, -1, CompileReasonOccurrenceUnavailable)
		}
		if !conditionOK {
			continue
		}
		seen := make(map[keyspace.ContentID]struct{})
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
		operand, target := keyspace.ContentID{}, keyspace.ContentID{}
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
			inputs := []keyspace.ContentID{binary.ID(), target, operand, routeID}
			code := uint64(0)
			if present {
				code = 1
			}
			if !id.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.appendOccurrence(OccurrenceBinaryPresenceRefinement, id, bodyID, []keyspace.ContentID{selected.To()}, inputs, code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, binaryIndex, armIndex, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.occurrences[len(compiler.occurrences)-1].Available() ||
				!compiler.recordOccurrencePredecessor(OccurrenceBinaryPresenceRefinement, id, routeID, []keyspace.ContentID{selected.To()}) {
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
	for siteIndex := 0; siteIndex < compiler.input.CausalSiteCount(); siteIndex++ {
		site, ok := compiler.input.CausalSiteAt(siteIndex)
		if !ok || !compiler.input.OwnsSite(site) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, -1, CompileReasonOccurrenceAttachment)
		}
		points, known := compiler.pointIDsBySite[site.ContextID()]
		if !known {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, -1, CompileReasonOccurrenceAttachment)
		}
		for index, pointID := range points {
			id := digest("analysis/program-artifact/point-attachment", artifactFormat, bytesField(site.ContextID()), bytesField(pointID))
			if !compiler.appendOccurrence(OccurrencePointAttachment, id, keyspace.ContentID{}, []keyspace.ContentID{pointID}, []keyspace.ContentID{site.ContextID()}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, siteIndex, index, CompileReasonOccurrenceAttachment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyValueSources() CompileFailure {
	type sourceAt func(int) (program.ValueSourceOccurrence, bool)
	rows := []struct {
		count int
		at    sourceAt
		code  uint64
	}{
		{compiler.input.NilSourceCount(), compiler.input.NilSourceAt, 1}, {compiler.input.BoolSourceCount(), compiler.input.BoolSourceAt, 2},
		{compiler.input.IntegerSourceCount(), compiler.input.IntegerSourceAt, 3}, {compiler.input.FloatSourceCount(), compiler.input.FloatSourceAt, 4},
		{compiler.input.StringSourceCount(), compiler.input.StringSourceAt, 5}, {compiler.input.TypeValueSourceCount(), compiler.input.TypeValueSourceAt, 6},
	}
	for _, family := range rows {
		for index := 0; index < family.count; index++ {
			source, ok := family.at(index)
			if !ok && family.code == 6 {
				// TypeValue's authored denominator includes dead candidates;
				// only an executable proof becomes a ValueSource rule row.
				continue
			}
			if !ok {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceProof)
			}
			if !compiler.input.OwnsValueSourceOccurrence(source) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceOwner)
			}
			body, bodyOK := source.Body()
			if !bodyOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceBody)
			}
			finish, finishOK := source.Finish()
			if !finishOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceFinish)
			}
			points := compiler.pointIDs(finish)
			span, spanOK := source.Span()
			if len(points) == 0 {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourcePoints)
			}
			if !spanOK || !compiler.input.OwnsSpan(span) || !compiler.appendOccurrence(OccurrenceValueSource, source.ContextID(), body.PathID(), points, []keyspace.ContentID{span.ContextID()}, family.code) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			literalFamily, literal, literalOK := source.LiteralPayload()
			if family.code != 6 && !literalOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, int(family.code), CompileReasonOccurrenceValueSourceAppend)
			}
			row := &compiler.occurrences[len(compiler.occurrences)-1]
			row.literalFamily, row.literal, row.literalOK = literalFamily, literal, literalOK
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyComputations() CompileFailure {
	for index := 0; index < compiler.input.BinaryArithmeticOccurrenceCount(); index++ {
		row, ok := compiler.input.BinaryArithmeticOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryArithmetic, row.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...)
		if !binaryArithmeticOperator(row.Op()) || !compiler.appendOccurrence(OccurrenceBinaryArithmetic, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.LeftID(), row.RightID()}, uint64(row.Op())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.BinaryEqualityOccurrenceCount(); index++ {
		row, ok := compiler.input.BinaryEqualityOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			// BinaryEqualityOccurrenceCount is Flow's dense executable primitive
			// denominator. A missing row is corruption, never a dead authored hole.
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryEquality, row.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		inputs := []keyspace.ContentID{row.LeftID(), row.RightID()}
		hasComparison, invert := false, row.Op() == flowkind.BinaryNotEqual
		if branch, whenTrue, whenFalse, comparisonInvert, comparisonOK := row.Comparison(); comparisonOK {
			inputs = append(inputs, branch, whenTrue, whenFalse)
			hasComparison, invert = true, comparisonInvert
		}
		code, codeOK := binaryEqualityCode(row.Op(), hasComparison, invert)
		points := append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...)
		if !codeOK || !compiler.appendOccurrence(OccurrenceBinaryEquality, row.ContextID(), row.BodyPathID(), points, inputs, code) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.BinaryOrderOccurrenceCount(); index++ {
		row, ok := compiler.input.BinaryOrderOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		if len(entryPoints) == 0 || len(finishPoints) == 0 || !compiler.recordOccurrenceSpan(OccurrenceBinaryOrder, row.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAttachment)
		}
		points := append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...)
		if !binaryOrderOperator(row.Op()) || !compiler.appendOccurrence(OccurrenceBinaryOrder, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.LeftID(), row.RightID()}, uint64(row.Op())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.UnaryOccurrenceCount(); index++ {
		row, ok := compiler.input.UnaryOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		entryPoints := compiler.pointIDs(entry)
		finishPoints := compiler.pointIDs(finish)
		points := append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...)
		if len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.recordOccurrenceSpan(OccurrenceUnary, row.ContextID(), entryPoints, finishPoints) ||
			!compiler.appendOccurrence(OccurrenceUnary, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.OperandID()}, uint64(row.Op())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.SelectOccurrenceCount(); index++ {
		row, ok := compiler.input.SelectOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceSelect, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.LeftID(), row.RightID()}, uint64(row.Op())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.ClaimOccurrenceCount(); index++ {
		row, ok := compiler.input.ClaimOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			continue
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceValueClaim, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.OperandID()}, uint64(row.Kind())) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index := 0; index < compiler.input.ReturnOccurrenceCount(); index++ {
		row, ok := compiler.input.ReturnOccurrenceAt(index)
		entry, entryOK := row.Entry()
		finish, finishOK := row.Finish()
		if !ok || !entryOK || !finishOK || !compiler.input.OwnsSite(entry) || !compiler.input.OwnsSite(finish) {
			// ReturnOccurrenceCount is Program's already-sealed executable
			// denominator. Every position must therefore issue one complete
			// Return receipt; omission here would turn corrupted executable
			// geometry into a silently missing artifact boundary.
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		points := append(compiler.pointIDs(entry), compiler.pointIDs(finish)...)
		if !compiler.appendOccurrence(OccurrenceReturnBoundary, row.ContextID(), row.BodyPathID(), points, []keyspace.ContentID{row.ValuesID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyStorage() CompileFailure {
	for index := 0; index < compiler.input.StorageReadCount(); index++ {
		read, ok := compiler.input.StorageReadAt(index)
		if !ok {
			// The Program facade deliberately preserves authored denominator
			// positions while withholding dead/non-executable occurrences.
			// Such a position has no executable artifact row.
			continue
		}
		body, bodyOK := read.Body()
		entry, entryOK := read.Entry()
		finish, finishOK := read.Finish()
		cell, cellOK := read.Cell()
		span, spanOK := read.Span()
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.input.OwnsStorageReadOccurrence(read) || !bodyOK || !entryOK || !finishOK || !cellOK || !spanOK || !compiler.input.OwnsCell(cell) || !compiler.input.OwnsSpan(span) ||
			// A one-input rule cannot select an Entry attachment from the
			// parent's deliberately multi-valued Site relation.  Refuse such
			// a Program until it publishes an explicit occurrence-to-point
			// pairing; never zip or cross-product attachments here.
			len(entryPoints) != 1 ||
			!compiler.appendOccurrence(OccurrenceStorageRead, read.ContextID(), body.PathID(), append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...), []keyspace.ContentID{cell.ContextID(), span.ContextID()}, 0) ||
			!compiler.recordOccurrenceSpan(OccurrenceStorageRead, read.ContextID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageRead)
		}
	}
	for index := 0; index < compiler.input.StorageBindCount(); index++ {
		bind, ok := compiler.input.StorageBindAt(index)
		if !ok {
			continue
		}
		body, bodyOK := bind.Body()
		entry, entryOK := bind.Entry()
		finish, finishOK := bind.Finish()
		values, valuesOK := bind.Values()
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.input.OwnsStorageBind(bind) || !bodyOK || !entryOK || !finishOK || !valuesOK || !compiler.appendOccurrence(OccurrenceStorageBind, bind.ContextID(), body.PathID(), append(entryPoints, finishPoints...), []keyspace.ContentID{values.ID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageBind)
		}
		for position := 0; position < bind.TransferCount(); position++ {
			transfer, transferOK := bind.TransferAt(position)
			if !transferOK {
				// A Bind may declare more destination Cells than its source
				// Values row fixes. The parent position denominator is exact;
				// only fixed positions issue transfer occurrences.
				continue
			}
			transferEntry, transferEntryOK := transfer.Entry()
			transferFinish, transferFinishOK := transfer.Finish()
			value, valueOK := transfer.Value()
			cell, cellOK := transfer.Cell()
			transferEntryPoints, transferFinishPoints := compiler.pointIDs(transferEntry), compiler.pointIDs(transferFinish)
			if !compiler.input.OwnsStorageBindOccurrence(transfer) || !transferEntryOK || !transferFinishOK || !valueOK || !cellOK ||
				// As with a read, the Program must issue one unambiguous Entry
				// attachment for this one-input transfer rule.
				len(transferEntryPoints) != 1 ||
				!compiler.appendOccurrence(OccurrenceStorageBindTransfer, transfer.ContextID(), body.PathID(), transferFinishPoints, []keyspace.ContentID{bind.ContextID(), value.ID(), cell.ContextID()}, uint64(position)) ||
				!compiler.recordOccurrenceSpan(OccurrenceStorageBindTransfer, transfer.ContextID(), transferEntryPoints, transferFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceStorageBind)
			}
		}
	}
	for index := 0; index < compiler.input.StorageAssignmentCount(); index++ {
		assignment, ok := compiler.input.StorageAssignmentAt(index)
		if !ok {
			continue
		}
		body, bodyOK := assignment.Body()
		span, spanOK := assignment.Span()
		values, valuesOK := assignment.Values()
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.input.OwnsStorageAssignment(assignment) || !bodyOK || !spanOK || !valuesOK || !entryOK || !finishOK || !compiler.appendOccurrence(OccurrenceStorageAssignment, assignment.ContextID(), body.PathID(), append(entryPoints, finishPoints...), []keyspace.ContentID{values.ID()}, 0) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceStorageAssignment)
		}
		for position := 0; position < assignment.TransferCount(); position++ {
			write, writeOK := assignment.TransferAt(position)
			if !writeOK {
				// Assignment width follows the destination denominator; a
				// non-fixed source Values position issues no transfer row.
				continue
			}
			writeBody, writeBodyOK := write.Body()
			writeFinish, writeFinishOK := write.Finish()
			predecessor, predecessorOK := write.Predecessor()
			route, routeOK := predecessor.RouteID()
			value, valueOK := write.Value()
			cell, cellOK := write.Cell()
			writeFinishPoints := compiler.pointIDs(writeFinish)
			if !compiler.input.OwnsStorageWriteOccurrence(write) || !writeBodyOK || !writeFinishOK || !predecessorOK || !routeOK || !valueOK || !cellOK ||
				!compiler.appendOccurrence(OccurrenceStorageWrite, write.ContextID(), writeBody.PathID(), writeFinishPoints, []keyspace.ContentID{assignment.ContextID(), value.ID(), cell.ContextID(), predecessor.ContextID(), route}, uint64(position)) ||
				!compiler.recordOccurrencePredecessor(OccurrenceStorageWrite, write.ContextID(), route, writeFinishPoints) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, position, CompileReasonOccurrenceStorageAssignment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyIndexAccess() CompileFailure {
	reads, writes := compiler.input.IndexReads(), compiler.input.IndexWrites()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok {
			// AccessGeometry preserves candidate ordinals whose executable
			// Span proof can be absent. Only an issued role is executable.
			continue
		}
		if !compiler.input.OwnsIndexRead(read) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexCandidate)
		}
		span, spanOK := read.Span()
		base, baseOK := read.Base()
		lens, lensOK := read.Lens()
		result, resultOK := read.Result()
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		if !spanOK || !baseOK || !lensOK || !resultOK || !entryOK || !finishOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		id := read.ContextID()
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.appendOccurrence(OccurrenceIndexRead, id, keyspace.ContentID{}, append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...), []keyspace.ContentID{base.ContextID(), lens.ContextID(), result.ContextID()}, 0) ||
			!compiler.recordOccurrenceSpan(OccurrenceIndexRead, id, entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	for index := 0; index < writes.Count(); index++ {
		write, ok := writes.At(index)
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexCandidate)
		}
		if !compiler.input.OwnsIndexWrite(write) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexCandidate)
		}
		finish, finishOK := write.Finish()
		predecessor, predecessorOK := write.Predecessor()
		route, routeOK := predecessor.RouteID()
		base, baseOK := write.Base()
		lens, lensOK := write.Lens()
		values, valuesOK := write.Values()
		finishPoints := compiler.pointIDs(finish)
		if !finishOK || !predecessorOK || !routeOK || !baseOK || !lensOK || !valuesOK || len(finishPoints) == 0 {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		id := write.ContextID()
		if !compiler.appendOccurrence(OccurrenceIndexWrite, id, keyspace.ContentID{}, finishPoints, []keyspace.ContentID{base.ContextID(), lens.ContextID(), values.ContextID(), predecessor.ContextID(), route}, 0) ||
			!compiler.recordOccurrencePredecessor(OccurrenceIndexWrite, id, route, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexAppend)
		}
	}
	return CompileFailure{}
}

// copyHeapGeometryFailure captures Heap's complete cold source denominator
// while the Program proof is live.  Link later substitutes these scalar rows
// through its own mounted Values/Keys authority; Heap never needs to reopen a
// Program or TransformerInput after artifact compilation.
func (compiler *compiler) copyHeapGeometryFailure() CompileFailure {
	allocations := compiler.input.Allocations()
	compiler.heapAllocations = make([]HeapAllocationRow, 0, allocations.Count())
	for allocationIndex := 0; allocationIndex < allocations.Count(); allocationIndex++ {
		allocation, allocationOK := allocations.At(allocationIndex)
		geometry := allocation.Geometry()
		rootSpan, rootSpanOK := geometry.RootSpan()
		if !allocationOK || !allocations.Owns(allocation) || !geometry.Available() || !rootSpanOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, allocationIndex, -1, CompileReasonOccurrenceAllocation)
		}
		row := HeapAllocationRow{id: allocation.ID(), role: allocation.Role(), form: allocation.Form(), rootSpan: rootSpan.ContextID()}
		if !row.id.Available() || (row.role != program.AllocationTable && row.role != program.AllocationClosure) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, allocationIndex, -1, CompileReasonOccurrenceAllocation)
		}
		for fieldIndex := 0; fieldIndex < geometry.FieldCount(); fieldIndex++ {
			field, fieldOK := geometry.FieldAt(fieldIndex)
			raw, rawOK := field.Field()
			kind, kindOK := field.Kind()
			_, width, finalOpen, valuesOK := field.Values()
			valueOccurrence, valueOccurrenceOK := field.ValueOccurrence()
			valueSpan, valueSpanOK := valueOccurrence.Span()
			normalized, normalizedOK := field.NormalizedKey()
			fieldSpan, fieldSpanOK := field.FieldSpan()
			fieldRow := HeapFieldRow{id: raw.ID(), kind: kind, fieldSpan: fieldSpan.ContextID(), valuesSpan: valueSpan.ContextID(), valuesID: valueOccurrence.ContextID(), width: width, finalOpen: finalOpen, sharesFirstValueCell: field.SharesFirstValueCell(), normalized: normalized, normalizedOK: normalizedOK}
			if kind == flowkind.FieldKey {
				selectorSpan, selectorSpanOK := field.SelectorSpan()
				if !selectorSpanOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, allocationIndex, fieldIndex, CompileReasonOccurrenceAllocation)
				}
				fieldRow.selectorSpan = selectorSpan.ContextID()
			}
			if !fieldOK || !rawOK || !kindOK || !valuesOK || !valueOccurrenceOK || !valueSpanOK || !fieldSpanOK || !fieldRow.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, allocationIndex, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			row.fields = append(row.fields, fieldRow)
		}
		if !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, allocationIndex, -1, CompileReasonOccurrenceAllocation)
		}
		compiler.heapAllocations = append(compiler.heapAllocations, row)
	}
	reads, writes := compiler.input.IndexReads(), compiler.input.IndexWrites()
	compiler.heapIndexes = make([]HeapIndexRow, 0, reads.Count()+writes.Count())
	for index := 0; index < reads.Count(); index++ {
		occurrence, occurrenceOK := reads.At(index)
		base, baseOK := occurrence.Base()
		baseSpan, baseSpanOK := base.Span()
		lens, lensOK := occurrence.Lens()
		result, resultOK := occurrence.Result()
		resultSpan, resultSpanOK := result.Span()
		row := HeapIndexRow{id: occurrence.ContextID(), read: true, baseSpan: baseSpan.ContextID(), resultSpan: resultSpan.ContextID(), position: -1}
		if lensOK && lens.Kind() == program.IndexLensExact {
			row.lensKind = 1
			row.exactKey, _ = lens.ExactKey()
		} else if lensOK && lens.Kind() == program.IndexLensDynamic {
			source, sourceOK := lens.Source()
			if sourceOK {
				row.lensKind, row.keySpan = 2, source.ContextID()
			}
		}
		if !occurrenceOK || !baseOK || !baseSpanOK || !lensOK || !resultOK || !resultSpanOK || !compiler.input.OwnsIndexRead(occurrence) || !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	for index := 0; index < writes.Count(); index++ {
		occurrence, occurrenceOK := writes.At(index)
		value, valueOK := occurrence.Values()
		valueOccurrence, valueOccurrenceOK := value.Occurrence()
		valueSpan, valueSpanOK := valueOccurrence.Span()
		base, baseOK := occurrence.Base()
		baseSpan, baseSpanOK := base.Span()
		lens, lensOK := occurrence.Lens()
		row := HeapIndexRow{id: occurrence.ContextID(), baseSpan: baseSpan.ContextID(), valuesSpan: valueSpan.ContextID(), valuesID: valueOccurrence.ContextID(), position: value.Position()}
		if lensOK && lens.Kind() == program.IndexLensExact {
			row.lensKind = 1
			row.exactKey, _ = lens.ExactKey()
		} else if lensOK && lens.Kind() == program.IndexLensDynamic {
			source, sourceOK := lens.Source()
			if sourceOK {
				row.lensKind, row.keySpan = 2, source.ContextID()
			}
		}
		if !occurrenceOK || !valueOK || !valueOccurrenceOK || !valueSpanOK || !baseOK || !baseSpanOK || !lensOK || !compiler.input.OwnsIndexWrite(occurrence) || !row.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		compiler.heapIndexes = append(compiler.heapIndexes, row)
	}
	return CompileFailure{}
}

func (compiler *compiler) copyAllocations() CompileFailure {
	allocations := compiler.input.Allocations()
	for index := 0; index < allocations.Count(); index++ {
		allocation, ok := allocations.At(index)
		geometry := allocation.Geometry()
		span := geometry.Span()
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		template := allocation.Template()
		occurrence := allocation.SemanticOccurrence()
		if !ok || !allocations.Owns(allocation) || !geometry.Available() || !entryOK || !finishOK || !template.Available() || !occurrence.Available() ||
			!compiler.appendOccurrence(OccurrenceAllocation, allocation.ID(), keyspace.ContentID{}, append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...), []keyspace.ContentID{template.ID(), occurrence.ID()}, uint64(allocation.Form())) ||
			!compiler.recordOccurrenceSpan(OccurrenceAllocation, allocation.ID(), entryPoints, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		for fieldIndex := 0; fieldIndex < geometry.FieldCount(); fieldIndex++ {
			field, fieldOK := geometry.FieldAt(fieldIndex)
			raw, rawOK := field.Field()
			values, valuesOK := field.ValueOccurrence()
			inputs := []keyspace.ContentID{allocation.ID()}
			if valuesOK {
				inputs = append(inputs, values.ID())
				for memberIndex := 0; memberIndex < values.Count(); memberIndex++ {
					member, memberOK := values.At(memberIndex)
					if !memberOK {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
					}
					inputs = append(inputs, member.ID())
				}
			}
			if !fieldOK || !rawOK || !valuesOK || !compiler.appendOccurrence(OccurrenceAllocationField, raw.ID(), keyspace.ContentID{}, nil, inputs, uint64(fieldIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) copyCalls() CompileFailure {
	for index := 0; index < compiler.input.CallCount(); index++ {
		call, ok := compiler.input.CallAt(index)
		if !ok {
			// CallCount preserves authored rows whose later executable proof
			// join is intentionally unavailable.
			continue
		}
		body, bodyOK := call.Body()
		span, spanOK := call.Span()
		callee, calleeOK := call.Callee()
		actuals, actualsOK := call.Actuals()
		values, valuesOK := call.Values()
		formal, formalOK := call.Formal()
		types, typesOK := call.TypeArguments()
		entry, entryOK := span.Entry()
		finish, finishOK := span.Finish()
		inputs := []keyspace.ContentID{callee.ContextID(), actuals.ContextID(), values.ContextID(), formal.ID(), types.ContextID()}
		if receiver, receiverOK := call.Receiver(); receiverOK {
			inputs = append(inputs, receiver.ContextID())
		}
		entryPoints, finishPoints := compiler.pointIDs(entry), compiler.pointIDs(finish)
		if !compiler.input.OwnsCallOccurrence(call) || !bodyOK || !spanOK || !calleeOK || !actualsOK || !valuesOK || !formalOK || !typesOK || !entryOK || !finishOK || len(entryPoints) == 0 || len(finishPoints) == 0 ||
			!compiler.appendOccurrence(OccurrenceCall, call.ContextID(), body.PathID(), append(append([]keyspace.ContentID(nil), entryPoints...), finishPoints...), inputs, uint64(call.Disposition())) ||
			!compiler.recordOccurrenceSpan(OccurrenceCall, call.ContextID(), entryPoints, finishPoints) ||
			!compiler.appendOccurrence(OccurrenceCallActivation, call.ContextID(), body.PathID(), append([]keyspace.ContentID(nil), finishPoints...), inputs, uint64(call.Disposition())) ||
			!compiler.recordOccurrenceSpan(OccurrenceCallActivation, call.ContextID(), nil, finishPoints) {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
		}
		for argIndex := 0; argIndex < values.Count(); argIndex++ {
			argument, argumentOK := values.At(argIndex)
			if !argumentOK || !compiler.input.OwnsCallArgument(argument) || !compiler.appendOccurrence(OccurrenceCallArgument, argument.ContextID(), body.PathID(), nil, []keyspace.ContentID{call.ContextID()}, uint64(argIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, argIndex, CompileReasonOccurrenceCall)
			}
		}
		for typeIndex := 0; typeIndex < types.Count(); typeIndex++ {
			argument, argumentOK := types.At(typeIndex)
			if !argumentOK || !compiler.input.OwnsCallTypeArgument(argument) || !compiler.appendOccurrence(OccurrenceCallTypeArgument, argument.ContextID(), body.PathID(), nil, []keyspace.ContentID{call.ContextID()}, uint64(typeIndex)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, typeIndex, CompileReasonOccurrenceCall)
			}
		}
		if boundary, boundaryOK := call.Boundary(); boundaryOK {
			if !compiler.input.OwnsCallBoundary(boundary) || !compiler.appendOccurrence(OccurrenceCallBoundary, boundary.ContextID(), body.PathID(), nil, []keyspace.ContentID{call.ContextID()}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceCall)
			}
			for armIndex := 0; armIndex < boundary.ArmCount(); armIndex++ {
				arm, armOK := boundary.ArmAt(armIndex)
				target, targetOK := arm.Target()
				route, routeOK := arm.RouteDigest()
				if !armOK || !targetOK || !routeOK || !compiler.input.OwnsCallArm(arm) || !compiler.appendOccurrence(OccurrenceCallArm, arm.ContextID(), body.PathID(), compiler.pointIDs(target), []keyspace.ContentID{boundary.ContextID(), route, target.ContextID()}, uint64(armIndex)) {
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
		appendBase := func(role RuleRole, inputKind RuleInputKind, input []keyspace.ContentID) bool {
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
		appendLocal := func(role RuleRole, inputKind RuleInputKind, inputs []keyspace.ContentID) bool {
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
			if ok && row.code == uint64(program.AllocationFormEmpty) {
				ok = appendLocal(RuleRoleHeapEmpty, RuleInputFinish, finish)
			}
			if ok && row.code == uint64(program.AllocationFormClosed) {
				ok = appendLocal(RuleRoleHeapClosed, RuleInputFinish, finish)
			}
		}
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}

type computationStage struct {
	base       keyspace.ContentID
	point      keyspace.ContentID
	occurrence keyspace.ContentID
	role       RuleRole
	left       keyspace.ContentID
	right      keyspace.ContentID
}

func (stage computationStage) available() bool {
	return stage.base.Available() && stage.point.Available() && stage.point != stage.base && stage.occurrence.Available() &&
		(stage.role == RuleRoleValueBinaryArithmetic || stage.role == RuleRoleValueBinaryEquality || stage.role == RuleRoleValueBinaryOrder) &&
		stage.left.Available() && stage.right.Available()
}

func (compiler *compiler) localComputationStage(base keyspace.ContentID, role RuleRole, occurrence, left, right keyspace.ContentID) (keyspace.ContentID, bool) {
	if compiler == nil || compiler.computationStages == nil || !base.Available() || !occurrence.Available() ||
		(role != RuleRoleValueBinaryArithmetic && role != RuleRoleValueBinaryEquality && role != RuleRoleValueBinaryOrder) || !left.Available() || !right.Available() {
		return keyspace.ContentID{}, false
	}
	point := digest("analysis/program-artifact/local-computation-stage", artifactFormat,
		bytesField(base), uintField(uint64(role)), bytesField(occurrence))
	stage := computationStage{base: base, point: point, occurrence: occurrence, role: role, left: left, right: right}
	if !stage.available() {
		return keyspace.ContentID{}, false
	}
	for _, prior := range compiler.computationStages[base] {
		if prior.point == point || prior.occurrence == occurrence {
			return keyspace.ContentID{}, false
		}
	}
	compiler.computationStages[base] = append(compiler.computationStages[base], stage)
	return point, true
}

// orderedLocalComputations closes the Program-local primitive dependency
// graph without consulting Link coordinates. Binary operands and binary
// results share the same Program semantic span identity, so nested
// computations induce exact edges. Unrelated ready rows use their stable
// stage identity only as a canonical serialization order.
func (compiler *compiler) orderedLocalComputations(base keyspace.ContentID) ([]computationStage, bool) {
	if compiler == nil || !base.Available() {
		return nil, false
	}
	rows := compiler.computationStages[base]
	if len(rows) == 0 {
		return nil, true
	}
	producer := make(map[keyspace.ContentID]int, len(rows))
	pointIndex := make(map[keyspace.ContentID]int, len(rows))
	for index, row := range rows {
		if !row.available() || row.base != base {
			return nil, false
		}
		if _, duplicate := producer[row.occurrence]; duplicate {
			return nil, false
		}
		if _, duplicate := pointIndex[row.point]; duplicate {
			return nil, false
		}
		producer[row.occurrence] = index
		pointIndex[row.point] = index
	}
	ordered := make([]computationStage, 0, len(rows))
	placed := make([]bool, len(rows))
	for len(ordered) < len(rows) {
		ready := make([]keyspace.ContentID, 0, len(rows)-len(ordered))
		for index, row := range rows {
			if placed[index] {
				continue
			}
			blocked := false
			for _, input := range [...]keyspace.ContentID{row.left, row.right} {
				if dependency, found := producer[input]; found && !placed[dependency] {
					blocked = true
					break
				}
			}
			if !blocked {
				ready = append(ready, row.point)
			}
		}
		if len(ready) == 0 {
			return nil, false
		}
		radixContentIDs(ready)
		index := pointIndex[ready[0]]
		placed[index] = true
		ordered = append(ordered, rows[index])
	}
	return ordered, true
}

type callStageSet struct {
	dispatch keyspace.ContentID
	summary  keyspace.ContentID
	effect   keyspace.ContentID
}

func (stages callStageSet) available(base keyspace.ContentID) bool {
	return base.Available() && stages.dispatch.Available() && stages.summary.Available() && stages.effect.Available() &&
		stages.dispatch != base && stages.summary != base && stages.effect != base &&
		stages.dispatch != stages.summary && stages.dispatch != stages.effect && stages.summary != stages.effect
}

func (compiler *compiler) callStage(base keyspace.ContentID) (callStageSet, bool) {
	if compiler == nil || compiler.callStages == nil || !base.Available() {
		return callStageSet{}, false
	}
	if stages := compiler.callStages[base]; stages.available(base) {
		return stages, true
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return callStageSet{}, false
	}
	stages := callStageSet{
		dispatch: digest("analysis/program-artifact/call-dispatch-stage", artifactFormat, bytesField(base)),
		summary:  digest("analysis/program-artifact/call-summary-stage", artifactFormat, bytesField(base)),
		effect:   digest("analysis/program-artifact/call-effect-stage", artifactFormat, bytesField(base)),
	}
	if !stages.available(base) {
		return callStageSet{}, false
	}
	compiler.callStages[base] = stages
	return stages, true
}

func (compiler *compiler) localStage(base keyspace.ContentID) (keyspace.ContentID, bool) {
	if compiler == nil || compiler.localStages == nil || !base.Available() {
		return keyspace.ContentID{}, false
	}
	if stage := compiler.localStages[base]; stage.Available() {
		return stage, true
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return keyspace.ContentID{}, false
	}
	stage := digest("analysis/program-artifact/local-stage", artifactFormat, bytesField(base))
	if !stage.Available() || stage == base {
		return keyspace.ContentID{}, false
	}
	compiler.localStages[base] = stage
	return stage, true
}

// installLocalStagesFailure splices every reusable synthetic execution cut
// into the exact Program WTO stream. A route-specific entry refinement is a
// forward overlay: its guarded ingress targets the stage and Program-issued
// successor continuations depart that stage. It never merges back into the
// base, because base→stage→base would fabricate a recurrence.
func (compiler *compiler) installLocalStagesFailure() CompileFailure {
	if len(compiler.localStages) == 0 && len(compiler.computationStages) == 0 && len(compiler.callStages) == 0 {
		return CompileFailure{}
	}
	baseSet := make(map[keyspace.ContentID]struct{}, len(compiler.localStages)+len(compiler.computationStages)+len(compiler.callStages))
	for base := range compiler.localStages {
		baseSet[base] = struct{}{}
	}
	for base := range compiler.computationStages {
		baseSet[base] = struct{}{}
	}
	for base := range compiler.callStages {
		baseSet[base] = struct{}{}
	}
	bases := make([]keyspace.ContentID, 0, len(baseSet))
	for base := range baseSet {
		bases = append(bases, base)
	}
	radixContentIDs(bases)
	stageFor := make(map[keyspace.ContentID][]keyspace.ContentID, len(bases))
	stageExit := make(map[keyspace.ContentID]keyspace.ContentID, len(bases))
	computationInput := make(map[keyspace.ContentID]keyspace.ContentID)
	callInput := make(map[keyspace.ContentID]keyspace.ContentID)
	appendTransfer := func(domain string, from, to keyspace.ContentID, full bool, roles ...RuleRole) bool {
		fields := []field{bytesField(from), bytesField(to), boolField(full), uintField(uint64(len(roles)))}
		for _, role := range roles {
			fields = append(fields, uintField(uint64(role)))
		}
		edge := LocalTransfer{id: digest(domain, artifactFormat, fields...), from: from, to: to, full: full, roles: append([]RuleRole(nil), roles...)}
		if !edge.Available() {
			return false
		}
		compiler.localTransfers = append(compiler.localTransfers, edge)
		return true
	}
	for index, base := range bases {
		geometry, baseOK := compiler.pointGeometry[base]
		if !baseOK || !geometry.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		sequence := make([]keyspace.ContentID, 0, 5)
		predecessor := base
		if local := compiler.localStages[base]; local.Available() {
			sequence = append(sequence, local)
			if !appendTransfer("analysis/program-artifact/local-transfer", base, local, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			predecessor = local
			stageExit[base] = local
		}
		computations, computationsOK := compiler.orderedLocalComputations(base)
		if !computationsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for _, computation := range computations {
			sequence = append(sequence, computation.point)
			if !appendTransfer("analysis/program-artifact/local-computation-transfer", predecessor, computation.point, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			computationInput[computation.point] = predecessor
			predecessor = computation.point
			stageExit[base] = computation.point
		}
		callBase := predecessor
		if stages := compiler.callStages[base]; stages.available(base) {
			sequence = append(sequence, stages.dispatch, stages.summary, stages.effect)
			if !appendTransfer("analysis/program-artifact/call-base-dispatch-transfer", callBase, stages.dispatch, false, RuleRoleValueSource, RuleRolePackSource, RuleRoleHeapIngress, RuleRoleCallDispatch) ||
				!appendTransfer("analysis/program-artifact/call-base-summary-transfer", callBase, stages.summary, false, RuleRoleEffectSelected) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-summary-transfer", stages.dispatch, stages.summary, false, RuleRoleCallDispatch) ||
				!appendTransfer("analysis/program-artifact/call-base-effect-transfer", callBase, stages.effect, true) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-effect-transfer", stages.dispatch, stages.effect, false, RuleRoleCallDispatch) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			callInput[stages.dispatch] = callBase
			stageExit[base] = stages.effect
		}
		for _, stage := range sequence {
			if !stage.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			if _, duplicate := compiler.points[stage]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			compiler.points[stage] = struct{}{}
			compiler.pointGeometry[stage] = Point{id: stage, decisions: append([]keyspace.ContentID(nil), geometry.decisions...)}
		}
		stageFor[base] = sequence
	}

	// Ordinary Local/Call stages replace their base's outgoing route source.
	// There is no route-local continuation overlay here: an unproved control
	// bridge must never be replayed through a shared base point.
	originalCount := len(compiler.environment)
	for index := 0; index < originalCount; index++ {
		edge := &compiler.environment[index]
		// A same-point, component-free route is the parent-issued predecessor
		// boundary consumed by StorageWrite itself. It is not a downstream
		// control successor and must remain base -> base so the rule reads the
		// pre-write environment rather than its own Local output.
		if edge.from == edge.to && !edge.component.Available() && !edge.hasMu && !edge.hasReset {
			continue
		}
		if exit := stageExit[edge.from]; exit.Available() {
			edge.from = exit
			if !edge.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
		}
	}
	compiler.environmentByRoute = make(map[keyspace.ContentID]EnvironmentEdge, len(compiler.environment))
	compiler.environmentRouteDuplicates = make(map[keyspace.ContentID]struct{})
	for _, edge := range compiler.environment {
		if _, duplicate := compiler.environmentByRoute[edge.route]; duplicate {
			compiler.environmentRouteDuplicates[edge.route] = struct{}{}
		} else {
			compiler.environmentByRoute[edge.route] = edge
		}
	}
	for role, rows := range compiler.ruleOccurrences {
		for index := range rows {
			if rows[index].inputKind == RuleInputNone {
				continue
			}
			if rows[index].inputKind == RuleInputPredecessor {
				edge, found := compiler.environmentByRoute[rows[index].route]
				if !found || !edge.from.Available() {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = edge.from
				continue
			}
			if input, computation := computationInput[rows[index].point]; computation {
				if rows[index].inputKind != RuleInputFinish || !input.Available() || input == rows[index].point {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = input
				continue
			}
			if input, dispatch := callInput[rows[index].point]; dispatch {
				if rows[index].stage != RuleStageCallDispatch || rows[index].inputKind != RuleInputFinish || !input.Available() || input == rows[index].point {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = input
				continue
			}

			// A synthetic stage is an execution splice for rule inputs as well as
			// structural routes. A rule producing the base's Local stage must read
			// the original base. Call Dispatch reads that Local result when one
			// exists. Every other consumer of the staged base reads the terminal
			// stage, so no Entry/Finish rule can bypass a prior strong write.
			base := rows[index].input
			exit := stageExit[base]
			if !exit.Available() {
				continue
			}
			local := compiler.localStages[base]
			if local.Available() && rows[index].point == local {
				continue
			}
			rows[index].input = exit
			if !rows[index].Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		compiler.ruleOccurrences[role] = rows
	}
	stageCount := 0
	for _, stages := range stageFor {
		stageCount += len(stages)
	}
	events := make([]WTOEvent, 0, len(compiler.events)+stageCount)
	seenPost := make(map[keyspace.ContentID]struct{}, len(stageFor))
	for _, event := range compiler.events {
		events = append(events, event)
		if event.kind != WTOEventPoint {
			continue
		}
		stages, staged := stageFor[event.point]
		if !staged {
			continue
		}
		if _, duplicate := seenPost[event.point]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventPointRepeated)
		}
		seenPost[event.point] = struct{}{}
		for _, stage := range stages {
			events = append(events, WTOEvent{kind: WTOEventPoint, point: stage})
		}
	}
	if len(seenPost) != len(stageFor) {
		return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventReference)
	}
	compiler.events = events

	regionMembership := make(map[keyspace.ContentID]int, len(stageFor))
	for regionIndex := range compiler.regions {
		members := compiler.regions[regionIndex].members
		rewritten := make([]keyspace.ContentID, 0, len(members)+len(stageFor))
		for _, member := range members {
			rewritten = append(rewritten, member)
			injected := false
			if stages, staged := stageFor[member]; staged {
				rewritten = append(rewritten, stages...)
				injected = true
			}
			if injected {
				regionMembership[member]++
			}
		}
		compiler.regions[regionIndex].members = rewritten
	}
	for base, count := range regionMembership {
		if count > 1 || !base.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowRegion, -1, -1, CompileReasonRegionReference)
		}
	}
	return CompileFailure{}
}
