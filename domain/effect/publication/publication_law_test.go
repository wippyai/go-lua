package publication

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/structure/structuretest"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func directoryLawIdentity(t *testing.T, tag string) identity.ContentID {
	t.Helper()
	id, derived := identity.DeriveContentID("effect-publication-law/"+tag, nil)
	if !derived {
		t.Fatalf("identity %s", tag)
	}
	return id
}

func directoryLawRow(t *testing.T, tag string) Row {
	t.Helper()
	return Row{
		ID:           directoryLawIdentity(t, "row/"+tag),
		Module:       directoryLawIdentity(t, "module"),
		Call:         directoryLawIdentity(t, "call/"+tag),
		Application:  directoryLawIdentity(t, "application"),
		Kind:         vocabulary.PublicationEffectKind(1),
		Escape:       vocabulary.PublicationEscapeDisposition(1),
		Mutability:   vocabulary.PublicationMutabilityDisposition(1),
		Lifetime:     vocabulary.PublicationLifetimeDisposition(1),
		DescriptorID: directoryLawIdentity(t, "descriptor/"+tag),
		OccurrenceID: directoryLawIdentity(t, "occurrence/"+tag),
		Operation:    vocabulary.Operation(1),
		Effect:       0,
		Subject:      directoryLawIdentity(t, "subject/"+tag),
	}
}

// directoryLawVocabulary seals the four publication descriptor catalogs alone.
// A directory row's dispositions resolve against exactly these ranks, so the
// law works against the same declaration the composition contributes.
func directoryLawVocabulary(t *testing.T) structure.Table {
	t.Helper()
	table, sealed := structuretest.Table(structure.PublicationEffectSpecs())
	if !sealed {
		t.Fatal("seal publication vocabularies")
	}
	return table
}

