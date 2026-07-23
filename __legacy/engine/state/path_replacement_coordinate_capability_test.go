package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPathReplacementCoordinateCapabilitySealsExactRegisteredEquality(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	target, ok := keys.InternStateKey("path-replacement-capability@1.target")
	if !ok {
		t.Fatal("intern target")
	}
	source, ok := keys.InternStateKey("path-replacement-capability@1.source")
	if !ok {
		t.Fatal("intern source")
	}
	capability, err := domain.SealPathReplacementCoordinateCapability(keys, target, source, true)
	if err != nil || !capability.ValidFor(domain, keys) {
		t.Fatalf("seal capability = (%#v, %v)", capability, err)
	}
	slots := capability.EmittedSlots()
	if len(slots) != 2 {
		t.Fatalf("emitted slots = %#v, want exact target refinement and equality", slots)
	}
	if _, err := domain.SealCoordinateFactorInventory(keys, slots); err != nil {
		t.Fatalf("emitted slot is not registered: %v", err)
	}
	empty, err := domain.SealPathReplacementCoordinateCapability(keys, target, keyspace.Key{}, false)
	if err != nil || len(empty.EmittedSlots()) != 1 {
		t.Fatalf("source-less capability = (%#v, %v), want only target refinement", empty, err)
	}
}

func TestPathReplacementCoordinateCapabilitySealsDynamicAndObjectEntryWrites(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	dynamic, ok := keys.InternStateKey("path-replacement-capability@2[\"dynamic\"]")
	if !ok {
		t.Fatal("intern dynamic target")
	}
	canonical, ok := keys.FieldCanonical(dynamic)
	if !ok || canonical == dynamic {
		t.Fatal("intern dynamic target did not produce its canonical alias")
	}
	member, ok := keys.InternStateKey("path-replacement-capability@2.object.member")
	if !ok {
		t.Fatal("intern object member target")
	}
	source, ok := keys.InternStateKey("path-replacement-capability@2.object.source")
	if !ok {
		t.Fatal("intern object member source")
	}
	capability, err := domain.SealPathReplacementCoordinateCapabilityForWrites(keys, []PathReplacementCoordinateWrite{
		{Target: dynamic},
		{Target: member, Source: source, HasSource: true},
	})
	if err != nil || !capability.ValidFor(domain, keys) {
		t.Fatalf("seal capability = (%#v, %v)", capability, err)
	}
	if slots := capability.EmittedSlots(); len(slots) != 4 {
		t.Fatalf("emitted slots = %#v, want dynamic refinement and canonical alias plus object member refinement/equality", slots)
	}
	canonicalSlot, err := domain.PathRefinementCoordinateSlot(keys, canonical)
	if err != nil {
		t.Fatalf("canonical refinement slot: %v", err)
	}
	foundCanonical := false
	for _, slot := range capability.EmittedSlots() {
		equal, equalErr := domain.CoordinateSlotEqual(slot, canonicalSlot)
		if equalErr != nil {
			t.Fatalf("compare canonical refinement slot: %v", equalErr)
		}
		foundCanonical = foundCanonical || equal
	}
	if !foundCanonical {
		t.Fatal("dynamic write omitted its transaction-produced canonical refinement")
	}
	if _, err := domain.SealCoordinateFactorInventory(keys, capability.EmittedSlots()); err != nil {
		t.Fatalf("emitted slots are not registered: %v", err)
	}
	if _, err := domain.SealPathReplacementCoordinateCapabilityForWrites(keys, []PathReplacementCoordinateWrite{{Target: dynamic, HasSource: true}}); err == nil {
		t.Fatal("source-less dynamic write was accepted as an equality emission")
	}
}
