package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
		values, changed := deleteValueEntry(l.values, slot)
		if !changed {
			return l, false
		}
		l.values = values
		return l, true
	}
	if valueDomain.Equal(l.read(reg, slot), value) {
		return l, false
	}
	values := cloneValueMap(l.values)
	if values == nil {
		values = make(map[key.Value]product.Value, 1)
	}
	values[slot] = value
	l.values = values
	return l, true
}

func cloneValueMap(in map[key.Value]product.Value) map[key.Value]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[key.Value]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deleteValueEntry(
	in map[key.Value]product.Value,
	slot key.Value,
) (map[key.Value]product.Value, bool) {
	if _, ok := in[slot]; !ok {
		return in, false
	}
	out := make(map[key.Value]product.Value, len(in)-1)
	for k, v := range in {
		if k != slot {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
