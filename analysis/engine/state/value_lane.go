package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type valueLane struct {
	mapLane[key.Value, product.Value]
}

func valueLaneFromMap(
	domain lattice.Lattice[map[key.Value]product.Value],
	values map[key.Value]product.Value,
) valueLane {
	return valueLane{mapLaneFromMap(domain, values)}
}

func (l valueLane) read(reg *axis.Registry, slot key.Value) product.Value {
	if slot == 0 {
		return product.Bottom(reg)
	}
	if l.isTop() {
		return product.Top()
	}
	if value, ok := l.get(slot); ok {
		return value
	}
	return product.Bottom(reg)
}

func (l valueLane) write(reg *axis.Registry, slot key.Value, value product.Value) (valueLane, bool) {
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		values, changed := l.mapLane.without(slot)
		if !changed {
			return l, false
		}
		return valueLane{values}, true
	}
	if valueDomain.Equal(l.read(reg, slot), value) {
		return l, false
	}
	return valueLane{l.mapLane.with(slot, value)}, true
}
