package plane_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
)

// The two categories these laws seal their layouts against. The laws are about
// the mechanism and not about either vocabulary: the seal ranks whatever the
// category it is named holds, so what matters here is that the members come
// from a sealed declaration rather than from a list beside the codec.
const (
	classes = structure.CategoryPublicationRowClass
	kinds   = structure.CategoryOccurrenceKind
)

// vocabulary seals one structural table declaring the given members of the two
// categories above, in the order given.
func vocabulary(t *testing.T, rowClasses, memberKinds []schema.Key) structure.Table {
	t.Helper()
	var specs []structure.Spec
	declare := func(category structure.Category, members []schema.Key) {
		for index, member := range members {
			specs = append(specs, structure.Spec{
				Key: member, Category: category, Ordinal: uint16(index + 1),
				Spelling: string(member), Accepted: true,
			})
		}
	}
	declare(classes, rowClasses)
	declare(kinds, memberKinds)
	table, ok := structuretest.Table(specs)
	if !ok {
		t.Fatal("the law vocabulary must seal")
	}
	return table
}

// shape derives the published shape of a family answered under one fold. It is
// the registration's own derivation: nothing here decides keying.
func shape(t *testing.T, family schema.Key, fold query.Fold) query.Shape {
	t.Helper()
	published, ok := query.NewShape(family, fold)
	if !ok {
		t.Fatalf("the shape of %q must derive", family)
	}
	return published
}

func id(seed byte) identity.ContentID {
	var value identity.ContentID
	for index := range value {
		value[index] = seed
	}
	return value
}

// keyedLayout exercises every fixed carrier plus a variable word column.
func keyedLayout(t *testing.T) *plane.Sealed {
	t.Helper()
	table := vocabulary(t,
		[]schema.Key{"stack", "owned", "shared", "unknown"},
		[]schema.Key{"table", "closure", "manifest"})
	sealed, ok := plane.Seal(shape(t, "law-keyed", query.FoldDistributive), table, classes, []plane.Column{
		{Key: "kind", Carrier: plane.CarrierMember, Members: kinds},
		{Key: "root", Carrier: plane.CarrierIdentity},
		{Key: "depth", Carrier: plane.CarrierOrdinal},
		{Key: "frozen", Carrier: plane.CarrierEvidence},
		{Key: "top", Carrier: plane.CarrierFlag},
		{Key: "image", Carrier: plane.CarrierWords},
	})
	if !ok {
		t.Fatal("the exercised layout must seal")
	}
	return sealed
}

// TestLayoutIsTheOnlyColumnOrder states that the byte geometry of a published
// answer is a function of the sealed declaration alone. Nothing in a producer
// or a consumer spells an offset, so the offsets must be derivable here.
func TestLayoutIsTheOnlyColumnOrder(t *testing.T) {
	sealed := keyedLayout(t)
	// state + member + (presence+identity) + (presence+ordinal) + evidence + flag
	if want := 1 + 1 + 33 + 5 + 1 + 1; sealed.RowWidth() != want {
		t.Fatalf("row width = %d, want %d derived from the declaration", sealed.RowWidth(), want)
	}
	variable, declared := sealed.Variable()
	if !declared || variable != 5 {
		t.Fatalf("variable column = %d/%v, want the declared position 5", variable, declared)
	}
	if sealed.ColumnCount() != 6 {
		t.Fatalf("column count = %d, want 6", sealed.ColumnCount())
	}
}

