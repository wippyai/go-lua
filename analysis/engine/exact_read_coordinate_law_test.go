package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// An exact read coordinate is issued, never computed by its consumer. A Factor
// owner hands out a Ref over its own dense key space and the engine turns that
// capability into the one surface the query binder will accept. These laws
// state what that translation is - the owner's zero-based key becomes the
// one-based local, and nothing else about the surface is populated - and what
// it refuses: a key the Factor never sealed, and a capability no Factor issued.
//
// They are the read half of the same statement exactRuleWriteSurface makes for
// a write, and they are what lets a routed publication be observed at all: a
// routed member has no committed exact write whose local could be reused, so
// the coordinate has to come from the owner.

func exactReadCoordinateBinding(t testing.TB, seed uint64) (*SchemaBinding, *FactorImplementation[uint64, uint64]) {
	t.Helper()
	schema, factor := factorOnlySlotSchema(t, coldKey(seed))
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !binding.Seal() {
		t.Fatal("sealed factor binding")
	}
	implementation, ok := FactorImplementationAt[uint64, uint64](binding, factor)
	if !ok || implementation == nil {
		t.Fatal("published factor implementation")
	}
	return binding, implementation
}

// TestAnExactReadCoordinateIsTheOwnerKeyPlusOne states the translation. The
// owner's key space is zero-based and the equation's local space is one-based
// so that zero stays "no coordinate"; the surface carries that local and
// nothing else - no target mode, no semantic, no normalizer - because an exact
// read is addressed by coordinate alone.
func TestAnExactReadCoordinateIsTheOwnerKeyPlusOne(t *testing.T) {
	_, implementation := exactReadCoordinateBinding(t, 949_101)
	for key := uint64(0); key < 2; key++ {
		ref, refOK := implementation.Ref(key)
		if !refOK {
			t.Fatalf("factor issued no capability for its own key %d", key)
		}
		surface, surfaceOK := ExactReadSurface(ref)
		if !surfaceOK {
			t.Fatalf("key %d issued a capability with no read coordinate", key)
		}
		if surface.value.Local != key+1 {
			t.Fatalf("key %d reads at local %d, want %d", key, surface.value.Local, key+1)
		}
		if surface.value.Form != equation.SurfaceReadExact || surface.value.Mode != equation.TargetModeNone {
			t.Fatalf("key %d reads through form %d mode %d", key, surface.value.Form, surface.value.Mode)
		}
		if surface.value.Semantic.Available() || surface.value.Normalizer.Available() || surface.value.Content != [32]byte{} {
			t.Fatalf("key %d carries a coordinate beside its own local", key)
		}
		if !surface.value.Factor.Available() {
			t.Fatalf("key %d names no Factor", key)
		}
	}
}

// TestAnExactReadCoordinateRefusesAKeyItsFactorDidNotSeal is the range fence,
// stated at both ends of the issuance. The Factor refuses to issue a
// capability outside its own sealed key space, and a capability no Factor
// issued names no coordinate - so a consumer cannot reach a cell by presenting
// an index it computed.
func TestAnExactReadCoordinateRefusesAKeyItsFactorDidNotSeal(t *testing.T) {
	_, implementation := exactReadCoordinateBinding(t, 949_102)
	if _, issued := implementation.Ref(2); issued {
		t.Fatal("a key outside the sealed space issued a capability")
	}
	if _, read := ExactReadSurface(Ref[uint64]{}); read {
		t.Fatal("an unissued capability named a read coordinate")
	}
}

// TestAnExactReadCoordinateCarriesItsIssuersAuthority states why equal content
// is not equal capability. Two bindings over one cold schema publish the same
// key space, and their coordinates agree by index; the surfaces still differ,
// because each carries the binding authority the query binder authenticates
// against. A coordinate issued by another binding is therefore refused there
// rather than accepted for having the right number in it.
func TestAnExactReadCoordinateCarriesItsIssuersAuthority(t *testing.T) {
	_, own := exactReadCoordinateBinding(t, 949_103)
	_, foreign := exactReadCoordinateBinding(t, 949_103)
	ownRef, ownRefOK := own.Ref(1)
	foreignRef, foreignRefOK := foreign.Ref(1)
	ownSurface, ownSurfaceOK := ExactReadSurface(ownRef)
	foreignSurface, foreignSurfaceOK := ExactReadSurface(foreignRef)
	if !ownRefOK || !foreignRefOK || !ownSurfaceOK || !foreignSurfaceOK {
		t.Fatal("two bindings over one schema issued a coordinate each")
	}
	if ownSurface.value != foreignSurface.value {
		t.Fatal("two bindings over one schema disagree about the coordinate of one key")
	}
	if ownSurface.authority == nil || ownSurface.authority == foreignSurface.authority {
		t.Fatal("a coordinate does not carry the authority that issued it")
	}
	if ownSurface == foreignSurface {
		t.Fatal("two capabilities from different bindings are one surface")
	}
}
