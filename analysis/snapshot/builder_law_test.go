package snapshot

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The construction laws state what a builder refuses and what a refusal costs.
// Every rejection arm is a fail-closed proof: the rule it guards is stated as
// the observable the publication must not carry afterwards, so a rejection
// that silently half-published something fails the law rather than the error
// comparison.
//
// # The arms no law drives
//
// Five statements of builder.go are unreachable through the exported surface,
// and each one is unreachable because another law here holds:
//
//   - DeclareQuery's rejection of the directory entry it writes itself. Publish
//     rejects an unavailable identity, an unfilled slot, and a second slot.
//     DeclareQuery rejects the unavailable identity before it starts, fills the
//     slot itself immediately before, and rejects a family already addressed
//     elsewhere against the same directory, which PutColumn does not write. The
//     remaining case is the identity restated at its own slot, which Publish
//     accepts and TestPublishRestatesOneDirectoryEntry fixes.
//
//   - DeclareQuery's rejection of the registration it writes itself.
//     RegisterQuery rejects only an unavailable identity, which DeclareQuery
//     rejected before it sealed anything.
//
//   - detach's refusal of a slot holding something that cannot answer what
//     denominator it reads against. Every value a slot holds is a sealed
//     column, whether this builder wrote it or inherited it, and every sealed
//     column answers that question.
//
//   - detach's refusal of a denominator identity the publication does not
//     publish, and withoutSlot's return of a slot list that does not hold the
//     slot. Both require a column that names a denominator the index does not
//     list at its slot. TestEveryProvenColumnIsPublishedByItsDenominator fixes
//     that the index is exact through sealing, sharing, restating, replacing
//     and unpublishing, and a builder starts either empty or from a sealed
//     publication, so no state a caller can reach breaks it.

var (
	armSchema      = identity.ContentID{0x1A, 0x2B}
	armStore       = identity.StoreID(23)
	armDenominator = identity.ContentID{0x3C, 0x4D}
	armFamily      = identity.ContentID{0x5E, 0x6F}
	armColumnID    = identity.ContentID{0x7A, 0x8B}

	// provenAxis and mirrorProvenAxis are total over one denominator,
	// unprovenAxis publishes none, so a publication can replace either kind.
	provenAxis       = Axis[int, int]{SchemaID: armSchema, Slot: 0}
	mirrorProvenAxis = Axis[int, int]{SchemaID: armSchema, Slot: 1}
	unprovenAxis     = Axis[int, int]{SchemaID: armSchema, Slot: 2}
)

// armSnapshot publishes the construction fixture: two columns total over one
// denominator and a third that publishes none.
func armSnapshot(t *testing.T, generation identity.Generation) Snapshot {
	t.Helper()
	builder := NewBuilder(armSchema, armStore, generation)
	put(t, &builder, provenAxis, Content[int, int]{
		Rows:        map[int]int{1: 10},
		Denominator: armDenominator,
		Members:     []int{1, 2},
	})
	put(t, &builder, mirrorProvenAxis, Content[int, int]{
		Rows:        map[int]int{1: 20},
		Denominator: armDenominator,
	})
	put(t, &builder, unprovenAxis, Content[int, int]{Rows: map[int]int{1: 30}})
	if err := builder.Publish(armColumnID, provenAxis.Slot); err != nil {
		t.Fatalf("publish proven column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal construction fixture: %v", err)
	}
	return sealed
}

