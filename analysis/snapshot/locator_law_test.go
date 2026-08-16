package snapshot

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestResolveReturnsOneAnchoredLocator fixes the directory contract: a
// published identity resolves to exactly one locator, anchored to this
// snapshot's store and generation, and an identity the directory does not
// publish resolves to nothing.
func TestResolveReturnsOneAnchoredLocator(t *testing.T) {
	sealed := newFixture(t)
	locator, resolved := Resolve(&sealed, fixtureTotalID)
	if !resolved {
		t.Fatal("published identity does not resolve")
	}
	if !locator.Valid(fixtureStore, fixtureGeneration) {
		t.Fatalf("locator %+v is not anchored to the publishing snapshot", locator)
	}
	again, _ := Resolve(&sealed, fixtureTotalID)
	if again != locator {
		t.Fatal("one identity resolves to two locators")
	}
	other, resolved := Resolve(&sealed, fixturePartialID)
	if !resolved || other == locator {
		t.Fatal("two identities resolve to one locator")
	}
	if _, resolved := Resolve(&sealed, fixtureUnknownID); resolved {
		t.Fatal("unpublished identity resolves")
	}
	if _, resolved := Resolve(&sealed, identity.ContentID{}); resolved {
		t.Fatal("unavailable identity resolves")
	}
	zero := Snapshot{}
	if _, resolved := Resolve(&zero, fixtureTotalID); resolved {
		t.Fatal("unpublished snapshot resolves")
	}
	if _, resolved := Resolve(nil, fixtureTotalID); resolved {
		t.Fatal("nil snapshot resolves")
	}
}

// TestLocatorAccessFailsClosed fixes locator validation. A locator addresses
// one store at one generation: the snapshot that issued it answers, and a
// snapshot of another store, another generation, or the zero locator rejects
// the read outright rather than reading whatever now occupies the slot.
func TestLocatorAccessFailsClosed(t *testing.T) {
	sealed := newFixture(t)
	locator, resolved := Resolve(&sealed, fixtureTotalID)
	if !resolved {
		t.Fatal("published identity does not resolve")
	}

	value, status := ReadAt[string, int](&sealed, locator, "present")
	if status != ReadHit || value != 11 {
		t.Fatalf("locator read = (%d, %v), want (11, hit)", value, status)
	}
	if _, status := ReadAt[string, int](&sealed, locator, "absent"); status != ReadProvenAbsent {
		t.Fatalf("locator absence = %v, want proven-absent", status)
	}
	if _, status := ReadAt[string, int](&sealed, locator, "unknown"); status != ReadMiss {
		t.Fatalf("locator miss = %v, want miss", status)
	}

	advanced := republish(t, fixtureStore, fixtureGeneration.Next())
	staleValue, staleStatus := ReadAt[string, int](&advanced, locator, "present")
	assertInvalid(t, staleValue, staleStatus)

	moved := republish(t, fixtureStore+1, fixtureGeneration)
	movedValue, movedStatus := ReadAt[string, int](&moved, locator, "present")
	assertInvalid(t, movedValue, movedStatus)

	zeroValue, zeroStatus := ReadAt[string, int](&sealed, Locator{}, "present")
	assertInvalid(t, zeroValue, zeroStatus)

	nilValue, nilStatus := ReadAt[string, int](nil, locator, "present")
	assertInvalid(t, nilValue, nilStatus)

	kindValue, kindStatus := ReadAt[int, int](&sealed, locator, 0)
	assertInvalid(t, kindValue, kindStatus)

	crossedValue, crossedStatus := ReadAt[string, uint64](&sealed, locator, "present")
	assertInvalid(t, crossedValue, crossedStatus)
}

// TestLocatorSlotIsNotPersistable keeps a locator an address rather than an
// identity. Its coordinate type has no exported field and no method at all,
// so a consumer cannot mint one, read its coordinate out as a durable key, or
// serialize one and replay it against another store.
func TestLocatorSlotIsNotPersistable(t *testing.T) {
	slotType := reflect.TypeOf(Locator{}.Slot)
	if slotType.Kind() != reflect.Struct {
		t.Fatalf("locator slot kind = %v, want struct", slotType.Kind())
	}
	if slotType.PkgPath() == "" || slotType.Name() != "address" {
		t.Fatalf("locator slot type %s is not the package-private coordinate", slotType)
	}
	for index := 0; index < slotType.NumField(); index++ {
		if slotType.Field(index).IsExported() {
			t.Errorf("locator slot exposes field %s", slotType.Field(index).Name)
		}
	}
	if slotType.NumMethod() != 0 || reflect.PointerTo(slotType).NumMethod() != 0 {
		t.Fatal("locator slot carries methods, which is an encoding surface")
	}

	sealed := newFixture(t)
	locator, resolved := Resolve(&sealed, fixtureRecordID)
	if !resolved {
		t.Fatal("published identity does not resolve")
	}
	encoded, err := json.Marshal(locator)
	if err != nil {
		t.Fatalf("marshal locator: %v", err)
	}
	var restored Locator
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("unmarshal locator: %v", err)
	}
	if restored == locator {
		t.Fatalf("a locator round-tripped through serialization: %s", encoded)
	}
	restoredValue, restoredStatus := ReadAt[int, record](&sealed, restored, 5)
	assertInvalid(t, restoredValue, restoredStatus)
}
