package pathevidence

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPathPresenceImplicationSnapshotHasStableTotalSemanticOrder(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	trigger := implicationTestKey(t, ks, "sym1@1.trigger")
	target := implicationTestKey(t, ks, "sym1@1.target")
	value := func(name string) product.Value {
		literal := typ.LiteralString(name)
		return typevalue.WithWitness(reg, typevalue.FromType(reg, literal), literal)
	}

	lane := Lane{}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		var changed bool
		lane, changed = lane.AddPathPresenceImplication(PathPresenceImplication{
			Trigger: trigger, TriggerPresence: presence.Present(), Target: target,
			TargetValue: value(name), HasTargetValue: true,
		})
		if !changed {
			t.Fatalf("adding %q implication reported unchanged", name)
		}
	}

	var want string
	for range 10_000 {
		got := implicationSnapshotOrder(reg, lane.PathPresenceImplicationsSnapshot(ks).Implications)
		if want == "" {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("implication snapshot order changed:\nfirst: %s\n  got: %s", want, got)
		}
	}
}

func TestPathPresenceImplicationOrderComparesEverySemanticField(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	trigger := implicationTestKey(t, ks, "sym2@1.trigger")
	target := implicationTestKey(t, ks, "sym2@1.target")
	value := func(name string) product.Value {
		literal := typ.LiteralString(name)
		return typevalue.WithWitness(reg, typevalue.FromType(reg, literal), literal)
	}

	implications := []PathPresenceImplication{
		{Trigger: trigger, TriggerPresence: presence.Present(), Target: target, TargetPresence: presence.Present()},
		{Trigger: trigger, TriggerValue: value("trigger-a"), HasTriggerValue: true, Target: target, TargetPresence: presence.Present()},
		{Trigger: trigger, TriggerValue: value("trigger-b"), HasTriggerValue: true, Target: target, TargetPresence: presence.Present()},
		{Trigger: trigger, TriggerPresence: presence.Present(), TriggerValue: value("trigger-a"), HasTriggerValue: true, HasTriggerPresence: true, Target: target, TargetPresence: presence.Present()},
		{Trigger: trigger, TriggerPresence: presence.Present(), Target: target, TargetPresence: presence.Present(), TargetValue: value("target-a"), HasTargetValue: true},
		{Trigger: trigger, TriggerPresence: presence.Present(), Target: target, TargetPresence: presence.Present(), TargetValue: value("target-b"), HasTargetValue: true},
	}
	for i := range implications {
		if !validPathPresenceImplication(implications[i]) {
			t.Fatalf("test implication %d is invalid: %#v", i, implications[i])
		}
		for j := i + 1; j < len(implications); j++ {
			if !pathPresenceImplicationLess(ks, implications[i], implications[j]) && !pathPresenceImplicationLess(ks, implications[j], implications[i]) {
				t.Fatalf("semantic implications %d and %d compare equal:\nleft:  %#v\nright: %#v", i, j, implications[i], implications[j])
			}
		}
	}
}

func implicationTestKey(t *testing.T, ks *keyspace.KeySpace, raw pathdom.PathKey) keyspace.Key {
	t.Helper()
	key, ok := ks.FromStateKey(raw)
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", raw)
	}
	return key
}

func implicationSnapshotOrder(reg *axis.Registry, implications []PathPresenceImplication) string {
	parts := make([]string, 0, len(implications))
	for _, implication := range implications {
		value, ok := typevalue.TypeOf(reg, implication.TargetValue)
		if !ok {
			parts = append(parts, "<missing>")
			continue
		}
		parts = append(parts, value.String())
	}
	return strings.Join(parts, ",")
}
