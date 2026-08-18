package analysis

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
)

// NativePublicationLane is the closed public partition of native analysis
// rows. It is Analysis vocabulary: Engine never classifies or renders a
// native row.
type NativePublicationLane uint8

const (
	NativePublicationLaneInvalid NativePublicationLane = iota
	NativePublicationLaneValues
	NativePublicationLaneOutcomes
	NativePublicationLaneDiagnostics
)

func (lane NativePublicationLane) Valid() bool {
	return lane >= NativePublicationLaneValues && lane <= NativePublicationLaneDiagnostics
}

func (lane NativePublicationLane) String() string {
	switch lane {
	case NativePublicationLaneValues:
		return "values"
	case NativePublicationLaneOutcomes:
		return "outcomes"
	case NativePublicationLaneDiagnostics:
		return "diagnostics"
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
	NativePublicationKindOutcome
	NativePublicationKindDiagnostic
)

func (kind NativePublicationKind) Valid() bool {
	return kind >= NativePublicationKindValue && kind <= NativePublicationKindDiagnostic
}

func (kind NativePublicationKind) String() string {
	switch kind {
	case NativePublicationKindValue:
		return "value"
	case NativePublicationKindOutcome:
		return "outcome"
	case NativePublicationKindDiagnostic:
		return "diagnostic"
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
	NativePublicationTrustClaimed
	NativePublicationTrustUnknown
)

func (trust NativePublicationTrust) Valid() bool {
	return trust >= NativePublicationTrustProven && trust <= NativePublicationTrustUnknown
}

func (trust NativePublicationTrust) String() string {
	switch trust {
	case NativePublicationTrustProven:
		return "proven"
	case NativePublicationTrustClaimed:
		return "claimed"
	case NativePublicationTrustUnknown:
		return "unknown"
	default:
		return ""
	}
}

// NativePublicationProvenance is the detached, owner-issued mount geometry of
// one row. All IDs are immutable scalar identities; it retains no Program,
// Link, Engine, State, or domain handle.
type NativePublicationProvenance struct {
	mount    identity.ContentID
	artifact identity.ContentID
	local    identity.ContentID
	body     identity.ContentID
	point    identity.ContentID
	span     identity.ContentID
}

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
// only through Result-issued cursors, which prevents foreign or stale receipt
// rows from being mistaken for a row of this result.
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

// Value preserves exact-empty publication: ok distinguishes an absent value
// from a deliberately published empty value.
func (row NativePublication) Value() (string, bool) {
	value, ok := row.resolve()
	if !ok || !value.valueOK {
		return "", false
	}
	return value.value, true
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
// issued a publication receipt for this Result. It is intentionally distinct
// from an available empty publication: an absent producer must not look like a
// successful analysis with zero native facts.
func (result *Result) NativePublicationAvailable() bool {
	return result != nil && result.valid() && result.native != nil && result.native.valid()
}

// NativePublicationCount, At, ByID, and ByToken are Result's closed native
// surface. At and the two lookup paths are O(1); the ID index holds only
// dense-row ordinals and never a second topology or row copy.
func (result *Result) NativePublicationCount() int {
	if !result.NativePublicationAvailable() {
		return 0
	}
	return len(result.native.rows)
}
func (result *Result) NativePublicationAt(index int) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || index < 0 || index >= len(result.native.rows) {
		return NativePublication{}, false
	}
	ordinal := uint32(index + 1)
	if _, ok := result.native.rowAt(ordinal); !ok {
		return NativePublication{}, false
	}
	return NativePublication{token: NativePublicationToken{owner: result, ordinal: ordinal}}, true
}
func (result *Result) NativePublicationByID(id identity.ContentID) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || !id.Available() {
		return NativePublication{}, false
	}
	ordinal, ok := result.native.byID[id]
	if !ok || ordinal == 0 {
		return NativePublication{}, false
	}
	row, rowOK := result.native.rowAt(ordinal)
	if !rowOK || row.id != id {
		return NativePublication{}, false
	}
	return NativePublication{token: NativePublicationToken{owner: result, ordinal: ordinal}}, true
}
func (result *Result) NativePublicationByToken(token NativePublicationToken) (NativePublication, bool) {
	if !result.NativePublicationAvailable() || token.owner != result || token.ordinal == 0 || int(token.ordinal) > len(result.native.rows) {
		return NativePublication{}, false
	}
	if _, ok := result.native.rowAt(token.ordinal); !ok {
		return NativePublication{}, false
	}
	return NativePublication{token: token}, true
}

func (row NativePublication) resolve() (nativePublicationRow, bool) {
	owner, ordinal := row.token.owner, row.token.ordinal
	if owner == nil || !owner.NativePublicationAvailable() {
		return nativePublicationRow{}, false
	}
	return owner.native.rowAt(ordinal)
}

// nativePublicationFamily is the closed semantic denominator for the first
// receipt-native publication slice. New families extend this owner; callers
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
	value        string
	valueOK      bool
	provenance   NativePublicationProvenance
	validity     NativePublicationValidity
	provenanceOK bool
	validityOK   bool
}

