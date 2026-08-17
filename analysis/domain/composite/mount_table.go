package composite

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// MountStage names the phase of the one mount transaction that rejected.
type MountStage uint8

const (
	MountStageNone MountStage = iota
	MountStageTable
	MountStageInput
	MountStageAxis
	MountStageAdopt
)

func (stage MountStage) String() string {
	switch stage {
	case MountStageTable:
		return "table"
	case MountStageInput:
		return "input"
	case MountStageAxis:
		return "axis"
	case MountStageAdopt:
		return "adopt"
	default:
		return "none"
	}
}

// MountFailure is the closed verdict of one rejected mount phase. It names the
// phase, for an axis phase the exact axis, and carries that axis's own domain
// rejection evidence erased. No sealed authority or partially mounted record
// escapes with it.
type MountFailure struct {
	Stage MountStage
	Axis  DiagnosticAxis

	// reason is the rejecting domain's own evidence, produced by its mount hook
	// and recovered at that domain's type by MountRejection. The composition
	// never reads it, so no domain rejection vocabulary enters this record.
	reason axis.Cell
}

func (failure MountFailure) Available() bool { return failure.Stage != MountStageNone }

func (failure MountFailure) String() string {
	if !failure.Available() {
		return "none"
	}
	if failure.Stage == MountStageAxis && failure.Axis != DiagnosticAxisUnknown {
		return failure.Stage.String() + "/" + failure.Axis.String()
	}
	return failure.Stage.String()
}

// MountRejection recovers a rejecting domain's own evidence at that domain's
// type. A caller that names the wrong type, or one that asks a verdict which
// carries no domain evidence, receives the absent value rather than a guess.
func MountRejection[R any](failure MountFailure) (R, bool) {
	var absent R
	if !failure.Available() || !failure.reason.Available() {
		return absent, false
	}
	return axis.Payload[R](failure.reason)
}

// MountLink runs the declaration table's Link mount phase. Every axis that
// declares a mount seals its own Link authority from the neutral mounted
// artifact view and its declared dependencies, in the order those dependencies
// admit, and the sealed authority is adopted into the record the binding
// transaction consumes. An axis that declares no mount is passed over and the
// authority its caller supplied stays exactly as supplied.
//
// The phase is the composition's, the seal is the domain's: this function names
// no domain's mount procedure and constructs no domain's mount row.
func MountLink(inputs LinkInputs) (LinkInputs, MountFailure) {
	sealRegistry()
	if registry.sealed == nil {
		return LinkInputs{}, MountFailure{Stage: MountStageTable}
	}
	if !inputs.mountable() {
		return LinkInputs{}, MountFailure{Stage: MountStageInput}
	}
	cells, failure := mountAxes(inputs)
	if failure.Available() {
		return LinkInputs{}, failure
	}
	mounted, failedAxis, ok := inputs.adopt(cells)
	if !ok {
		return LinkInputs{}, MountFailure{Stage: MountStageAdopt, Axis: failedAxis}
	}
	return mounted, MountFailure{}
}

// mountAxes invokes every declared mount exactly once, in the order the
// declared dependency edges admit, and returns each sealed authority in its own
// axis's cell.
//
// Each mount receives the phase's neutral input half plus exactly the
// authorities its own declared dependencies sealed. The order is therefore not
// a convenience: an axis whose dependency has not yet sealed could not read it,
// and an axis that reads a peer it declared no edge to reads nothing at all and
// rejects with its own evidence.
func mountAxes(inputs LinkInputs) (axisCells, MountFailure) {
	order, orderOK := axis.DependencyOrder(registry.axes)
	if !orderOK {
		return nil, MountFailure{Stage: MountStageTable}
	}
	cells := newAxisCells(registry.axes)
	neutral := inputs.neutral()
	for _, entry := range order {
		slot, slotOK := axisSlotForKey(entry.Key())
		if !slotOK {
			return nil, MountFailure{Stage: MountStageTable}
		}
		scoped, failedAxis, scopedOK := neutral.install(dependencyCells(entry, cells))
		if !scopedOK {
			return nil, MountFailure{Stage: MountStageAdopt, Axis: failedAxis}
		}
		authority, rejection, ok := entry.Mount(scoped)
		if !ok {
			return nil, MountFailure{Stage: MountStageAxis, Axis: DiagnosticAxis(slot), reason: rejection}
		}
		if !entry.MountDeclared() {
			continue
		}
		cells[slot] = authority
	}
	return cells, MountFailure{}
}

