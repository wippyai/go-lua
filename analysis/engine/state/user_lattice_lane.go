package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type userLatticeKey struct {
	axis userlattice.AxisSlot
	path keyspace.Key
}

type userLatticeLane struct {
	values map[userLatticeKey]userlattice.Element
	top    bool
}

func (l userLatticeLane) read(axis userlattice.Axis, path keyspace.Key) userlattice.Element {
	if l.top {
		return axis.Top()
	}
	if value, ok := l.values[userLatticeKey{axis: axis.Slot(), path: path}]; ok {
		return value
	}
	return axis.Bottom()
}

func (l userLatticeLane) write(axis userlattice.Axis, path keyspace.Key, value userlattice.Element) (userLatticeLane, bool) {
	if path.Kind == keyspace.KindInvalid {
		return l, false
	}
	key := userLatticeKey{axis: axis.Slot(), path: path}
	if axis.LessOrEq(value, axis.Bottom()) && axis.LessOrEq(axis.Bottom(), value) {
		values, changed := mapedit.Without(l.values, key)
		if !changed {
			return l, false
		}
		l.values = values
		l.top = false
		return l, true
	}
	if !l.top {
		if existing, ok := l.values[key]; ok && existing == value {
			return l, false
		}
	}
	values := mapedit.Clone(l.values)
	if values == nil {
		values = make(map[userLatticeKey]userlattice.Element, 1)
	}
	values[key] = value
	l.values = values
	l.top = false
	return l, true
}

func (l userLatticeLane) applyCallBoundary(rt userlattice.Runtime) (userLatticeLane, bool) {
	if l.top || len(l.values) == 0 {
		return l, false
	}
	var out map[userLatticeKey]userlattice.Element
	changed := false
	for key, value := range l.values {
		axis, ok := rt.AxisBySlot(key.axis)
		if !ok {
			changed = true
			continue
		}
		next := axis.CallBoundary(value)
		if next != value {
			changed = true
		}
		if axis.LessOrEq(next, axis.Bottom()) && axis.LessOrEq(axis.Bottom(), next) {
			continue
		}
		if out == nil {
			out = make(map[userLatticeKey]userlattice.Element, len(l.values))
		}
		out[key] = next
	}
	if !changed {
		return l, false
	}
	l.values = out
	return l, true
}

func (l userLatticeLane) rekey(from, to *keyspace.KeySpace) userLatticeLane {
	if from == nil || to == nil || from == to || l.top || len(l.values) == 0 {
		return l
	}
	rekeyed := make(map[userLatticeKey]userlattice.Element, len(l.values))
	for key, value := range l.values {
		next, ok := to.FromStateKey(from.Format(key.path))
		if !ok {
			continue
		}
		key.path = next
		rekeyed[key] = value
	}
	l.values = rekeyed
	return l
}

func (l userLatticeLane) snapshot(rt userlattice.Runtime, ks *keyspace.KeySpace) UserLatticesSnapshot {
	if l.top {
		return UserLatticesSnapshot{Top: true}
	}
	if len(l.values) == 0 {
		return UserLatticesSnapshot{}
	}
	out := UserLatticesSnapshot{Values: make(map[userlattice.AxisID]map[pathaddr.StateKey]userlattice.ElementID)}
	for key, value := range l.values {
		axis, ok := rt.AxisBySlot(key.axis)
		if !ok {
			continue
		}
		stateKey, ok := pathaddr.StateKeyFromPathKey(ks.Format(key.path))
		if !ok {
			continue
		}
		byPath := out.Values[axis.ID()]
		if byPath == nil {
			byPath = make(map[pathaddr.StateKey]userlattice.ElementID)
			out.Values[axis.ID()] = byPath
		}
		byPath[stateKey] = axis.ElementName(value)
	}
	return out
}

func userLatticeDomain(rt userlattice.Runtime) lattice.Lattice[userLatticeLane] {
	return lattice.Lattice[userLatticeLane]{
		Bottom: func() userLatticeLane { return userLatticeLane{} },
		Top:    func() userLatticeLane { return userLatticeLane{top: true} },
		Equal: func(a, b userLatticeLane) bool {
			return userLatticeEqual(a, b)
		},
		LessOrEq: func(a, b userLatticeLane) bool {
			return userLatticeLessOrEq(rt, a, b)
		},
		Join: func(a, b userLatticeLane) userLatticeLane {
			return userLatticeJoin(rt, a, b)
		},
		Widen: func(prev, next userLatticeLane) userLatticeLane {
			return userLatticeJoin(rt, prev, next)
		},
	}
}

func userLatticeEqual(a, b userLatticeLane) bool {
	if a.top || b.top {
		return a.top && b.top
	}
	if len(a.values) != len(b.values) {
		return false
	}
	for key, av := range a.values {
		if bv, ok := b.values[key]; !ok || av != bv {
			return false
		}
	}
	return true
}

func userLatticeLessOrEq(rt userlattice.Runtime, a, b userLatticeLane) bool {
	switch {
	case b.top:
		return true
	case a.top:
		return b.top
	}
	for key, av := range a.values {
		axis, ok := rt.AxisBySlot(key.axis)
		if !ok {
			return false
		}
		bv := axis.Bottom()
		if value, ok := b.values[key]; ok {
			bv = value
		}
		if !axis.LessOrEq(av, bv) {
			return false
		}
	}
	return true
}

func userLatticeJoin(rt userlattice.Runtime, a, b userLatticeLane) userLatticeLane {
	switch {
	case a.top || b.top:
		return userLatticeLane{top: true}
	case sameUserLatticeMap(a.values, b.values):
		return a
	case len(a.values) == 0:
		return b
	case len(b.values) == 0:
		return a
	case userLatticeLessOrEq(rt, b, a):
		return a
	case userLatticeLessOrEq(rt, a, b):
		return b
	}
	out := make(map[userLatticeKey]userlattice.Element, len(a.values)+len(b.values))
	for key, av := range a.values {
		axis, ok := rt.AxisBySlot(key.axis)
		if !ok {
			continue
		}
		bv := axis.Bottom()
		if value, ok := b.values[key]; ok {
			bv = value
		}
		joined := axis.Join(av, bv)
		if !axis.LessOrEq(joined, axis.Bottom()) || !axis.LessOrEq(axis.Bottom(), joined) {
			out[key] = joined
		}
	}
	for key, bv := range b.values {
		if _, ok := a.values[key]; ok {
			continue
		}
		axis, ok := rt.AxisBySlot(key.axis)
		if !ok {
			continue
		}
		joined := axis.Join(axis.Bottom(), bv)
		if !axis.LessOrEq(joined, axis.Bottom()) || !axis.LessOrEq(axis.Bottom(), joined) {
			out[key] = joined
		}
	}
	if len(out) == 0 {
		out = nil
	}
	return userLatticeLane{values: out}
}

func sameUserLatticeMap(a, b map[userLatticeKey]userlattice.Element) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for key, av := range a {
		if bv, ok := b[key]; !ok || av != bv {
			return false
		}
	}
	return len(a) == len(b)
}
