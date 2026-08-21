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
		if got := Apply(current, LifetimeFrame); got != current {
			t.Fatalf("frame store changed %s to %s", current, got)
		}
	}
}

func TestStorageLifetimeDemandIsOrdered(t *testing.T) {
	cases := []struct {
		lifetime Lifetime
		want     placement.Placement
	}{
		{LifetimeModule, placement.OwnedHeap},
		{LifetimeGlobal, placement.SharedHeap},
		{LifetimeExternal, placement.Unknown},
		{LifetimeUnknown, placement.Unknown},
	}
	for _, test := range cases {
		if got := Apply(placement.Stack, test.lifetime); got != test.want {
			t.Errorf("Apply(stack,%s) = %s, want %s", test.lifetime, got, test.want)
		}
	}
}

func TestStorageLifetimeNeverDowngradesAnotherEscape(t *testing.T) {
	for _, lifetime := range []Lifetime{LifetimeModule, LifetimeGlobal, LifetimeExternal, LifetimeUnknown} {
		for _, current := range []placement.Placement{placement.OwnedHeap, placement.SharedHeap, placement.Unknown} {
			got := Apply(current, lifetime)
			if !placement.LessOrEq(current, got) {
				t.Errorf("Apply(%s,%s) = %s, downgraded current placement", current, lifetime, got)
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
		if got := ObjectStore(test.destination, test.source); got != test.want {
			t.Errorf("ObjectStore(%s,%s) = %s, want %s", test.destination, test.source, got, test.want)
		}
	}
	if got := ObjectStore(placement.Stack, placement.Stack); got == placement.OwnedHeap || got == placement.SharedHeap {
		t.Fatalf("ordinary local object store was promoted to %s", got)
	}
}

func TestStorageRejectsUnclassifiedAndJITInputsConservatively(t *testing.T) {
	if got := Apply(placement.Stack, LifetimeInvalid); got != placement.Unknown {
		t.Fatalf("invalid lifetime = %s, want unknown", got)
	}
	if got := Apply(placement.Interpreter, LifetimeFrame); got != placement.Unknown {
		t.Fatalf("interpreter placement = %s, want unknown", got)
	}
	if got := ObjectStore(placement.Register, placement.Stack); got != placement.Unknown {
		t.Fatalf("register object store = %s, want unknown", got)
	}
}
