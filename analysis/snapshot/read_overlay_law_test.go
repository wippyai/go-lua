package snapshot

import "testing"

// TestReadOverlaySeesInheritedAndAuthoredRows fixes the overlay's read
// contract: a derived builder starts with the base's answers, a row edit
// shadows only that key, and rows in untouched columns remain visible.
func TestReadOverlaySeesInheritedAndAuthoredRows(t *testing.T) {
	base := newFixture(t)
	delta := NewDelta(base, fixtureGeneration+1)

	if value, status := ReadOverlay(&delta, totalAxis, "present"); value != 11 || status != ReadHit {
		t.Fatalf("inherited row = (%d, %v), want (11, hit)", value, status)
	}
	if _, status := ReadOverlay(&delta, totalAxis, "absent"); status != ReadProvenAbsent {
		t.Fatalf("inherited absence = %v, want proven-absent", status)
	}
	if value, status := ReadOverlay(&delta, partialAxis, "present"); value != 22 || status != ReadHit {
		t.Fatalf("untouched row = (%d, %v), want (22, hit)", value, status)
	}

	if err := SetRow(&delta, totalAxis, "present", 99); err != nil {
		t.Fatalf("set row: %v", err)
	}
	if value, status := ReadOverlay(&delta, totalAxis, "present"); value != 99 || status != ReadHit {
		t.Fatalf("authored row = (%d, %v), want (99, hit)", value, status)
	}
	if value, status := Read(&base, totalAxis, "present"); value != 11 || status != ReadHit {
		t.Fatalf("base row after overlay = (%d, %v), want (11, hit)", value, status)
	}
	if _, status := ReadOverlay(&delta, totalAxis, "absent"); status != ReadProvenAbsent {
		t.Fatalf("untouched denominator after set = %v, want proven-absent", status)
	}
}

// TestReadOverlayRemovalPublishesAbsence fixes the removal rule while the
// publication is still mutable: withdrawing an inherited row leaves its
// inherited denominator attached, so the row becomes proven absent, and the
// base remains unchanged.
func TestReadOverlayRemovalPublishesAbsence(t *testing.T) {
	base := newFixture(t)
	delta := NewDelta(base, fixtureGeneration+1)
	if err := RemoveRow(&delta, totalAxis, "present"); err != nil {
		t.Fatalf("remove row: %v", err)
	}

	if value, status := ReadOverlay(&delta, totalAxis, "present"); value != 0 || status != ReadProvenAbsent {
		t.Fatalf("removed row = (%d, %v), want (0, proven-absent)", value, status)
	}
	if value, status := Read(&base, totalAxis, "present"); value != 11 || status != ReadHit {
		t.Fatalf("base row after removal = (%d, %v), want (11, hit)", value, status)
	}
}

// TestReadOverlayFailsClosed fixes the overlay validation boundary. Every
// rejected axis produces the zero value and ReadInvalid, just like Read.
func TestReadOverlayFailsClosed(t *testing.T) {
	base := newFixture(t)
	delta := NewDelta(base, fixtureGeneration+1)
	cases := []struct {
		name string
		read func() (int, ReadStatus)
	}{
		{name: "nil builder", read: func() (int, ReadStatus) {
			return ReadOverlay(nil, totalAxis, "present")
		}},
		{name: "schema mismatch", read: func() (int, ReadStatus) {
			return ReadOverlay(&delta, Axis[string, int]{SchemaID: fixtureOtherSchema, Slot: totalAxis.Slot}, "present")
		}},
		{name: "unavailable schema", read: func() (int, ReadStatus) {
			return ReadOverlay(&delta, Axis[string, int]{Slot: totalAxis.Slot}, "present")
		}},
		{name: "slot mismatch", read: func() (int, ReadStatus) {
			return ReadOverlay(&delta, Axis[string, int]{SchemaID: fixtureSchema, Slot: uint32(len(delta.columns))}, "present")
		}},
		{name: "key mismatch", read: func() (int, ReadStatus) {
			return ReadOverlay(&delta, Axis[int, int]{SchemaID: fixtureSchema, Slot: totalAxis.Slot}, 0)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value, status := testCase.read()
			assertInvalid(t, value, status)
		})
	}

	value, status := ReadOverlay(&delta, Axis[string, uint64]{SchemaID: fixtureSchema, Slot: totalAxis.Slot}, "present")
	assertInvalid(t, int(value), status)
}

// TestReadOverlayAllocatesNothing fixes the hot path cost of reading a
// mutable publication. Validation and every read outcome stay allocation-free
// before or after a persistent edit.
func TestReadOverlayAllocatesNothing(t *testing.T) {
	base := newFixture(t)
	delta := NewDelta(base, fixtureGeneration+1)
	if err := SetRow(&delta, totalAxis, "present", 99); err != nil {
		t.Fatalf("set row: %v", err)
	}

	var value int
	var status ReadStatus
	cases := []struct {
		name string
		want ReadStatus
		read func()
	}{
		{name: "hit", want: ReadHit, read: func() {
			value, status = ReadOverlay(&delta, totalAxis, "present")
		}},
		{name: "proven absence", want: ReadProvenAbsent, read: func() {
			value, status = ReadOverlay(&delta, totalAxis, "absent")
		}},
		{name: "miss", want: ReadMiss, read: func() {
			value, status = ReadOverlay(&delta, totalAxis, "unknown")
		}},
		{name: "invalid", want: ReadInvalid, read: func() {
			value, status = ReadOverlay(&delta, Axis[string, int]{SchemaID: fixtureOtherSchema}, "present")
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(1000, testCase.read); allocations != 0 {
				t.Fatalf("allocations = %v, want 0", allocations)
			}
			if status != testCase.want {
				t.Fatalf("status = %v, want %v", status, testCase.want)
			}
			if testCase.want == ReadHit && value != 99 {
				t.Fatalf("value = %d, want 99", value)
			}
			if testCase.want != ReadHit && value != 0 {
				t.Fatalf("value = %d, want zero", value)
			}
		})
	}
}