// TestLayoutDigestSeparatesDeclarations states that the declaration is the
// payload's identity: two layouts that differ in anything a consumer reads
// reach different digests, and a payload of one refuses to open as the other.
// The vocabularies are declarations of the structural surface, so renaming or
// reordering a member of the category a layout names is one of those
// differences and the bytes written under the layout it replaced are refused.
func TestLayoutDigestSeparatesDeclarations(t *testing.T) {
	baseClasses := []schema.Key{"written"}
	baseKinds := []schema.Key{"a", "b"}
	baseColumns := []plane.Column{{Key: "top", Carrier: plane.CarrierFlag}}
	memberColumns := []plane.Column{{Key: "top", Carrier: plane.CarrierMember, Members: kinds}}
	sealed, ok := plane.Seal(shape(t, "law-digest", query.FoldDistributive),
		vocabulary(t, baseClasses, baseKinds), classes, baseColumns)
	if !ok {
		t.Fatal("base layout must seal")
	}
	drifted := []struct {
		name    string
		family  schema.Key
		fold    query.Fold
		classes []schema.Key
		kinds   []schema.Key
		columns []plane.Column
	}{
		{name: "family", family: "other"},
		{name: "keyed", fold: query.FoldGeneral},
		{name: "state-vocabulary", classes: []schema.Key{"written", "second"}},
		{name: "state-name", classes: []schema.Key{"renamed"}},
		{name: "column-name", columns: []plane.Column{{Key: "renamed", Carrier: plane.CarrierFlag}}},
		{name: "carrier", columns: []plane.Column{{Key: "top", Carrier: plane.CarrierEvidence}}},
		{name: "member-vocabulary", columns: memberColumns},
		{name: "member-arity", kinds: []schema.Key{"a"}, columns: memberColumns},
		{name: "member-name", kinds: []schema.Key{"a", "c"}, columns: memberColumns},
		{name: "member-order", kinds: []schema.Key{"b", "a"}, columns: memberColumns},
		{name: "arity", columns: append(append([]plane.Column(nil), baseColumns...),
			plane.Column{Key: "extra", Carrier: plane.CarrierFlag})},
	}
	digests := map[identity.ContentID]string{sealed.Digest(): "base"}
	for _, drift := range drifted {
		t.Run(drift.name, func(t *testing.T) {
			family, fold := drift.family, drift.fold
			if !family.Available() {
				family = "law-digest"
			}
			if !fold.Available() {
				fold = query.FoldDistributive
			}
			rowClasses, memberKinds, columns := drift.classes, drift.kinds, drift.columns
			if rowClasses == nil {
				rowClasses = baseClasses
			}
			if memberKinds == nil {
				memberKinds = baseKinds
			}
			if columns == nil {
				columns = baseColumns
			}
			other, otherOK := plane.Seal(shape(t, family, fold),
				vocabulary(t, rowClasses, memberKinds), classes, columns)
			if !otherOK {
				t.Fatal("drifted layout must still seal")
			}
			if prior, taken := digests[other.Digest()]; taken {
				t.Fatalf("a drifted declaration reached the layout digest of %s", prior)
			}
			digests[other.Digest()] = drift.name
		})
	}
}

// TestMemberDriftRefusesPriorBytes states the consequence of the law above at
// the wire: a payload written while the category ranked one vocabulary is
// refused once the category declares another.
func TestMemberDriftRefusesPriorBytes(t *testing.T) {
	published := shape(t, "law-drift", query.FoldGeneral)
	columns := []plane.Column{{Key: "kind", Carrier: plane.CarrierMember, Members: kinds}}
	before, beforeOK := plane.Seal(published, vocabulary(t, []schema.Key{"written"}, []schema.Key{"a", "b"}), classes, columns)
	after, afterOK := plane.Seal(published, vocabulary(t, []schema.Key{"written"}, []schema.Key{"b", "a"}), classes, columns)
	if !beforeOK || !afterOK {
		t.Fatal("both declarations must seal")
	}
	writer, begun := plane.Begin(before, identity.ContentID{}, 1, 0)
	if !begun || !writer.Row(identity.ContentID{}, "written") || !writer.Member(true, "a") || !writer.EndRow() {
		t.Fatal("write")
	}
	_, _, payload, encoded := writer.Finish(1)
	if !encoded {
		t.Fatal("encode")
	}
	if _, refusal := plane.Open(after, string(payload)); refusal != plane.RefusalLayout {
		t.Fatalf("refusal = %v (%s), want the foreign-layout refusal", refusal, refusal)
	}
}

// TestKeyingFollowsTheFold states that no layout decides its own keying. A
// family answered over a coordinate space carries the coordinates it holds;
// a family answered whole at one point carries none, and neither can be
// sealed the other way because neither states it.
func TestKeyingFollowsTheFold(t *testing.T) {
	table := vocabulary(t, []schema.Key{"written"}, []schema.Key{"a"})
	columns := []plane.Column{{Key: "top", Carrier: plane.CarrierFlag}}
	summary, summaryOK := plane.Seal(shape(t, "law-summary", query.FoldDistributive), table, classes, columns)
	exact, exactOK := plane.Seal(shape(t, "law-exact-fold", query.FoldGeneral), table, classes, columns)
	if !summaryOK || !exactOK {
		t.Fatal("both folds must seal")
	}
	if _, ok := plane.Begin(summary, identity.ContentID{}, 1, 0); ok {
		t.Fatal("a distributive family sealed an answer with no coordinate space")
	}
	if _, ok := plane.Begin(exact, id(1), 1, 0); ok {
		t.Fatal("a general fold sealed an answer that names a coordinate space")
	}
	if _, ok := exact.Size(2, 0); ok {
		t.Fatal("a general fold sized an answer of more than one row")
	}
}

