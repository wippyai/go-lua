package result

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/sendsafety"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// NativePublicationLane is the closed public partition of native analysis
// rows. It is Analysis vocabulary: Engine never classifies or renders a
// native row.
type NativePublicationLane uint8

const (
	NativePublicationLaneInvalid NativePublicationLane = iota
	NativePublicationLaneValues
	NativePublicationLaneSend
)

func (lane NativePublicationLane) Valid() bool {
	return lane == NativePublicationLaneValues || lane == NativePublicationLaneSend
}

func (lane NativePublicationLane) String() string {
	switch lane {
	case NativePublicationLaneValues:
		return "values"
	case NativePublicationLaneSend:
		return "send"
	default:
		return ""
	}
}

// NativePublicationKind is the closed semantic family of a native row. The
// user-facing family string is a render projection of this kind; arbitrary
// string families are not semantic authority.
type NativePublicationKind uint8

const (
	NativePublicationKindInvalid NativePublicationKind = iota
	NativePublicationKindValue
	NativePublicationKindSendSafety
)

func (kind NativePublicationKind) Valid() bool {
	return kind == NativePublicationKindValue || kind == NativePublicationKindSendSafety
}

func (kind NativePublicationKind) String() string {
	switch kind {
	case NativePublicationKindValue:
		return "value"
	case NativePublicationKindSendSafety:
		return "send_safety"
	default:
		return ""
	}
}

// NativePublicationTrust describes the authority of a published row. A
// missing or unavailable producer is not silently widened to Proven.
type NativePublicationTrust uint8

const (
	NativePublicationTrustInvalid NativePublicationTrust = iota
	NativePublicationTrustProven
)

func (trust NativePublicationTrust) Valid() bool {
	return trust == NativePublicationTrustProven
}

func (trust NativePublicationTrust) String() string {
	switch trust {
	case NativePublicationTrustProven:
		return "proven"
	default:
		return ""
	}
}

// NativePublicationProvenance is the detached, owner-issued mount geometry of
// one row. All IDs are immutable scalar identities; it retains no Program,
// Link, Engine, State, or domain handle.
type NativePublicationProvenance struct {
	// context is populated for context-qualified diagnostic branch rows. Native
	// artifact/value rows retain the zero value because they are Link-global
	// facts; an available context is part of the row identity when present.
	context  identity.ContentID
	mount    identity.ContentID
	artifact identity.ContentID
	local    identity.ContentID
	body     identity.ContentID
	point    identity.ContentID
	span     identity.ContentID
}

func (value NativePublicationProvenance) ContextID() identity.ContentID  { return value.context }
func (value NativePublicationProvenance) MountID() identity.ContentID    { return value.mount }
func (value NativePublicationProvenance) ArtifactID() identity.ContentID { return value.artifact }
func (value NativePublicationProvenance) LocalID() identity.ContentID    { return value.local }
func (value NativePublicationProvenance) BodyID() identity.ContentID     { return value.body }
func (value NativePublicationProvenance) PointID() identity.ContentID    { return value.point }
func (value NativePublicationProvenance) SourceSpanID() identity.ContentID {
	return value.span
}

// NativePublicationValidity records a producer-issued proof interval. Empty
// fields mean the producer intentionally issued no temporal qualification;
// consumers must not invent one from absent events.
type NativePublicationValidity struct {
	event              identity.ContentID
	established        identity.ContentID
	establishedOrdinal uint64
	revoked            identity.ContentID
	revokedOrdinal     uint64
}

func (value NativePublicationValidity) EventID() identity.ContentID       { return value.event }
func (value NativePublicationValidity) EstablishedID() identity.ContentID { return value.established }
func (value NativePublicationValidity) EstablishedOrdinal() uint64        { return value.establishedOrdinal }
func (value NativePublicationValidity) RevokedID() identity.ContentID     { return value.revoked }
func (value NativePublicationValidity) RevokedOrdinal() uint64            { return value.revokedOrdinal }

// Valid reports a producer-issued proof interval.
func (value NativePublicationValidity) Valid() bool { return value.valid() }

