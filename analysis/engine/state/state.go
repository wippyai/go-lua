package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/symbol"
)

var defaultReachableStateOps = defaultLaneCatalog.reachableOps()

// State carries point-local abstract values and facts. Missing entries in
// finite value-like lanes denote bottom for the caller's domain; must-fact
// lanes record bottom explicitly with their corresponding flags.
type State struct {
	laneMask  laneMask
	canonical bool

	values       valueLane
	pathEvidence pathevidence.Lane

	dynamicIndex      dynamicIndexLane
	heapTableIdentity heapTableIdentityLane
	frozenTables      frozenTableLane
	effectDeltas      effectDeltaLane
	escapeEvents      escapeevent.Lane
	channelSelect     channelselectfact.Lane
	storeRelations    storeRelationLane
	keyMemberships    keyMembershipLane
	typestates        typestate.Store
	placement         placementLane
	lenFloors         lenFloorLane
	numFloors         numFloorLane
	diffRelations     diffRelationLane
}

// Snapshot returns a point-in-time state value. State lanes are persistent by
// convention: exported write APIs copy any lane they change, so unchanged lanes
// can be shared safely across solver snapshots.
func (s State) Snapshot() State {
	return s
}

func (s State) laneEnabled(bit laneMask) bool {
	return s.laneMask.allows(bit)
}

// ReadValue reads a value slot. Missing slots read as product.Bottom(reg).
func (s State) ReadValue(reg *axis.Registry, slot key.Value) product.Value {
	if !s.laneEnabled(laneValuesBit) {
		return product.Bottom(reg)
	}
	return s.values.read(reg, slot)
}

// ReadSymbolValue reads the current point-local value for a lexical symbol.
func (s State) ReadSymbolValue(reg *axis.Registry, sym symbol.ID) product.Value {
	if sym == 0 {
		return product.Bottom(reg)
	}
	return s.ReadValue(reg, key.SymbolValue(sym))
}

// WriteValue returns a state with slot updated. Writing product.Bottom(reg)
// removes the finite entry so absence remains the canonical bottom spelling.
func (s State) WriteValue(reg *axis.Registry, slot key.Value, value product.Value) State {
	if slot == 0 || !s.laneEnabled(laneValuesBit) {
		return s
	}
	if s.values.top {
		panic("state: cannot finite-write value slot into top value lane")
	}
	values, changed := s.values.write(reg, slot, value)
	if !changed {
		return s
	}
	out := s.reachable()
	out.values = values
	return out
}

// ValueEdit batches point-local value writes against one State snapshot. It is
// semantically equivalent to repeated WriteValue/UpdateValue calls, including
// bottom canonicalization and read-after-write behavior, but clones the
// persistent value map at most once.
type ValueEdit struct {
	state        State
	reg          *axis.Registry
	valueDomain  lattice.Lattice[product.Value]
	enabled      bool
	changed      bool
	symbolCloned bool
	returnCloned bool
	symbols      map[key.Value]product.Value
	returns      map[key.Value]product.Value
}

// EditValues opens a value-lane edit transaction. Call Done to publish the
// edited state.
func (s State) EditValues(reg *axis.Registry) ValueEdit {
	return ValueEdit{
		state:       s,
		reg:         reg,
		valueDomain: product.Domain(reg),
		enabled:     s.laneEnabled(laneValuesBit),
	}
}

// Read returns the current value for slot, including writes already staged in
// this edit transaction.
func (e *ValueEdit) Read(slot key.Value) product.Value {
	if slot == 0 || !e.enabled {
		return e.valueDomain.Bottom()
	}
	if e.state.values.top {
		return product.Top()
	}
	if valueSlotIsReturn(slot) {
		if e.returnCloned {
			if value, ok := e.returns[slot]; ok {
				return value
			}
			return e.valueDomain.Bottom()
		}
	} else if e.symbolCloned {
		if value, ok := e.symbols[slot]; ok {
			return value
		}
		return e.valueDomain.Bottom()
	}
	return e.state.values.read(e.reg, slot)
}

