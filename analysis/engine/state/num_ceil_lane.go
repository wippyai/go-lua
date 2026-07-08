package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/numceil"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type numCeilLane struct {
	lane lift.MustMapLane[keyspace.Key, numceil.Ceiling]
}

func numCeilLaneFromLift(lane lift.MustMapLane[keyspace.Key, numceil.Ceiling]) numCeilLane {
	return numCeilLane{lane: lane}
}

func (l numCeilLane) asLift() lift.MustMapLane[keyspace.Key, numceil.Ceiling] {
	return l.lane
}

func (l numCeilLane) reachable() numCeilLane {
	if !l.lane.Bottom() {
		return l
	}
	return numCeilLane{lane: lift.MustMapValues(mapedit.Clone(l.lane.Values()))}
}

func (l numCeilLane) snapshot(ks *keyspace.KeySpace) NumCeilsSnapshot {
	out := NumCeilsSnapshot{Bottom: l.lane.Bottom()}
	if out.Bottom {
		return out
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return out
	}
	out.Ceils = make(map[pathaddr.StateKey]int64, len(values))
	for key, ceil := range values {
		stateKey, ok := pathaddr.StateKeyFromPathKey(ks.Format(key))
		if !ok {
			continue
		}
		out.Ceils[stateKey] = ceil.Hi
	}
	return out
}

func (l numCeilLane) read(key keyspace.Key) (int64, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return 0, false
	}
	ceil, ok := l.lane.Values()[key]
	if !ok {
		return 0, false
	}
	return ceil.Hi, true
}

func (l numCeilLane) write(key keyspace.Key, hi int64) (numCeilLane, bool) {
	if key.Kind == keyspace.KindInvalid {
		return l, false
	}
	if !l.lane.Bottom() {
		if existing, ok := l.lane.Values()[key]; ok && existing.Hi <= hi {
			return l, false
		}
	}
	ceils := mapedit.Clone(l.lane.Values())
	if ceils == nil {
		ceils = make(map[keyspace.Key]numceil.Ceiling, 1)
	}
	ceils[key] = numceil.Ceiling{Hi: hi}
	l.lane = lift.MustMapValues(ceils)
	return l, true
}

func (l numCeilLane) clear(key keyspace.Key) (numCeilLane, bool) {
	if key.Kind == keyspace.KindInvalid || l.lane.Bottom() {
		return l, false
	}
	ceils, changed := mapedit.Without(l.lane.Values(), key)
	if !changed {
		return l, false
	}
	l.lane = lift.MustMapValues(ceils)
	return l, true
}

func (l numCeilLane) rekey(from, to *keyspace.KeySpace) numCeilLane {
	if from == nil || to == nil || from == to || l.lane.Bottom() {
		return l
	}
	values := l.lane.Values()
	if len(values) == 0 {
		return l
	}
	rekeyed := make(map[keyspace.Key]numceil.Ceiling, len(values))
	for key, ceil := range values {
		next, ok := to.FromStateKey(from.Format(key))
		if !ok {
			continue
		}
		rekeyed[next] = ceil
	}
	return numCeilLane{lane: lift.MustMapValues(rekeyed)}
}

func numCeilMapDomain(thresholds []int64) lattice.Lattice[numCeilLane] {
	return wrapDomain(numceil.MapDomain(thresholds), numCeilLaneFromLift, numCeilLane.asLift)
}
