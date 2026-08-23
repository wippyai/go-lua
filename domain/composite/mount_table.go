package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// MountStage names the phase of the one mount transaction that rejected.
type MountStage uint8

const (
	MountStageNone MountStage = iota
	MountStageTable
	MountStageInput
	MountStageAxis
	MountStageAdopt
	// MountStageTopology and MountStageFormal are the phase's own post-mount
	// derivations. Each names the derivation that refused, which is the whole of
	// what a derivation over several sealed factors can blame: no axis owns one,
	// so no axis is named beside it.
	MountStageTopology
	MountStageFormal
	MountStageComposition
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
	case MountStageTopology:
		return "topology"
	case MountStageFormal:
		return "formal"
	case MountStageComposition:
		return "composition"
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
	if (failure.Stage == MountStageAxis || failure.Stage == MountStageAdopt) && failure.Axis != DiagnosticAxisUnknown {
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
// The phase closes on the derivations no axis owns: an authority derived from
// several sealed factors at once has no single owning axis to mount it, so the
// phase derives it itself once every mount has sealed.
//
// The phase is the composition's, the seal is the domain's: this function names
// no domain's mount procedure and constructs no domain's mount row.
func MountLink(compilation Compilation, inputs LinkInputs) (LinkInputs, MountFailure) {
	state := compilation.catalog
	if state == nil || state.sealed == nil || !state.structureOK {
		return LinkInputs{}, MountFailure{Stage: MountStageTable}
	}
	inputs.vocabulary = state.structure
	if !inputs.mountable() {
		return LinkInputs{}, MountFailure{Stage: MountStageInput}
	}
	cells, failure := mountAxes(state, inputs)
	if failure.Available() {
		return LinkInputs{}, failure
	}
	mounted, failedAxis, ok := inputs.adopt(state, cells)
	if !ok {
		return LinkInputs{}, MountFailure{Stage: MountStageAdopt, Axis: failedAxis}
	}
	return mounted.derive()
}

// derive runs the phase's post-mount derivations over the record every declared
// mount has already sealed into. Each derivation is opened by the package that
// owns it, from the mounted authorities and the neutral artifact view alone, and
// a refusal names that derivation rather than an axis: the factors it reads
// belong to several axes at once, so no axis is answerable for the join.
//
// Ordering is the reason this is a phase of its own. A derivation reads sealed
// authorities from axes that mount in different positions of the dependency
// order, so it can only run once the whole order has run.
func (inputs LinkInputs) derive() (LinkInputs, MountFailure) {
	// The Heap key/class projection is a pure function of the sealed Heap and
	// Value schemas, so it is one of this phase's derivations rather than
	// something each binding rebuilds for itself. Both readers - the index
	// topology and the closed-allocation rule - receive this one seal.
	keySelection, keySelectionOK := keymatch.NewSelectorProjection(inputs.HeapSchema, inputs.ValueSchema)
	if !keySelectionOK {
		return LinkInputs{}, MountFailure{Stage: MountStageTopology}
	}
	topology, topologyOK := heapindex.Seal(inputs.HeapSchema, inputs.ValueSchema, inputs.CallAlgebra, inputs.PackSchema, keySelection)
	if !topologyOK {
		return LinkInputs{}, MountFailure{Stage: MountStageTopology}
	}
	if inputs.Source == nil {
		return LinkInputs{}, MountFailure{Stage: MountStageFormal}
	}
	// Contextual Heap authority is a bounded composition derivation. The exact
	// Link directory is read from Source only after the Heap axis has mounted;
	// callers cannot provide a substitute directory or schema.
	contextSchema, contextOK := contextdomain.Seal(inputs.HeapSchema, inputs.Source.ContextDirectory())
	if !contextOK {
		// A valid Link/Heap mount always carries a matching directory. Keep the
		// existing formal post-mount refusal boundary for malformed records so
		// this foundation does not invent a transfer/result ABI or new verdict.
		return LinkInputs{}, MountFailure{Stage: MountStageFormal}
	}
	composition, compositionOK := inputs.Source.Module().BuildComposition(inputs.Source.ContentID(), inputs.Artifacts, inputs.Source.ContextDirectory())
	if !compositionOK {
		return LinkInputs{}, MountFailure{Stage: MountStageComposition}
	}
	// Target is read exactly once from Link's finalized Boundary. Mounted actual
	// completeness is checked directly from Call's sealed MountedCall rows and
	// Pack's detached projections; no cross-domain catalogue is retained.
	boundary := inputs.Source.Boundary()
	target, targetOK := boundary.Target()
	if !targetOK || target == nil {
		return LinkInputs{}, MountFailure{Stage: MountStageFormal}
	}
	if !mountedActualsComplete(inputs.CallAlgebra, inputs.PackSchema) {
		return LinkInputs{}, MountFailure{Stage: MountStageFormal}
	}
	inputs.topology = topology
	inputs.keySelection = keySelection
	inputs.contextSchema = contextSchema
	inputs.composition = composition
	inputs.targetContract = target
	return inputs, MountFailure{}
}

// mountedActualProjectionFor resolves Pack's detached actual geometry for an
// exact Call-owned mounted receipt. It authenticates both authorities and the
// receipt before Pack is asked to project the module-scoped call occurrence.
func mountedActualProjectionFor(calls *calldomain.Algebra, packSchema *packdomain.Schema, mounted calldomain.MountedCall) (packdomain.MountedActualProjection, bool) {
	if calls == nil || !calls.Valid() || packSchema == nil || !calls.LinkOwner().Available() || !packSchema.LinkOwner().Available() ||
		!calls.LinkOwner().Matches(packSchema.LinkOwner()) || !mounted.Valid() {
		return packdomain.MountedActualProjection{}, false
	}
	_, callID, moduleID, _, _, identityOK := calls.MountedCallIdentity(mounted)
	if !identityOK || !callID.Available() || !moduleID.Available() || !calls.OwnsMountedModule(moduleID) {
		return packdomain.MountedActualProjection{}, false
	}
	projection, projectionOK := packSchema.MountedActualProjection(moduleID, callID)
	return projection, projectionOK && projection.OwnedBy(packSchema)
}

// mountedActualsComplete proves that every exact Call mounted row has Pack's
// matching owner-fenced actual projection. The zero-row case remains complete
// only when the two sealed authorities share their Link owner.
func mountedActualsComplete(calls *calldomain.Algebra, packSchema *packdomain.Schema) bool {
	if calls == nil || !calls.Valid() || packSchema == nil || !calls.LinkOwner().Available() || !packSchema.LinkOwner().Available() ||
		!calls.LinkOwner().Matches(packSchema.LinkOwner()) {
		return false
	}
	for index := 0; index < calls.MountedCallCount(); index++ {
		mounted, mountedOK := calls.MountedCallAtHandle(index)
		if !mountedOK {
			return false
		}
		if _, projectionOK := mountedActualProjectionFor(calls, packSchema, mounted); !projectionOK {
			return false
		}
	}
	return true
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
func mountAxes(state *catalog, inputs LinkInputs) (axisCells, MountFailure) {
	if state == nil {
		return nil, MountFailure{Stage: MountStageTable}
	}
	order, orderOK := axis.DependencyOrder(state.axes)
	if !orderOK {
		return nil, MountFailure{Stage: MountStageTable}
	}
	cells := newAxisCells(state.axes)
	neutral := inputs.neutral()
	for _, entry := range order {
		slot, slotOK := axisSlotForKey(state, entry.Key())
		if !slotOK {
			return nil, MountFailure{Stage: MountStageTable}
		}
		scoped, failedAxis, scopedOK := neutral.install(state, dependencyCells(state, entry, cells))
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
func dependencyCells(state *catalog, entry *axisTemplate, sealed axisCells) axisCells {
	if state == nil {
		return nil
	}
	scoped := newAxisCells(state.axes)
	var include func(schema.Key)
	include = func(key schema.Key) {
		dependency, dependencyOK := axisForKey(state, key)
		if !dependencyOK {
			return
		}
		for index := 0; index < dependency.DependencyCount(); index++ {
			prerequisite, prerequisiteOK := dependency.DependencyAt(index)
			if prerequisiteOK {
				include(prerequisite)
			}
		}
		slot, slotOK := axisSlotForKey(state, key)
		if !slotOK || slot >= len(sealed) {
			return
		}
		scoped[slot] = sealed[slot]
	}
	if entry == nil {
		return scoped
	}
	for index := 0; index < entry.DependencyCount(); index++ {
		key, keyOK := entry.DependencyAt(index)
		if keyOK {
			include(key)
		}
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
	case axisKeyPlacement:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			schema, ok := axis.Payload[placementdomain.Schema](cell)
			if !ok || !schema.Valid() || !inputs.HeapSchema.Valid() || schema.Heap().ContentID() != inputs.HeapSchema.ContentID() {
				return LinkInputs{}, false
			}
			inputs.PlacementSchema = schema
			return inputs, true
		}, true
	case axisKeyPlacementEvidence:
		return func(inputs LinkInputs, cell axis.Cell) (LinkInputs, bool) {
			schema, ok := axis.Payload[placementdomain.Schema](cell)
			if !ok || !schema.Valid() || !inputs.PlacementSchema.Valid() || schema != inputs.PlacementSchema {
				return LinkInputs{}, false
			}
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
func (inputs LinkInputs) install(state *catalog, cells axisCells) (LinkInputs, DiagnosticAxis, bool) {
	for slot := 1; slot < len(cells); slot++ {
		cell := cells[slot]
		if !cell.Available() {
			continue
		}
		if state == nil || slot >= len(state.axisAdopters) || state.axisAdopters[slot] == nil {
			return LinkInputs{}, DiagnosticAxis(slot), false
		}
		adopted, ok := state.axisAdopters[slot](inputs, cell)
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
func (inputs LinkInputs) adopt(state *catalog, cells axisCells) (LinkInputs, DiagnosticAxis, bool) {
	if state == nil {
		return LinkInputs{}, DiagnosticAxisUnknown, false
	}
	for position, entry := range state.axes {
		slot := position + 1
		if slot >= len(state.axisAdopters) || state.axisAdopters[slot] == nil {
			continue
		}
		if entry.MountDeclared() && (slot >= len(cells) || !cells[slot].Available()) {
			return LinkInputs{}, DiagnosticAxis(slot), false
		}
	}
	return inputs.install(state, cells)
}
