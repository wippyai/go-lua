package plane_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// lawAnswer is the frozen answer shape these laws project. It is deliberately
// nothing like a domain observation: what the laws are about is that a family
// states its rows and its column values and states nothing about the wire.
type lawAnswer struct {
	owner   identity.ContentID
	rows    []lawRow
	written uint64
}

type lawRow struct {
	id    identity.ContentID
	class schema.Key
	flag  bool
	image []uint64
}

// lawPublication is one family's complete published declaration: the classes a
// written row is held at, the columns it publishes, and the projection that
// states each row and each column value.
func lawPublication() plane.Publication[lawAnswer] {
	return plane.Publication[lawAnswer]{
		States: classes,
		Columns: []plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "image", Carrier: plane.CarrierWords},
		},
		Projection: plane.Projection[lawAnswer]{
			Owner: func(answer lawAnswer) identity.ContentID { return answer.owner },
			Extent: func(answer lawAnswer) (int, int, bool) {
				elements := 0
				for _, row := range answer.rows {
					elements += len(row.image)
				}
				return len(answer.rows), elements, true
			},
			Cardinality: func(answer lawAnswer) uint64 { return answer.written },
			Row: func(answer lawAnswer, index int) (identity.ContentID, schema.Key, bool) {
				row := answer.rows[index]
				return row.id, row.class, true
			},
			Cell: func(answer lawAnswer, index, column int) (plane.Cell, bool) {
				row := answer.rows[index]
				switch column {
				case 0:
					return plane.FlagCell(row.flag), true
				case 1:
					return plane.WordsCell(row.image), true
				}
				return plane.Cell{}, false
			},
		},
	}
}

func lawPublicationLayout(t *testing.T) *plane.Sealed {
	t.Helper()
	sealed, ok := plane.SealPublication(
		shape(t, "law-publication", query.FoldDistributive),
		vocabulary(t, []schema.Key{structure.PublicationClassHeld}, []schema.Key{"a"}),
		lawPublication())
	if !ok {
		t.Fatal("the declared publication must seal one layout")
	}
	return sealed
}

// TestSealPublicationDerivesTheLayoutFromTheDeclaration states that a family's
// publication declaration is the layout: the states and columns a caller would
// otherwise hand to Seal separately are read off the one declaration, so the
// layout a family publishes under cannot disagree with the projection that
// writes it.
func TestSealPublicationDerivesTheLayoutFromTheDeclaration(t *testing.T) {
	declaration := lawPublication()
	sealed := lawPublicationLayout(t)
	table := vocabulary(t, []schema.Key{structure.PublicationClassHeld}, []schema.Key{"a"})
	separate, separateOK := plane.Seal(shape(t, "law-publication", query.FoldDistributive), table, declaration.States, declaration.Columns)
	if !separateOK || separate.Digest() != sealed.Digest() {
		t.Fatal("sealing a declaration must reach the layout its states and columns reach")
	}
	if sealed.ColumnCount() != len(declaration.Columns) {
		t.Fatalf("sealed column count = %d, want the declared %d", sealed.ColumnCount(), len(declaration.Columns))
	}
}

// TestPublishWritesTheDeclaredWalk states that the generic driver produces the
// payload the sealed declaration describes: rows in coordinate order, an
// unwritten row at its coordinate carrying no column content, and every
// declared column filled from the projection.
func TestPublishWritesTheDeclaredWalk(t *testing.T) {
	sealed := lawPublicationLayout(t)
	answer := lawAnswer{
		owner: id(9),
		rows: []lawRow{
			{id: id(1), class: structure.PublicationClassHeld, flag: true, image: []uint64{7, 8}},
			{id: id(2)},
			{id: id(3), class: structure.PublicationClassHeld, image: []uint64{9}},
		},
		written: 1,
	}
	present, rows, payload, ok := plane.Publish(sealed, lawPublication().Projection, answer)
	if !ok || !present || rows != 1 {
		t.Fatalf("publish = %v/%v/%v, want one present answer", present, rows, ok)
	}
	opened, refusal := plane.Open(sealed, string(payload))
	if refusal.Available() || !opened.Available() {
		t.Fatalf("a published payload must open under the layout that wrote it: %v", refusal)
	}
	if opened.RowCount() != 3 {
		t.Fatalf("row count = %d, want the three declared rows", opened.RowCount())
	}
	first, firstOK := opened.At(0)
	second, secondOK := opened.At(1)
	third, thirdOK := opened.At(2)
	if !firstOK || !secondOK || !thirdOK {
		t.Fatal("every published row must read back")
	}
	if !first.Written() || second.Written() || !third.Written() {
		t.Fatal("the unwritten row must read back unwritten and the written rows written")
	}
	if !first.Flag(0) {
		t.Fatal("the first row's declared flag must read back set")
	}
	if first.Count() != 2 {
		t.Fatalf("the first row's declared image holds %d words, want two", first.Count())
	}
	if word, wordOK := first.WordAt(0); !wordOK || word != 7 {
		t.Fatalf("the first image word = %v/%v, want the projected 7", word, wordOK)
	}
	if word, wordOK := first.WordAt(1); !wordOK || word != 8 {
		t.Fatalf("the second image word = %v/%v, want the projected 8", word, wordOK)
	}
	if third.Count() != 1 {
		t.Fatalf("the third row's declared image holds %d words, want one", third.Count())
	}
	if word, wordOK := third.WordAt(0); !wordOK || word != 9 {
		t.Fatalf("the third image word = %v/%v, want the projected 9", word, wordOK)
	}
}

