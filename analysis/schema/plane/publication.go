package plane

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// This file is the declaration half of the publication story. Seal above
// admits what a family's answers look like on the wire; what a family states
// here is where those bytes come from: the classes a written row is held at,
// the columns it publishes, and the projection that reads one row and one
// column value out of a frozen answer.
//
// The two halves are one declaration, so a family authors neither an encoder
// nor a second description of its layout. Publish is the analyzer's one
// producer: it sizes the payload from the declaration, walks the sealed
// columns, and derives presence and cardinality from the walk. A domain that
// spelled that walk itself would be a second wire authority, and there is
// exactly one.

// Cell is one column value a projection states. It is a value, not a write: a
// projection hands back what the column holds and the driver puts it where the
// sealed declaration says it goes, so a producer never reaches the output
// buffer and never sees a byte offset.
//
// A cell carries the carrier it was stated at. The sealed column's carrier is
// what admits it, so a value stated at another carrier is refused rather than
// reinterpreted under the one the declaration holds.
type Cell struct {
	carrier  Carrier
	present  bool
	flag     bool
	member   schema.Key
	evidence Evidence
	ordinal  uint32
	id       identity.ContentID
	words    []uint64
	atoms    []identity.ContentID
}

// MemberCell states one member of a column's declared vocabulary. Absence is
// stated by passing false, never by naming a member that means it.
func MemberCell(present bool, member schema.Key) Cell {
	return Cell{carrier: CarrierMember, present: present, member: member}
}

// EvidenceCell states one four-state proof.
func EvidenceCell(state Evidence) Cell {
	return Cell{carrier: CarrierEvidence, evidence: state}
}

// FlagCell states one decided boolean.
func FlagCell(set bool) Cell { return Cell{carrier: CarrierFlag, flag: set} }

// OrdinalCell states one optional unsigned measure.
func OrdinalCell(present bool, value uint32) Cell {
	return Cell{carrier: CarrierOrdinal, present: present, ordinal: value}
}

// IdentityCell states one optional portable identity.
func IdentityCell(present bool, id identity.ContentID) Cell {
	return Cell{carrier: CarrierIdentity, present: present, id: id}
}

// WordsCell states one row's complete variable word vector. The slice is
// borrowed for the length of the write and is never retained.
func WordsCell(words []uint64) Cell { return Cell{carrier: CarrierWords, words: words} }

// AtomsCell states one row's complete variable identity vector. The slice is
// borrowed for the length of the write and is never retained.
func AtomsCell(atoms []identity.ContentID) Cell {
	return Cell{carrier: CarrierAtoms, atoms: atoms}
}

// write puts one stated value into the writer's next declared column. The
// writer admits the carrier, so a mismatch fails the walk here rather than
// producing a payload whose columns disagree with its layout.
func (cell Cell) write(writer *Writer) bool {
	switch cell.carrier {
	case CarrierMember:
		return writer.Member(cell.present, cell.member)
	case CarrierEvidence:
		return writer.Evidence(cell.evidence)
	case CarrierFlag:
		return writer.Flag(cell.flag)
	case CarrierOrdinal:
		return writer.Ordinal(cell.present, cell.ordinal)
	case CarrierIdentity:
		return writer.Identity(cell.present, cell.id)
	case CarrierWords:
		for _, word := range cell.words {
			if !writer.Word(word) {
				return false
			}
		}
		return writer.CloseColumn()
	case CarrierAtoms:
		for _, atom := range cell.atoms {
			if !writer.Atom(atom) {
				return false
			}
		}
		return writer.CloseColumn()
	default:
		return false
	}
}