// TestDeclareQueryPublishesNothingWhenItRejects is the query declaration law.
// Declaring a family writes three facts at once -- the result column, its
// directory entry, and its registration -- so a rejection must leave all three
// unwritten: a family that is addressed but not answerable, or answerable
// against a column that was never sealed, is a query a consumer could open and
// read as a published answer.
func TestDeclareQueryPublishesNothingWhenItRejects(t *testing.T) {
	cases := []struct {
		name    string
		declare func(*Builder) error
		want    error
		// extraSlots is the dense range the case fills before it declares,
		// because a rejection leaves what was already written in place.
		extraSlots int
	}{
		{
			name: "result column of a slot this publication already wrote",
			declare: func(b *Builder) error {
				if err := PutColumn(b, Axis[int, int]{SchemaID: armSchema, Slot: 3}, Content[int, int]{
					Rows: map[int]int{1: 1},
				}); err != nil {
					return err
				}
				_, err := DeclareQuery(b, armFamily, 3, Content[int, int]{Rows: map[int]int{2: 2}})
				return err
			},
			want:       ErrSlotFilled,
			extraSlots: 1,
		},
		{
			name: "result column keyed by a shape that cannot be hashed",
			declare: func(b *Builder) error {
				_, err := DeclareQuery(b, armFamily, 3, Content[any, int]{Rows: map[any]int{"key": 1}})
				return err
			},
			want: ErrUnhashableKey,
		},
		{
			name: "result column with members and no denominator",
			declare: func(b *Builder) error {
				_, err := DeclareQuery(b, armFamily, 3, Content[int, int]{Members: []int{1}})
				return err
			},
			want: ErrUnprovenMembers,
		},
		{
			name: "result column claiming a denominator of another key type",
			declare: func(b *Builder) error {
				_, err := DeclareQuery(b, armFamily, 3, Content[string, int]{Denominator: armDenominator})
				return err
			},
			want: ErrColumnKind,
		},
		{
			name: "family already addressed to another column",
			declare: func(b *Builder) error {
				if err := b.Publish(armFamily, mirrorProvenAxis.Slot); err != nil {
					return err
				}
				_, err := DeclareQuery(b, armFamily, 3, Content[int, int]{Rows: map[int]int{1: 1}})
				return err
			},
			want: ErrDuplicatePublication,
		},
		{
			name: "family with no identity",
			declare: func(b *Builder) error {
				_, err := DeclareQuery(b, identity.ContentID{}, 3, Content[int, int]{})
				return err
			},
			want: ErrUnavailableIdentity,
		},
		{
			name: "no builder",
			declare: func(*Builder) error {
				_, err := DeclareQuery(nil, armFamily, 3, Content[int, int]{})
				return err
			},
			want: ErrUnavailableIdentity,
		},
	}
	base := armSnapshot(t, identity.Generation(1))
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			delta := NewDelta(base, identity.Generation(2))
			if err := testCase.declare(&delta); !errors.Is(err, testCase.want) {
				t.Fatalf("declaration error = %v, want %v", err, testCase.want)
			}
			sealed, err := delta.Seal()
			if err != nil {
				t.Fatalf("seal after a rejected declaration: %v", err)
			}
			if sealed.Queries().Published(armFamily) {
				t.Fatal("a rejected declaration registered its family as answerable")
			}
			if sealed.Queries().Len() != 0 {
				t.Fatalf("queries = %d, want a rejected declaration to publish none", sealed.Queries().Len())
			}
			if _, opened := OpenQuery[int, int](&sealed, armFamily); opened {
				t.Fatal("a rejected declaration opens as a published answer")
			}
			if sealed.Columns() != base.Columns()+testCase.extraSlots {
				t.Fatalf("columns = %d, want %d", sealed.Columns(), base.Columns()+testCase.extraSlots)
			}
			if value, status := Read(&sealed, provenAxis, 1); value != 10 || status != ReadHit {
				t.Fatalf("inherited row after a rejected declaration = (%d, %v), want (10, hit)", value, status)
			}
		})
	}
}

