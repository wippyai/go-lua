package snapshot

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The law fixture publishes one snapshot that exercises every read outcome:
// a column with a sealed denominator, a column without one, and a column of a
// different key and value type at a third slot.
var (
	fixtureSchema      = identity.ContentID{0x01, 0x11}
	fixtureOtherSchema = identity.ContentID{0x02, 0x22}
	fixtureStore       = identity.StoreID(7)
	fixtureGeneration  = identity.Generation(3)

	fixtureDenominator = identity.ContentID{0x03, 0x33}
	fixtureTotalID     = identity.ContentID{0x04, 0x44}
	fixturePartialID   = identity.ContentID{0x05, 0x55}
	fixtureRecordID    = identity.ContentID{0x06, 0x66}
	fixtureUnknownID   = identity.ContentID{0x07, 0x77}
	fixtureMount       = identity.ContentID{0x08, 0x88}
	fixtureQueryPlan   = identity.ContentID{0x09, 0x99}

	// totalAxis is total over {"present", "absent"} and stores only
	// "present", so "absent" is proven absent and "unknown" is a miss.
	totalAxis = Axis[string, int]{SchemaID: fixtureSchema, Slot: 0}
	// partialAxis publishes no denominator, so every unstored key is a miss.
	partialAxis = Axis[string, int]{SchemaID: fixtureSchema, Slot: 1}
	// recordAxis stores a different key and value type at a third slot.
	recordAxis = Axis[int, record]{SchemaID: fixtureSchema, Slot: 2}
)

// record is a value shape wide enough that returning one by value would show
// up as an allocation if a read ever boxed it.
type record struct {
	Weight uint64
	Reach  uint64
	Marked bool
}

// newFixtureBuilder returns the law fixture's builder, filled but unsealed.
func newFixtureBuilder(t testing.TB) Builder {
	t.Helper()
	builder := NewBuilder(fixtureSchema, fixtureStore, fixtureGeneration)
	put(t, &builder, totalAxis, Content[string, int]{
		Rows:        map[string]int{"present": 11},
		Denominator: fixtureDenominator,
		Members:     []string{"present", "absent"},
	})
	put(t, &builder, partialAxis, Content[string, int]{
		Rows: map[string]int{"present": 22},
	})
	put(t, &builder, recordAxis, Content[int, record]{
		Rows: map[int]record{5: {Weight: 1, Reach: 2, Marked: true}},
	})
	if err := builder.Publish(fixtureTotalID, totalAxis.Slot); err != nil {
		t.Fatalf("publish total axis: %v", err)
	}
	if err := builder.Publish(fixturePartialID, partialAxis.Slot); err != nil {
		t.Fatalf("publish partial axis: %v", err)
	}
	if err := builder.Publish(fixtureRecordID, recordAxis.Slot); err != nil {
		t.Fatalf("publish record axis: %v", err)
	}
	if err := builder.Bind(fixtureMount); err != nil {
		t.Fatalf("bind mount: %v", err)
	}
	if err := builder.RegisterQuery(fixtureQueryPlan); err != nil {
		t.Fatalf("register query: %v", err)
	}
	return builder
}

// newFixture returns the sealed law fixture.
func newFixture(t testing.TB) Snapshot {
	t.Helper()
	builder := newFixtureBuilder(t)
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal fixture: %v", err)
	}
	return sealed
}

// republish seals the same schema and columns under another store or
// generation, which is what a locator issued by the fixture must not survive.
func republish(t testing.TB, store identity.StoreID, generation identity.Generation) Snapshot {
	t.Helper()
	builder := NewBuilder(fixtureSchema, store, generation)
	put(t, &builder, totalAxis, Content[string, int]{
		Rows:        map[string]int{"present": 11},
		Denominator: fixtureDenominator,
		Members:     []string{"present", "absent"},
	})
	put(t, &builder, partialAxis, Content[string, int]{Rows: map[string]int{"present": 22}})
	put(t, &builder, recordAxis, Content[int, record]{Rows: map[int]record{5: {Weight: 1, Reach: 2, Marked: true}}})
	if err := builder.Publish(fixtureTotalID, totalAxis.Slot); err != nil {
		t.Fatalf("publish total axis: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal republication: %v", err)
	}
	return sealed
}

// put fills one column and fails the test when construction rejects.
func put[K comparable, V any](t testing.TB, b *Builder, ax Axis[K, V], content Content[K, V]) {
	t.Helper()
	if err := PutColumn(b, ax, content); err != nil {
		t.Fatalf("put column at slot %d: %v", ax.Slot, err)
	}
}
