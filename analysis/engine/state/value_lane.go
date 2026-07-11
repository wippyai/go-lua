package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

type valueLane struct {
	top     bool
	symbols map[key.Value]product.Value
	returns map[key.Value]product.Value
}

func valueLaneDomain(reg *axis.Registry) lattice.Lattice[valueLane] {
	domain := liftMapValueDomain(reg)
	return lattice.Lattice[valueLane]{
		Bottom: func() valueLane {
			return valueLane{}
		},
		Top: func() valueLane {
			return valueLane{top: true}
		},
		Equal: func(a, b valueLane) bool {
			if a.top || b.top {
				return a.top && b.top
			}
			return domain.Equal(a.symbols, b.symbols) && domain.Equal(a.returns, b.returns)
		},
		Same: func(a, b valueLane) bool {
			if a.top || b.top {
				return a.top && b.top
			}
			return domain.Same(a.symbols, b.symbols) && domain.Same(a.returns, b.returns)
		},
		LessOrEq: func(a, b valueLane) bool {
			if b.top {
				return true
			}
			if a.top {
				return false
			}
			return domain.LessOrEq(a.symbols, b.symbols) && domain.LessOrEq(a.returns, b.returns)
		},
		Join: func(a, b valueLane) valueLane {
			if a.top || b.top {
				return valueLane{top: true}
			}
			return valueLane{
				symbols: domain.Join(a.symbols, b.symbols),
				returns: domain.Join(a.returns, b.returns),
			}
		},
		Widen: func(prev, next valueLane) valueLane {
			if prev.top || next.top {
				return valueLane{top: true}
			}
			return valueLane{
				symbols: domain.Widen(prev.symbols, next.symbols),
				returns: domain.Widen(prev.returns, next.returns),
			}
		},
	}
}

func liftMapValueDomain(reg *axis.Registry) lattice.Lattice[map[key.Value]product.Value] {
	return lift.Map[key.Value, product.Value](product.Domain(reg))
}

func (l valueLane) read(reg *axis.Registry, slot key.Value) product.Value {
	if slot == 0 {
		return product.Bottom(reg)
	}
	if l.top {
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
		values, changed := mapedit.Without(l.valuesFor(slot), slot)
		if !changed {
			return l, false
		}
		l.setValuesFor(slot, values)
		return l, true
	}
	if valueDomain.Equal(l.read(reg, slot), value) {
		return l, false
	}
	l.setValuesFor(slot, mapedit.With(l.valuesFor(slot), slot, value))
	return l, true
}

func (l valueLane) seed(reg *axis.Registry, seeds []ValueSeed) (valueLane, bool) {
	valueDomain := product.Domain(reg)
	bottom := valueDomain.Bottom()
	changed := false
	symbols := l.symbols
	returns := l.returns
	symbolsCloned := false
	returnsCloned := false
	ensureValues := func(slot key.Value) map[key.Value]product.Value {
		if valueSlotIsReturn(slot) {
			if !returnsCloned {
				returns = mapedit.Clone(returns)
				returnsCloned = true
			}
			if returns == nil {
				returns = make(map[key.Value]product.Value)
			}
			changed = true
			return returns
		}
		if !symbolsCloned {
			symbols = mapedit.Clone(symbols)
			symbolsCloned = true
		}
		if symbols == nil {
			symbols = make(map[key.Value]product.Value)
		}
		changed = true
		return symbols
	}
	for _, seed := range seeds {
		if seed.Slot == 0 {
			continue
		}
		current, ok := valueLaneMapGet(valuesForSeed(seed.Slot, symbols, returns), seed.Slot)
		if !ok {
			current = bottom
		}
		if !valueDomain.Equal(current, bottom) {
			continue
		}
		if valueDomain.Equal(seed.Value, bottom) {
			if ok {
				delete(ensureValues(seed.Slot), seed.Slot)
			}
			continue
		}
		ensureValues(seed.Slot)[seed.Slot] = seed.Value
	}
	if !changed {
		return l, false
	}
	l.symbols = symbols
	l.returns = returns
	return l, true
}

func valueSlotIsReturn(slot key.Value) bool {
	_, ok := key.ParseReturnSlot(slot)
	return ok
}

func (l valueLane) get(slot key.Value) (product.Value, bool) {
	return valueLaneMapGet(l.valuesFor(slot), slot)
}

func valueLaneMapGet(values map[key.Value]product.Value, slot key.Value) (product.Value, bool) {
	value, ok := values[slot]
	return value, ok
}

func valuesForSeed(slot key.Value, symbols, returns map[key.Value]product.Value) map[key.Value]product.Value {
	if valueSlotIsReturn(slot) {
		return returns
	}
	return symbols
}

func (l valueLane) valuesFor(slot key.Value) map[key.Value]product.Value {
	if valueSlotIsReturn(slot) {
		return l.returns
	}
	return l.symbols
}

func (l *valueLane) setValuesFor(slot key.Value, values map[key.Value]product.Value) {
	if valueSlotIsReturn(slot) {
		l.returns = values
		return
	}
	l.symbols = values
}

func (l valueLane) hasFinite(slot key.Value) bool {
	if l.top {
		return false
	}
	_, ok := l.get(slot)
	return ok
}

func (l valueLane) cloneValues() map[key.Value]product.Value {
	if l.top {
		return nil
	}
	total := len(l.symbols) + len(l.returns)
	if total == 0 {
		return nil
	}
	out := make(map[key.Value]product.Value, total)
	for slot, value := range l.symbols {
		out[slot] = value
	}
	for slot, value := range l.returns {
		out[slot] = value
	}
	return out
}