// dependencyCells selects the authorities one axis declared an edge to. A
// dependency that sealed nothing contributes nothing: the dependent's own seal
// is the authority on what an absent peer means, and it states that in its own
// rejection evidence.
func dependencyCells(entry *axisTemplate, sealed axisCells) axisCells {
	scoped := newAxisCells(registry.axes)
	for index := 0; index < entry.DependencyCount(); index++ {
		key, keyOK := entry.DependencyAt(index)
		if !keyOK {
			continue
		}
		slot, slotOK := axisSlotForKey(key)
		if !slotOK || slot >= len(sealed) {
			continue
		}
		scoped[slot] = sealed[slot]
	}
	return scoped
}

// axisAdopter recovers one sealed authority at the exact type its owner sealed
// it as and writes it to the Link input record's own field. It restates no part
// of the mount itself.
type axisAdopter func(LinkInputs, axis.Cell) (LinkInputs, bool)

// axisAdopterFor is the Link input record's typed instantiation table: one
// adopter per axis that seals its own authority, named by that axis's declared
// key.
//
// The table's membership is the mount phase's coverage law: an axis that
// declares a mount and has no adopter here could never reach the binding
// transaction, and an adopter for an axis that declares no mount would install
// an authority nobody sealed. The surface's own law states both. The mount path
// itself reads the dense slot-indexed projection the seal builds from this
// table, so no lookup on that path is a map.
func axisAdopterFor(key schema.Key) (axisAdopter, bool) {
	switch key {
	case axisKeyValue:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			schema, ok := axis.Payload[*valuedomain.Schema](cell)
			if !ok || schema == nil {
				return LinkInputs{}, false
			}
			inputs.ValueSchema = schema
			return inputs, true
		}, true
	case axisKeyHeap:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			schema, ok := axis.Payload[heapdomain.Schema](cell)
			if !ok || !schema.Valid() {
				return LinkInputs{}, false
			}
			inputs.HeapSchema = schema
			return inputs, true
		}, true
	case axisKeyPack:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			schema, ok := axis.Payload[*packdomain.Schema](cell)
			if !ok || schema == nil {
				return LinkInputs{}, false
			}
			inputs.PackSchema = schema
			return inputs, true
		}, true
	case axisKeyCall:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			algebra, ok := axis.Payload[*calldomain.Algebra](cell)
			if !ok || algebra == nil || !algebra.Valid() {
				return LinkInputs{}, false
			}
			inputs.CallAlgebra = algebra
			return inputs, true
		}, true
	case axisKeyEffect:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			algebra, ok := axis.Payload[*effectfactor.Algebra](cell)
			if !ok || algebra == nil || !algebra.Valid() {
				return LinkInputs{}, false
			}
			inputs.EffectAlgebra = algebra
			return inputs, true
		}, true
	default:
		return nil, false
	}
}

// axisAdopterTable projects the authored adopters onto the axis slots of one
// inventory. Slot zero names no axis, so the row count is one more than the
// inventory size and an axis with no adopter leaves its own row absent.
func axisAdopterTable(entries []*axisTemplate) []axisAdopter {
	adopters := make([]axisAdopter, len(entries)+1)
	for position, entry := range entries {
		adopter, declared := axisAdopterFor(entry.Key())
		if !declared {
			continue
		}
		adopters[position+1] = adopter
	}
	return adopters
}

// install writes each supplied authority into the record through its own axis's
// adopter, in the sealed table's catalog order. It names no domain, and it
// installs nothing for an axis that supplied no cell.
func (inputs LinkInputs) install(cells axisCells) (LinkInputs, DiagnosticAxis, bool) {
	for slot := 1; slot < len(cells); slot++ {
		cell := cells[slot]
		if !cell.Available() {
			continue
		}
		if slot >= len(registry.axisAdopters) || registry.axisAdopters[slot] == nil {
			return LinkInputs{}, DiagnosticAxis(slot), false
		}
		adopted, ok := registry.axisAdopters[slot](inputs, cell)
		if !ok {
			return LinkInputs{}, DiagnosticAxis(slot), false
		}
		inputs = adopted
	}
	return inputs, DiagnosticAxisUnknown, true
}

// adopt installs the mount phase's sealed authorities into the record the
// binding transaction consumes and states the phase's coverage: an axis that
// owns its authority and sealed none leaves the record incomplete, and the
// phase says so at that axis.
func (inputs LinkInputs) adopt(cells axisCells) (LinkInputs, DiagnosticAxis, bool) {
	for position, entry := range registry.axes {
		slot := position + 1
		if slot >= len(registry.axisAdopters) || registry.axisAdopters[slot] == nil {
			continue
		}
		if entry.MountDeclared() && (slot >= len(cells) || !cells[slot].Available()) {
			return LinkInputs{}, DiagnosticAxis(slot), false
		}
	}
	return inputs.install(cells)
}
