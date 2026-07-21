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
