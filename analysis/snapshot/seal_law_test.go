package snapshot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestSealRejectsIncompletePublication fixes what publication refuses. A
// snapshot without a schema, a store, or a generation has nothing to anchor a
// read or a locator to, and a hole in the dense slot range is a column that
// was declared and never written.
func TestSealRejectsIncompletePublication(t *testing.T) {
	cases := []struct {
		name  string
		build func() Builder
		want  error
	}{
		{
			name:  "no schema",
			build: func() Builder { return NewBuilder(identity.ContentID{}, fixtureStore, fixtureGeneration) },
			want:  ErrUnavailableIdentity,
		},
		{
			name:  "no store",
			build: func() Builder { return NewBuilder(fixtureSchema, 0, fixtureGeneration) },
			want:  ErrUnavailableIdentity,
		},
		{
			name:  "no generation",
			build: func() Builder { return NewBuilder(fixtureSchema, fixtureStore, 0) },
			want:  ErrUnavailableIdentity,
		},
		{
			name: "slot hole",
			build: func() Builder {
				builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
				put(t, &builder, recordAxis, Content[int, record]{Rows: map[int]record{5: {}}})
				return builder
			},
			want: ErrSlotEmpty,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			builder := testCase.build()
			sealed, err := builder.Seal()
			if !errors.Is(err, testCase.want) {
				t.Fatalf("seal error = %v, want %v", err, testCase.want)
			}
			if sealed.Published() {
				t.Fatal("a rejected seal returned a published snapshot")
			}
		})
	}
}

// TestConstructionRejectsSecondAuthority fixes the one-authority rules of
// construction: one column per slot, one column publication per identity, one
// denominator per identity, and no column at all for an axis of another
// schema. A member set without a denominator identity is unprovable and is
// rejected rather than silently downgraded to a miss-only column.
func TestConstructionRejectsSecondAuthority(t *testing.T) {
	t.Run("second column at one slot", func(t *testing.T) {
		builder := newFixtureBuilder(t)
		err := PutColumn(&builder, totalAxis, Content[string, int]{Rows: map[string]int{"other": 1}})
		if !errors.Is(err, ErrSlotFilled) {
			t.Fatalf("error = %v, want %v", err, ErrSlotFilled)
		}
	})
	t.Run("column of another schema", func(t *testing.T) {
		builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
		foreign := Axis[string, int]{SchemaID: fixtureOtherSchema, Slot: 0}
		err := PutColumn(&builder, foreign, Content[string, int]{})
		if !errors.Is(err, ErrSchemaMismatch) {
			t.Fatalf("error = %v, want %v", err, ErrSchemaMismatch)
		}
	})
	t.Run("column of no schema", func(t *testing.T) {
		builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
		err := PutColumn(&builder, Axis[string, int]{}, Content[string, int]{})
		if !errors.Is(err, ErrSchemaMismatch) {
			t.Fatalf("error = %v, want %v", err, ErrSchemaMismatch)
		}
	})
	t.Run("members without a denominator", func(t *testing.T) {
		builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
		err := PutColumn(&builder, totalAxis, Content[string, int]{Members: []string{"absent"}})
		if !errors.Is(err, ErrUnprovenMembers) {
			t.Fatalf("error = %v, want %v", err, ErrUnprovenMembers)
		}
	})
	t.Run("second denominator under one identity", func(t *testing.T) {
		builder := newFixtureBuilder(t)
		err := PutColumn(&builder, Axis[string, int]{SchemaID: fixtureSchema, Slot: 3}, Content[string, int]{
			Denominator: fixtureDenominator,
			Members:     []string{"absent"},
		})
		if !errors.Is(err, ErrDuplicatePublication) {
			t.Fatalf("error = %v, want %v", err, ErrDuplicatePublication)
		}
	})
	t.Run("second directory entry under one identity", func(t *testing.T) {
		builder := newFixtureBuilder(t)
		if err := builder.Publish(fixtureTotalID, partialAxis.Slot); !errors.Is(err, ErrDuplicatePublication) {
			t.Fatalf("error = %v, want %v", err, ErrDuplicatePublication)
		}
	})
	t.Run("directory entry for an unfilled slot", func(t *testing.T) {
		builder := newFixtureBuilder(t)
		if err := builder.Publish(fixtureUnknownID, 9); !errors.Is(err, ErrUnknownSlot) {
			t.Fatalf("error = %v, want %v", err, ErrUnknownSlot)
		}
	})
	t.Run("unavailable publication identities", func(t *testing.T) {
		builder := newFixtureBuilder(t)
		if err := builder.Publish(identity.ContentID{}, 0); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("directory error = %v, want %v", err, ErrUnavailableIdentity)
		}
		if err := builder.Bind(identity.MountID{}); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("mount error = %v, want %v", err, ErrUnavailableIdentity)
		}
		if err := builder.RegisterQuery(identity.ContentID{}); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("query error = %v, want %v", err, ErrUnavailableIdentity)
		}
	})
	t.Run("nil builder", func(t *testing.T) {
		if err := PutColumn(nil, totalAxis, Content[string, int]{}); !errors.Is(err, ErrUnavailableIdentity) {
			t.Fatalf("error = %v, want %v", err, ErrUnavailableIdentity)
		}
	})
}