// TestForeignBytesRefuseByName states that a payload written under another
// declaration is refused, and refused by a name its caller can render.
func TestForeignBytesRefuseByName(t *testing.T) {
	table := vocabulary(t, []schema.Key{"written"}, []schema.Key{"a"})
	columns := []plane.Column{{Key: "top", Carrier: plane.CarrierFlag}}
	mine, _ := plane.Seal(shape(t, "law-mine", query.FoldGeneral), table, classes, columns)
	theirs, _ := plane.Seal(shape(t, "law-theirs", query.FoldGeneral), table, classes, columns)
	writer, ok := plane.Begin(theirs, identity.ContentID{}, 1, 0)
	if !ok {
		t.Fatal("begin")
	}
	writer.Row(identity.ContentID{}, "written")
	writer.Flag(true)
	writer.EndRow()
	_, _, payload, encoded := writer.Finish(1)
	if !encoded {
		t.Fatal("encode")
	}
	if _, refusal := plane.Open(mine, string(payload)); refusal != plane.RefusalLayout {
		t.Fatalf("refusal = %v (%s), want the foreign-layout refusal", refusal, refusal)
	}
	if _, refusal := plane.Open(theirs, string(payload)[:len(payload)-1]); refusal != plane.RefusalTruncated {
		t.Fatalf("refusal = %v (%s), want the truncated refusal", refusal, refusal)
	}
}

// TestMalformedPlanesRefuseByName states the admission laws over the encoded
// planes. Each mutation is a distinct way a payload can be wrong and each has
// its own name.
func TestMalformedPlanesRefuseByName(t *testing.T) {
	sealed := keyedLayout(t)
	writer, ok := plane.Begin(sealed, id(1), 2, 3)
	if !ok {
		t.Fatal("begin")
	}
	writeRow(t, &writer, id(2), "stack", 2)
	writeRow(t, &writer, id(3), "unknown", 1)
	_, _, payload, encoded := writer.Finish(1)
	if !encoded {
		t.Fatal("encode")
	}
	if _, refusal := plane.Open(sealed, string(payload)); refusal.Available() {
		t.Fatalf("the encoder's own payload refused: %v", refusal)
	}

	idAt := 8 + 32 + 32 + 8
	rowAt := idAt + 2*32
	cases := []struct {
		name string
		at   int
		to   byte
		want plane.Refusal
	}{
		{"descending-coordinate-plane", idAt, 0xff, plane.RefusalOrder},
		{"undeclared-row-state", rowAt, 9, plane.RefusalState},
		{"member-outside-its-vocabulary", rowAt + 1, 9, plane.RefusalColumn},
		{"evidence-outside-the-four-state-model", rowAt + 1 + 1 + 33 + 5, 9, plane.RefusalColumn},
		{"flag-outside-its-domain", rowAt + 1 + 1 + 33 + 5 + 1, 9, plane.RefusalColumn},
		{"identity-presence-outside-its-domain", rowAt + 1, 0, plane.RefusalColumn},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			mutated := append([]byte(nil), payload...)
			if one.name == "identity-presence-outside-its-domain" {
				mutated[rowAt+2] = 0
			} else {
				mutated[one.at] = one.to
			}
			if _, refusal := plane.Open(sealed, string(mutated)); refusal != one.want {
				t.Fatalf("refusal = %v (%s), want %v (%s)", refusal, refusal, one.want, one.want)
			}
		})
	}
}

