package plane

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Refusal names why a payload was not admitted. A codec that answers a foreign
// or malformed image with a bare false leaves its caller unable to say what
// was wrong with it, so every refusal this package raises has a name.
type Refusal uint8

const (
	RefusalNone Refusal = iota
	// RefusalLayout is a payload written under a different sealed layout, or
	// under a different revision of this codec. The layout digest is the
	// declaration's identity, so a declaration that has changed refuses the
	// bytes of the one it replaced instead of reinterpreting them.
	RefusalLayout
	// RefusalTruncated is a payload whose length is not the exact width its
	// own header and layout state.
	RefusalTruncated
	// RefusalOwner is a payload whose coordinate space identity is absent
	// where the layout declares one.
	RefusalOwner
	// RefusalOrder is a coordinate plane that does not ascend, which is also
	// how the plane proves its rows unique.
	RefusalOrder
	// RefusalState is a row state naming no member of the declared state
	// vocabulary.
	RefusalState
	// RefusalColumn is a column byte outside the domain its carrier declares.
	RefusalColumn
	// RefusalTail is a variable column whose extents are not monotone, do not
	// begin at zero, do not end at the payload, or do not hold a whole number
	// of items.
	RefusalTail
	// RefusalAbsentRow is a row no producer wrote that nevertheless carries
	// column content. Absence is a state of the row, so a row at that state
	// carrying a decided column would publish a fact under an unwritten row.
	RefusalAbsentRow
	// RefusalMetadata is publication metadata that disagrees with the payload
	// it was carried beside. Presence and result cardinality are already stated
	// by the bytes, so a cell that says otherwise is a malformed answer rather
	// than a reading to prefer.
	RefusalMetadata
)

func (refusal Refusal) Available() bool { return refusal != RefusalNone }

func (refusal Refusal) String() string {
	switch refusal {
	case RefusalLayout:
		return "foreign layout"
	case RefusalTruncated:
		return "truncated payload"
	case RefusalOwner:
		return "absent coordinate space"
	case RefusalOrder:
		return "unordered coordinate plane"
	case RefusalState:
		return "undeclared row state"
	case RefusalColumn:
		return "column outside its declared domain"
	case RefusalTail:
		return "malformed variable extent"
	case RefusalAbsentRow:
		return "absent row carrying column content"
	case RefusalMetadata:
		return "publication metadata disagreeing with the payload"
	default:
		return "admitted"
	}
}

// View is an immutable, allocation-free reading of one encoded answer. It
// retains the caller's payload string and the plane offsets derived from the
// layout; it materializes no row, no identity, and no column vector. Open
// admits the complete image once, so every accessor past it is a read.
type View struct {
	layout *Sealed
	body   string
	rows   int
	idAt   int
	rowAt  int
	offAt  int
	tailAt int
	ok     bool
}

// Row is one row's reading. Its fields are offsets into the immutable payload,
// so retaining a row copies no part of the wire image.
type Row struct {
	layout   *Sealed
	body     string
	idAt     int
	recordAt int
	tailFrom int
	tailTo   int
	ok       bool
}

// Open admits one payload against the layout it must have been written under.
// The complete image is validated here: the revision, the declaration it was
// written under, the plane geometry, the coordinate order, and every column
// byte's declared domain. A row read afterwards restates none of it.
func Open(layout *Sealed, payload string) (View, Refusal) {
	if !layout.Available() || len(payload) < layout.header {
		return View{}, RefusalTruncated
	}
	if readUint64(payload, 0) != Format || !equalIdentity(payload, scalarWidth, layout.digest) {
		return View{}, RefusalLayout
	}
	if layout.owner && !identityAvailable(payload, scalarWidth+identityWidth) {
		return View{}, RefusalOwner
	}
	count := readUint64(payload, layout.header-scalarWidth)
	rowBytes := layout.rowWidth
	if layout.keyed {
		rowBytes += identityWidth
	}
	if count > uint64((len(payload)-layout.header)/rowBytes) {
		return View{}, RefusalTruncated
	}
	rows := int(count)
	if !layout.keyed && rows != 1 {
		return View{}, RefusalTruncated
	}
	view := View{layout: layout, body: payload, rows: rows, idAt: layout.header}
	view.rowAt = view.idAt
	if layout.keyed {
		view.rowAt += rows * identityWidth
	}
	view.offAt = view.rowAt + rows*layout.rowWidth
	view.tailAt = view.offAt
	if layout.variable >= 0 {
		if rows+1 > (len(payload)-view.offAt)/scalarWidth {
			return View{}, RefusalTruncated
		}
		view.tailAt += (rows + 1) * scalarWidth
	} else if view.offAt != len(payload) {
		return View{}, RefusalTruncated
	}
	if refusal := view.admitPlanes(); refusal.Available() {
		return View{}, refusal
	}
	view.ok = true
	return view, RefusalNone
}