// TestDeclareQueryPublishesAllThreeFacts is the other half of the declaration
// law: an accepted declaration addresses the family, registers it, and seals
// the result column, and the plan it returns opens against the snapshot.
func TestDeclareQueryPublishesAllThreeFacts(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	plan, err := DeclareQuery(&delta, armFamily, 3, Content[int, string]{
		Rows:        map[int]string{7: "answer"},
		Denominator: identity.ContentID{0x9C},
		Members:     []int{7, 8},
	})
	if err != nil {
		t.Fatalf("declare query: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !plan.Available() || plan.Axis().Slot != 3 {
		t.Fatalf("plan = %+v, want the result column at slot 3", plan)
	}
	if !sealed.Queries().Published(armFamily) {
		t.Fatal("a declared family is not registered")
	}
	opened, ok := OpenQuery[int, string](&sealed, armFamily)
	if !ok || opened != plan {
		t.Fatalf("opened plan = (%+v, %t), want %+v", opened, ok, plan)
	}
	if answer, status := Query(&sealed, plan, 7); answer != "answer" || status != ReadHit {
		t.Fatalf("published answer = (%q, %v), want (answer, hit)", answer, status)
	}
	if _, status := Query(&sealed, plan, 8); status != ReadProvenAbsent {
		t.Fatalf("materialized absence = %v, want proven-absent", status)
	}
}

// TestPublishRestatesOneDirectoryEntry fixes what publishing an identity twice
// means. An identity addresses at most one slot, so restating the slot it
// already addresses is the same fact and is accepted -- which is what a
// derived publication that reseals an addressed column does -- while naming a
// second slot is a second authority and is rejected.
func TestPublishRestatesOneDirectoryEntry(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := delta.Publish(armColumnID, provenAxis.Slot); err != nil {
		t.Fatalf("restating a directory entry: %v", err)
	}
	if err := PutColumn(&delta, provenAxis, Content[int, int]{Rows: map[int]int{5: 50}}); err != nil {
		t.Fatalf("reseal addressed column: %v", err)
	}
	if err := delta.Publish(armColumnID, provenAxis.Slot); err != nil {
		t.Fatalf("restating the directory entry of a resealed column: %v", err)
	}
	if err := delta.Publish(armColumnID, mirrorProvenAxis.Slot); !errors.Is(err, ErrDuplicatePublication) {
		t.Fatalf("second directory slot error = %v, want %v", err, ErrDuplicatePublication)
	}
	if _, err := delta.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}
}

