package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeplacement"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// State carries point-local abstract values and facts. Missing entries in
// finite value-like lanes denote bottom for the caller's domain; must-fact
// lanes record bottom explicitly with their corresponding flags.
type State struct {
	values       map[key.Value]product.Value
	pathEvidence pathevidence.Lane

	dynamicIndex      map[dynamicindex.Key]dynamicindex.Fact
	heapTableIdentity map[identity.ID]heapidentity.TableObject
	effectDeltas      map[effectdelta.Key]effectdelta.Value
	channelSelect     channelselectfact.Lane
	escapePlacement   map[identity.ID]escapeplacement.Value

	valuesTop bool

	dynamicIndexTop      bool
	heapTableIdentityTop bool
	effectDeltasTop      bool
	escapePlacementTop   bool
}

// Clone returns an independent copy of the finite lanes in s.
func (s State) Clone() State {
	return State{
		values:               cloneValueMap(s.values),
		pathEvidence:         s.pathEvidence.Clone(),
		dynamicIndex:         dynamicindex.CloneMap(s.dynamicIndex),
		heapTableIdentity:    heapidentity.CloneMap(s.heapTableIdentity),
		effectDeltas:         effectdelta.CloneMap(s.effectDeltas),
		channelSelect:        s.channelSelect.Clone(),
		escapePlacement:      escapeplacement.CloneMap(s.escapePlacement),
		valuesTop:            s.valuesTop,
		dynamicIndexTop:      s.dynamicIndexTop,
		heapTableIdentityTop: s.heapTableIdentityTop,
		effectDeltasTop:      s.effectDeltasTop,
		escapePlacementTop:   s.escapePlacementTop,
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
		out := s.reachable()
		out.values = values
		return out
	}
	values := cloneValueMap(s.values)
	if values == nil {
		values = make(map[key.Value]product.Value, 1)
	}
	values[slot] = value
	out := s.reachable()
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

// Domain builds the State lattice as a product of value, path, must-fact,
// identity, effect, channel-select, and placement lanes.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	valueDomain := product.Domain(reg)
	ops := domainOps{
		values:            lift.Map[key.Value, product.Value](valueDomain),
		pathEvidence:      pathevidence.Domain(reg),
		dynamicIndex:      dynamicindex.MapDomain(reg),
		heapTableIdentity: heapidentity.MapDomain(reg),
		effectDeltas:      effectdelta.MapDomain(reg),
		channelSelect:     channelselectfact.Domain(),
		escapePlacement:   escapeplacement.MapDomain(),
	}
	return lattice.Lattice[State]{
		Bottom: func() State {
			return State{
				pathEvidence:  ops.pathEvidence.Bottom(),
				channelSelect: ops.channelSelect.Bottom(),
			}
		},
		Top: func() State {
			return State{
				valuesTop:            true,
				pathEvidence:         ops.pathEvidence.Top(),
				dynamicIndexTop:      true,
				heapTableIdentityTop: true,
				effectDeltasTop:      true,
				escapePlacementTop:   true,
			}
		},
		Equal: func(a, b State) bool {
			return ops.values.Equal(ops.valueLane(a), ops.valueLane(b)) &&
				ops.pathEvidence.Equal(a.pathEvidence, b.pathEvidence) &&
				ops.dynamicIndex.Equal(ops.dynamicIndexLane(a), ops.dynamicIndexLane(b)) &&
				ops.heapTableIdentity.Equal(ops.heapTableIdentityLane(a), ops.heapTableIdentityLane(b)) &&
				ops.effectDeltas.Equal(ops.effectDeltaLane(a), ops.effectDeltaLane(b)) &&
				ops.channelSelect.Equal(a.channelSelect, b.channelSelect) &&
				ops.escapePlacement.Equal(ops.escapePlacementLane(a), ops.escapePlacementLane(b))
		},
		LessOrEq: func(a, b State) bool {
			return ops.values.LessOrEq(ops.valueLane(a), ops.valueLane(b)) &&
				ops.pathEvidence.LessOrEq(a.pathEvidence, b.pathEvidence) &&
				ops.dynamicIndex.LessOrEq(ops.dynamicIndexLane(a), ops.dynamicIndexLane(b)) &&
				ops.heapTableIdentity.LessOrEq(ops.heapTableIdentityLane(a), ops.heapTableIdentityLane(b)) &&
				ops.effectDeltas.LessOrEq(ops.effectDeltaLane(a), ops.effectDeltaLane(b)) &&
				ops.channelSelect.LessOrEq(a.channelSelect, b.channelSelect) &&
				ops.escapePlacement.LessOrEq(ops.escapePlacementLane(a), ops.escapePlacementLane(b))
		},
		Join: func(a, b State) State {
			return ops.fromLanes(
				ops.values.Join(ops.valueLane(a), ops.valueLane(b)),
				ops.pathEvidence.Join(a.pathEvidence, b.pathEvidence),
				ops.dynamicIndex.Join(ops.dynamicIndexLane(a), ops.dynamicIndexLane(b)),
				ops.heapTableIdentity.Join(ops.heapTableIdentityLane(a), ops.heapTableIdentityLane(b)),
				ops.effectDeltas.Join(ops.effectDeltaLane(a), ops.effectDeltaLane(b)),
				ops.channelSelect.Join(a.channelSelect, b.channelSelect),
				ops.escapePlacement.Join(ops.escapePlacementLane(a), ops.escapePlacementLane(b)),
			)
		},
		Widen: func(prev, next State) State {
			return ops.fromLanes(
				ops.values.Widen(ops.valueLane(prev), ops.valueLane(next)),
				ops.pathEvidence.Widen(prev.pathEvidence, next.pathEvidence),
				ops.dynamicIndex.Widen(ops.dynamicIndexLane(prev), ops.dynamicIndexLane(next)),
				ops.heapTableIdentity.Widen(ops.heapTableIdentityLane(prev), ops.heapTableIdentityLane(next)),
				ops.effectDeltas.Widen(ops.effectDeltaLane(prev), ops.effectDeltaLane(next)),
				ops.channelSelect.Widen(prev.channelSelect, next.channelSelect),
				ops.escapePlacement.Widen(ops.escapePlacementLane(prev), ops.escapePlacementLane(next)),
			)
		},
	}
}