func (value NativePublicationValidity) valid() bool {
	if !value.established.Available() {
		return !value.event.Available() && !value.revoked.Available() && value.establishedOrdinal == 0 && value.revokedOrdinal == 0
	}
	if value.establishedOrdinal == 0 {
		return false
	}
	if !value.revoked.Available() {
		return !value.event.Available() && value.revokedOrdinal == 0
	}
	return value.event.Available() && value.revoked != value.established && value.revokedOrdinal > value.establishedOrdinal
}

// NativePublicationToken is an opaque Result-local cursor. It cannot be
// replayed against another Result, even one with equal content.
type NativePublicationToken struct {
	owner   *Result
	ordinal uint32
}

// NativePublication is one immutable native row. Its fields are available
// only through Result-issued cursors, which prevents foreign or stale
// publication rows from being mistaken for a row of this result.
type NativePublication struct{ token NativePublicationToken }

func (row NativePublication) Token() NativePublicationToken { return row.token }
func (row NativePublication) ID() (identity.ContentID, bool) {
	value, ok := row.resolve()
	return value.id, ok
}
func (row NativePublication) Lane() NativePublicationLane {
	value, ok := row.resolve()
	if !ok {
		return NativePublicationLaneInvalid
	}
	return value.lane
}
func (row NativePublication) Trust() NativePublicationTrust {
	value, ok := row.resolve()
	if !ok {
		return NativePublicationTrustInvalid
	}
	return value.trust
}
func (row NativePublication) Kind() NativePublicationKind {
	value, ok := row.resolve()
	if !ok {
		return NativePublicationKindInvalid
	}
	return value.kind
}

// SemanticID is the closed schema key that authorized the row's kind.
func (row NativePublication) SemanticID() identity.ContentID {
	value, ok := row.resolve()
	if !ok {
		return identity.ContentID{}
	}
	return value.semantic
}
func (row NativePublication) Family() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.family.String()
}
func (row NativePublication) Key() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.key
}
func (row NativePublication) Module() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.module
}
func (row NativePublication) Term() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.term
}
func (row NativePublication) Subject() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.subject
}
func (row NativePublication) Occurrence() string {
	value, ok := row.resolve()
	if !ok {
		return ""
	}
	return value.occurrence
}

// The typed column surface. A native row publishes facts, not a sentence:
// every accessor below reports one column and whether the row publishes it, so
// no consumer recovers a fact by taking a rendered string apart.

// Exact reports that the published carrier is the value's exact carrier rather
// than a widened one.
func (row NativePublication) Exact() bool {
	value, ok := row.resolve()
	return ok && value.content.exact
}

// Literal is the proved constant: its kind and its exact bits. Infinities,
// NaN, and signed zero are ordinary members of this column.
func (row NativePublication) Literal() (keyspace.LiteralValue, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.literalAvailable() {
		return keyspace.LiteralValue{}, false
	}
	return value.content.literal, true
}

// ScalarRepresentation is the carrier a proved exact scalar is published
// under, including Lua nil.
func (row NativePublication) ScalarRepresentation() (NativeScalarRepresentation, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.scalar.Available() {
		return NativeScalarRepresentationInvalid, false
	}
	return value.content.scalar, true
}

// Representation is the proved numeric carrier of the row's result.
func (row NativePublication) Representation() (programschema.NumericRepresentation, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.representation.Valid() {
		return programschema.NumericRepresentationInvalid, false
	}
	return value.content.representation, true
}

// LeftRepresentation and RightRepresentation are the proved numeric carriers of
// a binary operator's operands; Operand is the carrier of a unary operator's
// only operand.
func (row NativePublication) LeftRepresentation() (programschema.NumericRepresentation, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.left.Valid() {
		return programschema.NumericRepresentationInvalid, false
	}
	return value.content.left, true
}

func (row NativePublication) RightRepresentation() (programschema.NumericRepresentation, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.right.Valid() {
		return programschema.NumericRepresentationInvalid, false
	}
	return value.content.right, true
}

func (row NativePublication) OperandRepresentation() (programschema.NumericRepresentation, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.operand.Valid() {
		return programschema.NumericRepresentationInvalid, false
	}
	return value.content.operand, true
}

// BinaryOperator and UnaryOperator are the proved operator. A row publishes at
// most one of them.
func (row NativePublication) BinaryOperator() (flowkind.BinaryOp, bool) {
	value, ok := row.resolve()
	if !ok || !flowkind.IsBinaryArithmetic(value.content.binary) {
		return 0, false
	}
	return value.content.binary, true
}