// TestAbsentRowCarriesNothing states the four-state model's floor: a row no
// producer wrote carries no column content, and one that claims to is refused
// rather than read as a decided fact under an unwritten row.
func TestAbsentRowCarriesNothing(t *testing.T) {
	sealed := keyedLayout(t)
	writer, ok := plane.Begin(sealed, id(1), 2, 1)
	if !ok {
		t.Fatal("begin")
	}
	if !writer.Absent(id(2)) || !writer.EndRow() {
		t.Fatal("an absent row must open and close with no column")
	}
	writeRow(t, &writer, id(3), "owned", 1)
	_, rows, payload, encoded := writer.Finish(1)
	if !encoded || rows != 1 {
		t.Fatalf("encode = %v rows = %d, want one present result row", encoded, rows)
	}
	view, refusal := plane.Open(sealed, string(payload))
	if refusal.Available() {
		t.Fatalf("open: %v", refusal)
	}
	absent, _ := view.At(0)
	if absent.Written() {
		t.Fatal("the absent row read as written")
	}
	if _, decided := absent.Class(); decided {
		t.Fatal("the absent row published a class")
	}
	if absent.Evidence(3) != plane.EvidenceAbsent || absent.Count() != 0 {
		t.Fatal("the absent row published column content")
	}

	rowAt := 8 + 32 + 32 + 8 + 2*32
	mutated := append([]byte(nil), payload...)
	mutated[rowAt+1+1+33+5] = byte(plane.EvidenceProven)
	if _, refusal := plane.Open(sealed, string(mutated)); refusal != plane.RefusalAbsentRow {
		t.Fatalf("refusal = %v (%s), want the absent-row refusal", refusal, refusal)
	}
}

// TestEvidenceFourStateRoundTrip states that every one of the four proof
// states survives the wire distinctly, absence included.
func TestEvidenceFourStateRoundTrip(t *testing.T) {
	sealed, ok := plane.Seal(shape(t, "law-evidence", query.FoldDistributive),
		vocabulary(t, []schema.Key{"written"}, []schema.Key{"a"}), classes,
		[]plane.Column{{Key: "proof", Carrier: plane.CarrierEvidence}})
	if !ok {
		t.Fatal("seal")
	}
	states := []plane.Evidence{plane.EvidenceAbsent, plane.EvidenceUnknown, plane.EvidenceRefuted, plane.EvidenceProven}
	writer, _ := plane.Begin(sealed, id(200), len(states), 0)
	for index, state := range states {
		if !writer.Row(id(byte(index+1)), "written") || !writer.Evidence(state) || !writer.EndRow() {
			t.Fatalf("row %d", index)
		}
	}
	_, _, payload, encoded := writer.Finish(1)
	if !encoded {
		t.Fatal("encode")
	}
	view, refusal := plane.Open(sealed, string(payload))
	if refusal.Available() {
		t.Fatalf("open: %v", refusal)
	}
	for index, want := range states {
		row, _ := view.At(index)
		if got := row.Evidence(0); got != want {
			t.Fatalf("row %d evidence = %d, want %d", index, got, want)
		}
		if want.Decided() != (want == plane.EvidenceRefuted || want == plane.EvidenceProven) {
			t.Fatalf("row %d decided disagrees with the four-state model", index)
		}
	}
}

// TestRoundTripCarriesEveryColumn states that every declared carrier survives
// the wire at the value the producer stated.
func TestRoundTripCarriesEveryColumn(t *testing.T) {
	sealed := keyedLayout(t)
	writer, _ := plane.Begin(sealed, id(1), 1, 2)
	if !writer.Row(id(7), "unknown") ||
		!writer.Member(true, "manifest") ||
		!writer.Identity(true, id(9)) ||
		!writer.Ordinal(true, 0x01020304) ||
		!writer.Evidence(plane.EvidenceRefuted) ||
		!writer.Flag(true) ||
		!writer.Word(0xdeadbeefcafef00d) || !writer.Word(7) || !writer.CloseColumn() ||
		!writer.EndRow() {
		t.Fatal("the declaration-order walk must admit every carrier")
	}
	present, rows, payload, encoded := writer.Finish(1)
	if !encoded || !present || rows != 1 {
		t.Fatalf("finish = %v present=%v rows=%d", encoded, present, rows)
	}
	view, refusal := plane.Open(sealed, string(payload))
	if refusal.Available() {
		t.Fatalf("open: %v", refusal)
	}
	if view.Owner() != id(1) || view.RowCount() != 1 || !view.Present() {
		t.Fatal("header did not round-trip")
	}
	row, found := view.Lookup(id(7))
	if !found {
		t.Fatal("the coordinate plane did not resolve its own row")
	}
	class, written := row.Class()
	member, decided := row.Member(0)
	root, hasRoot := row.Identity(1)
	depth, hasDepth := row.Ordinal(2)
	word0, word0OK := row.WordAt(0)
	word1, word1OK := row.WordAt(1)
	switch {
	case !written || class != "unknown":
		t.Fatalf("class = %q/%v, want the declared class", class, written)
	case !decided || member != "manifest":
		t.Fatalf("member = %q/%v", member, decided)
	case !hasRoot || root != id(9):
		t.Fatalf("identity column = %v/%v", root, hasRoot)
	case !hasDepth || depth != 0x01020304:
		t.Fatalf("ordinal column = %d/%v", depth, hasDepth)
	case row.Evidence(3) != plane.EvidenceRefuted:
		t.Fatalf("evidence column = %d", row.Evidence(3))
	case !row.Flag(4):
		t.Fatal("flag column")
	case row.Count() != 2 || !word0OK || word0 != 0xdeadbeefcafef00d || !word1OK || word1 != 7:
		t.Fatalf("variable column = %d items %x/%x", row.Count(), word0, word1)
	case row.ID() != id(7):
		t.Fatalf("coordinate = %v", row.ID())
	}
}

