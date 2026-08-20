package snapshot

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The sequence laws publish one emitted plane: a column whose key universe is
// its own dense ordinal range, which is the discipline every plane of a
// compiled program has. The plane is published three ways -- as a cold
// sequence, as the same sequence under a revisable lifecycle, and as a keyed
// mapping over the same ordinals -- because the claim under test is that the
// three are one published value stored three ways.
const sequenceRows = 4096

var (
	sequenceSchema      = identity.ContentID{0x1A, 0xA1}
	sequenceStore       = identity.StoreID(21)
	sequenceDenominator = identity.ContentID{0x1B, 0xB1}
	sequenceGeneration  = identity.Generation(5)

	sequenceAxis = Axis[ordinal, payload]{SchemaID: sequenceSchema, Slot: 0}

	sinkPayload  payload
	sinkSpan     []payload
	sinkSpanHeld bool
	sinkFrozen   Frozen
)

// ordinal is a dense key: a row's position in the plane its family published
// is the key itself.
type ordinal uint32

// payload is a row wide enough that a read that boxed it would show up as an
// allocation.
type payload struct {
	Weight uint64
	Reach  uint64
	Marked bool
}

// bytes returns the row's contribution to a digest of the plane.
func (row payload) bytes() []byte {
	encoded := make([]byte, 0, 17)
	encoded = binary.LittleEndian.AppendUint64(encoded, row.Weight)
	encoded = binary.LittleEndian.AppendUint64(encoded, row.Reach)
	if row.Marked {
		return append(encoded, 1)
	}
	return append(encoded, 0)
}

// sequenceRowsOf returns the emitted plane of rows rows.
func sequenceRowsOf(rows int) []payload {
	emitted := make([]payload, rows)
	for index := range emitted {
		emitted[index] = payload{Weight: uint64(index), Reach: uint64(index) * 3, Marked: index%2 == 0}
	}
	return emitted
}

// sequenceContent states the plane as what it is: a sequence whose universe
// is its own position range.
func sequenceContent(rows []payload) Content[ordinal, payload] {
	return Content[ordinal, payload]{Sequence: rows, Denominator: sequenceDenominator}
}

// keyedContent states the same plane as a keyed mapping over the ordinal
// range, which is what a writer with no dense discipline hands to a slot.
func keyedContent(rows []payload) Content[ordinal, payload] {
	stored := make(map[ordinal]payload, len(rows))
	members := make([]ordinal, 0, len(rows))
	for index, row := range rows {
		stored[ordinal(index)] = row
		members = append(members, ordinal(index))
	}
	return Content[ordinal, payload]{Rows: stored, Denominator: sequenceDenominator, Members: members}
}

// sealedCold seals content into a cold publication.
func sealedCold(t testing.TB, content Content[ordinal, payload]) Frozen {
	t.Helper()
	builder := NewFrozen(sequenceSchema, sequenceStore)
	if err := PutFrozenColumn(&builder, sequenceAxis, content); err != nil {
		t.Fatalf("put cold column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal cold publication: %v", err)
	}
	return sealed
}

// sealedHot seals content into a revisable publication.
func sealedHot(t testing.TB, content Content[ordinal, payload]) Snapshot {
	t.Helper()
	builder := NewBuilder(sequenceSchema, sequenceStore, sequenceGeneration)
	if err := PutColumn(&builder, sequenceAxis, content); err != nil {
		t.Fatalf("put hot column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal hot publication: %v", err)
	}
	return sealed
}

// planeIdentity digests what a column answers: the outcome and the row of
// every key a reader can name, one past the sealed width, in key order. It
// reads the published value and never its storage, so two columns that hold
// one row set digest alike whatever they hold it in.
func planeIdentity(t testing.TB, width int, read func(key ordinal) (payload, ReadStatus)) identity.ContentID {
	t.Helper()
	preimage := make([]byte, 0, (width+1)*18)
	for index := 0; index <= width; index++ {
		row, status := read(ordinal(index))
		preimage = append(preimage, byte(status))
		preimage = append(preimage, row.bytes()...)
	}
	digest, derived := identity.DeriveContentID("snapshot/law/plane", preimage)
	if !derived {
		t.Fatal("derive plane identity")
	}
	return digest
}

