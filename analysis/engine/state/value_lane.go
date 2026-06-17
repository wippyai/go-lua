package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type valueLane struct {
	values map[key.Value]product.Value
	top    bool
}

func valueLaneFromMap(
	domain lattice.Lattice[map[key.Value]product.Value],
	values map[key.Value]product.Value,
) valueLane {
	if domain.Equal(values, domain.Top()) {
		return valueLane{top: true}
	}
	return valueLane{values: values}
}

func (l valueLane) asMap(domain lattice.Lattice[map[key.Value]product.Value]) map[key.Value]product.Value {
	if l.top {
		return domain.Top()
	}
	return l.values
}

func (l valueLane) read(reg *axis.Registry, slot key.Value) product.Value {
	if slot == "" {
		return product.Bottom(reg)
	}
	if l.top {
		return product.Top()
	}
	if value, ok := l.values[slot]; ok {
		return value
	}
	return product.Bottom(reg)
}

func (l valueLane) hasFinite(slot key.Value) bool {
	if l.top {
		return false
	}
	_, ok := l.values[slot]
	return ok
}

func (l valueLane) write(reg *axis.Registry, slot key.Value, value product.Value) (valueLane, bool) {
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		values, changed := mapedit.Without(l.values, slot)
		if !changed {
			return l, false
		}
		l.values = values
		return l, true
	}
	if valueDomain.Equal(l.read(reg, slot), value) {
		return l, false
	}
	l.values = mapedit.With(l.values, slot, value)
	return l, true
}