func (row nativePublicationRow) valid() bool {
	semantic, semanticOK := row.family.semanticID()
	if !row.id.Available() || !semanticOK || row.semantic != semantic || row.lane != NativePublicationLaneValues ||
		row.kind != NativePublicationKindValue || !row.trust.Valid() || row.key == "" || row.module == "" ||
		!row.valueOK || !row.provenanceOK || !row.validityOK || !row.provenance.valid() || !row.validity.valid() {
		return false
	}
	id, ok := nativePublicationRowID(row)
	return ok && id == row.id
}

func (value NativePublicationProvenance) valid() bool {
	return value.mount.Available() && value.artifact.Available() && value.local.Available() && value.body.Available() && value.point.Available() && value.span.Available()
}

type nativePublicationReceipt struct {
	content identity.ContentID
	rows    []nativePublicationRow
	byID    map[identity.ContentID]uint32
	sealed  bool
}

func (receipt *nativePublicationReceipt) valid() bool {
	return receipt != nil && receipt.sealed && receipt.content.Available() && receipt.rows != nil && receipt.byID != nil
}

func (receipt *nativePublicationReceipt) rowAt(ordinal uint32) (nativePublicationRow, bool) {
	if !receipt.valid() || ordinal == 0 || int(ordinal) > len(receipt.rows) {
		return nativePublicationRow{}, false
	}
	row := receipt.rows[ordinal-1]
	stored, indexed := receipt.byID[row.id]
	if !indexed || stored != ordinal || !row.valid() {
		return nativePublicationRow{}, false
	}
	return row, true
}

func newNativePublicationReceipt(rows []nativePublicationRow) (*nativePublicationReceipt, bool) {
	copyRows := make([]nativePublicationRow, len(rows))
	copy(copyRows, rows)
	sort.Slice(copyRows, func(i, j int) bool { return bytes.Compare(copyRows[i].id[:], copyRows[j].id[:]) < 0 })
	byID := make(map[identity.ContentID]uint32, len(copyRows))
	parts := make([][]byte, len(copyRows))
	for index, row := range copyRows {
		if !row.valid() {
			return nil, false
		}
		if _, duplicate := byID[row.id]; duplicate {
			return nil, false
		}
		byID[row.id] = uint32(index + 1)
		parts[index] = row.id[:]
	}
	content, ok := identity.DeriveContentID("analysis/native-publication/receipt/v1", parts...)
	if !ok {
		return nil, false
	}
	return &nativePublicationReceipt{content: content, rows: copyRows, byID: byID, sealed: true}, true
}

func nativePublicationRowID(row nativePublicationRow) (identity.ContentID, bool) {
	if row.family.String() == "" || !row.semantic.Available() || !row.lane.Valid() || !row.kind.Valid() || !row.trust.Valid() {
		return identity.ContentID{}, false
	}
	var enums [8]byte
	binary.BigEndian.PutUint64(enums[:], uint64(row.lane)|uint64(row.kind)<<8|uint64(row.family)<<16|uint64(row.trust)<<24)
	var flags [1]byte
	if row.valueOK {
		flags[0] |= 1
	}
	if row.provenanceOK {
		flags[0] |= 2
	}
	if row.validityOK {
		flags[0] |= 4
	}
	var ordinals [24]byte
	binary.BigEndian.PutUint64(ordinals[0:8], row.validity.establishedOrdinal)
	binary.BigEndian.PutUint64(ordinals[8:16], row.validity.revokedOrdinal)
	return identity.DeriveContentID(
		"analysis/native-publication/row/v1",
		row.semantic[:], enums[:], flags[:], []byte(row.key), []byte(row.module), []byte(row.term), []byte(row.subject), []byte(row.occurrence), []byte(row.value),
		row.provenance.mount[:], row.provenance.artifact[:], row.provenance.local[:], row.provenance.body[:], row.provenance.point[:], row.provenance.span[:],
		row.validity.event[:], row.validity.established[:], row.validity.revoked[:], ordinals[:],
	)
}