// TestUnkeyedAnswerCarriesNoCoordinatePlane states that a family answering one
// point publishes no coordinate identity: restating the query site's own
// identity on the wire would publish a second authority for it.
func TestUnkeyedAnswerCarriesNoCoordinatePlane(t *testing.T) {
	sealed, ok := plane.Seal(shape(t, "law-exact", query.FoldGeneral),
		vocabulary(t, []schema.Key{"written"}, []schema.Key{"a"}), classes,
		[]plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "atoms", Carrier: plane.CarrierAtoms},
		})
	if !ok {
		t.Fatal("seal")
	}
	writer, _ := plane.Begin(sealed, identity.ContentID{}, 1, 2)
	if !writer.Row(identity.ContentID{}, "written") || !writer.Flag(false) ||
		!writer.Atom(id(4)) || !writer.Atom(id(5)) || !writer.CloseColumn() || !writer.EndRow() {
		t.Fatal("unkeyed walk")
	}
	present, rows, payload, encoded := writer.Finish(1)
	if !encoded || !present || rows != 1 {
		t.Fatalf("finish = %v/%v/%d", encoded, present, rows)
	}
	if want := 8 + 32 + 8 + sealed.RowWidth() + 2*8 + 2*32; len(payload) != want {
		t.Fatalf("payload = %d bytes, want %d with no coordinate plane", len(payload), want)
	}
	view, refusal := plane.Open(sealed, string(payload))
	if refusal.Available() {
		t.Fatalf("open: %v", refusal)
	}
	row, _ := view.At(0)
	first, firstOK := row.AtomAt(0)
	second, secondOK := row.AtomAt(1)
	if row.ID().Available() {
		t.Fatal("an unkeyed row published a coordinate identity")
	}
	if row.Count() != 2 || !firstOK || first != id(4) || !secondOK || second != id(5) {
		t.Fatal("atoms did not round-trip")
	}
	if _, found := view.Lookup(id(4)); found {
		t.Fatal("an unkeyed answer resolved a coordinate")
	}
}

// TestDecodeAndReadAllocateNothing states the reading contract: opening an
// answer and walking every column of every row materializes nothing at all.
func TestDecodeAndReadAllocateNothing(t *testing.T) {
	sealed := keyedLayout(t)
	writer, _ := plane.Begin(sealed, id(1), 8, 16)
	for index := 0; index < 8; index++ {
		writeRow(t, &writer, id(byte(index+2)), []schema.Key{"stack", "owned", "shared", "unknown"}[index%4], 2)
	}
	_, _, encoded, ok := writer.Finish(1)
	if !ok {
		t.Fatal("encode")
	}
	payload := string(encoded)
	if allocations := testing.AllocsPerRun(200, func() {
		view, refusal := plane.Open(sealed, payload)
		if refusal.Available() {
			t.Fatal(refusal)
		}
		for index := 0; index < view.RowCount(); index++ {
			row, _ := view.At(index)
			sinkIdentity = row.ID()
			sinkKey, sinkBool = row.Class()
			sinkKey, sinkBool = row.Member(0)
			sinkIdentity, sinkBool = row.Identity(1)
			sinkUint32, sinkBool = row.Ordinal(2)
			sinkEvidence = row.Evidence(3)
			sinkBool = row.Flag(4)
			for word := 0; word < row.Count(); word++ {
				sinkUint64, sinkBool = row.WordAt(word)
			}
		}
		sinkRow, sinkBool = view.Lookup(id(5))
	}); allocations != 0 {
		t.Fatalf("decode and read allocated %v times per run, want 0", allocations)
	}
}

