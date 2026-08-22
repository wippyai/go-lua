// Package plane owns the analyzer's one detached summary-result wire format.
//
// A published query answer is a table: rows keyed by the portable identity of
// the coordinate they hold, and columns declared by the family that publishes
// them. Naming those columns is a declaration, not a codec: the seal already
// says what a family publishes, so the bytes a family's answer is detached as
// are derived from that declaration rather than spelled a second time in the
// domain that produced it. A per-domain encoder re-describing a sealed column
// is a parallel implementation of the seal, and this package exists so there
// is exactly one.
//
// The declaration is the layout. A sealed Layout fixes the row state's member
// space, the carrier and byte width of every column, and the one variable
// column a row may carry, and folds all of it into a layout digest the wire
// carries. Encoding is a linear walk over that declaration straight into the
// output buffer: the writer holds a cursor per plane and no intermediate row
// object, so a payload is built with exactly one allocation of its exact final
// size. Decoding validates the whole image once and then hands out views: a
// Row is offsets over the encoded bytes, every accessor is a read, and nothing
// is materialized, so reading a decoded answer allocates nothing at all.
//
// Availability is a sealed fact. Open admits or refuses a payload by name, and
// every accessor past it reads a byte the admission already proved is in its
// declared domain rather than checking it again.
package plane

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Format is the wire revision of this codec. It is the analyzer's one
// summary-result revision: a family changes what it publishes by changing its
// sealed layout, which changes the layout digest the payload carries, so a
// format bump is reserved for a change in the plane structure itself.
const Format uint64 = 1

const (
	identityWidth = 32
	scalarWidth   = 8
	ordinalWidth  = 4
	stateWidth    = 1
	presenceWidth = 1
)

const layoutDigestDomain = "wippy.analysis/schema/plane/layout"

// Carrier is the closed catalog of column carriers. A carrier fixes both the
// byte width of a column and the domain its bytes are admitted over, so a
// declared column needs no further wire description.
type Carrier uint8

const (
	CarrierInvalid Carrier = iota
	// CarrierMember is one member of a declared closed vocabulary: a single
	// byte holding zero for a column no producer wrote, and one plus the
	// declaration rank of a named member otherwise. The column's Members list
	// is the vocabulary, and it is folded into the layout digest, so the byte
	// on the wire is the seal's rank of an identity both sides resolve by name
	// rather than a private number either side agreed to privately.
	CarrierMember
	// CarrierEvidence is one proof column under the four-state evidence model:
	// absent, unknown, refuted, proven. Absence is a state of the column and
	// not a missing column, so an unwritten proof never occupies the ordinal
	// that means a producer authenticated an undecidable verdict.
	CarrierEvidence
	// CarrierFlag is one decided boolean. It carries no absence: a column that
	// may go unwritten is a member or an evidence column.
	CarrierFlag
	// CarrierOrdinal is an optional unsigned 32-bit measure: a presence byte
	// followed by the big-endian value.
	CarrierOrdinal
	// CarrierIdentity is an optional portable identity: a presence byte
	// followed by the content identity.
	CarrierIdentity
	// CarrierWords is a variable-length vector of 64-bit words, carried in the
	// row's tail extent. A layout declares at most one variable column.
	CarrierWords
	// CarrierAtoms is a variable-length vector of portable identities, carried
	// in the row's tail extent. A layout declares at most one variable column.
	CarrierAtoms
	carrierLimit
)

func (carrier Carrier) Available() bool {
	return carrier > CarrierInvalid && carrier < carrierLimit
}

// Variable reports whether this carrier lives in the row's tail extent rather
// than in the fixed row record.
func (carrier Carrier) Variable() bool {
	return carrier == CarrierWords || carrier == CarrierAtoms
}

// Width is the number of bytes this carrier occupies in the fixed row record.
// A variable carrier occupies none.
func (carrier Carrier) Width() int {
	switch carrier {
	case CarrierMember, CarrierEvidence, CarrierFlag:
		return 1
	case CarrierOrdinal:
		return presenceWidth + ordinalWidth
	case CarrierIdentity:
		return presenceWidth + identityWidth
	default:
		return 0
	}
}

// element is the byte width of one item of a variable carrier's vector.
func (carrier Carrier) element() int {
	switch carrier {
	case CarrierWords:
		return scalarWidth
	case CarrierAtoms:
		return identityWidth
	default:
		return 0
	}
}

// Evidence is the four-state proof model every CarrierEvidence column is read
// under. Absent is the state of a column no producer decided; it is not the
// absence of the column and never a substitute for an unwritten row.
type Evidence uint8

const (
	EvidenceAbsent Evidence = iota
	EvidenceUnknown
	EvidenceRefuted
	EvidenceProven
	evidenceLimit
)

func (state Evidence) Available() bool { return state < evidenceLimit }

