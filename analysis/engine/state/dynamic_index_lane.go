package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type dynamicIndexLane struct {
	values map[dynamicindex.Key]dynamicindex.Fact
	top    bool
}

func dynamicIndexLaneFromMap(
	domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
	values map[dynamicindex.Key]dynamicindex.Fact,
) dynamicIndexLane {
	if domain.Equal(values, domain.Top()) {
		return dynamicIndexLane{top: true}
	}
	return dynamicIndexLane{values: values}
}

func (l dynamicIndexLane) asMap(domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact]) map[dynamicindex.Key]dynamicindex.Fact {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l dynamicIndexLane) read(reg *axis.Registry, key dynamicindex.Key) dynamicindex.Fact {
	if key.Table == "" {
		return dynamicindex.Bottom(reg)
	}
	if l.top {
		return dynamicindex.Top()
	}
	if fact, ok := l.values[key]; ok {
		return fact
	}
	return dynamicindex.Bottom(reg)
}

func (l dynamicIndexLane) hasFinite(key dynamicindex.Key) bool {
	if l.top {
		return false
	}
	_, ok := l.values[key]
	return ok
}

func (l dynamicIndexLane) without(key dynamicindex.Key) (dynamicIndexLane, bool) {
	values, changed := dynamicindex.DeleteEntry(l.values, key)
	if !changed {
		return l, false
	}
	l.values = values
	return l, true
}

func (l dynamicIndexLane) with(key dynamicindex.Key, fact dynamicindex.Fact) dynamicIndexLane {
	values := dynamicindex.CloneMap(l.values)
	if values == nil {
		values = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
	}
	values[key] = fact
	l.values = values
	return l
}

func (l dynamicIndexLane) cloneValues() map[dynamicindex.Key]dynamicindex.Fact {
	if l.top {
		return nil
	}
	return dynamicindex.CloneMap(l.values)
}
