package plane

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Writer encodes one answer as a linear walk over its sealed layout. It holds
// one monotone cursor per plane and no intermediate row object: a value is
// written into the output the moment the caller states it, so a payload costs
// exactly the one allocation of its exact final size.
//
// The walk is the declaration. A row is opened at its state, every declared
// column is filled in declaration order by the setter its carrier admits, and
// the row is closed; a caller that fills a column out of order, with the wrong
// carrier, or not at all is refused rather than silently misaligned. There is
// no column index and no byte offset anywhere in a producer.
//
// A Writer is a value. Begin returns one by value and every method takes a
// pointer to it, so an encoder holds it on its own stack.
type Writer struct {
	layout  *Sealed
	out     []byte
	idAt    int
	rowAt   int
	offAt   int
	tailAt  int
	tail    int
	rows    int
	row     int
	column  int
	last    identity.ContentID
	open    bool
	present bool
	failed  bool
}

// Begin opens one payload. Rows is the complete row count and elements is the
// total number of items the variable column will hold across every row; both
// are known to the producer before the walk, so the buffer is sized once and
// never grown.
func Begin(layout *Sealed, owner identity.ContentID, rows, elements int) (Writer, bool) {
	size, ok := layout.Size(rows, elements)
	if !ok {
		return Writer{}, false
	}
	if layout.owner != owner.Available() {
		return Writer{}, false
	}
	out := make([]byte, size)
	binary.BigEndian.PutUint64(out[0:scalarWidth], Format)
	cursor := scalarWidth
	digest := layout.digest
	copy(out[cursor:cursor+identityWidth], digest[:])
	cursor += identityWidth
	if layout.owner {
		copy(out[cursor:cursor+identityWidth], owner[:])
		cursor += identityWidth
	}
	binary.BigEndian.PutUint64(out[cursor:cursor+scalarWidth], uint64(rows))
	cursor += scalarWidth
	writer := Writer{layout: layout, out: out, rows: rows, idAt: cursor}
	writer.rowAt = cursor
	if layout.keyed {
		writer.rowAt += rows * identityWidth
	}
	writer.offAt = writer.rowAt + rows*layout.rowWidth
	writer.tailAt = writer.offAt
	if layout.variable >= 0 {
		writer.tailAt += (rows + 1) * scalarWidth
	}
	if layout.variable < 0 {
		writer.tail = len(out)
	} else {
		writer.tail = writer.tailAt
	}
	return writer, true
}

// Row opens one row at the named class of the layout's declared state
// vocabulary. A keyed layout requires the coordinate identity, and requires the
// coordinate plane to ascend: the plane is the wire's one row order and its
// uniqueness proof, and it is what a consumer binary-searches.
func (writer *Writer) Row(id identity.ContentID, class schema.Key) bool {
	state, declared := rank(writer.layout.states, class)
	if !declared {
		return writer.fail()
	}
	return writer.open_(id, state)
}

// Absent opens one row no producer wrote. It is a distinct act from opening a
// row at a class, so an unwritten row is stated rather than spelled as a
// reserved value of the class vocabulary.
func (writer *Writer) Absent(id identity.ContentID) bool { return writer.open_(id, 0) }

func (writer *Writer) open_(id identity.ContentID, state uint8) bool {
	if writer.failed || writer.open || writer.row >= writer.rows {
		return writer.fail()
	}
	if writer.layout.keyed {
		if !id.Available() || writer.row > 0 && compareIdentity(writer.last, id) >= 0 {
			return writer.fail()
		}
		at := writer.idAt + writer.row*identityWidth
		copy(writer.out[at:at+identityWidth], id[:])
		writer.last = id
	} else if id.Available() {
		return writer.fail()
	}
	writer.out[writer.rowAt+writer.row*writer.layout.rowWidth] = state
	if writer.layout.variable >= 0 {
		at := writer.offAt + writer.row*scalarWidth
		binary.BigEndian.PutUint64(writer.out[at:at+scalarWidth], uint64(writer.tail-writer.tailAt))
	}
	writer.open = true
	writer.column = 0
	if state != 0 {
		writer.present = true
	}
	return true
}

// Member fills the next declared column with one named member of its declared
// vocabulary. Absence is the state of a column no producer decided, and it is
// stated by passing false rather than by naming a member that means it.
func (writer *Writer) Member(present bool, member schema.Key) bool {
	column, at, ok := writer.next(CarrierMember)
	if !ok {
		return false
	}
	if !present {
		if member.Available() {
			return writer.fail()
		}
		return true
	}
	state, declared := rank(column.members, member)
	if !declared {
		return writer.fail()
	}
	writer.out[at] = state
	return true
}

// Evidence fills the next declared column with one four-state proof.
func (writer *Writer) Evidence(state Evidence) bool {
	_, at, ok := writer.next(CarrierEvidence)
	if !ok {
		return false
	}
	if !state.Available() {
		return writer.fail()
	}
	writer.out[at] = byte(state)
	return true
}

// Flag fills the next declared column with one decided boolean.
func (writer *Writer) Flag(set bool) bool {
	_, at, ok := writer.next(CarrierFlag)
	if !ok {
		return false
	}
	writer.out[at] = boolByte(set)
	return true
}

