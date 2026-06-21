package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/lenbound"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type lenFloorLane struct {
	lane lift.MustMapLane[keyspace.Key, lenbound.Floor]
}

func lenFloorLaneFromLift(lane lift.MustMapLane[keyspace.Key, lenbound.Floor]) lenFloorLane {
	return lenFloorLane{lane: lane}
}

func (l lenFloorLane) asLift() lift.MustMapLane[keyspace.Key, lenbound.Floor] {
	return l.lane
}

func (l lenFloorLane) reachable() lenFloorLane {
	if !l.lane.Bottom() {
		return l
	}
	return lenFloorLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l lenFloorLane) read(key keyspace.Key) (int64, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return 0, false
	}
	floor, ok := l.lane.Values()[key]
	if !ok || floor.Lo <= 0 {
		return 0, false
	}
	return floor.Lo, true
}

func (l lenFloorLane) write(key keyspace.Key, lo int64) (lenFloorLane, bool) {
	if key.Kind == keyspace.KindInvalid || lo <= 0 {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[key]; ok && existing.Lo >= lo {
			return l, false
		}
	}
	floors := mapedit.Clone(l.lane.Values())
	if floors == nil {
		floors = make(map[keyspace.Key]lenbound.Floor, 1)
	}
	floors[key] = lenbound.Floor{Lo: lo}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func (l lenFloorLane) rekey(from, to *keyspace.KeySpace) lenFloorLane {
	if from == nil || to == nil || from == to || l.lane.Bottom() {
		return l
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return l
	}
	rekeyed := make(map[keyspace.Key]lenbound.Floor, len(values))
	for key, floor := range values {
		next, ok := to.FromStateKey(from.Format(key))
		if !ok {
			continue
		}
		rekeyed[next] = floor
	}
	return lenFloorLane{lane: lift.MustMapValues(rekeyed)}
}

func (l lenFloorLane) clearPathKeySubtrees(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) (lenFloorLane, bool) {
	prefixKeys := prefixKeysOf(ks, prefixes)
	if len(prefixKeys) == 0 {
		return l, false
	}
	return l.clearMatching(func(candidate keyspace.Key) bool {
		for _, prefix := range prefixKeys {
			if ks.HasPrefix(candidate, prefix) {
				return true
			}
		}
		return false
	})
}

func (l lenFloorLane) clearPathKeyDescendantMutation(ks *keyspace.KeySpace, prefixes pathevidence.PathKeyDescendantInvalidationPrefixes) (lenFloorLane, bool) {
	descendants := prefixKeysOf(ks, prefixes.Descendants)
	subtrees := prefixKeysOf(ks, prefixes.Subtrees)
	if len(descendants) == 0 && len(subtrees) == 0 {
		return l, false
	}
	return l.clearMatching(func(candidate keyspace.Key) bool {
		for _, prefix := range descendants {
			if ks.HasPrefix(candidate, prefix) {
				return true
			}
		}
		for _, prefix := range subtrees {
			if ks.HasPrefix(candidate, prefix) {
				return true
			}
		}
		return false
	})
}

func prefixKeysOf(ks *keyspace.KeySpace, prefixes []pathdom.PathKey) []keyspace.Key {
	if len(prefixes) == 0 {
		return nil
	}
	out := make([]keyspace.Key, 0, len(prefixes))
	for _, prefix := range prefixes {
		key, ok := ks.FromStateKey(prefix)
		if !ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

func (l lenFloorLane) clearMatching(match func(keyspace.Key) bool) (lenFloorLane, bool) {
	if l.lane.Bottom() || len(l.lane.Values()) == 0 {
		return l, false
	}
	floors := mapedit.Clone(l.lane.Values())
	changed := false
	for key := range floors {
		if match(key) {
			delete(floors, key)
			changed = true
		}
	}
	if !changed {
		return l, false
	}
	if len(floors) == 0 {
		floors = nil
	}
	l.lane = lift.MustMapValues(floors)
	return l, true
}

func lenFloorMapDomain() lattice.Lattice[lenFloorLane] {
	return wrapDomain(lenbound.MapDomain(), lenFloorLaneFromLift, lenFloorLane.asLift)
}
