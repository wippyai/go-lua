package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
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
	for _, current := range []placement.Fact{
		placement.DefaultFact(),
		{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted},
		{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted},
		placement.UnknownFact(),
	} {
		if got, ok := Apply(current, LifetimeFrame); !ok || got != current {
			t.Fatalf("frame store changed %v to %v (ok=%t)", current, got, ok)
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
		want := placement.Fact{Class: test.want, RetainEscape: placement.EvidenceProven}
		if test.want == placement.Unknown {
			want = placement.UnknownFact()
		}
		if got, ok := Apply(placement.DefaultFact(), test.lifetime); !ok || got != want {
			t.Errorf("Apply(stack/refuted,%s) = %v (ok=%t), want %v", test.lifetime, got, ok, want)
		}
	}
}

func TestStorageLifetimeNeverDowngradesAnotherEscape(t *testing.T) {
	for _, lifetime := range []Lifetime{LifetimeModule, LifetimeClosure, LifetimeGlobal, LifetimeExternal, LifetimeUnknown} {
		for _, current := range []placement.Fact{
			{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted},
			{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted},
			placement.UnknownFact(),
		} {
			got, ok := Apply(current, lifetime)
			if !ok || !placement.LessOrEq(current.Class, got.Class) {
				t.Errorf("Apply(%v,%s) = %v (ok=%t), downgraded current placement", current, lifetime, got, ok)
			}
			if lifetime == LifetimeExternal || lifetime == LifetimeUnknown {
				if got != placement.UnknownFact() {
					t.Errorf("Apply(%v,%s) = %v, want authenticated UnknownFact", current, lifetime, got)
				}
			} else if got.RetainEscape != placement.EvidenceProven {
				t.Errorf("Apply(%v,%s) = %v, want retain provenance", current, lifetime, got)
			}
		}
	}
}

func TestOrdinaryObjectStoreUsesContainmentNotLifetimePromotion(t *testing.T) {
	cases := []struct {
		destination placement.Fact
		source      placement.Fact
		want        placement.Fact
	}{
		{placement.DefaultFact(), placement.DefaultFact(), placement.DefaultFact()},
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}},
		{placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}},
		{placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}, placement.DefaultFact(), placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}},
		{placement.DefaultFact(), placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}},
		// A retained container carries its prior-retain proof to a contained
		// child even though ObjectStore itself is not a new lifetime demand.
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}, placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
	}
	for _, test := range cases {
		if got, ok := ObjectStore(test.destination, test.source); !ok || got != test.want {
			t.Errorf("ObjectStore(%v,%v) = %v (ok=%t), want %v", test.destination, test.source, got, ok, test.want)
		}
	}
	if got, ok := ObjectStore(placement.DefaultFact(), placement.DefaultFact()); !ok || got.Class == placement.OwnedHeap || got.Class == placement.SharedHeap || got.RetainEscape != placement.EvidenceRefuted {
		t.Fatalf("ordinary local object store was promoted to %v", got)
	}
}

func TestStorageRejectsUnclassifiedAndJITInputsWithoutFabricatingUnknown(t *testing.T) {
	if got, ok := Apply(placement.DefaultFact(), LifetimeInvalid); ok || got.Class == placement.Unknown || got.RetainEscape == placement.EvidenceUnknown {
		t.Fatalf("invalid lifetime = %v (ok=%t), must refuse without Unknown", got, ok)
	}
	badInterpreter := placement.Fact{Class: placement.Interpreter, RetainEscape: placement.EvidenceRefuted}
	if got, ok := Apply(badInterpreter, LifetimeFrame); ok || got.Class == placement.Unknown || got.RetainEscape == placement.EvidenceUnknown {
		t.Fatalf("interpreter placement = %v (ok=%t), must refuse without Unknown", got, ok)
	}
	badRegister := placement.Fact{Class: placement.Register, RetainEscape: placement.EvidenceRefuted}
	if got, ok := ObjectStore(badRegister, placement.DefaultFact()); ok || got.Class == placement.Unknown || got.RetainEscape == placement.EvidenceUnknown {
		t.Fatalf("register object store = %v (ok=%t), must refuse without Unknown", got, ok)
	}
	if got, ok := ObjectStore(placement.DefaultFact(), badInterpreter); ok || got.Class == placement.Unknown || got.RetainEscape == placement.EvidenceUnknown {
		t.Fatalf("interpreter source store = %v (ok=%t), must refuse without Unknown", got, ok)
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