// Projection is a family's declared production of published rows from one
// frozen answer. It is five statements and no wire knowledge:
//
// Owner names the coordinate space the rows are fenced by; a family answered
// at one point states the zero identity and the sealed layout refuses an owner
// it did not declare.
//
// Extent is the published row count and the total number of items every row's
// variable column holds together. It is also the family's admission of its own
// answer: an observation whose internal invariants do not hold states no
// extent and publishes nothing.
//
// Cardinality is the number of result rows the fold observed, which is a
// different question from which coordinates were written.
//
// Row addresses one published row: the coordinate it holds and the declared
// class it is published at. An unavailable class publishes an unwritten row,
// so absence is a state of the row and never a reserved member of the class
// vocabulary.
//
// Cell states one written row's value for one declared column, in declaration
// order.
type Projection[A any] struct {
	Owner       func(answer A) identity.ContentID
	Extent      func(answer A) (rows int, elements int, ok bool)
	Cardinality func(answer A) uint64
	Row         func(answer A, row int) (identity.ContentID, schema.Key, bool)
	Cell        func(answer A, row int, column int) (Cell, bool)
}

// Available reports whether every statement the driver runs on is declared. A
// projection is all-or-nothing: a partially stated one publishes nothing
// rather than filling the gap with a default.
func (projection Projection[A]) Available() bool {
	return projection.Owner != nil && projection.Extent != nil && projection.Cardinality != nil &&
		projection.Row != nil && projection.Cell != nil
}

// Publication is one family's complete published declaration: the row state
// vocabulary its written rows are held at, the columns it publishes, and the
// projection those columns are read out of. It is the single thing a family
// authors about publishing, and the composition seals exactly one layout from
// it.
type Publication[A any] struct {
	// States is the structural category whose members a written row may be
	// published at. The members are that category's declaration and are read
	// from the sealed vocabulary, never listed here.
	States structure.Category
	// Columns are the columns this family publishes, in declaration order.
	Columns []Column
	// Projection reads one answer's rows and column values.
	Projection Projection[A]
}

// Available reports whether this declaration states a row vocabulary, at least
// one column, and a complete projection.
func (publication Publication[A]) Available() bool {
	return publication.States.Available() && len(publication.Columns) > 0 && publication.Projection.Available()
}

// SealPublication admits one family's declaration and issues the one layout
// its answers are published under. The family and keying come from the query
// registration's shape and the vocabularies from the sealed structural table,
// exactly as Seal takes them; what this adds is that the states and columns
// are read off the same declaration the projection belongs to, so a layout and
// the projection that writes under it cannot be paired wrongly.
func SealPublication[A any](shape query.Shape, vocabulary structure.Table, publication Publication[A]) (*Sealed, bool) {
	if !publication.Available() {
		return nil, false
	}
	return Seal(shape, vocabulary, publication.States, publication.Columns)
}

// Publish writes one frozen answer onto its sealed layout through the family's
// declared projection. It is the analyzer's one producer of published bytes.
//
// The walk is the declaration: rows are opened in the order the projection
// addresses them, an unwritten row carries no column content, every declared
// column of a written row is filled from the projection in declaration order,
// and presence and cardinality are derived from the walk rather than restated.
// The payload costs the one allocation of its exact final size.
func Publish[A any](layout *Sealed, projection Projection[A], answer A) (present bool, rows uint64, payload []byte, ok bool) {
	if !layout.Available() || !projection.Available() {
		return false, 0, nil, false
	}
	count, elements, extentOK := projection.Extent(answer)
	if !extentOK {
		return false, 0, nil, false
	}
	writer, begun := Begin(layout, projection.Owner(answer), count, elements)
	if !begun {
		return false, 0, nil, false
	}
	columns := len(layout.columns)
	for row := 0; row < count; row++ {
		id, class, addressed := projection.Row(answer, row)
		if !addressed {
			return false, 0, nil, false
		}
		if !class.Available() {
			if !writer.Absent(id) || !writer.EndRow() {
				return false, 0, nil, false
			}
			continue
		}
		if !writer.Row(id, class) {
			return false, 0, nil, false
		}
		for column := 0; column < columns; column++ {
			cell, stated := projection.Cell(answer, row, column)
			if !stated || !cell.write(&writer) {
				return false, 0, nil, false
			}
		}
		if !writer.EndRow() {
			return false, 0, nil, false
		}
	}
	return writer.Finish(projection.Cardinality(answer))
}