// Admit opens one payload against the layout it must have been written under
// and holds the publication metadata carried beside it to the same bytes.
// Presence and result cardinality are derived facts: the encoder published
// them from the walk, and a reader that trusted the metadata over the payload
// would give a caller a second authority for them. Every consumer of a
// published answer opens it here.
func Admit(layout *Sealed, present bool, rows uint64, payload string) (View, Refusal) {
	view, refusal := Open(layout, payload)
	if refusal.Available() {
		return View{}, refusal
	}
	// A present answer is one observed result row; a fold that observed a row
	// and wrote nothing into it is one row that is not present. Both are the
	// encoder's own derivation, restated here against the bytes.
	if rows > 1 || view.Present() != present || present && rows != 1 {
		return View{}, RefusalMetadata
	}
	return view, RefusalNone
}

// admitPlanes states every law the encoded planes are subject to, once.
func (view View) admitPlanes() Refusal {
	layout := view.layout
	if layout.keyed {
		for index := 0; index < view.rows; index++ {
			at := view.idAt + index*identityWidth
			if !identityAvailable(view.body, at) {
				return RefusalOrder
			}
			if index > 0 && comparePayloadIdentity(view.body, at-identityWidth, at) >= 0 {
				return RefusalOrder
			}
		}
	}
	tailLength := len(view.body) - view.tailAt
	if layout.variable >= 0 {
		element := layout.columns[layout.variable].carrier.element()
		if tailLength%element != 0 {
			return RefusalTail
		}
		previous := uint64(0)
		if readUint64(view.body, view.offAt) != 0 {
			return RefusalTail
		}
		for index := 1; index <= view.rows; index++ {
			offset := readUint64(view.body, view.offAt+index*scalarWidth)
			if offset < previous || offset > uint64(tailLength) || (offset-previous)%uint64(element) != 0 {
				return RefusalTail
			}
			previous = offset
		}
		if previous != uint64(tailLength) {
			return RefusalTail
		}
		// A portable identity is available or it is not an identity. The
		// vector carries no presence byte of its own, so the zero image is
		// refused here rather than read back as a row that names nothing.
		if layout.columns[layout.variable].carrier == CarrierAtoms {
			for at := view.tailAt; at < len(view.body); at += identityWidth {
				if !identityAvailable(view.body, at) {
					return RefusalColumn
				}
			}
		}
	}
	for index := 0; index < view.rows; index++ {
		if refusal := view.admitRow(index); refusal.Available() {
			return refusal
		}
	}
	return RefusalNone
}

func (view View) admitRow(index int) Refusal {
	layout := view.layout
	recordAt := view.rowAt + index*layout.rowWidth
	state := view.body[recordAt]
	if int(state) > len(layout.states) {
		return RefusalState
	}
	if state == 0 {
		for offset := recordAt + stateWidth; offset < recordAt+layout.rowWidth; offset++ {
			if view.body[offset] != 0 {
				return RefusalAbsentRow
			}
		}
		if from, to, variable := view.extent(index); variable && from != to {
			return RefusalAbsentRow
		}
		return RefusalNone
	}
	for column, declared := range layout.columns {
		if declared.carrier.Variable() {
			continue
		}
		at := recordAt + layout.offsets[column]
		switch declared.carrier {
		case CarrierMember:
			if int(view.body[at]) > len(declared.members) {
				return RefusalColumn
			}
		case CarrierEvidence:
			if !Evidence(view.body[at]).Available() {
				return RefusalColumn
			}
		case CarrierFlag:
			if view.body[at] > 1 {
				return RefusalColumn
			}
		case CarrierOrdinal:
			if view.body[at] > 1 {
				return RefusalColumn
			}
			if view.body[at] == 0 && readUint32(view.body, at+presenceWidth) != 0 {
				return RefusalColumn
			}
		case CarrierIdentity:
			if view.body[at] > 1 {
				return RefusalColumn
			}
			if identityAvailable(view.body, at+presenceWidth) != (view.body[at] == 1) {
				return RefusalColumn
			}
		}
	}
	return RefusalNone
}