// TestPublishedIdentityIsTheRowsNotTheStorage is the representation law. A
// column's identity is what it answers, and what it answers is its rows: the
// same plane sealed as a cold sequence, as a revisable sequence and as a
// keyed mapping publishes one value under one identity. The law reads every
// key one past the sealed width, so a difference in an outcome, in a row, or
// in where the universe ends would move the digest.
//
// It is the law that has to hold before storage may be chosen at all: a
// publication that stored its rows differently and therefore identified
// differently would make the choice observable, and every consumer's
// dependency on a snapshot would depend on how the snapshot was built.
func TestPublishedIdentityIsTheRowsNotTheStorage(t *testing.T) {
	rows := sequenceRowsOf(64)
	cold := sealedCold(t, sequenceContent(rows))
	hot := sealedHot(t, sequenceContent(rows))
	keyed := sealedCold(t, keyedContent(rows))

	coldIdentity := planeIdentity(t, len(rows), func(key ordinal) (payload, ReadStatus) {
		return ReadFrozen(&cold, sequenceAxis, key)
	})
	hotIdentity := planeIdentity(t, len(rows), func(key ordinal) (payload, ReadStatus) {
		return Read(&hot, sequenceAxis, key)
	})
	keyedIdentity := planeIdentity(t, len(rows), func(key ordinal) (payload, ReadStatus) {
		return ReadFrozen(&keyed, sequenceAxis, key)
	})

	if coldIdentity != hotIdentity {
		t.Fatalf("cold sequence identity %s, revisable sequence identity %s", coldIdentity, hotIdentity)
	}
	if coldIdentity != keyedIdentity {
		t.Fatalf("sequence identity %s, keyed identity %s", coldIdentity, keyedIdentity)
	}

	revised := make([]payload, len(rows))
	copy(revised, rows)
	revised[9] = payload{Weight: 1}
	changed := sealedCold(t, sequenceContent(revised))
	changedIdentity := planeIdentity(t, len(revised), func(key ordinal) (payload, ReadStatus) {
		return ReadFrozen(&changed, sequenceAxis, key)
	})
	if changedIdentity == coldIdentity {
		t.Fatal("one changed row is a different plane: the digest reads the rows")
	}

	coldWidth, coldPublished := cold.Denominators().Size(sequenceDenominator)
	keyedWidth, keyedPublished := keyed.Denominators().Size(sequenceDenominator)
	hotWidth, hotPublished := hot.Denominators().Size(sequenceDenominator)
	if !coldPublished || !keyedPublished || !hotPublished {
		t.Fatal("every publication publishes the denominator it names")
	}
	if coldWidth != len(rows) || keyedWidth != len(rows) || hotWidth != len(rows) {
		t.Fatalf("universe cardinality = %d, %d and %d, want %d", coldWidth, keyedWidth, hotWidth, len(rows))
	}
}

// TestStorageFollowsTheLifecycle fixes which storage each lifecycle chooses,
// so the law above is a statement about two storages rather than about one.
// A cold column of a dense universe holds its sequence, because the only cost
// it can ever pay again is the read. The same content under a revisable
// lifecycle holds a persistent trie, because a revision must cost its change
// set. A keyed content holds a trie whatever the lifecycle.
func TestStorageFollowsTheLifecycle(t *testing.T) {
	rows := sequenceRowsOf(16)
	cold := sealedCold(t, sequenceContent(rows))
	hot := sealedHot(t, sequenceContent(rows))
	keyed := sealedCold(t, keyedContent(rows))

	coldColumn, recovered := columnAt[ordinal, payload](&cold.publication, sequenceSchema, sequenceAxis.Slot)
	if !recovered || !coldColumn.sequence || len(coldColumn.values) != len(rows) || coldColumn.rows != nil {
		t.Fatal("a cold column of a dense universe holds its sequence")
	}
	hotColumn, recovered := columnAt[ordinal, payload](&hot.publication, sequenceSchema, sequenceAxis.Slot)
	if !recovered || hotColumn.sequence || hotColumn.rows == nil {
		t.Fatal("a revisable column holds a persistent trie")
	}
	keyedColumn, recovered := columnAt[ordinal, payload](&keyed.publication, sequenceSchema, sequenceAxis.Slot)
	if !recovered || keyedColumn.sequence || keyedColumn.rows == nil {
		t.Fatal("a keyed column holds a persistent trie")
	}
	if coldColumn.members == nil || !coldColumn.members.ordinal || coldColumn.members.width != len(rows) {
		t.Fatal("a dense universe is stated by its width")
	}
	if keyedColumn.members == nil || keyedColumn.members.ordinal || keyedColumn.members.members == nil {
		t.Fatal("a named universe holds its members")
	}
}

