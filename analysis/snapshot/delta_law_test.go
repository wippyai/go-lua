package snapshot

import (
	"errors"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The delta laws publish one wide column so a per-publication cost that
// scales with the column shows up as a measurement rather than as an opinion,
// and a second column that shares its denominator so sharing shows up as
// pointer identity rather than as equal answers.
const (
	wideRows  = 10000
	widerRows = wideRows * 10
)

var (
	wideSchema      = identity.ContentID{0x0A, 0xA0}
	wideStore       = identity.StoreID(11)
	wideDenominator = identity.ContentID{0x0B, 0xB0}
	wideID          = identity.ContentID{0x0C, 0xC0}
	wideMirrorID    = identity.ContentID{0x0D, 0xD0}

	wideAxis   = Axis[int, int]{SchemaID: wideSchema, Slot: 0}
	mirrorAxis = Axis[int, int]{SchemaID: wideSchema, Slot: 1}

	sinkSnapshot Snapshot
	sinkPlan     QueryPlan[string, int]
	sinkOpened   bool
)

// wideContent returns the rows and the denominator members of the wide
// column: rows for the first half of the key universe, membership over all of
// it, so every unstored member reads as a proven absence.
func wideContent(rows int) Content[int, int] {
	stored := make(map[int]int, rows)
	for key := 0; key < rows; key++ {
		stored[key] = key * 3
	}
	members := make([]int, 0, rows*2)
	for member := 0; member < rows*2; member++ {
		members = append(members, member)
	}
	return Content[int, int]{Rows: stored, Denominator: wideDenominator, Members: members}
}

// wideSnapshot seals the wide column and a mirror column that is total over
// the very same denominator.
func wideSnapshot(t testing.TB, rows int, generation identity.Generation) Snapshot {
	t.Helper()
	builder := NewBuilder(wideSchema, wideStore, generation)
	if err := PutColumn(&builder, wideAxis, wideContent(rows)); err != nil {
		t.Fatalf("put wide column: %v", err)
	}
	if err := PutColumn(&builder, mirrorAxis, Content[int, int]{
		Rows:        map[int]int{1: 7},
		Denominator: wideDenominator,
	}); err != nil {
		t.Fatalf("put mirror column: %v", err)
	}
	if err := builder.Publish(wideID, wideAxis.Slot); err != nil {
		t.Fatalf("publish wide column: %v", err)
	}
	if err := builder.Publish(wideMirrorID, mirrorAxis.Slot); err != nil {
		t.Fatalf("publish mirror column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal wide snapshot: %v", err)
	}
	return sealed
}

// TestDeltaPublicationCostsItsChangeSet is the delta cost law. A publication
// derived from a sealed snapshot pays for the rows it changes and for nothing
// else: it copies the changed rows' paths, keeps every other node of the
// column, and keeps the denominator and the untouched columns by pointer. The
// law is measured rather than asserted, and it is measured twice, because the
// claim is that the change set and not the column is the unit of cost: a
// column ten times as wide must not cost ten times as much to republish.
func TestDeltaPublicationCostsItsChangeSet(t *testing.T) {
	const bound = 40
	narrowAllocations, narrowBytes := publicationCost(t, wideRows)
	wideAllocations, wideBytes := publicationCost(t, widerRows)
	t.Logf("%d rows: %v allocations, %d bytes", wideRows, narrowAllocations, narrowBytes)
	t.Logf("%d rows: %v allocations, %d bytes", widerRows, wideAllocations, wideBytes)

	if narrowAllocations > bound || wideAllocations > bound {
		t.Fatalf("delta publication allocations = %v and %v, want at most %d", narrowAllocations, wideAllocations, bound)
	}
	if wideAllocations > narrowAllocations+4 {
		t.Fatalf("a %dx wider column costs %v allocations against %v: publication scales with the column", widerRows/wideRows, wideAllocations, narrowAllocations)
	}
	if wideBytes > narrowBytes*2 {
		t.Fatalf("a %dx wider column costs %d bytes against %d: publication copies the column", widerRows/wideRows, wideBytes, narrowBytes)
	}
	if wideBytes > 4096 {
		t.Fatalf("delta publication = %d bytes, want a change-set sized publication", wideBytes)
	}
}

// publicationCost measures one derived publication that changes one row of a
// column holding rows rows.
func publicationCost(t *testing.T, rows int) (float64, uint64) {
	t.Helper()
	base := wideSnapshot(t, rows, identity.Generation(1))
	return measure(t, func() {
		delta := NewDelta(base, identity.Generation(2))
		if err := SetRow(&delta, wideAxis, 7, 700); err != nil {
			t.Fatalf("set row: %v", err)
		}
		sealed, err := delta.Seal()
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		sinkSnapshot = sealed
	})
}

// measure reports the allocations and the allocated bytes one publication
// costs. Bytes are measured as well as allocations because the claim under
// test is that a publication does not copy what it did not change, and a
// structure copied in one allocation would satisfy a count alone.
func measure(t *testing.T, publish func()) (float64, uint64) {
	t.Helper()
	const runs = 100
	allocations := testing.AllocsPerRun(runs, publish)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for iteration := 0; iteration < runs; iteration++ {
		publish()
	}
	runtime.ReadMemStats(&after)
	return allocations, (after.TotalAlloc - before.TotalAlloc) / runs
}

// TestDeltaSharesWhatItDoesNotChange fixes what a derived publication keeps.
// An untouched column is the very column the base published, an untouched
// denominator is the very set the base sealed, and the base keeps answering
// what it answered: a delta derives a new value and never edits a published
// one.
func TestDeltaSharesWhatItDoesNotChange(t *testing.T) {
	base := wideSnapshot(t, 64, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := SetRow(&delta, wideAxis, 7, 700); err != nil {
		t.Fatalf("set row: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}

	if base.columns[mirrorAxis.Slot] != sealed.columns[mirrorAxis.Slot] {
		t.Fatal("an untouched column was republished as a copy")
	}
	if base.columns[wideAxis.Slot] == sealed.columns[wideAxis.Slot] {
		t.Fatal("an edited column was published as the base's column")
	}
	if denominatorOf(t, base, wideAxis.Slot) != denominatorOf(t, sealed, wideAxis.Slot) {
		t.Fatal("an unchanged denominator was copied by the edit")
	}
	if value, status := Read(&base, wideAxis, 7); value != 21 || status != ReadHit {
		t.Fatalf("base row = (%d, %v), want (21, hit)", value, status)
	}
	if value, status := Read(&sealed, wideAxis, 7); value != 700 || status != ReadHit {
		t.Fatalf("published row = (%d, %v), want (700, hit)", value, status)
	}
	for key := 0; key < 64; key++ {
		if key == 7 {
			continue
		}
		value, status := Read(&sealed, wideAxis, key)
		wantValue, wantStatus := Read(&base, wideAxis, key)
		if value != wantValue || status != wantStatus {
			t.Fatalf("row %d = (%d, %v), want (%d, %v)", key, value, status, wantValue, wantStatus)
		}
	}
	if _, status := Read(&sealed, wideAxis, 100); status != ReadProvenAbsent {
		t.Fatalf("inherited denominator status = %v, want proven-absent", status)
	}
	if sealed.Generation() != identity.Generation(2) || sealed.Store() != base.Store() || sealed.Schema() != base.Schema() {
		t.Fatalf("derived anchors = (%s, %d, %d)", sealed.Schema(), sealed.Store(), sealed.Generation())
	}
}

// TestDenominatorIsSealedOnceAndShared is the denominator law. Membership is
// a value with its own identity: the column that names the identity first
// seals it, every later column that names it reads against that very set, and
// no edit to a column can reach the set. Two columns total over one key
// universe cost one membership set.
func TestDenominatorIsSealedOnceAndShared(t *testing.T) {
	base := wideSnapshot(t, 64, identity.Generation(1))
	if base.Denominators().Len() != 1 {
		t.Fatalf("denominators = %d, want 1", base.Denominators().Len())
	}
	if !base.Denominators().Proves(wideDenominator, wideAxis.Slot) || !base.Denominators().Proves(wideDenominator, mirrorAxis.Slot) {
		t.Fatal("one denominator does not report proving both columns that declared it")
	}
	if denominatorOf(t, base, wideAxis.Slot) != denominatorOf(t, base, mirrorAxis.Slot) {
		t.Fatal("two columns total over one identity hold two membership sets")
	}
	if _, status := Read(&base, mirrorAxis, 100); status != ReadProvenAbsent {
		t.Fatalf("shared denominator status = %v, want proven-absent", status)
	}

	delta := NewDelta(base, identity.Generation(2))
	if err := SetRow(&delta, wideAxis, 100, 1); err != nil {
		t.Fatalf("set row: %v", err)
	}
	if err := RemoveRow(&delta, mirrorAxis, 1); err != nil {
		t.Fatalf("remove row: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}
	if denominatorOf(t, base, wideAxis.Slot) != denominatorOf(t, sealed, wideAxis.Slot) {
		t.Fatal("editing a column rebuilt its denominator")
	}
	if denominatorOf(t, sealed, wideAxis.Slot) != denominatorOf(t, sealed, mirrorAxis.Slot) {
		t.Fatal("editing one column detached the other from the shared denominator")
	}
	if _, status := Read(&base, wideAxis, 100); status != ReadProvenAbsent {
		t.Fatalf("base status after a derived write = %v, want proven-absent", status)
	}
	if _, status := Read(&sealed, mirrorAxis, 1); status != ReadProvenAbsent {
		t.Fatalf("removed row status = %v, want proven-absent", status)
	}
}

// TestSharedDenominatorRejectsSecondAuthority fixes what sharing does not
// allow. Membership is declared once: a second declaration under one identity
// is a second authority, and a column of another key type cannot read against
// a set sealed for keys it cannot even compare.
func TestSharedDenominatorRejectsSecondAuthority(t *testing.T) {
	builder := NewBuilder(wideSchema, wideStore, identity.Generation(1))
	if err := PutColumn(&builder, wideAxis, wideContent(8)); err != nil {
		t.Fatalf("put wide column: %v", err)
	}
	err := PutColumn(&builder, mirrorAxis, Content[int, int]{
		Denominator: wideDenominator,
		Members:     []int{99},
	})
	if !errors.Is(err, ErrDuplicatePublication) {
		t.Fatalf("second membership authority = %v, want %v", err, ErrDuplicatePublication)
	}
	crossed := Axis[string, int]{SchemaID: wideSchema, Slot: 2}
	err = PutColumn(&builder, crossed, Content[string, int]{Denominator: wideDenominator})
	if !errors.Is(err, ErrColumnKind) {
		t.Fatalf("denominator of another key type = %v, want %v", err, ErrColumnKind)
	}
}

// TestRemovedRowBecomesAbsence is the stale row law. A publication withdraws
// a row it no longer stands behind at the cost of that row's path: the key
// reads as a proven absence when the column's denominator covers it and as a
// miss when nothing covers it, the base keeps its row, and removing a row the
// column never held changes nothing.
func TestRemovedRowBecomesAbsence(t *testing.T) {
	base := wideSnapshot(t, 64, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := RemoveRow(&delta, wideAxis, 7); err != nil {
		t.Fatalf("remove covered row: %v", err)
	}
	if err := RemoveRow(&delta, wideAxis, 4096); err != nil {
		t.Fatalf("remove unheld row: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}
	if _, status := Read(&sealed, wideAxis, 7); status != ReadProvenAbsent {
		t.Fatalf("removed covered row = %v, want proven-absent", status)
	}
	if _, status := Read(&sealed, wideAxis, 4096); status != ReadMiss {
		t.Fatalf("uncovered key = %v, want miss", status)
	}
	if value, status := Read(&base, wideAxis, 7); value != 21 || status != ReadHit {
		t.Fatalf("base row after a derived removal = (%d, %v), want (21, hit)", value, status)
	}
	for key := 0; key < 64; key++ {
		if key == 7 {
			continue
		}
		if value, status := Read(&sealed, wideAxis, key); value != key*3 || status != ReadHit {
			t.Fatalf("row %d survived removal as (%d, %v)", key, value, status)
		}
	}

	const bound = 40
	wide := wideSnapshot(t, widerRows, identity.Generation(1))
	allocations, bytes := measure(t, func() {
		removal := NewDelta(wide, identity.Generation(2))
		if err := RemoveRow(&removal, wideAxis, 7); err != nil {
			t.Fatalf("remove row: %v", err)
		}
		published, err := removal.Seal()
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		sinkSnapshot = published
	})
	if allocations > bound {
		t.Fatalf("removal publication allocations = %v, want at most %d", allocations, bound)
	}
	if bytes > 4096 {
		t.Fatalf("removal publication = %d bytes, want a change-set sized publication", bytes)
	}
}

// TestDeltaRejectsUnpublishableDerivation fixes what a derived publication
// refuses. It must advance the store it derives from, because two snapshots
// of one store at one generation would publish two different contents under
// one store revision, and it derives nothing at all from an unpublished
// snapshot.
func TestDeltaRejectsUnpublishableDerivation(t *testing.T) {
	base := wideSnapshot(t, 8, identity.Generation(3))
	for name, generation := range map[string]identity.Generation{
		"same generation":    identity.Generation(3),
		"earlier generation": identity.Generation(2),
	} {
		t.Run(name, func(t *testing.T) {
			delta := NewDelta(base, generation)
			sealed, err := delta.Seal()
			if !errors.Is(err, ErrStaleGeneration) {
				t.Fatalf("seal error = %v, want %v", err, ErrStaleGeneration)
			}
			if sealed.Published() {
				t.Fatal("a rejected derivation returned a published snapshot")
			}
		})
	}
	t.Run("unpublished base", func(t *testing.T) {
		delta := NewDelta(Snapshot{}, identity.Generation(1))
		if _, err := delta.Seal(); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("seal error = %v, want %v", err, ErrUnavailableIdentity)
		}
	})
}

// TestDerivedPublicationReplacesInheritedColumns fixes the authority rule of
// a derived publication. An inherited column belongs to the publication it
// came from, so a publication may reseal that slot wholesale, and the
// denominator the replaced column read against stops being proved on it. A
// second write by one publication to one slot is still a second authority.
func TestDerivedPublicationReplacesInheritedColumns(t *testing.T) {
	base := wideSnapshot(t, 8, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := PutColumn(&delta, wideAxis, Content[int, int]{Rows: map[int]int{1: 42}}); err != nil {
		t.Fatalf("replace inherited column: %v", err)
	}
	if err := PutColumn(&delta, wideAxis, Content[int, int]{Rows: map[int]int{2: 43}}); !errors.Is(err, ErrSlotFilled) {
		t.Fatalf("second write error = %v, want %v", err, ErrSlotFilled)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal delta: %v", err)
	}
	if value, status := Read(&sealed, wideAxis, 1); value != 42 || status != ReadHit {
		t.Fatalf("replaced column row = (%d, %v), want (42, hit)", value, status)
	}
	if _, status := Read(&sealed, wideAxis, 3); status != ReadMiss {
		t.Fatalf("replaced column absence = %v, want miss", status)
	}
	if sealed.Denominators().Proves(wideDenominator, wideAxis.Slot) {
		t.Fatal("a replaced column still reports its old denominator")
	}
	if !sealed.Denominators().Proves(wideDenominator, mirrorAxis.Slot) {
		t.Fatal("replacing one column unpublished a denominator another column reads")
	}
	if value, status := Read(&base, wideAxis, 1); value != 3 || status != ReadHit {
		t.Fatalf("base row after replacement = (%d, %v), want (3, hit)", value, status)
	}

	last := NewDelta(sealed, identity.Generation(3))
	if err := PutColumn(&last, mirrorAxis, Content[int, int]{Rows: map[int]int{5: 1}}); err != nil {
		t.Fatalf("replace mirror column: %v", err)
	}
	published, err := last.Seal()
	if err != nil {
		t.Fatalf("seal last: %v", err)
	}
	if published.Denominators().Len() != 0 || published.Denominators().Published(wideDenominator) {
		t.Fatalf("denominators = %d, want a denominator no column reads to be unpublished", published.Denominators().Len())
	}
}

// TestRowEditsFailClosed fixes every rejection of a row edit. An edit
// validates exactly what a read validates -- the schema, the slot, and the
// column's key and value types -- so no edit can reach a column it does not
// name, and an edit that is rejected changes nothing.
func TestRowEditsFailClosed(t *testing.T) {
	base := wideSnapshot(t, 8, identity.Generation(1))
	cases := []struct {
		name string
		edit func(*Builder) error
		want error
	}{
		{
			name: "unknown slot",
			edit: func(b *Builder) error {
				return SetRow(b, Axis[int, int]{SchemaID: wideSchema, Slot: 9}, 1, 1)
			},
			want: ErrUnknownSlot,
		},
		{
			name: "another schema",
			edit: func(b *Builder) error {
				return SetRow(b, Axis[int, int]{SchemaID: fixtureSchema, Slot: 0}, 1, 1)
			},
			want: ErrSchemaMismatch,
		},
		{
			name: "another key type",
			edit: func(b *Builder) error {
				return SetRow(b, Axis[string, int]{SchemaID: wideSchema, Slot: 0}, "one", 1)
			},
			want: ErrColumnKind,
		},
		{
			name: "another value type",
			edit: func(b *Builder) error {
				return RemoveRow(b, Axis[int, record]{SchemaID: wideSchema, Slot: 0}, 1)
			},
			want: ErrColumnKind,
		},
		{
			name: "nil builder",
			edit: func(*Builder) error { return SetRow(nil, wideAxis, 1, 1) },
			want: ErrUnavailableIdentity,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			delta := NewDelta(base, identity.Generation(2))
			if err := testCase.edit(&delta); !errors.Is(err, testCase.want) {
				t.Fatalf("edit error = %v, want %v", err, testCase.want)
			}
			sealed, err := delta.Seal()
			if err != nil {
				t.Fatalf("seal after a rejected edit: %v", err)
			}
			for key := 0; key < 8; key++ {
				if value, status := Read(&sealed, wideAxis, key); value != key*3 || status != ReadHit {
					t.Fatalf("a rejected edit changed row %d to (%d, %v)", key, value, status)
				}
			}
		})
	}
}

// denominatorOf returns the sealed membership set the column at slot reads
// against, as the pointer identity that proves whether it was shared.
func denominatorOf(t *testing.T, s Snapshot, slot uint32) *denominator[int] {
	t.Helper()
	stored, recovered := s.columns[slot].(*column[int, int])
	if !recovered {
		t.Fatalf("column at slot %d is not the wide column", slot)
	}
	return stored.members
}

// BenchmarkPublication reports what one published change costs. A full reseal
// rebuilds every row and every denominator member; a derived publication is
// bounded by the change set.
func BenchmarkPublication(b *testing.B) {
	content := wideContent(wideRows)
	b.Run("reseal", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			builder := NewBuilder(wideSchema, wideStore, identity.Generation(2))
			if err := PutColumn(&builder, wideAxis, content); err != nil {
				b.Fatalf("put wide column: %v", err)
			}
			sealed, err := builder.Seal()
			if err != nil {
				b.Fatalf("seal: %v", err)
			}
			sinkSnapshot = sealed
		}
	})
	base := wideSnapshot(b, wideRows, identity.Generation(1))
	b.Run("delta set", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			delta := NewDelta(base, identity.Generation(2))
			if err := SetRow(&delta, wideAxis, 7, 700); err != nil {
				b.Fatalf("set row: %v", err)
			}
			sealed, err := delta.Seal()
			if err != nil {
				b.Fatalf("seal: %v", err)
			}
			sinkSnapshot = sealed
		}
	})
	b.Run("delta remove", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			delta := NewDelta(base, identity.Generation(2))
			if err := RemoveRow(&delta, wideAxis, 7); err != nil {
				b.Fatalf("remove row: %v", err)
			}
			sealed, err := delta.Seal()
			if err != nil {
				b.Fatalf("seal: %v", err)
			}
			sinkSnapshot = sealed
		}
	})
}