type domainOps struct {
	values            lattice.Lattice[map[key.Value]product.Value]
	pathEvidence      lattice.Lattice[pathevidence.Lane]
	dynamicIndex      lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact]
	heapTableIdentity lattice.Lattice[map[identity.ID]heapidentity.TableObject]
	effectDeltas      lattice.Lattice[map[effectdelta.Key]effectdelta.Value]
	channelSelect     lattice.Lattice[channelselectfact.Lane]
	escapePlacement   lattice.Lattice[map[identity.ID]escapeplacement.Value]
}

func (o domainOps) valueLane(s State) map[key.Value]product.Value {
	if s.valuesTop {
		return o.values.Top()
	}
	return s.values
}

func (o domainOps) dynamicIndexLane(s State) map[dynamicindex.Key]dynamicindex.Fact {
	if s.dynamicIndexTop {
		return o.dynamicIndex.Top()
	}
	return s.dynamicIndex
}

func (o domainOps) heapTableIdentityLane(s State) map[identity.ID]heapidentity.TableObject {
	if s.heapTableIdentityTop {
		return o.heapTableIdentity.Top()
	}
	return s.heapTableIdentity
}

func (o domainOps) effectDeltaLane(s State) map[effectdelta.Key]effectdelta.Value {
	if s.effectDeltasTop {
		return o.effectDeltas.Top()
	}
	return s.effectDeltas
}

func (o domainOps) escapePlacementLane(s State) map[identity.ID]escapeplacement.Value {
	if s.escapePlacementTop {
		return o.escapePlacement.Top()
	}
	return s.escapePlacement
}

func (o domainOps) fromLanes(
	values map[key.Value]product.Value,
	pathEvidence pathevidence.Lane,
	dynamicIndex map[dynamicindex.Key]dynamicindex.Fact,
	heapTableIdentity map[identity.ID]heapidentity.TableObject,
	effectDeltas map[effectdelta.Key]effectdelta.Value,
	channelSelect channelselectfact.Lane,
	escapePlacement map[identity.ID]escapeplacement.Value,
) State {
	out := State{}
	if o.values.Equal(values, o.values.Top()) {
		out.valuesTop = true
	} else {
		out.values = values
	}
	out.pathEvidence = pathEvidence
	if o.dynamicIndex.Equal(dynamicIndex, o.dynamicIndex.Top()) {
		out.dynamicIndexTop = true
	} else {
		out.dynamicIndex = dynamicIndex
	}
	if o.heapTableIdentity.Equal(heapTableIdentity, o.heapTableIdentity.Top()) {
		out.heapTableIdentityTop = true
	} else {
		out.heapTableIdentity = heapTableIdentity
	}
	if o.effectDeltas.Equal(effectDeltas, o.effectDeltas.Top()) {
		out.effectDeltasTop = true
	} else {
		out.effectDeltas = effectDeltas
	}
	out.channelSelect = channelSelect
	if o.escapePlacement.Equal(escapePlacement, o.escapePlacement.Top()) {
		out.escapePlacementTop = true
	} else {
		out.escapePlacement = escapePlacement
	}
	return out
}

func (s State) reachable() State {
	s.pathEvidence = s.pathEvidence.Reachable()
	s.channelSelect = s.channelSelect.Reachable()
	return s
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
