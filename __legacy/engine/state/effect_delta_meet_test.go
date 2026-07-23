package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEffectDeltaLaneRegistersExactPointwiseMeet(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneEffectDeltas})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LaneEffectDeltas)
	if !ok {
		t.Fatal("effect-deltas lane is not registered")
	}

	keys := keyspace.New()
	sharedTarget, ok := keys.FromStateKey(pathdom.PathKey("global:shared"))
	if !ok {
		t.Fatal("shared target key")
	}
	leftTarget, ok := keys.FromStateKey(pathdom.PathKey("global:left"))
	if !ok {
		t.Fatal("left target key")
	}
	shared := effectdelta.Key{Target: sharedTarget, Site: "shared", Kind: effectdelta.Mutation}
	leftOnly := effectdelta.Key{Target: leftTarget, Site: "left", Kind: effectdelta.Escape}
	present := presentValue(reg)
	absent := absentValue(reg)
	unknown := effectdelta.Value{Before: product.Top(), After: absent, Change: effectdelta.ChangeUnknown}
	changed := effectdelta.Value{Before: present, After: product.Top(), Change: effectdelta.ChangeChanged}

	leftState := State{}.
		WriteEffectDelta(shared, unknown).
		WriteEffectDelta(leftOnly, changed)
	rightState := State{}.WriteEffectDelta(shared, changed)
	left := mustOnlyLaneFactor(t, domain, leftState)
	right := mustOnlyLaneFactor(t, domain, rightState)

	met, err := domain.LaneMeet(left, right)
	if err != nil {
		t.Fatalf("registered Meet: %v", err)
	}
	got, err := domain.Compose([]LaneFactor{met})
	if err != nil {
		t.Fatal(err)
	}
	want := effectdelta.Value{Before: present, After: product.Top(), Change: effectdelta.ChangeChanged}
	// The right operand's After is Top, while the left operand constrains it to
	// absent. Build the exact expected product explicitly.
	want.After = absent
	if delta := got.ReadEffectDelta(shared); !effectdelta.Domain(reg).Equal(delta, want) {
		t.Fatalf("shared effect Meet = %#v, want %#v", delta, want)
	}
	if delta := got.ReadEffectDelta(leftOnly); !effectdelta.Domain(reg).Equal(delta, effectdelta.Bottom(reg)) {
		t.Fatalf("one-sided effect survived pointwise Meet: %#v", delta)
	}

	joined, err := domain.LaneJoin(left, right)
	if err != nil {
		t.Fatal(err)
	}
	absorbed, err := domain.LaneMeet(left, joined)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, absorbed, left, "meet absorption")

	top, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topIdentity, err := domain.LaneMeet(top, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, topIdentity, left, "top identity")

	bottom, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomAbsorbed, err := domain.LaneMeet(bottom, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, bottomAbsorbed, bottom, "bottom absorption")
}
