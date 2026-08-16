package snapshot

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestReadOutcomes fixes the three read outcomes and their difference. A
// stored row is a hit. A key the column's sealed denominator covers is proven
// absent, which is a published fact. A key that no denominator covers is a
// miss, which is only ignorance, and a column without a denominator can never
// report anything but a miss for an unstored key.
func TestReadOutcomes(t *testing.T) {
	sealed := newFixture(t)
	cases := []struct {
		name  string
		axis  Axis[string, int]
		key   string
		value int
		want  ReadStatus
	}{
		{name: "stored row hits", axis: totalAxis, key: "present", value: 11, want: ReadHit},
		{name: "denominator proves absence", axis: totalAxis, key: "absent", want: ReadProvenAbsent},
		{name: "uncovered key misses", axis: totalAxis, key: "unknown", want: ReadMiss},
		{name: "column without denominator hits", axis: partialAxis, key: "present", value: 22, want: ReadHit},
		{name: "column without denominator only misses", axis: partialAxis, key: "absent", want: ReadMiss},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, status := Read(&sealed, testCase.axis, testCase.key)
			if status != testCase.want {
				t.Fatalf("status = %v, want %v", status, testCase.want)
			}
			if value != testCase.value {
				t.Fatalf("value = %d, want %d", value, testCase.value)
			}
			if status.Outcome() != (status != ReadInvalid) {
				t.Fatalf("Outcome disagrees with status %v", status)
			}
		})
	}
}

// TestReadFailsClosed fixes every rejection. A read that names another
// schema, an out-of-bounds slot, or a column of another key or value type
// produces no answer at all: it returns ReadInvalid, never a miss a causal
// reader could record as a dependency on an absent row.
func TestReadFailsClosed(t *testing.T) {
	sealed := newFixture(t)
	zero := Snapshot{}
	t.Run("schema mismatch", func(t *testing.T) {
		mismatched := Axis[string, int]{SchemaID: fixtureOtherSchema, Slot: totalAxis.Slot}
		value, status := Read(&sealed, mismatched, "present")
		assertInvalid(t, value, status)
	})
	t.Run("unavailable schema", func(t *testing.T) {
		unavailable := Axis[string, int]{Slot: totalAxis.Slot}
		value, status := Read(&sealed, unavailable, "present")
		assertInvalid(t, value, status)
	})
	t.Run("slot out of bounds", func(t *testing.T) {
		beyond := Axis[string, int]{SchemaID: fixtureSchema, Slot: uint32(sealed.Columns())}
		value, status := Read(&sealed, beyond, "present")
		assertInvalid(t, value, status)
	})
	t.Run("far slot out of bounds", func(t *testing.T) {
		beyond := Axis[string, int]{SchemaID: fixtureSchema, Slot: ^uint32(0)}
		value, status := Read(&sealed, beyond, "present")
		assertInvalid(t, value, status)
	})
	t.Run("wrong key type", func(t *testing.T) {
		wrongKey := Axis[int, int]{SchemaID: fixtureSchema, Slot: totalAxis.Slot}
		value, status := Read(&sealed, wrongKey, 0)
		assertInvalid(t, value, status)
	})
	t.Run("wrong value type", func(t *testing.T) {
		wrongValue := Axis[string, uint64]{SchemaID: fixtureSchema, Slot: totalAxis.Slot}
		value, status := Read(&sealed, wrongValue, "present")
		if status != ReadInvalid || value != 0 {
			t.Fatalf("read = (%d, %v), want (0, invalid)", value, status)
		}
	})
	t.Run("wrong value type at record slot", func(t *testing.T) {
		wrongValue := Axis[int, uint64]{SchemaID: fixtureSchema, Slot: recordAxis.Slot}
		value, status := Read(&sealed, wrongValue, 5)
		if status != ReadInvalid || value != 0 {
			t.Fatalf("read = (%d, %v), want (0, invalid)", value, status)
		}
	})
	t.Run("unpublished snapshot", func(t *testing.T) {
		value, status := Read(&zero, totalAxis, "present")
		assertInvalid(t, value, status)
	})
	t.Run("nil snapshot", func(t *testing.T) {
		value, status := Read(nil, totalAxis, "present")
		assertInvalid(t, value, status)
	})
}

// TestReadRecoversPerColumnTypes proves the column kind check is per slot and
// not per package: two columns of different key and value types coexist, and
// each answers only the axis that claims its own types.
func TestReadRecoversPerColumnTypes(t *testing.T) {
	sealed := newFixture(t)
	stored, status := Read(&sealed, recordAxis, 5)
	if status != ReadHit {
		t.Fatalf("record status = %v, want hit", status)
	}
	if stored != (record{Weight: 1, Reach: 2, Marked: true}) {
		t.Fatalf("record = %+v", stored)
	}
	if _, status := Read(&sealed, recordAxis, 6); status != ReadMiss {
		t.Fatalf("unstored record status = %v, want miss", status)
	}
	crossed := Axis[string, int]{SchemaID: fixtureSchema, Slot: recordAxis.Slot}
	crossedValue, crossedStatus := Read(&sealed, crossed, "present")
	assertInvalid(t, crossedValue, crossedStatus)
}