// TestSequenceReadsAllocateNothing extends the point-read cost law to the
// storage a cold dense column holds: every outcome is answered by an index
// and allocates nothing.
func TestSequenceReadsAllocateNothing(t *testing.T) {
	rows := sequenceRowsOf(32)
	cold := sealedCold(t, sequenceContent(rows))

	cases := []struct {
		name string
		want ReadStatus
		read func()
	}{
		{
			name: "hit",
			want: ReadHit,
			read: func() { sinkPayload, sinkStatus = ReadFrozen(&cold, sequenceAxis, 7) },
		},
		{
			name: "past the universe",
			want: ReadMiss,
			read: func() { sinkPayload, sinkStatus = ReadFrozen(&cold, sequenceAxis, ordinal(len(rows))) },
		},
		{
			name: "rejected axis",
			want: ReadInvalid,
			read: func() {
				sinkPayload, sinkStatus = ReadFrozen(&cold, Axis[ordinal, payload]{SchemaID: fixtureOtherSchema}, 7)
			},
		},
		{
			name: "span",
			want: ReadHit,
			read: func() {
				sinkSpan, sinkSpanHeld = ReadSpan(&cold, sequenceAxis, 4, 16)
				sinkStatus = ReadHit
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(1000, testCase.read); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
			if sinkStatus != testCase.want {
				t.Fatalf("status = %v, want %v", sinkStatus, testCase.want)
			}
		})
	}
}

// TestSpanBorrowsTheSealedPlane is the span law. A run of a cold sequence is
// borrowed out of the sealed column rather than copied out of it: the rows
// are the published rows, two spans of one range name one storage, and the
// borrowed slice cannot be grown into the rows that follow it. A range that
// runs past the sealed width borrows nothing at all, and a column that holds
// no sequence answers no span.
func TestSpanBorrowsTheSealedPlane(t *testing.T) {
	rows := sequenceRowsOf(32)
	cold := sealedCold(t, sequenceContent(rows))

	span, held := ReadSpan(&cold, sequenceAxis, 8, 4)
	if !held || len(span) != 4 {
		t.Fatalf("span = %v, %v, want four rows", len(span), held)
	}
	for index := range span {
		if span[index] != rows[8+index] {
			t.Fatalf("span row %d = %v, want %v", index, span[index], rows[8+index])
		}
	}
	if cap(span) != len(span) {
		t.Fatalf("span capacity = %d, want %d: an append must copy rather than write the rows that follow", cap(span), len(span))
	}
	again, _ := ReadSpan(&cold, sequenceAxis, 8, 4)
	if &again[0] != &span[0] {
		t.Fatal("two spans of one range borrow one storage")
	}

	empty, heldEmpty := ReadSpan(&cold, sequenceAxis, uint32(len(rows)), 0)
	if !heldEmpty || len(empty) != 0 {
		t.Fatal("the empty span at the end of the plane is a span of no rows")
	}
	if _, past := ReadSpan(&cold, sequenceAxis, uint32(len(rows)-1), 2); past {
		t.Fatal("a span past the sealed width borrows nothing")
	}
	if _, wide := ReadSpan(&cold, sequenceAxis, 0, 1<<31); wide {
		t.Fatal("a span whose range overflows the plane borrows nothing")
	}

	keyed := sealedCold(t, keyedContent(rows))
	if _, held := ReadSpan(&keyed, sequenceAxis, 0, 4); held {
		t.Fatal("a keyed column holds no contiguous run to borrow")
	}
	if _, held := ReadSpan[ordinal, payload](nil, sequenceAxis, 0, 4); held {
		t.Fatal("no publication answers no span")
	}
	if _, held := ReadSpan(&cold, Axis[ordinal, payload]{SchemaID: fixtureOtherSchema}, 0, 4); held {
		t.Fatal("an axis of another schema answers no span")
	}
}

