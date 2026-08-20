package snapshot

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The addressing laws state what an axis is and what a slot answers. An axis
// is a claim -- a schema, a slot, and the key and value types the caller
// believes the column holds -- so the laws are about what the claim admits and
// what it must refuse, and about the column storage answering exactly the slot
// that was addressed and nothing beside it.

var (
	denseSchema = identity.ContentID{0xC0, 0x1A}
	denseStore  = identity.StoreID(31)
)

// rowName and rowCount have the memory layout of the fixture column's key and
// value types and are different types, so a claim that recovered a column by
// layout would answer them.
type (
	rowName  string
	rowCount int
)

// TestAxisCarriesNoStorage is the structural half of the axis contract. An
// axis is an address: it holds a schema identity and a slot and nothing that
// can reference published storage, so carrying one costs a copy and holding
// one keeps no snapshot alive. The same is required of a query plan, which is
// an address in exactly the same sense.
func TestAxisCarriesNoStorage(t *testing.T) {
	type anchor struct {
		SchemaID identity.ContentID
		Slot     uint32
	}
	for _, addressType := range []reflect.Type{
		reflect.TypeOf(Axis[string, record]{}),
		reflect.TypeOf(QueryPlan[string, record]{}),
	} {
		assertHoldsNoReference(t, addressType, addressType.String())
	}
	if size := unsafe.Sizeof(Axis[string, record]{}); size != unsafe.Sizeof(anchor{}) {
		t.Fatalf("axis = %d bytes, want the %d of a schema identity and a slot", size, unsafe.Sizeof(anchor{}))
	}
	axisType := reflect.TypeOf(Axis[string, record]{})
	if axisType.NumField() != 2 || axisType.Field(0).Name != "SchemaID" || axisType.Field(1).Name != "Slot" {
		t.Fatalf("axis fields = %+v, want a schema identity and a slot", axisType)
	}
}

// TestAxisClaimIsTypeIdentityNotLayout fixes what the column kind check
// compares. The claim a read carries is the key and value types themselves,
// not their memory layout: a named type over the very same representation is
// another type, so it names another column and fails closed. An edit compares
// the same way, so a rejected claim can neither read nor write.
func TestAxisClaimIsTypeIdentityNotLayout(t *testing.T) {
	axis := Axis[string, int]{SchemaID: denseSchema, Slot: 0}
	builder := NewBuilder(denseSchema, denseStore, identity.Generation(1))
	put(t, &builder, axis, Content[string, int]{
		Rows:        map[string]int{"present": 11},
		Denominator: identity.ContentID{0xD1},
		Members:     []string{"present", "absent"},
	})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	t.Run("renamed key type", func(t *testing.T) {
		value, status := Read(&sealed, Axis[rowName, int]{SchemaID: denseSchema, Slot: 0}, "present")
		assertInvalid(t, value, status)
	})
	t.Run("renamed value type", func(t *testing.T) {
		value, status := Read(&sealed, Axis[string, rowCount]{SchemaID: denseSchema, Slot: 0}, "present")
		assertInvalid(t, value, status)
	})
	t.Run("both renamed", func(t *testing.T) {
		value, status := Read(&sealed, Axis[rowName, rowCount]{SchemaID: denseSchema, Slot: 0}, "present")
		assertInvalid(t, value, status)
	})
	t.Run("renamed claim cannot prove absence", func(t *testing.T) {
		value, status := Read(&sealed, Axis[rowName, int]{SchemaID: denseSchema, Slot: 0}, "absent")
		assertInvalid(t, value, status)
	})
	t.Run("renamed claim cannot edit", func(t *testing.T) {
		delta := NewDelta(sealed, identity.Generation(2))
		if err := SetRow(&delta, Axis[rowName, int]{SchemaID: denseSchema, Slot: 0}, "present", 99); !errors.Is(err, ErrColumnKind) {
			t.Fatalf("edit error = %v, want %v", err, ErrColumnKind)
		}
		if err := RemoveRow(&delta, Axis[string, rowCount]{SchemaID: denseSchema, Slot: 0}, "present"); !errors.Is(err, ErrColumnKind) {
			t.Fatalf("edit error = %v, want %v", err, ErrColumnKind)
		}
		published, err := delta.Seal()
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		if value, status := Read(&published, axis, "present"); value != 11 || status != ReadHit {
			t.Fatalf("row after two rejected edits = (%d, %v), want (11, hit)", value, status)
		}
	})
	t.Run("the claim the column was built for still answers", func(t *testing.T) {
		if value, status := Read(&sealed, axis, "present"); value != 11 || status != ReadHit {
			t.Fatalf("read = (%d, %v), want (11, hit)", value, status)
		}
	})
}