var (
	sinkIdentity identity.ContentID
	sinkEvidence plane.Evidence
	sinkRow      plane.Row
	sinkUint64   uint64
	sinkUint32   uint32
	sinkKey      schema.Key
	sinkBool     bool
)

// TestWalkRefusesAnUndeclaredOrder states that the declaration is the only
// column order a producer may write in: a setter for the wrong carrier, a
// skipped column, and an unclosed row are all refused.
func TestWalkRefusesAnUndeclaredOrder(t *testing.T) {
	sealed := keyedLayout(t)
	t.Run("wrong-carrier", func(t *testing.T) {
		writer, _ := plane.Begin(sealed, id(1), 1, 0)
		writer.Row(id(2), "stack")
		if writer.Flag(true) {
			t.Fatal("a flag filled a member column")
		}
		if _, _, _, ok := writer.Finish(1); ok {
			t.Fatal("a misaligned walk sealed a payload")
		}
	})
	t.Run("skipped-column", func(t *testing.T) {
		writer, _ := plane.Begin(sealed, id(1), 1, 0)
		writer.Row(id(2), "stack")
		writer.Member(true, "table")
		if writer.EndRow() {
			t.Fatal("a written row closed with columns unfilled")
		}
	})
	t.Run("absent-row-carrying-a-column", func(t *testing.T) {
		writer, _ := plane.Begin(sealed, id(1), 1, 0)
		writer.Absent(id(2))
		if writer.Member(true, "table") {
			t.Fatal("an absent row admitted a column")
		}
	})
	t.Run("descending-coordinates", func(t *testing.T) {
		writer, _ := plane.Begin(sealed, id(1), 2, 0)
		writeRow(t, &writer, id(5), "stack", 0)
		if writer.Row(id(3), "stack") {
			t.Fatal("the coordinate plane admitted a descending row")
		}
	})
	t.Run("short-count", func(t *testing.T) {
		writer, _ := plane.Begin(sealed, id(1), 2, 0)
		writeRow(t, &writer, id(5), "stack", 0)
		if _, _, _, ok := writer.Finish(1); ok {
			t.Fatal("a payload sealed with a row unwritten")
		}
	})
}

// TestSealRefusesAnInadmissibleDeclaration states the layout laws themselves.
// What a member vocabulary must be is no longer among them: uniqueness,
// density, and naming are the structural surface's own laws over the category,
// and a seal that restated them would be a second declaration of the catalog.
func TestSealRefusesAnInadmissibleDeclaration(t *testing.T) {
	table := vocabulary(t, []schema.Key{"written"}, []schema.Key{"m"})
	published := shape(t, "f", query.FoldDistributive)
	cases := map[string]struct {
		shape   query.Shape
		states  structure.Category
		columns []plane.Column
	}{
		"no-shape": {states: classes,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierFlag}}},
		"undeclared-state-category": {shape: published,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierFlag}}},
		"no-column": {shape: published, states: classes},
		"unnamed-column": {shape: published, states: classes,
			columns: []plane.Column{{Carrier: plane.CarrierFlag}}},
		"duplicate-column": {shape: published, states: classes,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierFlag}, {Key: "a", Carrier: plane.CarrierFlag}}},
		"member-without-a-vocabulary": {shape: published, states: classes,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierMember}}},
		"vocabulary-on-a-non-member": {shape: published, states: classes,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierFlag, Members: kinds}}},
		"two-variable-columns": {shape: published, states: classes,
			columns: []plane.Column{{Key: "a", Carrier: plane.CarrierWords}, {Key: "b", Carrier: plane.CarrierAtoms}}},
	}
	for name, one := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := plane.Seal(one.shape, table, one.states, one.columns); ok {
				t.Fatal("an inadmissible declaration sealed")
			}
		})
	}
}

func writeRow(t *testing.T, writer *plane.Writer, coordinate identity.ContentID, class schema.Key, words int) {
	t.Helper()
	if !writer.Row(coordinate, class) {
		t.Fatal("row")
	}
	ok := writer.Member(true, "closure") &&
		writer.Identity(true, coordinate) &&
		writer.Ordinal(true, uint32(words)) &&
		writer.Evidence(plane.EvidenceProven) &&
		writer.Flag(false)
	for index := 0; index < words; index++ {
		ok = ok && writer.Word(uint64(index))
	}
	if !ok || !writer.CloseColumn() || !writer.EndRow() {
		t.Fatal("write row")
	}
}