// Ordinal fills the next declared column with one optional measure.
func (writer *Writer) Ordinal(present bool, value uint32) bool {
	_, at, ok := writer.next(CarrierOrdinal)
	if !ok {
		return false
	}
	if !present {
		if value != 0 {
			return writer.fail()
		}
		return true
	}
	writer.out[at] = 1
	binary.BigEndian.PutUint32(writer.out[at+presenceWidth:at+presenceWidth+ordinalWidth], value)
	return true
}

// Identity fills the next declared column with one optional portable identity.
func (writer *Writer) Identity(present bool, id identity.ContentID) bool {
	_, at, ok := writer.next(CarrierIdentity)
	if !ok {
		return false
	}
	if !present {
		if id.Available() {
			return writer.fail()
		}
		return true
	}
	if !id.Available() {
		return writer.fail()
	}
	writer.out[at] = 1
	copy(writer.out[at+presenceWidth:at+presenceWidth+identityWidth], id[:])
	return true
}

// Word appends one 64-bit word to the open row's variable column.
func (writer *Writer) Word(value uint64) bool {
	at, ok := writer.appendTail(CarrierWords)
	if !ok {
		return false
	}
	binary.BigEndian.PutUint64(writer.out[at:at+scalarWidth], value)
	return true
}

// Atom appends one portable identity to the open row's variable column.
func (writer *Writer) Atom(id identity.ContentID) bool {
	at, ok := writer.appendTail(CarrierAtoms)
	if !ok {
		return false
	}
	if !id.Available() {
		return writer.fail()
	}
	copy(writer.out[at:at+identityWidth], id[:])
	return true
}

// EndRow closes the open row. A written row must have filled every declared
// column; a row no producer wrote must have filled none, so an absent row can
// never carry column content.
func (writer *Writer) EndRow() bool {
	if writer.failed || !writer.open {
		return writer.fail()
	}
	written := writer.out[writer.rowAt+writer.row*writer.layout.rowWidth] != 0
	if written != (writer.column == len(writer.layout.columns)) {
		return writer.fail()
	}
	writer.open = false
	writer.row++
	return true
}

// Finish seals the payload. Presence is derived from the walk rather than
// restated by the producer: an answer is present exactly when some row of it
// was written, and a producer that believed otherwise is refused here.
//
// Cardinality is the number of result rows the fold observed, which is a
// different question from which coordinates it wrote: a fold that observed a
// row and found every coordinate of it unwritten still observed one row. An
// answer that wrote a coordinate is one observed row by construction, and a
// cardinality above one is not a single answer at all.
func (writer *Writer) Finish(cardinality uint64) (present bool, rows uint64, payload []byte, ok bool) {
	if writer.failed || writer.open || writer.row != writer.rows {
		return false, 0, nil, false
	}
	if cardinality > 1 || writer.present && cardinality != 1 {
		return false, 0, nil, false
	}
	if writer.layout.variable >= 0 {
		at := writer.offAt + writer.rows*scalarWidth
		binary.BigEndian.PutUint64(writer.out[at:at+scalarWidth], uint64(writer.tail-writer.tailAt))
	}
	if writer.tail != len(writer.out) {
		return false, 0, nil, false
	}
	return writer.present, cardinality, writer.out, true
}

// next admits the caller's setter against the next declared column and
// returns its byte offset inside the open row's record.
func (writer *Writer) next(carrier Carrier) (sealedColumn, int, bool) {
	if writer.failed || !writer.open || writer.column >= len(writer.layout.columns) {
		writer.fail()
		return sealedColumn{}, 0, false
	}
	// A row no producer wrote carries no column content, so a setter aimed at
	// one is refused before it can put a decided byte into an absent record.
	if writer.out[writer.rowAt+writer.row*writer.layout.rowWidth] == 0 {
		writer.fail()
		return sealedColumn{}, 0, false
	}
	column := writer.layout.columns[writer.column]
	if column.carrier != carrier {
		writer.fail()
		return sealedColumn{}, 0, false
	}
	at := writer.rowAt + writer.row*writer.layout.rowWidth + writer.layout.offsets[writer.column]
	writer.column++
	return column, at, true
}

// appendTail admits one item of the open row's variable column. The variable
// column is filled in its declaration position like any other, and the items
// land in the row's tail extent.
func (writer *Writer) appendTail(carrier Carrier) (int, bool) {
	if writer.failed || !writer.open || writer.column != writer.layout.variable ||
		writer.out[writer.rowAt+writer.row*writer.layout.rowWidth] == 0 {
		writer.fail()
		return 0, false
	}
	if writer.layout.columns[writer.column].carrier != carrier {
		writer.fail()
		return 0, false
	}
	width := carrier.element()
	at := writer.tail
	if at+width > len(writer.out) {
		writer.fail()
		return 0, false
	}
	writer.tail = at + width
	return at, true
}

// CloseColumn closes the open row's variable column. A variable column is a
// vector, so its end is stated rather than inferred from the next setter.
func (writer *Writer) CloseColumn() bool {
	if writer.failed || !writer.open || writer.column != writer.layout.variable ||
		writer.out[writer.rowAt+writer.row*writer.layout.rowWidth] == 0 {
		return writer.fail()
	}
	writer.column++
	return true
}

func (writer *Writer) fail() bool {
	writer.failed = true
	return false
}

func compareIdentity(left, right identity.ContentID) int {
	for index := 0; index < identityWidth; index++ {
		switch {
		case left[index] < right[index]:
			return -1
		case left[index] > right[index]:
			return 1
		}
	}
	return 0
}
