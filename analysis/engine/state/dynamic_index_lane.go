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
	if fact.Admission == dynamicindex.AdmissionBottom {
		panic("state: dynamic index lane with requires non-bottom fact")
	}
	return dynamicIndexLane{l.mapLane.with(key, fact)}
}