// TestReplacingAnUnprovenColumnDetachesNothing fixes what replacing a column
// that publishes no denominator costs. The replaced column reads against no
// membership set, so the publication's denominators are exactly what it
// inherited and no other column loses its proof.
func TestReplacingAnUnprovenColumnDetachesNothing(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := PutColumn(&delta, unprovenAxis, Content[int, int]{Rows: map[int]int{9: 90}}); err != nil {
		t.Fatalf("replace unproven column: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed.Denominators().Len() != 1 || !sealed.Denominators().Published(armDenominator) {
		t.Fatalf("denominators = %d, want the one the publication inherited", sealed.Denominators().Len())
	}
	for _, slot := range []uint32{provenAxis.Slot, mirrorProvenAxis.Slot} {
		if !sealed.Denominators().Proves(armDenominator, slot) {
			t.Fatalf("replacing an unproven column detached slot %d from its denominator", slot)
		}
	}
	if sealed.Denominators().Proves(armDenominator, unprovenAxis.Slot) {
		t.Fatal("a column that never declared a denominator is proved by one")
	}
	if value, status := Read(&sealed, unprovenAxis, 9); value != 90 || status != ReadHit {
		t.Fatalf("replacement row = (%d, %v), want (90, hit)", value, status)
	}
	if _, status := Read(&sealed, unprovenAxis, 1); status != ReadMiss {
		t.Fatalf("replaced row = %v, want miss", status)
	}
	if value, status := Read(&base, unprovenAxis, 1); value != 30 || status != ReadHit {
		t.Fatalf("base row after replacement = (%d, %v), want (30, hit)", value, status)
	}
}

// TestRestatedDenominatorSlotIsHeldOnce fixes that a column resealed against
// the denominator it already read is proved once rather than twice. The slot
// list is a set, so restating it keeps the column proved and leaves every
// other column's proof untouched.
func TestRestatedDenominatorSlotIsHeldOnce(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	delta := NewDelta(base, identity.Generation(2))
	if err := PutColumn(&delta, provenAxis, Content[int, int]{
		Rows:        map[int]int{1: 11},
		Denominator: armDenominator,
	}); err != nil {
		t.Fatalf("reseal against the same denominator: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed.Denominators().Len() != 1 {
		t.Fatalf("denominators = %d, want 1", sealed.Denominators().Len())
	}
	for _, slot := range []uint32{provenAxis.Slot, mirrorProvenAxis.Slot} {
		if !sealed.Denominators().Proves(armDenominator, slot) {
			t.Fatalf("slot %d lost its denominator when the column was resealed", slot)
		}
	}
	if denominatorOf(t, sealed, provenAxis.Slot) != denominatorOf(t, sealed, mirrorProvenAxis.Slot) {
		t.Fatal("reseal against a published denominator sealed a second membership set")
	}
	if denominatorOf(t, sealed, provenAxis.Slot) != denominatorOf(t, base, provenAxis.Slot) {
		t.Fatal("reseal replaced the membership set the base publishes")
	}
	if _, status := Read(&sealed, provenAxis, 2); status != ReadProvenAbsent {
		t.Fatalf("resealed column absence = %v, want proven-absent", status)
	}
}

// TestWithdrawnProofDoesNotReachThePublicationItDerivesFrom is the path-copy
// law of the denominator publication, stated from the outside. A derived
// publication that withdraws one column's proof publishes its own slot list,
// so the publication it derives from keeps proving every column it proved, and
// a second publication derived from the same base is unaffected by the first.
func TestWithdrawnProofDoesNotReachThePublicationItDerivesFrom(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	first := NewDelta(base, identity.Generation(2))
	if err := PutColumn(&first, provenAxis, Content[int, int]{Rows: map[int]int{1: 11}}); err != nil {
		t.Fatalf("replace proven column: %v", err)
	}
	withdrawn, err := first.Seal()
	if err != nil {
		t.Fatalf("seal first derivation: %v", err)
	}

	if withdrawn.Denominators().Proves(armDenominator, provenAxis.Slot) {
		t.Fatal("a replaced column keeps the proof it no longer reads against")
	}
	if !withdrawn.Denominators().Proves(armDenominator, mirrorProvenAxis.Slot) {
		t.Fatal("withdrawing one proof withdrew another column's")
	}
	for _, slot := range []uint32{provenAxis.Slot, mirrorProvenAxis.Slot} {
		if !base.Denominators().Proves(armDenominator, slot) {
			t.Fatalf("a derived withdrawal reached the base's proof of slot %d", slot)
		}
	}
	if _, status := Read(&base, provenAxis, 2); status != ReadProvenAbsent {
		t.Fatalf("base absence after a derived withdrawal = %v, want proven-absent", status)
	}

	second := NewDelta(base, identity.Generation(3))
	if err := SetRow(&second, provenAxis, 3, 33); err != nil {
		t.Fatalf("edit second derivation: %v", err)
	}
	sibling, err := second.Seal()
	if err != nil {
		t.Fatalf("seal second derivation: %v", err)
	}
	for _, slot := range []uint32{provenAxis.Slot, mirrorProvenAxis.Slot} {
		if !sibling.Denominators().Proves(armDenominator, slot) {
			t.Fatalf("one derivation's withdrawal reached a sibling's proof of slot %d", slot)
		}
	}

	last := NewDelta(withdrawn, identity.Generation(4))
	if err := PutColumn(&last, mirrorProvenAxis, Content[int, int]{Rows: map[int]int{1: 21}}); err != nil {
		t.Fatalf("replace mirror column: %v", err)
	}
	unproven, err := last.Seal()
	if err != nil {
		t.Fatalf("seal last derivation: %v", err)
	}
	if unproven.Denominators().Len() != 0 || unproven.Denominators().Published(armDenominator) {
		t.Fatalf("denominators = %d, want a denominator no column reads to be unpublished",
			unproven.Denominators().Len())
	}
	if !base.Denominators().Published(armDenominator) || base.Denominators().Len() != 1 {
		t.Fatal("unpublishing a denominator in a derivation reached the base")
	}
}

// TestEveryProvenColumnIsPublishedByItsDenominator is the structural invariant
// the denominator bookkeeping maintains, and it is what makes the defensive
// arms of detach unreachable: a column that names a denominator is always
// published under that identity and always listed at its own slot, and a
// published identity lists exactly the slots whose columns name it. It is
// stated over a chain of publications, because every arm that could break it
// -- sealing, sharing, restating, replacing, and unpublishing -- runs there.
func TestEveryProvenColumnIsPublishedByItsDenominator(t *testing.T) {
	second := identity.ContentID{0xAC, 0xDC}
	base := armSnapshot(t, identity.Generation(1))
	assertDenominatorIndexIsExact(t, "base", base, armDenominator, second)

	share := NewDelta(base, identity.Generation(2))
	if err := PutColumn(&share, unprovenAxis, Content[int, int]{
		Rows:        map[int]int{1: 31},
		Denominator: armDenominator,
	}); err != nil {
		t.Fatalf("share the denominator with a third column: %v", err)
	}
	shared, err := share.Seal()
	if err != nil {
		t.Fatalf("seal shared: %v", err)
	}
	assertDenominatorIndexIsExact(t, "shared", shared, armDenominator, second)

	move := NewDelta(shared, identity.Generation(3))
	if err := PutColumn(&move, provenAxis, Content[int, int]{
		Rows:        map[int]int{1: 12},
		Denominator: second,
		Members:     []int{1, 4},
	}); err != nil {
		t.Fatalf("move a column to a second denominator: %v", err)
	}
	moved, err := move.Seal()
	if err != nil {
		t.Fatalf("seal moved: %v", err)
	}
	assertDenominatorIndexIsExact(t, "moved", moved, armDenominator, second)
	if !moved.Denominators().Proves(second, provenAxis.Slot) || moved.Denominators().Len() != 2 {
		t.Fatalf("moved publication proves = %d denominators", moved.Denominators().Len())
	}
	if _, status := Read(&moved, provenAxis, 4); status != ReadProvenAbsent {
		t.Fatalf("moved column absence = %v, want proven-absent", status)
	}

	strip := NewDelta(moved, identity.Generation(4))
	for _, axis := range []Axis[int, int]{provenAxis, mirrorProvenAxis, unprovenAxis} {
		if err := PutColumn(&strip, axis, Content[int, int]{Rows: map[int]int{1: 1}}); err != nil {
			t.Fatalf("strip slot %d: %v", axis.Slot, err)
		}
	}
	stripped, err := strip.Seal()
	if err != nil {
		t.Fatalf("seal stripped: %v", err)
	}
	assertDenominatorIndexIsExact(t, "stripped", stripped, armDenominator, second)
	if stripped.Denominators().Len() != 0 {
		t.Fatalf("denominators = %d, want none once no column reads one", stripped.Denominators().Len())
	}
}

// TestWithdrawingARowFromAColumnWithoutRows fixes the empty-column edit. A
// column sealed with no rows holds no storage at all, so withdrawing a row it
// never held is a publication that changes nothing and still seals.
func TestWithdrawingARowFromAColumnWithoutRows(t *testing.T) {
	builder := NewBuilder(armSchema, armStore, identity.Generation(1))
	put(t, &builder, provenAxis, Content[int, int]{
		Denominator: armDenominator,
		Members:     []int{1},
	})
	base, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal empty column: %v", err)
	}
	delta := NewDelta(base, identity.Generation(2))
	if err := RemoveRow(&delta, provenAxis, 1); err != nil {
		t.Fatalf("withdraw from an empty column: %v", err)
	}
	if err := RemoveRow(&delta, provenAxis, 99); err != nil {
		t.Fatalf("withdraw an uncovered key from an empty column: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for _, s := range []Snapshot{base, sealed} {
		held := s
		if _, status := Read(&held, provenAxis, 1); status != ReadProvenAbsent {
			t.Fatalf("member of an empty column = %v, want proven-absent", status)
		}
		if _, status := Read(&held, provenAxis, 99); status != ReadMiss {
			t.Fatalf("uncovered key of an empty column = %v, want miss", status)
		}
	}
}

// assertDenominatorIndexIsExact checks both directions of the denominator
// index against the columns themselves: every column that names a denominator
// is published and listed at its slot, and every listed slot holds a column
// that names that denominator.
func assertDenominatorIndexIsExact(t *testing.T, stage string, s Snapshot, candidates ...identity.ContentID) {
	t.Helper()
	for slot := 0; slot < s.Columns(); slot++ {
		held, known := s.columns[slot].(provenColumn)
		if !known {
			t.Fatalf("%s: slot %d does not hold a column", stage, slot)
		}
		id := held.denominatorID()
		if !id.Available() {
			continue
		}
		if !s.Denominators().Published(id) {
			t.Fatalf("%s: slot %d reads against unpublished denominator %s", stage, slot, id)
		}
		if !s.Denominators().Proves(id, uint32(slot)) {
			t.Fatalf("%s: denominator %s does not list slot %d, which reads against it", stage, id, slot)
		}
	}
	for _, id := range candidates {
		for slot := 0; slot < s.Columns(); slot++ {
			if !s.Denominators().Proves(id, uint32(slot)) {
				continue
			}
			held, known := s.columns[slot].(provenColumn)
			if !known || held.denominatorID() != id {
				t.Fatalf("%s: denominator %s lists slot %d, which does not read against it", stage, id, slot)
			}
		}
	}
}
