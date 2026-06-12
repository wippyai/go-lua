package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// State carries point-local abstract values. Absence in either lane denotes
// product.Bottom for the registry used by the caller.
type State struct {
	values map[key.Value]product.Value
	paths  map[pathdom.PathKey]product.Value

	valuesTop bool
	pathsTop  bool
}

// Clone returns an independent copy of the finite lanes in s.
func (s State) Clone() State {
	return State{
		values:    cloneValueMap(s.values),
		paths:     clonePathMap(s.paths),
		valuesTop: s.valuesTop,
		pathsTop:  s.pathsTop,
	}
}

// ReadValue reads a value slot. Missing slots read as product.Bottom(reg).
func (s State) ReadValue(reg *axis.Registry, slot key.Value) product.Value {
	if slot == "" {
		return product.Bottom(reg)
	}
	if s.valuesTop {
		return product.Top()
	}
	if v, ok := s.values[slot]; ok {
		return v
	}
	return product.Bottom(reg)
}

// WriteValue returns a state with slot updated. Writing product.Bottom(reg)
// removes the finite entry so absence remains the canonical bottom spelling.
func (s State) WriteValue(reg *axis.Registry, slot key.Value, value product.Value) State {
	if slot == "" {
		return s
	}
	if s.valuesTop {
		panic("state: cannot finite-write value slot into top value lane")
	}
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		values, changed := deleteValueEntry(s.values, slot)
		if !changed {
			return s
		}
		out := s
		out.values = values
		return out
	}
	values := cloneValueMap(s.values)
	if values == nil {
		values = make(map[key.Value]product.Value, 1)
	}
	values[slot] = value
	out := s
	out.values = values
	return out
}

// UpdateValue reads slot, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdateValue(reg *axis.Registry, slot key.Value, fn func(product.Value) product.Value) State {
	if slot == "" {
		return s
	}
	return s.WriteValue(reg, slot, fn(s.ReadValue(reg, slot)))
}

// ReadReturnSlot reads a non-symbol return value slot.
func (s State) ReadReturnSlot(reg *axis.Registry, index int) product.Value {
	return s.ReadValue(reg, key.ReturnSlot(index))
}

// WriteReturnSlot writes a non-symbol return value slot.
func (s State) WriteReturnSlot(reg *axis.Registry, index int, value product.Value) State {
	return s.WriteValue(reg, key.ReturnSlot(index), value)
}

// UpdateReturnSlot reads a non-symbol return value slot, applies fn, and writes
// the transformed value.
func (s State) UpdateReturnSlot(reg *axis.Registry, index int, fn func(product.Value) product.Value) State {
	return s.UpdateValue(reg, key.ReturnSlot(index), fn)
}

// Domain builds the State lattice as the product of two pointwise map lattices
// over product.Value.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	valueDomain := product.Domain(reg)
	ops := domainOps{
		values: lift.Map[key.Value, product.Value](valueDomain),
		paths:  lift.Map[pathdom.PathKey, product.Value](valueDomain),
	}
	return lattice.Lattice[State]{
		Bottom: func() State {
			return State{}
		},
		Top: func() State {
			return State{valuesTop: true, pathsTop: true}
		},
		Equal: func(a, b State) bool {
			return ops.values.Equal(ops.valueLane(a), ops.valueLane(b)) &&
				ops.paths.Equal(ops.pathLane(a), ops.pathLane(b))
		},
		LessOrEq: func(a, b State) bool {
			return ops.values.LessOrEq(ops.valueLane(a), ops.valueLane(b)) &&
				ops.paths.LessOrEq(ops.pathLane(a), ops.pathLane(b))
		},
		Join: func(a, b State) State {
			return ops.fromLanes(
				ops.values.Join(ops.valueLane(a), ops.valueLane(b)),
				ops.paths.Join(ops.pathLane(a), ops.pathLane(b)),
			)
		},
		Widen: func(prev, next State) State {
			return ops.fromLanes(
				ops.values.Widen(ops.valueLane(prev), ops.valueLane(next)),
				ops.paths.Widen(ops.pathLane(prev), ops.pathLane(next)),
			)
		},
	}
}

type domainOps struct {
	values lattice.Lattice[map[key.Value]product.Value]
	paths  lattice.Lattice[map[pathdom.PathKey]product.Value]
}

func (o domainOps) valueLane(s State) map[key.Value]product.Value {
	if s.valuesTop {
		return o.values.Top()
	}
	return s.values
}

func (o domainOps) pathLane(s State) map[pathdom.PathKey]product.Value {
	if s.pathsTop {
		return o.paths.Top()
	}
	return s.paths
}

func (o domainOps) fromLanes(
	values map[key.Value]product.Value,
	paths map[pathdom.PathKey]product.Value,
) State {
	out := State{}
	if o.values.Equal(values, o.values.Top()) {
		out.valuesTop = true
	} else {
		out.values = values
	}
	if o.paths.Equal(paths, o.paths.Top()) {
		out.pathsTop = true
	} else {
		out.paths = paths
	}
	return out
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

func clonePathMap(in map[pathdom.PathKey]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
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