func (row NativePublication) UnaryOperator() (flowkind.UnaryOp, bool) {
	value, ok := row.resolve()
	if !ok || value.content.unary == 0 {
		return 0, false
	}
	return value.content.unary, true
}

// Overflow is the arithmetic discipline the proved operator evaluates under.
func (row NativePublication) Overflow() (valuedomain.NumericOverflow, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.overflow.Valid() {
		return valuedomain.NumericOverflowInvalid, false
	}
	return value.content.overflow, true
}

// Divisor is the divisor proof an integer division carries.
func (row NativePublication) Divisor() (NativeDivisorProperty, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.divisor.Available() {
		return NativeDivisorPropertyInvalid, false
	}
	return value.content.divisor, true
}

// Truthiness is the branch condition's verdict over its whole evidence set. Its
// unobserved member is an incomplete fold, which is not the same answer as a
// condition proved to take both truths.
func (row NativePublication) Truthiness() (NativeTruthinessClass, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.truthiness.Available() {
		return NativeTruthinessClassInvalid, false
	}
	return value.content.truthiness, true
}

// Partition is the branch geometry the truth fold licenses.
func (row NativePublication) Partition() (NativeBranchPartition, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.partition.Available() {
		return NativeBranchPartitionInvalid, false
	}
	return value.content.partition, true
}

// DeadArm is the arm a proved partition proves dead, and DeadArmReachable is
// that arm's reachability. A row that proves no partition publishes neither.
func (row NativePublication) DeadArm() (NativeBranchArm, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.deadArm.Available() {
		return NativeBranchArmInvalid, false
	}
	return value.content.deadArm, true
}

func (row NativePublication) DeadArmReachable() (bool, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.deadArm.Available() {
		return false, false
	}
	return value.content.deadArmReachable, true
}

// SendSafety is the proved allocation-level send strategy. It is present only
// on a send-safety publication; other native rows leave this column absent.
func (row NativePublication) SendSafety() (sendsafety.Verdict, bool) {
	value, ok := row.resolve()
	if !ok || !value.content.sendSafety.Available() {
		return sendsafety.VerdictNone, false
	}
	return value.content.sendSafety, true
}

// Column resolves one vocabulary-valued column to its member ordinal in the
// column's declared category. A renderer reads the declared spelling at that
// ordinal; nothing here renders a name of its own.
func (row NativePublication) Column(column NativePublicationColumn) (uint16, bool) {
	value, ok := row.resolve()
	if !ok || !column.Available() {
		return 0, false
	}
	return value.content.column(column)
}

// EvidencePointCount and EvidencePointAt are the row's evidence set: the whole
// set of points the row was folded over, in ascending identity order.
func (row NativePublication) EvidencePointCount() int {
	value, ok := row.resolve()
	if !ok {
		return 0
	}
	return len(value.content.points)
}

func (row NativePublication) EvidencePointAt(index int) (identity.ContentID, bool) {
	value, ok := row.resolve()
	if !ok || index < 0 || index >= len(value.content.points) {
		return identity.ContentID{}, false
	}
	return value.content.points[index], true
}
func (row NativePublication) Provenance() (NativePublicationProvenance, bool) {
	value, ok := row.resolve()
	if !ok || !value.provenanceOK {
		return NativePublicationProvenance{}, false
	}
	return value.provenance, true
}
func (row NativePublication) Validity() (NativePublicationValidity, bool) {
	value, ok := row.resolve()
	if !ok || !value.validityOK {
		return NativePublicationValidity{}, false
	}
	return value.validity, true
}

// NativePublicationAvailable reports whether a post-convergence typed owner
// published native output for this Result. It is intentionally distinct from
// an available empty publication: an absent producer must not look like a
// successful analysis with zero native facts.
func (result *Result) NativePublicationAvailable() bool {
	return result != nil && result.valid() && nativePublicationStateAvailable(result.nativePublished, result.nativeContent, result.nativeRows, result.nativeByID)
}