// TestSequenceContentStatesOneRowSet fixes what a content may state. Rows and
// a sequence are two statements of one row set, and a sequence states its own
// universe, so naming members alongside it is a second membership authority.
// A key type that has no positions has no sequence to be addressed by.
func TestSequenceContentStatesOneRowSet(t *testing.T) {
	rows := sequenceRowsOf(4)
	builder := NewFrozen(sequenceSchema, sequenceStore)

	doubled := sequenceContent(rows)
	doubled.Rows = map[ordinal]payload{0: rows[0]}
	if err := PutFrozenColumn(&builder, sequenceAxis, doubled); err == nil {
		t.Fatal("content that states its rows twice seals nothing")
	}

	remembered := sequenceContent(rows)
	remembered.Members = []ordinal{0, 1}
	if err := PutFrozenColumn(&builder, sequenceAxis, remembered); err == nil {
		t.Fatal("a sequence that also names members seals nothing")
	}

	named := NewFrozen(sequenceSchema, sequenceStore)
	textual := Axis[string, payload]{SchemaID: sequenceSchema, Slot: 0}
	if err := PutFrozenColumn(&named, textual, Content[string, payload]{Sequence: rows}); err == nil {
		t.Fatal("a key type without positions addresses no sequence")
	}

	narrow := NewFrozen(sequenceSchema, sequenceStore)
	narrowAxis := Axis[uint8, payload]{SchemaID: sequenceSchema, Slot: 0}
	if err := PutFrozenColumn(&narrow, narrowAxis, Content[uint8, payload]{Sequence: sequenceRowsOf(300)}); err == nil {
		t.Fatal("a sequence longer than its key type can count seals nothing")
	}
	if err := PutFrozenColumn(&narrow, narrowAxis, Content[uint8, payload]{Sequence: sequenceRowsOf(256)}); err != nil {
		t.Fatalf("a sequence its key type can count seals: %v", err)
	}
}