// TestPublishAllocatesOnePayload states the driver's cost law: a declared
// publication is written straight into the output buffer, so the projection
// costs nothing beyond the payload it produces.
func TestPublishAllocatesOnePayload(t *testing.T) {
	sealed := lawPublicationLayout(t)
	projection := lawPublication().Projection
	answer := lawAnswer{
		owner: id(9),
		rows: []lawRow{
			{id: id(1), class: structure.PublicationClassHeld, flag: true, image: []uint64{7, 8}},
			{id: id(2)},
		},
		written: 1,
	}
	allocations := testing.AllocsPerRun(200, func() {
		if _, _, _, ok := plane.Publish(sealed, projection, answer); !ok {
			t.Fatal("the law answer must publish")
		}
	})
	if allocations != 1 {
		t.Fatalf("publish allocations = %v, want one payload allocation", allocations)
	}
}

// TestPublishRefusesAColumnStatedAtAnotherCarrier states that the declaration
// is what a projection is held to: a family that states a column value the
// sealed carrier does not admit is refused rather than silently reinterpreted.
func TestPublishRefusesAColumnStatedAtAnotherCarrier(t *testing.T) {
	sealed := lawPublicationLayout(t)
	projection := lawPublication().Projection
	projection.Cell = func(_ lawAnswer, _, column int) (plane.Cell, bool) {
		if column == 0 {
			return plane.OrdinalCell(true, 3), true
		}
		return plane.WordsCell(nil), true
	}
	answer := lawAnswer{
		owner:   id(9),
		rows:    []lawRow{{id: id(1), class: structure.PublicationClassHeld}},
		written: 1,
	}
	if _, _, _, ok := plane.Publish(sealed, projection, answer); ok {
		t.Fatal("a column stated at a carrier the declaration does not seal must refuse")
	}
}

// TestPublishRefusesAnUndeclaredRowClass states that the row state vocabulary
// is the sealed category's: a projection that names a class the declaration
// does not hold is refused, so an unwritten row is the only way a producer
// states absence.
func TestPublishRefusesAnUndeclaredRowClass(t *testing.T) {
	sealed := lawPublicationLayout(t)
	projection := lawPublication().Projection
	projection.Row = func(answer lawAnswer, index int) (identity.ContentID, schema.Key, bool) {
		return answer.rows[index].id, "not-a-declared-class", true
	}
	answer := lawAnswer{
		owner:   id(9),
		rows:    []lawRow{{id: id(1)}},
		written: 1,
	}
	if _, _, _, ok := plane.Publish(sealed, projection, answer); ok {
		t.Fatal("a row published at an undeclared class must refuse")
	}
}

// TestPublishRefusesAnIncompleteDeclaration states that a projection is
// all-or-nothing: a family cannot publish through a declaration that leaves
// any of the five statements the driver runs on unstated.
func TestPublishRefusesAnIncompleteDeclaration(t *testing.T) {
	sealed := lawPublicationLayout(t)
	complete := lawPublication()
	if !complete.Available() {
		t.Fatal("the complete law declaration must be available")
	}
	partial := []plane.Publication[lawAnswer]{
		{States: complete.States, Columns: complete.Columns},
		{Columns: complete.Columns, Projection: complete.Projection},
		{States: complete.States, Projection: complete.Projection},
	}
	for index, declaration := range partial {
		if declaration.Available() {
			t.Fatalf("partial declaration %d must not be available", index)
		}
		if _, ok := plane.SealPublication(shape(t, "law-partial", query.FoldDistributive),
			vocabulary(t, []schema.Key{structure.PublicationClassHeld}, []schema.Key{"a"}), declaration); ok {
			t.Fatalf("partial declaration %d must not seal a layout", index)
		}
	}
	empty := plane.Projection[lawAnswer]{}
	if _, _, _, ok := plane.Publish(sealed, empty, lawAnswer{}); ok {
		t.Fatal("an unstated projection must not publish")
	}
}

// TestPublishIsTheOnlyWireAuthority states that the driver and a hand-written
// walk over the same declaration reach the same bytes. It is the law that lets
// a family delete its own encoder: the payload is a function of the sealed
// declaration and the projected values, and of nothing the producer spells.
func TestPublishIsTheOnlyWireAuthority(t *testing.T) {
	sealed := lawPublicationLayout(t)
	answer := lawAnswer{
		owner: id(9),
		rows: []lawRow{
			{id: id(1), class: structure.PublicationClassHeld, flag: true, image: []uint64{7, 8}},
			{id: id(2)},
			{id: id(3), class: structure.PublicationClassHeld, image: []uint64{9}},
		},
		written: 1,
	}
	_, _, driven, drivenOK := plane.Publish(sealed, lawPublication().Projection, answer)

	writer, begun := plane.Begin(sealed, answer.owner, len(answer.rows), 3)
	if !begun {
		t.Fatal("the hand-written walk must begin")
	}
	written := true
	for _, row := range answer.rows {
		if !row.class.Available() {
			written = written && writer.Absent(row.id)
		} else {
			written = written && writer.Row(row.id, row.class) && writer.Flag(row.flag)
			for _, word := range row.image {
				written = written && writer.Word(word)
			}
			written = written && writer.CloseColumn()
		}
		written = written && writer.EndRow()
	}
	_, _, walked, walkedOK := writer.Finish(answer.written)
	if !drivenOK || !walkedOK {
		t.Fatal("both walks must produce a payload")
	}
	if !bytes.Equal(driven, walked) {
		t.Fatal("the declared projection and the walk it replaces must reach identical bytes")
	}
}