// Decided reports whether a producer reached a verdict on this column.
func (state Evidence) Decided() bool {
	return state == EvidenceRefuted || state == EvidenceProven
}

// Column is one declared column of a published answer: the name a consumer
// reads it under, the carrier its bytes are admitted over, and, for a member
// column, the declared vocabulary its byte ranks.
type Column struct {
	// Key names this column inside its layout. It is content: two layouts that
	// differ in a column name reach different layout digests, so a consumer
	// that opens a payload under the wrong declaration is refused rather than
	// silently reinterpreted.
	Key     schema.Key
	Carrier Carrier
	// Members names a CarrierMember column's declared vocabulary. It is a
	// category of the structural vocabulary surface and not a list written
	// here: the members, their order, and their identities are that category's
	// declaration, and the seal reads them from it. The resolved identities are
	// folded into the layout digest, so adding, renaming, or reordering a
	// member of the category is a different declaration and refuses the bytes
	// of the one it replaced instead of silently re-ranking them.
	Members structure.Category
}

// sealedColumn is one admitted column: the authored declaration together with
// the member identities the seal resolved its category to. A producer states a
// member by naming it and a consumer reads back that name, so neither side
// ever holds an ordinal the declaration did not issue.
type sealedColumn struct {
	key      schema.Key
	carrier  Carrier
	category structure.Category
	members  []schema.Key
}

func (column sealedColumn) available() bool {
	if !column.key.Available() || !column.carrier.Available() {
		return false
	}
	if column.carrier != CarrierMember {
		return column.category == structure.CategoryInvalid && len(column.members) == 0
	}
	return column.category.Available() && declaredMembers(column.members)
}

// declaredMembers admits one member vocabulary: nonempty, uniquely named, and
// small enough that its rank plus the unwritten state fits the column's byte.
func declaredMembers(members []schema.Key) bool {
	if len(members) == 0 || len(members) > 255 {
		return false
	}
	for index, member := range members {
		if !member.Available() {
			return false
		}
		for previous := 0; previous < index; previous++ {
			if members[previous] == member {
				return false
			}
		}
	}
	return true
}

// rank resolves one member identity to the wire byte the seal issues for it.
// The vocabularies a published column declares are small closed catalogs, so
// the walk is a scan over the declaration and allocates nothing.
func rank(members []schema.Key, member schema.Key) (byte, bool) {
	for index, declared := range members {
		if declared == member {
			return byte(index) + 1, true
		}
	}
	return 0, false
}

// Sealed is one admitted layout: the family and keying its query registration
// publishes it under, the row state and column declarations resolved against
// the sealed structural vocabulary, the byte geometry derived from all of it,
// and the digest the wire carries. It is immutable and safe for concurrent
// readers.
type Sealed struct {
	family   schema.Key
	keyed    bool
	owner    bool
	states   []schema.Key
	columns  []sealedColumn
	offsets  []int
	rowWidth int
	variable int
	header   int
	digest   identity.ContentID
}

// Seal admits one family's published answer declaration.
//
// Nothing about the layout is authored twice. The family the payload is
// interpreted under and whether its rows carry the coordinates they hold are
// the query registration's shape; the classes a written row may be published
// at and the vocabulary of every member column are categories of the sealed
// structural vocabulary, read from it here rather than restated as lists
// beside the codec. What a family states is only the columns it publishes and
// the carrier each is admitted over.
//
// A rejected declaration returns false rather than a partially usable
// descriptor.
func Seal(shape query.Shape, vocabulary structure.Table, states structure.Category, columns []Column) (*Sealed, bool) {
	if !shape.Available() || len(columns) == 0 {
		return nil, false
	}
	declaredStates, statesOK := vocabulary.Vocabulary(states)
	if !statesOK || !declaredMembers(declaredStates) {
		return nil, false
	}
	sealed := &Sealed{
		family:   shape.Family(),
		keyed:    shape.Keyed(),
		owner:    shape.Keyed(),
		states:   declaredStates,
		columns:  make([]sealedColumn, len(columns)),
		offsets:  make([]int, len(columns)),
		variable: -1,
	}
	names := make(map[schema.Key]struct{}, len(columns))
	cursor := stateWidth
	for index, authored := range columns {
		column := sealedColumn{key: authored.Key, carrier: authored.Carrier, category: authored.Members}
		if authored.Carrier == CarrierMember {
			members, membersOK := vocabulary.Vocabulary(authored.Members)
			if !membersOK {
				return nil, false
			}
			column.members = members
		}
		if !column.available() {
			return nil, false
		}
		if _, duplicate := names[column.key]; duplicate {
			return nil, false
		}
		names[column.key] = struct{}{}
		sealed.columns[index] = column
		if column.carrier.Variable() {
			// One variable column keeps every fixed column at a computed
			// offset and the row record at a computed width, so a random row
			// lookup never walks a prefix of the payload. A layout needing two
			// is a layout this seal has not been extended for, and it is
			// refused rather than answered with a scan.
			if sealed.variable >= 0 {
				return nil, false
			}
			sealed.variable = index
			sealed.offsets[index] = -1
			continue
		}
		sealed.offsets[index] = cursor
		cursor += column.carrier.Width()
	}
	sealed.rowWidth = cursor
	sealed.header = scalarWidth + identityWidth + scalarWidth
	if sealed.owner {
		sealed.header += identityWidth
	}
	digest, ok := sealLayoutDigest(sealed)
	if !ok {
		return nil, false
	}
	sealed.digest = digest
	return sealed, true
}