// TestEachSlotAnswersItsOwnColumn is the slot addressing law. Slots are dense
// and a column is addressed by its slot alone, so columns that share a key and
// value type are told apart by nothing but the slot: every axis answers its
// own column's rows, its own column's denominator, and no other's, and a slot
// beyond the dense range answers nothing at all.
func TestEachSlotAnswersItsOwnColumn(t *testing.T) {
	const columns = 4
	builder := NewBuilder(denseSchema, denseStore, identity.Generation(1))
	for slot := uint32(0); slot < columns; slot++ {
		put(t, &builder, Axis[int, int]{SchemaID: denseSchema, Slot: slot}, Content[int, int]{
			Rows:        map[int]int{int(slot): int(slot) * 100},
			Denominator: identity.ContentID{byte(slot + 1)},
			Members:     []int{int(slot), int(slot) + 10},
		})
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed.Columns() != columns {
		t.Fatalf("columns = %d, want %d", sealed.Columns(), columns)
	}
	for slot := uint32(0); slot < columns; slot++ {
		axis := Axis[int, int]{SchemaID: denseSchema, Slot: slot}
		for key := 0; key < columns; key++ {
			value, status := Read(&sealed, axis, key)
			switch {
			case key == int(slot):
				if value != key*100 || status != ReadHit {
					t.Fatalf("slot %d key %d = (%d, %v), want (%d, hit)", slot, key, value, status, key*100)
				}
			default:
				if value != 0 || status != ReadMiss {
					t.Fatalf("slot %d key %d = (%d, %v), want (0, miss)", slot, key, value, status)
				}
			}
		}
		if _, status := Read(&sealed, axis, int(slot)+10); status != ReadProvenAbsent {
			t.Fatalf("slot %d does not prove absence over its own denominator: %v", slot, status)
		}
		for other := uint32(0); other < columns; other++ {
			if other == slot {
				continue
			}
			if _, status := Read(&sealed, axis, int(other)+10); status != ReadMiss {
				t.Fatalf("slot %d proves absence over slot %d's denominator: %v", slot, other, status)
			}
		}
	}
	beyond := Axis[int, int]{SchemaID: denseSchema, Slot: columns}
	value, status := Read(&sealed, beyond, 0)
	assertInvalid(t, value, status)
}

// TestColumnAnswersRowsBeforeAbsence fixes the precedence a column read
// carries. One hash answers both questions a column holds, and the row wins: a
// key that is both a stored row and a member of the column's denominator is a
// hit, never a proven absence, because the denominator states what the column
// is total over and not what it withholds.
func TestColumnAnswersRowsBeforeAbsence(t *testing.T) {
	axis := Axis[int, int]{SchemaID: denseSchema, Slot: 0}
	builder := NewBuilder(denseSchema, denseStore, identity.Generation(1))
	put(t, &builder, axis, Content[int, int]{
		Rows:        map[int]int{1: 10, 2: 20},
		Denominator: identity.ContentID{0xE1},
		Members:     []int{1, 2, 3},
	})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for key, want := range map[int]int{1: 10, 2: 20} {
		if value, status := Read(&sealed, axis, key); value != want || status != ReadHit {
			t.Fatalf("member with a row %d = (%d, %v), want (%d, hit)", key, value, status, want)
		}
	}
	if _, status := Read(&sealed, axis, 3); status != ReadProvenAbsent {
		t.Fatalf("member without a row = %v, want proven-absent", status)
	}
	if _, status := Read(&sealed, axis, 4); status != ReadMiss {
		t.Fatalf("key the denominator does not cover = %v, want miss", status)
	}
}

// TestDenominatorWithoutMembersProvesNothing fixes the empty membership set. A
// column may declare the identity of a key universe whose membership is empty:
// the identity is published and proves the column, and the column reports a
// miss for every key, because absence is proven by membership rather than by
// the declaration.
func TestDenominatorWithoutMembersProvesNothing(t *testing.T) {
	empty := identity.ContentID{0xF1}
	axis := Axis[int, int]{SchemaID: denseSchema, Slot: 0}
	builder := NewBuilder(denseSchema, denseStore, identity.Generation(1))
	put(t, &builder, axis, Content[int, int]{Rows: map[int]int{1: 10}, Denominator: empty})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !sealed.Denominators().Published(empty) || !sealed.Denominators().Proves(empty, axis.Slot) {
		t.Fatal("an empty denominator is not published against the column that declared it")
	}
	if sealed.Denominators().Len() != 1 {
		t.Fatalf("denominators = %d, want 1", sealed.Denominators().Len())
	}
	if value, status := Read(&sealed, axis, 1); value != 10 || status != ReadHit {
		t.Fatalf("stored row = (%d, %v), want (10, hit)", value, status)
	}
	for _, key := range []int{0, 2, 99} {
		if _, status := Read(&sealed, axis, key); status != ReadMiss {
			t.Fatalf("key %d against an empty membership = %v, want miss", key, status)
		}
	}
}

// TestSealedViewsAreValuesFixedAtPublication is the sealed-view law. A
// denominator publication and a query publication are values read out of a
// snapshot, so one held across later publications keeps answering what it was
// published with, and a copy of one answers exactly what it was copied from.
func TestSealedViewsAreValuesFixedAtPublication(t *testing.T) {
	base := armSnapshot(t, identity.Generation(1))
	denominators, queries := base.Denominators(), base.Queries()

	delta := NewDelta(base, identity.Generation(2))
	if err := delta.RegisterQuery(identity.ContentID{0x0C}); err != nil {
		t.Fatalf("register query: %v", err)
	}
	if err := PutColumn(&delta, unprovenAxis, Content[int, int]{
		Denominator: identity.ContentID{0x0D},
		Members:     []int{1},
	}); err != nil {
		t.Fatalf("publish a second denominator: %v", err)
	}
	sealed, err := delta.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if denominators.Len() != 1 || !denominators.Published(armDenominator) {
		t.Fatalf("held denominator view = %d entries, want the 1 it was published with", denominators.Len())
	}
	if denominators.Published(identity.ContentID{0x0D}) {
		t.Fatal("a later publication reached a held denominator view")
	}
	if queries.Len() != 0 || queries.Published(identity.ContentID{0x0C}) {
		t.Fatal("a later registration reached a held query view")
	}
	if sealed.Denominators().Len() != 2 || sealed.Queries().Len() != 1 {
		t.Fatalf("derived publication = %d denominators, %d queries",
			sealed.Denominators().Len(), sealed.Queries().Len())
	}

	copiedDenominators, copiedQueries := sealed.Denominators(), sealed.Queries()
	if copiedDenominators.Len() != sealed.Denominators().Len() ||
		!copiedDenominators.Proves(armDenominator, provenAxis.Slot) ||
		!copiedQueries.Published(identity.ContentID{0x0C}) {
		t.Fatal("a copied sealed view answers less than the view it was copied from")
	}
}

// TestConstructionInputIsConsumedByCopy fixes that the containers a writer
// hands a column stop being storage the moment the column is filled. One rows
// map and one member slice fill two columns with different contents, and each
// column answers what it was handed rather than what the writer's containers
// hold afterwards.
func TestConstructionInputIsConsumedByCopy(t *testing.T) {
	first := Axis[string, int]{SchemaID: denseSchema, Slot: 0}
	second := Axis[string, int]{SchemaID: denseSchema, Slot: 1}
	firstDenominator := identity.ContentID{0xA1}
	secondDenominator := identity.ContentID{0xA2}

	rows := map[string]int{"first": 1}
	members := []string{"first", "shared"}
	builder := NewBuilder(denseSchema, denseStore, identity.Generation(1))
	put(t, &builder, first, Content[string, int]{
		Rows:        rows,
		Denominator: firstDenominator,
		Members:     members,
	})

	delete(rows, "first")
	rows["second"] = 2
	members[0] = "second"
	put(t, &builder, second, Content[string, int]{
		Rows:        rows,
		Denominator: secondDenominator,
		Members:     members,
	})

	rows["late"] = 3
	members[1] = "late"
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	rows["later"] = 4
	members[0] = "later"

	for _, expectation := range []struct {
		axis   Axis[string, int]
		key    string
		value  int
		status ReadStatus
	}{
		{axis: first, key: "first", value: 1, status: ReadHit},
		{axis: first, key: "second", status: ReadMiss},
		{axis: first, key: "shared", status: ReadProvenAbsent},
		{axis: first, key: "late", status: ReadMiss},
		{axis: second, key: "second", value: 2, status: ReadHit},
		{axis: second, key: "first", status: ReadMiss},
		{axis: second, key: "shared", status: ReadProvenAbsent},
		{axis: second, key: "late", status: ReadMiss},
		{axis: second, key: "later", status: ReadMiss},
	} {
		value, status := Read(&sealed, expectation.axis, expectation.key)
		if value != expectation.value || status != expectation.status {
			t.Fatalf("slot %d key %q = (%d, %v), want (%d, %v)",
				expectation.axis.Slot, expectation.key, value, status, expectation.value, expectation.status)
		}
	}
}

// assertHoldsNoReference fails unless every field of held, at every depth, is a
// scalar an address may carry rather than a reference to storage.
func assertHoldsNoReference(t *testing.T, held reflect.Type, path string) {
	t.Helper()
	switch held.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
	case reflect.Array:
		assertHoldsNoReference(t, held.Elem(), path+"[]")
	case reflect.Struct:
		for index := 0; index < held.NumField(); index++ {
			field := held.Field(index)
			assertHoldsNoReference(t, field.Type, path+"."+field.Name)
		}
	default:
		t.Fatalf("%s is a %s, so an address can reference published storage", path, held.Kind())
	}
}