// TestSequenceUniverseIsSealedOnce fixes that a dense universe is a
// membership authority like any other: the identity carries it once, a second
// column names the identity alone and reads against the very same universe,
// and a second declaration of it is rejected.
func TestSequenceUniverseIsSealedOnce(t *testing.T) {
	rows := sequenceRowsOf(8)
	mirror := Axis[ordinal, payload]{SchemaID: sequenceSchema, Slot: 1}
	builder := NewFrozen(sequenceSchema, sequenceStore)
	if err := PutFrozenColumn(&builder, sequenceAxis, sequenceContent(rows)); err != nil {
		t.Fatalf("put sequence column: %v", err)
	}
	if err := PutFrozenColumn(&builder, mirror, Content[ordinal, payload]{Denominator: sequenceDenominator}); err != nil {
		t.Fatalf("put mirror column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, status := ReadFrozen(&sealed, mirror, 3); status != ReadProvenAbsent {
		t.Fatalf("mirror read = %v, want a proven absence over the shared universe", status)
	}
	if _, status := ReadFrozen(&sealed, mirror, ordinal(len(rows))); status != ReadMiss {
		t.Fatalf("mirror read past the universe = %v, want a miss", status)
	}

	second := NewFrozen(sequenceSchema, sequenceStore)
	if err := PutFrozenColumn(&second, sequenceAxis, sequenceContent(rows)); err != nil {
		t.Fatalf("put sequence column: %v", err)
	}
	if err := PutFrozenColumn(&second, mirror, sequenceContent(rows)); err == nil {
		t.Fatal("a second declaration of one universe is a second authority")
	}
}

// TestDeltaOverSequenceContentBehavesIdentically fixes that stating rows as a
// sequence changes nothing about a revisable publication: the rows are the
// same rows, a row edit reaches them, and the edited key becomes an absence
// the shared universe proves.
func TestDeltaOverSequenceContentBehavesIdentically(t *testing.T) {
	rows := sequenceRowsOf(64)
	base := sealedHot(t, sequenceContent(rows))

	delta := NewDelta(base, sequenceGeneration+1)
	edited := payload{Weight: 99, Reach: 99, Marked: true}
	if err := SetRow(&delta, sequenceAxis, 5, edited); err != nil {
		t.Fatalf("set row: %v", err)
	}
	if err := RemoveRow(&delta, sequenceAxis, 6); err != nil {
		t.Fatalf("remove row: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}

	if row, status := Read(&sealed, sequenceAxis, 5); status != ReadHit || row != edited {
		t.Fatalf("edited row = %v, %v, want the published edit", row, status)
	}
	if _, status := Read(&sealed, sequenceAxis, 6); status != ReadProvenAbsent {
		t.Fatalf("withdrawn row = %v, want a proven absence", status)
	}
	if row, status := Read(&sealed, sequenceAxis, 7); status != ReadHit || row != rows[7] {
		t.Fatalf("untouched row = %v, %v, want the base row", row, status)
	}
	if row, status := Read(&base, sequenceAxis, 5); status != ReadHit || row != rows[5] {
		t.Fatalf("base row = %v, %v, want what the base published", row, status)
	}
}

// BenchmarkColdSequenceRead reports what a consumer of a cold plane pays per
// row: the point read of every ordinal, against the slice index the same
// plane costs when it is read out of the emitted sequence directly, and
// against the keyed storage a column without the dense declaration holds.
func BenchmarkColdSequenceRead(b *testing.B) {
	rows := sequenceRowsOf(sequenceRows)
	cold := sealedCold(b, sequenceContent(rows))
	keyed := sealedCold(b, keyedContent(rows))

	b.Run("sequence", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for index := 0; index < sequenceRows; index++ {
				sinkPayload, sinkStatus = ReadFrozen(&cold, sequenceAxis, ordinal(index))
			}
		}
	})
	b.Run("keyed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for index := 0; index < sequenceRows; index++ {
				sinkPayload, sinkStatus = ReadFrozen(&keyed, sequenceAxis, ordinal(index))
			}
		}
	})
	b.Run("slice-baseline", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for index := 0; index < sequenceRows; index++ {
				sinkPayload = rows[index]
			}
		}
	})
}

// BenchmarkColdSequenceSpan reports what borrowing a run of a plane costs
// against reading the same run one row at a time.
func BenchmarkColdSequenceSpan(b *testing.B) {
	rows := sequenceRowsOf(sequenceRows)
	cold := sealedCold(b, sequenceContent(rows))
	const width = 64

	b.Run("span", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			sinkSpan, sinkSpanHeld = ReadSpan(&cold, sequenceAxis, 128, width)
		}
	})
	b.Run("row-by-row", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			run := make([]payload, 0, width)
			for index := 0; index < width; index++ {
				row, _ := ReadFrozen(&cold, sequenceAxis, ordinal(128+index))
				run = append(run, row)
			}
			sinkSpan = run
		}
	})
}

// BenchmarkColdSequenceSeal reports what publishing one cold plane costs in
// each of the two forms a content can state it.
func BenchmarkColdSequenceSeal(b *testing.B) {
	rows := sequenceRowsOf(sequenceRows)
	b.Run("sequence", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			builder := NewFrozen(sequenceSchema, sequenceStore)
			if err := PutFrozenColumn(&builder, sequenceAxis, sequenceContent(rows)); err != nil {
				b.Fatalf("put sequence column: %v", err)
			}
			sealed, err := builder.Seal()
			if err != nil {
				b.Fatalf("seal: %v", err)
			}
			sinkFrozen = sealed
		}
	})
	b.Run("keyed", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			builder := NewFrozen(sequenceSchema, sequenceStore)
			if err := PutFrozenColumn(&builder, sequenceAxis, keyedContent(rows)); err != nil {
				b.Fatalf("put keyed column: %v", err)
			}
			sealed, err := builder.Seal()
			if err != nil {
				b.Fatalf("seal: %v", err)
			}
			sinkFrozen = sealed
		}
	})
}