func (view View) extent(index int) (from, to int, variable bool) {
	if view.layout.variable < 0 {
		return 0, 0, false
	}
	from = view.tailAt + int(readUint64(view.body, view.offAt+index*scalarWidth))
	to = view.tailAt + int(readUint64(view.body, view.offAt+(index+1)*scalarWidth))
	return from, to, true
}

// Available reports whether this view was admitted.
func (view View) Available() bool { return view.ok }

// Layout is the sealed declaration this view was admitted under.
func (view View) Layout() *Sealed { return view.layout }

// Owner is the identity of the coordinate space these rows were issued by. A
// layout declaring no owner reads the zero identity.
func (view View) Owner() identity.ContentID {
	var id identity.ContentID
	if !view.ok || !view.layout.owner {
		return id
	}
	copy(id[:], view.body[scalarWidth+identityWidth:scalarWidth+2*identityWidth])
	return id
}

// RowCount is the number of rows in the answer.
func (view View) RowCount() int {
	if !view.ok {
		return 0
	}
	return view.rows
}

// Present reports whether some row of the answer was written. It is the same
// derivation the encoder published beside the payload, read back off the wire
// rather than trusted from the metadata.
func (view View) Present() bool {
	for index := 0; index < view.RowCount(); index++ {
		if view.body[view.rowAt+index*view.layout.rowWidth] != 0 {
			return true
		}
	}
	return false
}

// At opens one row by its position in the coordinate plane.
func (view View) At(index int) (Row, bool) {
	if !view.ok || index < 0 || index >= view.rows {
		return Row{}, false
	}
	row := Row{
		layout:   view.layout,
		body:     view.body,
		recordAt: view.rowAt + index*view.layout.rowWidth,
		ok:       true,
	}
	if view.layout.keyed {
		row.idAt = view.idAt + index*identityWidth
	} else {
		row.idAt = -1
	}
	row.tailFrom, row.tailTo, _ = view.extent(index)
	return row, true
}

// Lookup resolves one row by its coordinate identity. The coordinate plane
// ascends, which the admission already proved, so the search is a bisection
// over the encoded bytes and materializes nothing.
func (view View) Lookup(id identity.ContentID) (Row, bool) {
	if !view.ok || !view.layout.keyed {
		return Row{}, false
	}
	low, high := 0, view.rows
	for low < high {
		middle := int(uint(low+high) >> 1)
		switch compareIdentityAt(view.body, view.idAt+middle*identityWidth, id) {
		case 0:
			return view.At(middle)
		case -1:
			low = middle + 1
		default:
			high = middle
		}
	}
	return Row{}, false
}

// Available reports whether this row came from an admitted view.
func (row Row) Available() bool { return row.ok }

// ID is the portable identity of the coordinate this row holds. An unkeyed
// layout reads the zero identity: the row's identity is the query site's own.
func (row Row) ID() identity.ContentID {
	var id identity.ContentID
	if !row.ok || row.idAt < 0 {
		return id
	}
	copy(id[:], row.body[row.idAt:row.idAt+identityWidth])
	return id
}

// Written reports whether a producer published this row.
func (row Row) Written() bool { return row.ok && row.body[row.recordAt] != 0 }

// Class is the declared identity of the row's state. An unwritten row has no
// class. The identity comes from the sealed vocabulary, so a consumer names the
// class it read rather than holding the rank the wire carried it at.
func (row Row) Class() (schema.Key, bool) {
	if !row.Written() {
		return "", false
	}
	return row.layout.states[row.body[row.recordAt]-1], true
}

// ClassIs reports whether the row's state is the named class. It is the read a
// consumer wants when it is testing one class rather than rendering the answer.
func (row Row) ClassIs(class schema.Key) bool {
	held, written := row.Class()
	return written && held == class
}

// Member reads one declared member column as the identity the producer named.
// The second result is whether a producer decided the column.
func (row Row) Member(column int) (schema.Key, bool) {
	at, ok := row.fixed(column, CarrierMember)
	if !ok || row.body[at] == 0 {
		return "", false
	}
	return row.layout.columns[column].members[row.body[at]-1], true
}

// MemberIs reports whether one declared member column names the given member.
func (row Row) MemberIs(column int, member schema.Key) bool {
	held, decided := row.Member(column)
	return decided && held == member
}

