package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type dynamicIndexLane struct {
	mapLane[dynamicindex.Key, dynamicindex.Fact]
}

func dynamicIndexLaneFromMap(
	domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
	values map[dynamicindex.Key]dynamicindex.Fact,
) dynamicIndexLane {
	return dynamicIndexLane{mapLaneFromMap(domain, values)}
}

func (l dynamicIndexLane) read(reg *axis.Registry, key dynamicindex.Key) dynamicindex.Fact {
	if key.Table.Kind == keyspace.KindInvalid {
		return dynamicindex.Bottom(reg)
	}
	if l.isTop() {
		return dynamicindex.Top()
	}
	if fact, ok := l.get(key); ok {
		return fact
	}
	return dynamicindex.Bottom(reg)
}

func (l dynamicIndexLane) without(key dynamicindex.Key) (dynamicIndexLane, bool) {
	values, changed := l.mapLane.without(key)
	if !changed {
		return l, false
	}
	return dynamicIndexLane{values}, true
}

func (l dynamicIndexLane) with(key dynamicindex.Key, fact dynamicindex.Fact) dynamicIndexLane {
	requireNonBottomLaneValue(fact.Admission == dynamicindex.AdmissionBottom, "dynamic index", "fact")
	return dynamicIndexLane{l.mapLane.with(key, fact)}
}

func (l dynamicIndexLane) rekey(from, to *keyspace.KeySpace) (dynamicIndexLane, bool) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return l, false
	}
	if l.top || len(l.values) == 0 {
		return l, true
	}
	if from == nil || to == nil {
		return l, false
	}
	values := make(map[dynamicindex.Key]dynamicindex.Fact, len(l.values))
	for key, fact := range l.values {
		table, ok := to.ImportKey(from, key.Table)
		if !ok {
			return l, false
		}
		key.Table = table
		values[key] = fact
	}
	l.values = values
	return l, true
}