func sealDirectoryLaw(t *testing.T, rows []Row) (snapshot.Snapshot, snapshot.Axis[identity.ContentID, Row]) {
	t.Helper()
	schema := directoryLawIdentity(t, "link-schema")
	denominator, derived := DenominatorID(directoryLawIdentity(t, "link"), rows)
	if !derived {
		t.Fatal("directory denominator")
	}
	content, sealed := Content(rows, denominator, directoryLawVocabulary(t))
	if !sealed {
		t.Fatal("directory content")
	}
	address := Axis(schema, 0)
	builder := snapshot.NewBuilder(schema, identity.StoreID(11), identity.Generation(1))
	if err := snapshot.PutColumn(&builder, address, content); err != nil {
		t.Fatalf("put directory column: %v", err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return published, address
}

// A directory row is a fact that crosses to readers holding neither Effect's
// algebra nor Pack's mounted inputs, so it carries no pointer and no owner-
// fenced capability of either. A row that carried one would publish a live
// handle under the name of a sealed fact.
func TestDirectoryRowCarriesNoLiveCapability(t *testing.T) {
	row := reflect.TypeOf(Row{})
	for index := 0; index < row.NumField(); index++ {
		field := row.Field(index)
		switch field.Type.Kind() {
		case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
			t.Fatalf("Row.%s is %s", field.Name, field.Type)
		}
		for _, forbidden := range []string{"factor", "packtransfer", "transfer.MountedInput"} {
			if strings.Contains(field.Type.String(), forbidden) {
				t.Fatalf("Row.%s is %s", field.Name, field.Type)
			}
		}
	}
}

// The directory answers for exactly the publications it admitted. An identity
// it did not admit resolves to no row at all, never to another admission's
// row, so a consumer that holds an identity learns whether this Link
// published it rather than being handed a neighbour.
func TestDirectoryAnswersForItsOwnAdmissionsOnly(t *testing.T) {
	first, second := directoryLawRow(t, "a"), directoryLawRow(t, "b")
	published, address := sealDirectoryLaw(t, []Row{first, second})

	for _, want := range []Row{first, second} {
		row, status := Published(&published, address, want.ID)
		if status != snapshot.ReadHit {
			t.Fatalf("admitted %v: status %v", want.ID, status)
		}
		if !reflect.DeepEqual(row, want) {
			t.Fatalf("admitted %v resolved to another row", want.ID)
		}
	}
	stranger := directoryLawIdentity(t, "row/c")
	if row, status := Published(&published, address, stranger); status == snapshot.ReadHit {
		t.Fatalf("unadmitted %v resolved to %v", stranger, row.ID)
	}
}

// The directory is walkable in its own sealed order, and the walk names
// exactly the rows it admitted: this is how a consumer selects the
// publications of one mounted call without reconstructing a per-call batch.
func TestDirectoryEnumeratesEveryAdmittedRowInSealedOrder(t *testing.T) {
	rows := []Row{directoryLawRow(t, "a"), directoryLawRow(t, "b"), directoryLawRow(t, "c")}
	published, address := sealDirectoryLaw(t, rows)

	count, counted := Count(&published, address)
	if !counted || count != len(rows) {
		t.Fatalf("count %d ok %v, want %d", count, counted, len(rows))
	}
	for index, want := range rows {
		id, walked := At(&published, address, index)
		if !walked || id != want.ID {
			t.Fatalf("member %d is %v ok %v, want %v", index, id, walked, want.ID)
		}
		row, status := Published(&published, address, id)
		if status != snapshot.ReadHit || !reflect.DeepEqual(row, want) {
			t.Fatalf("member %d does not resolve to its row", index)
		}
	}
	if _, walked := At(&published, address, len(rows)); walked {
		t.Fatalf("directory names a member past its own admission")
	}
}

// Reading the directory does not change it: the same identity read twice
// yields the identical row, so a consumer's own reads cannot make one
// publication mean two things.
func TestDirectoryReadsAreRepeatable(t *testing.T) {
	row := directoryLawRow(t, "a")
	published, address := sealDirectoryLaw(t, []Row{row})

	first, firstStatus := Published(&published, address, row.ID)
	second, secondStatus := Published(&published, address, row.ID)
	if firstStatus != snapshot.ReadHit || secondStatus != snapshot.ReadHit || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated read differs: %v/%v", firstStatus, secondStatus)
	}
}

// The directory's identity is its membership. Two admissions of one Link that
// admitted different rows are different directories, and an admission is
// byte-stable under repetition, so nothing can open one admission's rows under
// another's name.
func TestDirectoryIdentityIsItsMembership(t *testing.T) {
	link := directoryLawIdentity(t, "link")
	first, second := directoryLawRow(t, "a"), directoryLawRow(t, "b")

	one, oneOK := DenominatorID(link, []Row{first})
	again, againOK := DenominatorID(link, []Row{first})
	two, twoOK := DenominatorID(link, []Row{first, second})
	swapped, swappedOK := DenominatorID(link, []Row{second, first})
	empty, emptyOK := DenominatorID(link, nil)
	if !oneOK || !againOK || !twoOK || !swappedOK || !emptyOK {
		t.Fatal("directory identity")
	}
	if one != again {
		t.Fatal("one admission has two identities")
	}
	for _, other := range []identity.ContentID{two, swapped, empty} {
		if one == other {
			t.Fatal("two admissions share one identity")
		}
	}
	if two == swapped {
		t.Fatal("admission identity ignores its sealed order")
	}
}

// A Link that authored no publication states that: the directory seals, names
// no member, and admits nothing. It is not a missing column a reader has to
// treat as ignorance.
func TestEmptyDirectoryIsAPublishedAdmission(t *testing.T) {
	published, address := sealDirectoryLaw(t, nil)

	count, counted := Count(&published, address)
	if !counted || count != 0 {
		t.Fatalf("empty directory count %d ok %v", count, counted)
	}
	if _, status := Published(&published, address, directoryLawIdentity(t, "row/a")); status == snapshot.ReadHit {
		t.Fatal("empty directory admitted a row")
	}
}

// The directory refuses an admission it cannot state completely: a duplicate
// identity would make one coordinate answer twice, and an incomplete row is
// not a weaker publication but not one at all.
func TestDirectoryRefusesADuplicateOrIncompleteAdmission(t *testing.T) {
	denominator := directoryLawIdentity(t, "denominator")
	declared := directoryLawVocabulary(t)
	row := directoryLawRow(t, "a")
	if _, sealed := Content([]Row{row, row}, denominator, declared); sealed {
		t.Fatal("directory sealed a duplicate admission")
	}
	partial := row
	partial.Subject = identity.ContentID{}
	if _, sealed := Content([]Row{partial}, denominator, declared); sealed {
		t.Fatal("directory sealed an incomplete admission")
	}
	contextless := row
	contextless.HasContext = true
	if _, sealed := Content([]Row{contextless}, denominator, declared); sealed {
		t.Fatal("directory sealed a destination without its context")
	}
	if _, sealed := Content([]Row{row}, identity.ContentID{}, declared); sealed {
		t.Fatal("directory sealed without a universe")
	}
}

// A published disposition is a member of the catalog that declares it. A row
// carrying a value past the vocabulary's last rank is a byte no consumer can
// resolve, so it is refused at the seal rather than published as an opaque
// number.
func TestDirectoryRefusesADispositionTheVocabularyDoesNotDeclare(t *testing.T) {
	denominator := directoryLawIdentity(t, "denominator")
	declared := directoryLawVocabulary(t)
	row := directoryLawRow(t, "a")

	for _, undeclared := range []Row{
		func() Row {
			r := row
			r.Kind = vocabulary.PublicationEffectKind(declared.Count(structure.CategoryPublicationEffectKind) + 1)
			return r
		}(),
		func() Row {
			r := row
			r.Escape = vocabulary.PublicationEscapeDisposition(declared.Count(structure.CategoryPublicationEscape) + 1)
			return r
		}(),
		func() Row {
			r := row
			r.Mutability = vocabulary.PublicationMutabilityDisposition(declared.Count(structure.CategoryPublicationMutability) + 1)
			return r
		}(),
		func() Row {
			r := row
			r.Lifetime = vocabulary.PublicationLifetimeDisposition(declared.Count(structure.CategoryPublicationLifetime) + 1)
			return r
		}(),
	} {
		if _, sealed := Content([]Row{undeclared}, denominator, declared); sealed {
			t.Fatal("directory published a disposition the vocabulary does not declare")
		}
	}
}

// The four catalogs rank exactly the authored enums they stand for: the
// member at each rank is the enum value of that number, so a published byte
// is read as the disposition its producer meant and a renumbering on either
// side is a refusal rather than a silent reinterpretation.
func TestPublicationVocabulariesRankTheAuthoredEnums(t *testing.T) {
	declared := directoryLawVocabulary(t)
	for _, category := range []struct {
		category structure.Category
		last     uint16
		spelling string
	}{
		{structure.CategoryPublicationEffectKind, uint16(vocabulary.PublicationEffectCloseRelease), "close-release"},
		{structure.CategoryPublicationEscape, uint16(vocabulary.PublicationEscapeCallback), "callback"},
		{structure.CategoryPublicationMutability, uint16(vocabulary.PublicationMutabilityCopyOnWrite), "copy-on-write"},
		{structure.CategoryPublicationLifetime, uint16(vocabulary.PublicationLifetimeRelease), "release"},
	} {
		if count := declared.Count(category.category); uint16(count) != category.last {
			t.Fatalf("category %d ranks %d members, enum names %d", category.category, count, category.last)
		}
		ordinal, spelled := declared.Spelling(category.category, category.spelling)
		if !spelled || ordinal != category.last {
			t.Fatalf("category %d spells %q at %d, enum names %d", category.category, category.spelling, ordinal, category.last)
		}
	}
	if _, ranked := declared.At(structure.CategoryPublicationEffectKind, uint16(vocabulary.PublicationEffectInvalid)); ranked {
		t.Fatal("the invalid zero is ranked as a member")
	}
}

// directoryLawCall is one mounted call claiming a span of the row column.
func directoryLawCall(t *testing.T, tag string, offset, length uint32) CallRow {
	t.Helper()
	return CallRow{
		ID:          directoryLawIdentity(t, "batch/"+tag),
		Module:      directoryLawIdentity(t, "module"),
		Call:        directoryLawIdentity(t, "call/"+tag),
		Application: directoryLawIdentity(t, "application"),
		RowOffset:   offset,
		RowLength:   length,
	}
}

// A mounted call that published nothing is a published fact. The row column
// states rows, so it cannot distinguish a call whose program authored no
// publication from a call the directory never saw; the calls column states
// the call itself, and an empty span is that call's complete answer.
func TestMountedCallWithNoPublicationIsAPublishedFact(t *testing.T) {
	rows := []Row{directoryLawRow(t, "a")}
	calls := []CallRow{directoryLawCall(t, "loud", 0, 1), directoryLawCall(t, "silent", 1, 0)}

	denominator, derived := CallsDenominatorID(directoryLawIdentity(t, "link"), calls)
	if !derived {
		t.Fatal("calls denominator")
	}
	content, sealed := CallsContent(calls, len(rows), denominator)
	if !sealed {
		t.Fatal("calls content")
	}
	schemaID := directoryLawIdentity(t, "link-schema")
	address := CallsAxis(schemaID, 0)
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(11), identity.Generation(1))
	if err := snapshot.PutColumn(&builder, address, content); err != nil {
		t.Fatalf("put calls column: %v", err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	silent, status := MountedCall(&published, address, calls[1].ID)
	if status != snapshot.ReadHit {
		t.Fatalf("silent call status %v", status)
	}
	if silent.RowLength != 0 {
		t.Fatalf("silent call claims %d rows", silent.RowLength)
	}
	stranger := directoryLawIdentity(t, "batch/absent")
	if _, status := MountedCall(&published, address, stranger); status == snapshot.ReadHit {
		t.Fatal("a call the directory never admitted resolved")
	}
}

// The calls tile the rows exactly. A span that skipped a row would publish a
// receipt no call admits, and a span that overlapped would let one receipt
// answer under two calls, so the seal refuses both rather than publishing a
// directory a consumer has to reconcile.
func TestCallSpansTileTheRowColumnExactly(t *testing.T) {
	denominator, derived := CallsDenominatorID(directoryLawIdentity(t, "link"), nil)
	if !derived {
		t.Fatal("calls denominator")
	}
	for name, calls := range map[string][]CallRow{
		"gap":       {directoryLawCall(t, "a", 0, 1), directoryLawCall(t, "b", 2, 1)},
		"overlap":   {directoryLawCall(t, "a", 0, 2), directoryLawCall(t, "b", 1, 1)},
		"short":     {directoryLawCall(t, "a", 0, 1)},
		"past-end":  {directoryLawCall(t, "a", 0, 4)},
		"duplicate": {directoryLawCall(t, "a", 0, 2), directoryLawCall(t, "a", 2, 1)},
	} {
		if _, sealed := CallsContent(calls, 3, denominator); sealed {
			t.Fatalf("%s spans sealed", name)
		}
	}
	exact := []CallRow{directoryLawCall(t, "a", 0, 2), directoryLawCall(t, "b", 2, 0), directoryLawCall(t, "c", 2, 1)}
	if _, sealed := CallsContent(exact, 3, denominator); !sealed {
		t.Fatal("exact tiling refused")
	}
}

// One mounted coordinate names one call. Two rows sharing a module and
// occurrence would make the provenance a consumer selects by ambiguous, so
// the seal refuses them even though their own identities differ.
func TestOneMountedCoordinateNamesOneCall(t *testing.T) {
	denominator, derived := CallsDenominatorID(directoryLawIdentity(t, "link"), nil)
	if !derived {
		t.Fatal("calls denominator")
	}
	first, second := directoryLawCall(t, "a", 0, 1), directoryLawCall(t, "b", 1, 1)
	second.Call = first.Call
	if _, sealed := CallsContent([]CallRow{first, second}, 2, denominator); sealed {
		t.Fatal("two calls sealed under one mounted coordinate")
	}
}

// A row's subject members are read through the row's own span, so a consumer
// reaches them holding neither Effect's algebra nor the mounted pack. The
// span addresses that row's members and no other row's.
func TestSubjectMembersAreReadThroughTheRowsOwnSpan(t *testing.T) {
	first, second := directoryLawRow(t, "a"), directoryLawRow(t, "b")
	first.SubjectOffset, first.SubjectLength = 0, 2
	second.SubjectOffset, second.SubjectLength = 2, 1
	rows := []Row{first, second}

	members := make([]MemberRow, 0, 3)
	for _, row := range rows {
		for position := 0; position < int(row.SubjectLength); position++ {
			id, derived := MemberID(row.ID, position)
			if !derived {
				t.Fatal("member identity")
			}
			members = append(members, MemberRow{
				ID:       id,
				RowID:    row.ID,
				Semantic: directoryLawIdentity(t, fmt.Sprintf("semantic/%v/%d", row.ID, position)),
				Member:   uint32(position),
			})
		}
	}

	denominator, derived := MembersDenominatorID(directoryLawIdentity(t, "link"), members)
	if !derived {
		t.Fatal("members denominator")
	}
	content, sealed := MembersContent(members, rows, denominator)
	if !sealed {
		t.Fatal("members content")
	}
	schemaID := directoryLawIdentity(t, "link-schema")
	address := MembersAxis(schemaID, 0)
	builder := snapshot.NewBuilder(schemaID, identity.StoreID(11), identity.Generation(1))
	if err := snapshot.PutColumn(&builder, address, content); err != nil {
		t.Fatalf("put members column: %v", err)
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	for _, row := range rows {
		for position := 0; position < int(row.SubjectLength); position++ {
			member, read := SubjectMember(&published, address, row, position)
			if !read {
				t.Fatalf("row %v member %d unread", row.ID, position)
			}
			if member.RowID != row.ID || member.Member != uint32(position) {
				t.Fatalf("row %v member %d resolved to another row's member", row.ID, position)
			}
		}
		if _, read := SubjectMember(&published, address, row, int(row.SubjectLength)); read {
			t.Fatalf("row %v reads past its own span", row.ID)
		}
	}
}

// A member column that did not tile the rows' spans would let a row's span
// address another row's members, so the seal refuses it.
func TestMemberSpansTileTheRowsExactly(t *testing.T) {
	row := directoryLawRow(t, "a")
	row.SubjectOffset, row.SubjectLength = 0, 2
	denominator, derived := MembersDenominatorID(directoryLawIdentity(t, "link"), nil)
	if !derived {
		t.Fatal("members denominator")
	}
	first, _ := MemberID(row.ID, 0)
	second, _ := MemberID(row.ID, 1)
	semantic := directoryLawIdentity(t, "semantic")
	foreign := directoryLawIdentity(t, "row/z")

	for name, members := range map[string][]MemberRow{
		"short":       {{ID: first, RowID: row.ID, Semantic: semantic, Member: 0}},
		"foreign-row": {{ID: first, RowID: row.ID, Semantic: semantic, Member: 0}, {ID: second, RowID: foreign, Semantic: semantic, Member: 1}},
		"misordered":  {{ID: first, RowID: row.ID, Semantic: semantic, Member: 1}, {ID: second, RowID: row.ID, Semantic: semantic, Member: 0}},
		"overlong": {
			{ID: first, RowID: row.ID, Semantic: semantic, Member: 0},
			{ID: second, RowID: row.ID, Semantic: semantic, Member: 1},
			{ID: directoryLawIdentity(t, "member/extra"), RowID: row.ID, Semantic: semantic, Member: 2},
		},
	} {
		if _, sealed := MembersContent(members, []Row{row}, denominator); sealed {
			t.Fatalf("%s members sealed", name)
		}
	}
}
