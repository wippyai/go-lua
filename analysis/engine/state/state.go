package state

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// State carries point-local abstract values and facts. Missing entries in
// finite value-like lanes denote bottom for the caller's domain; must-fact
// lanes record bottom explicitly with their corresponding flags.
type State struct {
	values       valueLane
	pathEvidence pathevidence.Lane

	dynamicIndex      dynamicIndexLane
	heapTableIdentity heapTableIdentityLane
	frozenTables      frozenTableLane
	effectDeltas      effectDeltaLane
	escapeEvents      escapeevent.Lane
	channelSelect     channelselectfact.Lane
	storeRelations    storeRelationLane
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

// ReadValue reads a value slot. Missing slots read as product.Bottom(reg).
func (s State) ReadValue(reg *axis.Registry, slot key.Value) product.Value {
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
	if slot == 0 {
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
	s.pathEvidence = s.pathEvidence.Reachable()
	s.frozenTables = s.frozenTables.reachable()
	s.escapeEvents = s.escapeEvents.Reachable()
	s.channelSelect = s.channelSelect.Reachable()
	s.storeRelations = s.storeRelations.reachable()
	s.lenFloors = s.lenFloors.reachable()
	s.numFloors = s.numFloors.reachable()
	return s
}