// Write stages a value-lane write. Writing product bottom removes the finite
// entry, preserving the map lane's canonical absence-as-bottom spelling.
func (e *ValueEdit) Write(slot key.Value, value product.Value) {
	if slot == 0 || !e.enabled {
		return
	}
	if e.state.values.top {
		panic("state: cannot finite-write value slot into top value lane")
	}
	bottom := e.valueDomain.Bottom()
	current := e.Read(slot)
	if e.valueDomain.Equal(current, value) {
		return
	}
	values := e.ensureValuesFor(slot)
	if e.valueDomain.Equal(value, bottom) {
		delete(values, slot)
	} else {
		values[slot] = value
	}
	e.changed = true
}

func (e *ValueEdit) ensureValuesFor(slot key.Value) map[key.Value]product.Value {
	if valueSlotIsReturn(slot) {
		if !e.returnCloned {
			e.returns = mapedit.Clone(e.state.values.returns)
			e.returnCloned = true
		}
		if e.returns == nil {
			e.returns = make(map[key.Value]product.Value, 1)
		}
		return e.returns
	}
	if !e.symbolCloned {
		e.symbols = mapedit.Clone(e.state.values.symbols)
		e.symbolCloned = true
	}
	if e.symbols == nil {
		e.symbols = make(map[key.Value]product.Value, 1)
	}
	return e.symbols
}

// Update reads the current staged value, applies fn, and stages the result.
func (e *ValueEdit) Update(slot key.Value, fn func(product.Value) product.Value) {
	if slot == 0 || fn == nil {
		return
	}
	e.Write(slot, fn(e.Read(slot)))
}

// WriteReturnSlot stages a return-slot write using the same key spelling as
// State.WriteReturnSlot.
func (e *ValueEdit) WriteReturnSlot(index int, value product.Value) {
	e.Write(key.ReturnSlot(index), value)
}

// UpdateReturnSlot stages a return-slot update using the same key spelling as
// State.UpdateReturnSlot.
func (e *ValueEdit) UpdateReturnSlot(index int, fn func(product.Value) product.Value) {
	e.Update(key.ReturnSlot(index), fn)
}

// Done returns the original state if no effective writes occurred, otherwise a
// state with the staged value-lane contents published.
func (e *ValueEdit) Done() State {
	return e.DoneOn(e.state)
}

// DoneOn publishes the staged value lane onto base. This is useful when a
// transfer batches value writes while also producing non-value facts; callers
// must ensure no independent value-lane writes were made to base while the edit
// was open.
func (e *ValueEdit) DoneOn(base State) State {
	if !e.changed {
		return base
	}
	out := base.reachable()
	values := e.state.values
	if e.symbolCloned {
		values.symbols = e.symbols
	}
	if e.returnCloned {
		values.returns = e.returns
	}
	out.values = values
	return out
}

// ValueSeed is an entry-state seed for a value slot. SeedValues applies seeds
// only to slots that currently read as product.Bottom(reg), preserving precise
// caller- or context-supplied values.
type ValueSeed struct {
	Slot  key.Value
	Value product.Value
}

// SeedValues writes independent entry seeds in one value-lane update. It has
// the same "only if current value is bottom" semantics as repeatedly checking
// ReadValue then calling WriteValue, but clones the persistent value map at most
// once.
func (s State) SeedValues(reg *axis.Registry, seeds []ValueSeed) State {
	if reg == nil || len(seeds) == 0 || !s.laneEnabled(laneValuesBit) {
		return s
	}
	if s.values.top {
		panic("state: cannot seed value slots into top value lane")
	}
	values, changed := s.values.seed(reg, seeds)
	if !changed {
		return s
	}
	out := s.reachable()
	out.values = values
	return out
}

// UpdateValue reads slot, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdateValue(reg *axis.Registry, slot key.Value, fn func(product.Value) product.Value) State {
	if slot == 0 {
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

func (s State) reachable() State {
	for _, op := range defaultReachableStateOps {
		if s.laneMask.allows(op.bit) {
			s = op.markReachable(s)
		}
	}
	s.canonical = true
	return s
}