// Evidence reads one declared four-state proof column.
func (row Row) Evidence(column int) Evidence {
	at, ok := row.fixed(column, CarrierEvidence)
	if !ok {
		return EvidenceAbsent
	}
	return Evidence(row.body[at])
}

// Flag reads one declared boolean column.
func (row Row) Flag(column int) bool {
	at, ok := row.fixed(column, CarrierFlag)
	return ok && row.body[at] == 1
}

// Ordinal reads one declared optional measure.
func (row Row) Ordinal(column int) (uint32, bool) {
	at, ok := row.fixed(column, CarrierOrdinal)
	if !ok || row.body[at] != 1 {
		return 0, false
	}
	return readUint32(row.body, at+presenceWidth), true
}

// Identity reads one declared optional portable identity.
func (row Row) Identity(column int) (identity.ContentID, bool) {
	var id identity.ContentID
	at, ok := row.fixed(column, CarrierIdentity)
	if !ok || row.body[at] != 1 {
		return id, false
	}
	copy(id[:], row.body[at+presenceWidth:at+presenceWidth+identityWidth])
	return id, true
}

// Count is the number of items in this row's variable column.
func (row Row) Count() int {
	if !row.ok || row.layout.variable < 0 {
		return 0
	}
	return (row.tailTo - row.tailFrom) / row.layout.columns[row.layout.variable].carrier.element()
}

// WordAt reads one 64-bit word of this row's variable column.
func (row Row) WordAt(index int) (uint64, bool) {
	if !row.variableIs(CarrierWords) || index < 0 || index >= row.Count() {
		return 0, false
	}
	return readUint64(row.body, row.tailFrom+index*scalarWidth), true
}

// AtomAt reads one portable identity of this row's variable column.
func (row Row) AtomAt(index int) (identity.ContentID, bool) {
	var id identity.ContentID
	if !row.variableIs(CarrierAtoms) || index < 0 || index >= row.Count() {
		return id, false
	}
	at := row.tailFrom + index*identityWidth
	copy(id[:], row.body[at:at+identityWidth])
	return id, true
}

func (row Row) variableIs(carrier Carrier) bool {
	return row.ok && row.layout.variable >= 0 && row.layout.columns[row.layout.variable].carrier == carrier
}

// fixed resolves one declared fixed column's byte offset inside this row. The
// bounds it states are the declaration's, not the wire's: the wire was
// admitted once by Open and is never re-read for validity here.
func (row Row) fixed(column int, carrier Carrier) (int, bool) {
	if !row.ok || column < 0 || column >= len(row.layout.columns) {
		return 0, false
	}
	if row.layout.columns[column].carrier != carrier || row.body[row.recordAt] == 0 {
		return 0, false
	}
	return row.recordAt + row.layout.offsets[column], true
}

func readUint64(payload string, offset int) uint64 {
	return uint64(payload[offset])<<56 | uint64(payload[offset+1])<<48 |
		uint64(payload[offset+2])<<40 | uint64(payload[offset+3])<<32 |
		uint64(payload[offset+4])<<24 | uint64(payload[offset+5])<<16 |
		uint64(payload[offset+6])<<8 | uint64(payload[offset+7])
}

func readUint32(payload string, offset int) uint32 {
	return uint32(payload[offset])<<24 | uint32(payload[offset+1])<<16 |
		uint32(payload[offset+2])<<8 | uint32(payload[offset+3])
}

func identityAvailable(payload string, offset int) bool {
	var fold byte
	for index := 0; index < identityWidth; index++ {
		fold |= payload[offset+index]
	}
	return fold != 0
}

func equalIdentity(payload string, offset int, id identity.ContentID) bool {
	if offset+identityWidth > len(payload) {
		return false
	}
	for index := 0; index < identityWidth; index++ {
		if payload[offset+index] != id[index] {
			return false
		}
	}
	return true
}

func comparePayloadIdentity(payload string, left, right int) int {
	for index := 0; index < identityWidth; index++ {
		switch {
		case payload[left+index] < payload[right+index]:
			return -1
		case payload[left+index] > payload[right+index]:
			return 1
		}
	}
	return 0
}

func compareIdentityAt(payload string, offset int, id identity.ContentID) int {
	for index := 0; index < identityWidth; index++ {
		switch {
		case payload[offset+index] < id[index]:
			return -1
		case payload[offset+index] > id[index]:
			return 1
		}
	}
	return 0
}