// TestReadBorrowsWithoutSharingStorage proves a read hands back a value and
// never the column's backing map, and that a writer's own map stops being the
// published storage the moment the column is filled.
func TestReadBorrowsWithoutSharingStorage(t *testing.T) {
	builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
	rows := map[string]int{"present": 11}
	members := []string{"present", "absent"}
	put(t, &builder, totalAxis, Content[string, int]{
		Rows:        rows,
		Denominator: fixtureDenominator,
		Members:     members,
	})
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	rows["present"] = 99
	rows["injected"] = 1
	delete(rows, "absent")
	members[1] = "rewritten"

	if value, status := Read(&sealed, totalAxis, "present"); value != 11 || status != ReadHit {
		t.Fatalf("published row = (%d, %v), want (11, hit)", value, status)
	}
	if _, status := Read(&sealed, totalAxis, "injected"); status != ReadMiss {
		t.Fatalf("injected key status = %v, want miss", status)
	}
	if _, status := Read(&sealed, totalAxis, "absent"); status != ReadProvenAbsent {
		t.Fatalf("denominator member status = %v, want proven-absent", status)
	}
	if _, status := Read(&sealed, totalAxis, "rewritten"); status != ReadMiss {
		t.Fatalf("rewritten member status = %v, want miss", status)
	}
}

// TestReadStatusNamesAreStable keeps diagnostic rendering honest about which
// outcome it is reporting.
func TestReadStatusNamesAreStable(t *testing.T) {
	cases := map[ReadStatus]string{
		ReadInvalid:      "invalid",
		ReadHit:          "hit",
		ReadMiss:         "miss",
		ReadProvenAbsent: "proven-absent",
		ReadStatus(9):    "invalid",
	}
	for status, want := range cases {
		if got := status.String(); got != want {
			t.Errorf("ReadStatus(%d).String() = %q, want %q", status, got, want)
		}
	}
	if ReadInvalid != 0 {
		t.Fatal("ReadInvalid must be the zero value so an ignored status never reads as an outcome")
	}
}

// TestAxisAvailability keeps the zero Axis from naming a column.
func TestAxisAvailability(t *testing.T) {
	if (Axis[string, int]{}).Available() {
		t.Fatal("zero axis names a column")
	}
	if !totalAxis.Available() {
		t.Fatal("fixture axis is unavailable")
	}
	if (Snapshot{}).Published() {
		t.Fatal("zero snapshot reports itself published")
	}
	sealed := newFixture(t)
	if !sealed.Published() {
		t.Fatal("sealed snapshot reports itself unpublished")
	}
	if sealed.Schema() != fixtureSchema || sealed.Store() != fixtureStore || sealed.Generation() != fixtureGeneration {
		t.Fatalf("snapshot anchors = (%s, %d, %d)", sealed.Schema(), sealed.Store(), sealed.Generation())
	}
	if sealed.Columns() != 3 {
		t.Fatalf("columns = %d, want 3", sealed.Columns())
	}
}

// TestSealedSubValuesPublishTheirBindings fixes the minimal contracts the
// sealed sub-values carry today: a denominator resolves to the column that
// proves it, a bound mount reports as bound, and a registered plan reports as
// published. Anything not published reports nothing.
func TestSealedSubValuesPublishTheirBindings(t *testing.T) {
	sealed := newFixture(t)
	slot, published := sealed.Denominators().Slot(fixtureDenominator)
	if !published || slot != totalAxis.Slot {
		t.Fatalf("denominator slot = (%d, %t), want (%d, true)", slot, published, totalAxis.Slot)
	}
	if _, published := sealed.Denominators().Slot(fixtureUnknownID); published {
		t.Fatal("unpublished denominator resolves")
	}
	if sealed.Denominators().Len() != 1 {
		t.Fatalf("denominators = %d, want 1", sealed.Denominators().Len())
	}
	if !sealed.Mounts().Bound(fixtureMount) || sealed.Mounts().Bound(identity.MountID{0xEE}) {
		t.Fatal("mount bindings do not report the sealed set")
	}
	if sealed.Mounts().Len() != 1 {
		t.Fatalf("mounts = %d, want 1", sealed.Mounts().Len())
	}
	if !sealed.Queries().Published(fixtureQueryPlan) || sealed.Queries().Published(fixtureUnknownID) {
		t.Fatal("query publication does not report the sealed set")
	}
	if sealed.Queries().Len() != 1 {
		t.Fatalf("queries = %d, want 1", sealed.Queries().Len())
	}
	zero := Snapshot{}
	if zero.Denominators().Len() != 0 || zero.Mounts().Len() != 0 || zero.Queries().Len() != 0 {
		t.Fatal("zero snapshot publishes bindings")
	}
	if _, published := zero.Denominators().Slot(fixtureDenominator); published {
		t.Fatal("zero snapshot resolves a denominator")
	}
	if zero.Mounts().Bound(fixtureMount) || zero.Queries().Published(fixtureQueryPlan) {
		t.Fatal("zero snapshot reports a binding")
	}
}

// assertInvalid fails unless a read was rejected outright.
func assertInvalid[V comparable](t *testing.T, value V, status ReadStatus) {
	t.Helper()
	var zero V
	if status != ReadInvalid {
		t.Fatalf("status = %v, want invalid", status)
	}
	if value != zero {
		t.Fatalf("rejected read returned %v, want the zero value", value)
	}
	if status.Outcome() {
		t.Fatal("rejected read reports itself an outcome")
	}
}