// TestSealedSnapshotIgnoresLaterConstruction proves publication detaches the
// snapshot from the builder that produced it. Seal consumes the builder by
// value and copies every container it publishes, so a builder that keeps
// filling slots, publishing identities, binding mounts, and registering plans
// after Seal changes nothing a consumer can observe.
//
// The rest is enforced by the type system rather than by this test: a column
// is unexported and no exported function or field anywhere in the package
// names one, so no caller can write a source line that reaches published
// storage, and every Snapshot field is unexported, so no caller can replace
// one. Both are asserted structurally in
// TestSnapshotExposesNoMutableSurface.
func TestSealedSnapshotIgnoresLaterConstruction(t *testing.T) {
	builder := newFixtureBuilder(t)
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	columns := sealed.Columns()

	put(t, &builder, Axis[string, int]{SchemaID: fixtureSchema, Slot: 3}, Content[string, int]{
		Rows: map[string]int{"late": 1},
	})
	if err := builder.Publish(fixtureUnknownID, 3); err != nil {
		t.Fatalf("late publish: %v", err)
	}
	if err := builder.Bind(identity.MountID{0xAA}); err != nil {
		t.Fatalf("late bind: %v", err)
	}
	if err := builder.RegisterQuery(identity.ContentID{0xBB}); err != nil {
		t.Fatalf("late query: %v", err)
	}

	if sealed.Columns() != columns {
		t.Fatalf("columns = %d, want %d", sealed.Columns(), columns)
	}
	late := Axis[string, int]{SchemaID: fixtureSchema, Slot: 3}
	lateValue, lateStatus := Read(&sealed, late, "late")
	assertInvalid(t, lateValue, lateStatus)
	if _, resolved := Resolve(&sealed, fixtureUnknownID); resolved {
		t.Fatal("a late directory entry reached the sealed snapshot")
	}
	if sealed.Mounts().Bound(identity.MountID{0xAA}) {
		t.Fatal("a late mount binding reached the sealed snapshot")
	}
	if sealed.Queries().Published(identity.ContentID{0xBB}) {
		t.Fatal("a late query registration reached the sealed snapshot")
	}
	if sealed.Denominators().Len() != 1 {
		t.Fatalf("denominators = %d, want 1", sealed.Denominators().Len())
	}
}

// TestSnapshotExposesNoMutableSurface is the structural half of the sealing
// law. Every Snapshot field is unexported and its whole method set is
// read-only accessors, so the only way to obtain a Snapshot is to seal a
// Builder and there is no exported surface through which a published one can
// be written.
func TestSnapshotExposesNoMutableSurface(t *testing.T) {
	snapshotType := reflect.TypeOf(Snapshot{})
	for index := 0; index < snapshotType.NumField(); index++ {
		if snapshotType.Field(index).IsExported() {
			t.Errorf("Snapshot exposes field %s", snapshotType.Field(index).Name)
		}
	}
	readers := map[string]bool{
		"Schema": true, "Store": true, "Generation": true, "Published": true,
		"Columns": true, "Denominators": true, "Mounts": true, "Queries": true,
	}
	pointer := reflect.PointerTo(snapshotType)
	for index := 0; index < pointer.NumMethod(); index++ {
		name := pointer.Method(index).Name
		if !readers[name] {
			t.Errorf("Snapshot exposes non-reader method %s", name)
		}
	}
	if pointer.NumMethod() != len(readers) {
		t.Fatalf("Snapshot method set = %d methods, want %d readers", pointer.NumMethod(), len(readers))
	}
	for _, sealedType := range []reflect.Type{
		reflect.TypeOf(Denominators{}),
		reflect.TypeOf(Mounts{}),
		reflect.TypeOf(Queries{}),
	} {
		for index := 0; index < sealedType.NumField(); index++ {
			if sealedType.Field(index).IsExported() {
				t.Errorf("%s exposes field %s", sealedType, sealedType.Field(index).Name)
			}
		}
	}
}