func sealLayoutDigest(sealed *Sealed) (identity.ContentID, bool) {
	parts := make([][]byte, 0, 8+len(sealed.states)+4*len(sealed.columns))
	var shape [scalarWidth]byte
	binary.BigEndian.PutUint64(shape[:], Format)
	parts = append(parts, append([]byte(nil), shape[:]...))
	parts = append(parts, []byte(sealed.family))
	parts = append(parts, []byte{boolByte(sealed.keyed), boolByte(sealed.owner)})
	parts = appendMemberIdentities(parts, sealed.states)
	binary.BigEndian.PutUint64(shape[:], uint64(len(sealed.columns)))
	parts = append(parts, append([]byte(nil), shape[:]...))
	for _, column := range sealed.columns {
		parts = append(parts, []byte(column.key), []byte{byte(column.carrier)})
		parts = appendMemberIdentities(parts, column.members)
	}
	return identity.DeriveContentID(layoutDigestDomain, parts...)
}

// appendMemberIdentities folds one declared vocabulary into the digest
// preimage: its arity followed by each identity in declaration order, so a
// renamed or reordered member reaches a different layout.
func appendMemberIdentities(parts [][]byte, members []schema.Key) [][]byte {
	var arity [scalarWidth]byte
	binary.BigEndian.PutUint64(arity[:], uint64(len(members)))
	parts = append(parts, append([]byte(nil), arity[:]...))
	for _, member := range members {
		parts = append(parts, []byte(member))
	}
	return parts
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func (sealed *Sealed) Available() bool { return sealed != nil && sealed.digest.Available() }

// Family is the query family this layout publishes.
func (sealed *Sealed) Family() schema.Key { return sealed.family }

// Digest is the content identity of the declaration. Every payload written
// under this layout carries it, so a payload opened under another declaration
// refuses instead of being reinterpreted.
func (sealed *Sealed) Digest() identity.ContentID { return sealed.digest }

// ColumnCount is the number of declared columns.
func (sealed *Sealed) ColumnCount() int { return len(sealed.columns) }

// States is the row state's declared class vocabulary, in seal rank order.
func (sealed *Sealed) States() []schema.Key {
	if sealed == nil {
		return nil
	}
	return sealed.states
}

// ColumnAt returns one declared column at its declaration position.
func (sealed *Sealed) ColumnAt(index int) (Column, bool) {
	if sealed == nil || index < 0 || index >= len(sealed.columns) {
		return Column{}, false
	}
	column := sealed.columns[index]
	return Column{Key: column.key, Carrier: column.carrier, Members: column.category}, true
}

// RowWidth is the byte width of one fixed row record, the row state included.
func (sealed *Sealed) RowWidth() int { return sealed.rowWidth }

// Variable returns the declaration position of the layout's variable column.
func (sealed *Sealed) Variable() (int, bool) {
	if sealed == nil || sealed.variable < 0 {
		return 0, false
	}
	return sealed.variable, true
}

// Size is the exact byte width of a payload of this layout holding the given
// number of rows and tail elements. It is the encoder's one allocation and the
// decoder's one length law.
func (sealed *Sealed) Size(rows, elements int) (int, bool) {
	if !sealed.Available() || rows < 0 || elements < 0 {
		return 0, false
	}
	if !sealed.keyed && rows != 1 {
		return 0, false
	}
	rowBytes := sealed.rowWidth
	if sealed.keyed {
		rowBytes += identityWidth
	}
	maxInt := int(^uint(0) >> 1)
	size := sealed.header
	if rows > (maxInt-size)/rowBytes {
		return 0, false
	}
	size += rows * rowBytes
	if sealed.variable >= 0 {
		element := sealed.columns[sealed.variable].carrier.element()
		if rows+1 > (maxInt-size)/scalarWidth {
			return 0, false
		}
		size += (rows + 1) * scalarWidth
		if elements > (maxInt-size)/element {
			return 0, false
		}
		size += elements * element
	} else if elements != 0 {
		return 0, false
	}
	return size, true
}
