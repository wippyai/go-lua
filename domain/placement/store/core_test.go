package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestFromProgramUsesAnExplicitConservativeMapping(t *testing.T) {
	cases := []struct {
		program lifecycle.StorageLifetime
		want    Lifetime
	}{
		{lifecycle.StorageLifetimeFrame, LifetimeFrame},
		{lifecycle.StorageLifetimeModule, LifetimeModule},
		{lifecycle.StorageLifetimeGlobal, LifetimeGlobal},
		{lifecycle.StorageLifetimeExternal, LifetimeExternal},
		{lifecycle.StorageLifetimeUnknown, LifetimeUnknown},
		{lifecycle.StorageLifetimeClosure, LifetimeClosure},
		{lifecycle.StorageLifetimeInvalid, LifetimeInvalid},
	}
	for _, test := range cases {
		if got := FromProgram(test.program); got != test.want {
			t.Errorf("FromProgram(%d) = %s, want %s", test.program, got, test.want)
		}
	}
}

func TestFrameStorageDoesNotPromoteEveryRHS(t *testing.T) {
	for _, current := range []placement.Placement{placement.Bottom, placement.Stack, placement.OwnedHeap, placement.SharedHeap, placement.Unknown} {
		if got, ok := Apply(current, LifetimeFrame); !ok || got != current {
			t.Fatalf("frame store changed %s to %s (ok=%t)", current, got, ok)
		}
	}
}

func TestStorageLifetimeDemandIsOrdered(t *testing.T) {
	cases := []struct {
		lifetime Lifetime
		want     placement.Placement
	}{
		{LifetimeModule, placement.OwnedHeap},
		{LifetimeClosure, placement.OwnedHeap},
		{LifetimeGlobal, placement.SharedHeap},
		{LifetimeExternal, placement.Unknown},
		{LifetimeUnknown, placement.Unknown},
	}
	for _, test := range cases {
		if got, ok := Apply(placement.Stack, test.lifetime); !ok || got != test.want {
			t.Errorf("Apply(stack,%s) = %s (ok=%t), want %s", test.lifetime, got, ok, test.want)
		}
	}
}

func TestStorageLifetimeNeverDowngradesAnotherEscape(t *testing.T) {
	for _, lifetime := range []Lifetime{LifetimeModule, LifetimeClosure, LifetimeGlobal, LifetimeExternal, LifetimeUnknown} {
		for _, current := range []placement.Placement{placement.OwnedHeap, placement.SharedHeap, placement.Unknown} {
			got, ok := Apply(current, lifetime)
			if !ok || !placement.LessOrEq(current, got) {
				t.Errorf("Apply(%s,%s) = %s (ok=%t), downgraded current placement", current, lifetime, got, ok)
			}
		}
	}
}

func TestOrdinaryObjectStoreUsesContainmentNotLifetimePromotion(t *testing.T) {
	cases := []struct {
		destination placement.Placement
		source      placement.Placement
		want        placement.Placement
	}{
		{placement.Stack, placement.Stack, placement.Stack},
		{placement.OwnedHeap, placement.Stack, placement.OwnedHeap},
		{placement.Stack, placement.OwnedHeap, placement.OwnedHeap},
		{placement.SharedHeap, placement.Stack, placement.SharedHeap},
		{placement.Stack, placement.SharedHeap, placement.SharedHeap},
	}
	for _, test := range cases {
		if got, ok := ObjectStore(test.destination, test.source); !ok || got != test.want {
			t.Errorf("ObjectStore(%s,%s) = %s (ok=%t), want %s", test.destination, test.source, got, ok, test.want)
		}
	}
	if got, ok := ObjectStore(placement.Stack, placement.Stack); !ok || got == placement.OwnedHeap || got == placement.SharedHeap {
		t.Fatalf("ordinary local object store was promoted to %s", got)
	}
}

func TestStorageRejectsUnclassifiedAndJITInputsWithoutFabricatingUnknown(t *testing.T) {
	if got, ok := Apply(placement.Stack, LifetimeInvalid); ok || got == placement.Unknown {
		t.Fatalf("invalid lifetime = %s (ok=%t), must refuse without Unknown", got, ok)
	}
	if got, ok := Apply(placement.Interpreter, LifetimeFrame); ok || got == placement.Unknown {
		t.Fatalf("interpreter placement = %s (ok=%t), must refuse without Unknown", got, ok)
	}
	if got, ok := ObjectStore(placement.Register, placement.Stack); ok || got == placement.Unknown {
		t.Fatalf("register object store = %s (ok=%t), must refuse without Unknown", got, ok)
	}
	if got, ok := ObjectStore(placement.Stack, placement.Interpreter); ok || got == placement.Unknown {
		t.Fatalf("interpreter source store = %s (ok=%t), must refuse without Unknown", got, ok)
	}
}

func TestDemandDistinguishesAuthenticatedUnknownFromInvalid(t *testing.T) {
	if got, forced, ok := Demand(LifetimeUnknown); !ok || !forced || got != placement.Unknown {
		t.Fatalf("authenticated unknown demand = %s (forced=%t, ok=%t), want Unknown,true,true", got, forced, ok)
	}
	if got, forced, ok := Demand(LifetimeInvalid); ok || forced || got == placement.Unknown {
		t.Fatalf("invalid demand = %s (forced=%t, ok=%t), must refuse without Unknown", got, forced, ok)
	}
}

func TestAuthenticatedSourceRejectsSparseOrMalformedCells(t *testing.T) {
	var zero valuedomain.Value
	for _, test := range []struct {
		name      string
		present   bool
		available bool
	}{
		{name: "sparse", present: false, available: true},
		{name: "unavailable", present: true, available: false},
		{name: "both absent", present: false, available: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := authenticatedSource(nil, zero, test.present, test.available); ok {
				t.Fatal("source gate accepted an unauthenticated cell")
			}
		})
	}
	if _, ok := authenticatedSource(nil, zero, true, true); ok {
		t.Fatal("source gate accepted a malformed Value")
	}
}