// NativePublicationCount, At, ByID, and ByToken are Result's closed native
// surface. At and the two lookup paths are O(1); the ID index holds only
// dense-row ordinals and never a second topology or row copy.
func (result *Result) NativePublicationCount() int {
	if !result.NativePublicationAvailable() {
		return 0
	}
	return len(result.nativeRows)
}
func (result *Result) NativePublicationAt(index int) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || index < 0 || index >= len(result.nativeRows) {
		return NativePublication{}, false
	}
	ordinal := uint32(index + 1)
	if _, ok := result.nativeRowAt(ordinal); !ok {
		return NativePublication{}, false
	}
	return NativePublication{token: NativePublicationToken{owner: result, ordinal: ordinal}}, true
}
func (result *Result) NativePublicationByID(id identity.ContentID) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || !id.Available() {
		return NativePublication{}, false
	}
	ordinal, ok := result.nativeByID[id]
	if !ok || ordinal == 0 {
		return NativePublication{}, false
	}
	row, rowOK := result.nativeRowAt(ordinal)
	if !rowOK || row.id != id {
		return NativePublication{}, false
	}
	return NativePublication{token: NativePublicationToken{owner: result, ordinal: ordinal}}, true
}
func (result *Result) NativePublicationByToken(token NativePublicationToken) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || token.owner != result || token.ordinal == 0 || int(token.ordinal) > len(result.nativeRows) {
		return NativePublication{}, false
	}
	if _, ok := result.nativeRowAt(token.ordinal); !ok {
		return NativePublication{}, false
	}
	return NativePublication{token: token}, true
}

func (row NativePublication) resolve() (nativePublicationRow, bool) {
	owner, ordinal := row.token.owner, row.token.ordinal
	if owner == nil || !owner.NativePublicationAvailable() {
		return nativePublicationRow{}, false
	}
	return owner.nativeRowAt(ordinal)
}

// nativePublicationFamily is the closed semantic denominator for the first
// native publication slice. New families extend this owner; callers
// cannot authorize a row by supplying a string.
type nativePublicationFamily uint8

const (
	nativePublicationFamilyInvalid nativePublicationFamily = iota
	nativePublicationFamilyConstantValue
	nativePublicationFamilyRepresentation
	nativePublicationFamilyTruthinessClass
	nativePublicationFamilyBranchPartition
	nativePublicationFamilyScalarOperator
	nativePublicationFamilyDivisorProperty
	nativePublicationFamilySendSafety
)

func (family nativePublicationFamily) String() string {
	switch family {
	case nativePublicationFamilyConstantValue:
		return "constant_value"
	case nativePublicationFamilyRepresentation:
		return "representation"
	case nativePublicationFamilyTruthinessClass:
		return "truthiness_class"
	case nativePublicationFamilyBranchPartition:
		return "branch_partition"
	case nativePublicationFamilyScalarOperator:
		return "scalar_operator"
	case nativePublicationFamilyDivisorProperty:
		return "divisor_property"
	case nativePublicationFamilySendSafety:
		return "send_safety"
	default:
		return ""
	}
}

func (family nativePublicationFamily) semanticID() (identity.ContentID, bool) {
	name := family.String()
	if name == "" {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/native-publication/family/v1", []byte(name))
}

type nativePublicationRow struct {
	id           identity.ContentID
	semantic     identity.ContentID
	lane         NativePublicationLane
	kind         NativePublicationKind
	family       nativePublicationFamily
	trust        NativePublicationTrust
	key          string
	module       string
	term         string
	subject      string
	occurrence   string
	content      nativePublicationContent
	provenance   NativePublicationProvenance
	validity     NativePublicationValidity
	provenanceOK bool
	validityOK   bool
}

func (row nativePublicationRow) valid() bool {
	semantic, semanticOK := row.family.semanticID()
	if !row.id.Available() || !semanticOK || row.semantic != semantic || !nativePublicationLaneKindValid(row.lane, row.kind, row.family) ||
		!row.trust.Valid() || row.key == "" || row.module == "" ||
		!row.content.valid(row.family) || !row.provenanceOK || !row.validityOK || !row.provenance.valid() || !row.validity.valid() {
		return false
	}
	id, ok := nativePublicationRowID(row)
	return ok && id == row.id
}

// nativePublicationLaneKindValid keeps the public partitions closed: value
// families are on the values lane, while send safety owns its dedicated lane
// and kind. A caller cannot authenticate one by pairing an otherwise valid
// family with another row class.
func nativePublicationLaneKindValid(lane NativePublicationLane, kind NativePublicationKind, family nativePublicationFamily) bool {
	switch family {
	case nativePublicationFamilySendSafety:
		return lane == NativePublicationLaneSend && kind == NativePublicationKindSendSafety
	default:
		return lane == NativePublicationLaneValues && kind == NativePublicationKindValue
	}
}

func (value NativePublicationProvenance) valid() bool {
	return value.mount.Available() && value.artifact.Available() && value.local.Available() && value.body.Available() && value.point.Available() && value.span.Available()
}

func nativePublicationStateAvailable(published bool, content identity.ContentID, rows []nativePublicationRow, byID map[identity.ContentID]uint32) bool {
	return published && content.Available() && rows != nil && byID != nil
}

func (result *Result) nativeRowAt(ordinal uint32) (nativePublicationRow, bool) {
	if result == nil || !nativePublicationStateAvailable(result.nativePublished, result.nativeContent, result.nativeRows, result.nativeByID) || ordinal == 0 || int(ordinal) > len(result.nativeRows) {
		return nativePublicationRow{}, false
	}
	row := result.nativeRows[ordinal-1]
	stored, indexed := result.nativeByID[row.id]
	if !indexed || stored != ordinal || !row.valid() {
		return nativePublicationRow{}, false
	}
	return row, true
}

func sealNativePublication(rows []nativePublicationRow) (identity.ContentID, []nativePublicationRow, map[identity.ContentID]uint32, bool) {
	copyRows := make([]nativePublicationRow, len(rows))
	copy(copyRows, rows)
	sort.Slice(copyRows, func(i, j int) bool { return bytes.Compare(copyRows[i].id[:], copyRows[j].id[:]) < 0 })
	byID := make(map[identity.ContentID]uint32, len(copyRows))
	parts := make([][]byte, len(copyRows))
	for index, row := range copyRows {
		if !row.valid() {
			return identity.ContentID{}, nil, nil, false
		}
		if _, duplicate := byID[row.id]; duplicate {
			return identity.ContentID{}, nil, nil, false
		}
		byID[row.id] = uint32(index + 1)
		parts[index] = row.id[:]
	}
	content, ok := identity.DeriveContentID("analysis/native-publication/receipt/v1", parts...)
	if !ok {
		return identity.ContentID{}, nil, nil, false
	}
	return content, copyRows, byID, true
}

func nativePublicationRowID(row nativePublicationRow) (identity.ContentID, bool) {
	if row.family.String() == "" || !row.semantic.Available() || !row.lane.Valid() || !row.kind.Valid() || !row.trust.Valid() {
		return identity.ContentID{}, false
	}
	var enums [8]byte
	binary.BigEndian.PutUint64(enums[:], uint64(row.lane)|uint64(row.kind)<<8|uint64(row.family)<<16|uint64(row.trust)<<24)
	var flags [1]byte
	if row.provenanceOK {
		flags[0] |= 1
	}
	if row.validityOK {
		flags[0] |= 2
	}
	var ordinals [24]byte
	binary.BigEndian.PutUint64(ordinals[0:8], row.validity.establishedOrdinal)
	binary.BigEndian.PutUint64(ordinals[8:16], row.validity.revokedOrdinal)
	// The identity is derived from the row's typed content. A column is one
	// framed part of the preimage, so two rows are the same row exactly when
	// their columns agree; no rendering of any column enters the digest, and a
	// respelling therefore cannot mint or merge an identity.
	parts := [][]byte{
		row.semantic[:], enums[:], flags[:], []byte(row.key), []byte(row.module), []byte(row.term), []byte(row.subject), []byte(row.occurrence),
		row.provenance.mount[:], row.provenance.artifact[:], row.provenance.local[:], row.provenance.body[:], row.provenance.point[:], row.provenance.span[:],
		row.validity.event[:], row.validity.established[:], row.validity.revoked[:], ordinals[:],
	}
	if row.provenance.context.Available() {
		parts = append(parts, row.provenance.context[:])
	}
	return identity.DeriveContentID("analysis/native-publication/row/v2", append(parts, row.content.contentParts()...)...)
}
