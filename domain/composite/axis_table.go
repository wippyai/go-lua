package composite

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	denominatorpublication "github.com/wippyai/go-lua/analysis/schema/denominator/publication"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
	executionowner "github.com/wippyai/go-lua/domain/execution/owner"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	contextowner "github.com/wippyai/go-lua/domain/heap/context/owner"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type axisTemplate = axis.Template[LinkInputs]

// The axis keys the composition addresses its own inventory by. Each is the key
// the owning domain declared its axis under, so a pass that needs one axis's
// payload names it exactly as its owner spells it rather than by a factor lane
// ordinal declared somewhere else.
const (
	axisKeyValue                 schema.Key = "value"
	axisKeyPack                  schema.Key = "pack"
	axisKeyHeap                  schema.Key = "heap"
	axisKeyPlacement             schema.Key = "placement"
	axisKeyPlacementEvidence     schema.Key = "placement-suspension-evidence"
	axisKeyContext               schema.Key = "context"
	axisKeyCall                  schema.Key = "call"
	axisKeyEffect                schema.Key = "effect"
	axisKeyStaticType            schema.Key = staticowner.AxisKey
	axisKeyExecutionReachability schema.Key = "execution-reachability"
	axisKeyChannelSelectCase     schema.Key = "channel-select-case"
)

// axisCells is one pass's per-axis payload, indexed by axis slot: the axis's
// dense declaration position, numbered from one. An axis is a writer principal,
// so the slot is the principal slot and nothing here indexes by a foreign
// factor-lane enum. The cold pass fills it with fragments and the hot pass with
// bound axes.
type axisCells []axis.Cell

// newAxisCells opens one pass's payload over an inventory. Slot zero is the
// absent axis, so the row count is one more than the inventory size.
func newAxisCells(entries []*axisTemplate) axisCells { return make(axisCells, len(entries)+1) }

// available states the pass's coverage over the inventory it ran on: every
// bound axis carries its cell. An engine-published axis instantiates no factor
// binding, so the pass produces no cell for it and its absence is the declared
// storage rather than an incomplete pass.
func (cells axisCells) available(entries []*axisTemplate) bool {
	if len(cells) != len(entries)+1 {
		return false
	}
	for position, entry := range entries {
		if entry.Storage().Bound() && !cells[position+1].Available() {
			return false
		}
	}
	return true
}

// axisPayload recovers one axis's cell at its declared type. It is the single
// typed recovery the composition performs; no pass reads a cell it did not
// index by the axis's own slot.
func axisPayload[T any](cells axisCells, slot int) (T, bool) {
	var absent T
	if slot <= 0 || slot >= len(cells) {
		return absent, false
	}
	return axis.Payload[T](cells[slot])
}

// axisPayloadForKey recovers one axis's cell by the key its owner declared it
// under. The composition addresses its own axes by their declared identity, so
// no lane vocabulary outside this table names them.
func axisPayloadForKey[T any](state *catalog, cells axisCells, key schema.Key) (T, bool) {
	slot, ok := axisSlotForKey(state, key)
	if !ok {
		var absent T
		return absent, false
	}
	return axisPayload[T](cells, slot)
}

// axisTemplates is the authored analyzer axis inventory. Each row instantiates
// one owning domain's own axis declaration with this composition's Link input
// record, which that domain admits through its own need interface.
//
// The declaration itself lives with the domain that owns the factor, so the
// inventory here states membership and order alone.
func axisTemplates() ([]*axisTemplate, []axisContributor, bool) {
	var admitted []*axisTemplate
	var contributors []axisContributor
	rejected := false
	addBound := func(entry *axisTemplate, contributor axisContributor, ok bool) {
		if !ok || !contributor.complete() {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, contributor)
	}
	addPublished := func(entry *axisTemplate, ok bool) {
		if !ok {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
		contributors = append(contributors, axisContributor{})
	}
	addPublishedSpec := func(spec axis.Spec[LinkInputs]) {
		entry, ok := axis.New(spec)
		addPublished(entry, ok)
	}

	addBound(wireAxis(valueowner.AxisEntry[LinkInputs](), valueowner.DeclareAxis, valueowner.BindAxis[LinkInputs], valueowner.AlgebraAxis))
	addBound(wireAxis(packowner.AxisEntry[LinkInputs](), packowner.DeclareAxis, packowner.BindAxis[LinkInputs], packowner.AlgebraAxis))
	addBound(wireAxis(heapowner.AxisEntry[LinkInputs](), heapowner.DeclareAxis, heapowner.BindAxis[LinkInputs], heapowner.AlgebraAxis))
	addBound(wireAxis(callowner.AxisEntry[LinkInputs](), callowner.DeclareAxis, callowner.BindAxis[LinkInputs], callowner.AlgebraAxis))
	addBound(wireAxis(effectowner.AxisEntry[LinkInputs](), effectowner.DeclareAxis, effectowner.BindAxis[LinkInputs], effectowner.AlgebraAxis))
	addBound(wireAxis(placementowner.AxisEntry[LinkInputs](), placementowner.DeclareAxis, placementowner.BindAxis[LinkInputs], placementowner.AlgebraAxis))
	addBound(wireAxis(placementsuspension.AxisEntry[LinkInputs](), placementsuspension.DeclareAxis, placementsuspension.BindAxis[LinkInputs], placementsuspension.AlgebraAxis))
	addBound(wireAxis(contextowner.AxisEntry[LinkInputs](), contextowner.DeclareAxis, contextowner.BindAxis[LinkInputs], contextowner.AlgebraAxis))
	addBound(wireAxis(staticowner.AxisEntry[LinkInputs](), staticowner.DeclareAxis, staticowner.BindAxis[LinkInputs], staticowner.AlgebraAxis))
	// Engine-published axes are declared after the factor axes so artifact
	// lane ordinals keep the prefix they address. Snapshot slots are assigned
	// separately: engine-published outputs lead that range, and the first
	// engine axis is the compile-time ChannelSelect column so a select-only
	// publication can seal the prefix.
	//
	// The composition publication is that range's leading run, and it is a
	// run rather than a set: its builder seals a contiguous slot span, so a
	// column the composition does not author must be declared after every
	// column it does. An axis added in the wrong place leaves a slot the
	// composition spans and never fills, and the whole publication refuses.
	addPublishedSpec(selectapply.AxisEntry[LinkInputs]())
	addPublishedSpec(programmount.AxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.ImportAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.CacheAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.ModuleCallTransitionAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.GenerationAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.OutcomeAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.ModuleReturnStateEdgeAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.TerminalAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.ModuleExportCallableOriginAxisEntry[LinkInputs]())
	addPublishedSpec(modulecomposition.ModuleExportCallableIngressAxisEntry[LinkInputs]())
	addPublishedSpec(effectpublication.AxisEntry[LinkInputs]())
	addPublishedSpec(executionowner.AxisEntry[LinkInputs]())
	addPublishedSpec(denominatorpublication.AxisEntry[LinkInputs]())

	if rejected {
		return nil, nil, false
	}
	return admitted, contributors, true
}

// DiagnosticAxis is the closed analyzer-owned classification of one axis. It
// is the axis's slot: its dense declaration position, numbered from one.
// Unknown covers empty, foreign, and generic lifecycle failures without a bound
// analyzer axis.
type DiagnosticAxis uint8

const DiagnosticAxisUnknown DiagnosticAxis = 0

// DiagnosticAxisForKey classifies one axis by the key its owner declared it
// under.
func DiagnosticAxisForKey(compilation Compilation, key schema.Key) DiagnosticAxis {
	slot, ok := axisSlotForKey(compilation.catalog, key)
	if !ok {
		return DiagnosticAxisUnknown
	}
	return DiagnosticAxis(slot)
}

// diagnosticAxisNames is the slot-to-key table of the authored axis inventory,
// minted once. The slot is that inventory's own declaration position, so the
// name a slot carries is fixed by the table every compilation seals rather than
// by any one compilation instance.
var diagnosticAxisNames = func() []schema.Key {
	entries, _, ok := axisTemplates()
	if !ok {
		return nil
	}
	names := make([]schema.Key, len(entries)+1)
	for position, entry := range entries {
		if entry == nil {
			continue
		}
		names[position+1] = entry.Key()
	}
	return names
}()

// String spells the axis as its owner declared it. A slot outside the sealed
// inventory names no axis and stays unknown.
func (diagnostic DiagnosticAxis) String() string {
	slot := int(diagnostic)
	if slot <= 0 || slot >= len(diagnosticAxisNames) || diagnosticAxisNames[slot] == "" {
		return "unknown"
	}
	return string(diagnosticAxisNames[slot])
}

// axisAtSlot resolves one axis by its slot. The slot is the declaration
// position numbered from one, so slot zero names no axis.
func axisAtSlot(state *catalog, slot int) (*axisTemplate, bool) {
	if state == nil || state.sealed == nil || slot <= 0 || slot > len(state.axes) {
		return nil, false
	}
	entry := state.axes[slot-1]
	return entry, entry != nil
}

// axisSlotForKey resolves one axis's slot from its declared key. It is the one
// place the composition turns an authored axis identity into the dense index
// every per-axis projection is held at.
func axisSlotForKey(state *catalog, key schema.Key) (int, bool) {
	if state == nil || state.sealed == nil {
		return 0, false
	}
	for position, entry := range state.axes {
		if entry.Key() == key {
			return position + 1, true
		}
	}
	return 0, false
}

func axisForKey(state *catalog, key schema.Key) (*axisTemplate, bool) {
	slot, ok := axisSlotForKey(state, key)
	if !ok {
		return nil, false
	}
	return axisAtSlot(state, slot)
}

// AxisCount is the size of the sealed axis inventory.
func AxisCount(compilation Compilation) int {
	state := compilation.catalog
	if state == nil {
		return 0
	}
	return len(state.axes)
}

// AxisKeyAt returns the declared key of one axis at its table position. The
// position is the axis's slot less one; the key is the identity.
func AxisKeyAt(compilation Compilation, position int) (schema.Key, bool) {
	state := compilation.catalog
	if state == nil || position < 0 || position >= len(state.axes) {
		return "", false
	}
	return state.axes[position].Key(), true
}

// AxisEntryID returns one axis's stable table identity.
func AxisEntryID(compilation Compilation, key schema.Key) (schema.EntryID, bool) {
	entry, ok := axisForKey(compilation.catalog, key)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// AxisSemantic returns one axis's canonical Engine identity.
func AxisSemantic(compilation Compilation, key schema.Key) (identity.SemanticKey, bool) {
	state := compilation.catalog
	entry, ok := axisForKey(state, key)
	if !ok {
		return identity.SemanticKey{}, false
	}
	if state == nil {
		return identity.SemanticKey{}, false
	}
	return state.roles.Key(entry.Semantic())
}

// AxisMountDeclared reports whether one axis seals its own Link authority from
// the mounted artifacts. A derived inventory reads this to tell an axis whose
// authority is composed for it from one that composes its own.
func AxisMountDeclared(compilation Compilation, key schema.Key) (bool, bool) {
	entry, ok := axisForKey(compilation.catalog, key)
	if !ok {
		return false, false
	}
	return entry.MountDeclared(), true
}

// AxisStorage returns where one axis's facts live.
func AxisStorage(compilation Compilation, key schema.Key) (axis.Storage, bool) {
	entry, ok := axisForKey(compilation.catalog, key)
	if !ok {
		return axis.StorageInvalid, false
	}
	return entry.Storage(), true
}

// AxisCardinality returns the shape of one axis's key space. A later inventory
// that materializes its own coordinates reads this rather than assuming a
// dense ordinal range.
func AxisCardinality(compilation Compilation, key schema.Key) (axis.Cardinality, bool) {
	entry, ok := axisForKey(compilation.catalog, key)
	if !ok {
		return axis.CardinalityInvalid, false
	}
	return entry.Cardinality(), true
}

// AxisLifetime returns the scope one axis's facts are valid for.
func AxisLifetime(compilation Compilation, key schema.Key) (axis.Lifetime, bool) {
	entry, ok := axisForKey(compilation.catalog, key)
	if !ok {
		return axis.LifetimeInvalid, false
	}
	return entry.Lifetime(), true
}

// declareAxes runs the table's cold axis pass and returns each axis's fragment
// cell at its principal. It is the only place a factor's Schema shape is
// recorded, and it runs before the rule pass because a rule declares against
// the principals produced here.
func axisContributorFor(state *catalog, key schema.Key) (axisContributor, bool) {
	if state == nil || len(state.axisContributors) != len(state.axes) {
		return axisContributor{}, false
	}
	for index, entry := range state.axes {
		if entry != nil && entry.Key() == key {
			contributor := state.axisContributors[index]
			return contributor, contributor.complete()
		}
	}
	return axisContributor{}, false
}

func declareAxes(state *catalog, builder *engine.SchemaBuilder, roles vocabulary.Roles) (axisCells, DiagnosticAxis, bool) {
	if state == nil || state.sealed == nil {
		return nil, DiagnosticAxisUnknown, false
	}
	return declareAxisInventory(state, state.axes, builder, roles)
}

// declareAxisInventory is the cold pass over one axis inventory. A bound axis
// records its Schema shape and returns its fragment; an engine-published axis
// instantiates no factor binding, so the pass passes over it and the column its
// own publisher fills reaches the record without a cold half here.
func declareAxisInventory(state *catalog, entries []*axisTemplate, builder *engine.SchemaBuilder, roles vocabulary.Roles) (axisCells, DiagnosticAxis, bool) {
	fragments := newAxisCells(entries)
	if builder == nil {
		return fragments, DiagnosticAxisUnknown, false
	}
	for position, entry := range entries {
		if !entry.Storage().Bound() {
			continue
		}
		contributor, contributorOK := axisContributorFor(state, entry.Key())
		if !contributorOK {
			return fragments, DiagnosticAxis(position + 1), false
		}
		fragment, ok := contributor.declare(builder, roles)
		if !ok {
			return fragments, DiagnosticAxis(position + 1), false
		}
		fragments[position+1] = fragment
	}
	if !fragments.available(entries) {
		return fragments, DiagnosticAxisUnknown, false
	}
	return fragments, DiagnosticAxisUnknown, true
}

// bindAxes runs the table's hot axis pass: every axis instantiates its own
// typed factor binding and publishes the algebra of that binding. It is the
// first pass of the binding transaction, so every later pass binds against a
// declared axis rather than a hand-ordered owner sequence.
func bindAxes(state *catalog, binding *engine.SchemaBinding, fragments axisCells, inputs LinkInputs) (axisCells, DiagnosticAxis, bool) {
	if state == nil || state.sealed == nil {
		return nil, DiagnosticAxisUnknown, false
	}
	return bindAxisInventory(state, state.axes, binding, fragments, inputs)
}

// bindAxisInventory is the hot pass over one axis inventory. It binds exactly
// the axes the cold pass declared a fragment for: an engine-published axis has
// no factor binding to instantiate and no algebra to publish, so the pass
// passes over it here as well.
func bindAxisInventory(state *catalog, entries []*axisTemplate, binding *engine.SchemaBinding, fragments axisCells, inputs LinkInputs) (axisCells, DiagnosticAxis, bool) {
	bound := newAxisCells(entries)
	if binding == nil || !fragments.available(entries) {
		return bound, DiagnosticAxisUnknown, false
	}
	for position, entry := range entries {
		if !entry.Storage().Bound() {
			continue
		}
		slot := position + 1
		contributor, contributorOK := axisContributorFor(state, entry.Key())
		if !contributorOK {
			return bound, DiagnosticAxis(slot), false
		}
		hot, ok := contributor.bind(binding, inputs, fragments[slot])
		if !ok {
			return bound, DiagnosticAxis(slot), false
		}
		bound[slot] = hot
	}
	if !bound.available(entries) {
		return bound, DiagnosticAxisUnknown, false
	}
	return bound, DiagnosticAxisUnknown, true
}

// coldPrincipals projects the declared axis fragments into the rule surface's
// principal record.
func (cells axisCells) coldPrincipals(state *catalog) (principals, bool) {
	value, valueOK := axisPayloadForKey[*valueowner.SchemaFragment](state, cells, axisKeyValue)
	staticType, staticTypeOK := axisPayloadForKey[*staticowner.SchemaFragment](state, cells, axisKeyStaticType)
	call, callOK := axisPayloadForKey[*callowner.SchemaFragment](state, cells, axisKeyCall)
	heap, heapOK := axisPayloadForKey[*heapowner.SchemaFragment](state, cells, axisKeyHeap)
	placement, placementOK := axisPayloadForKey[*placementowner.SchemaFragment](state, cells, axisKeyPlacement)
	context, contextOK := axisPayloadForKey[*contextowner.SchemaFragment](state, cells, axisKeyContext)
	evidence, evidenceOK := axisPayloadForKey[*placementsuspension.EvidenceFactorFragment](state, cells, axisKeyPlacementEvidence)
	pack, packOK := axisPayloadForKey[*packowner.SchemaFragment](state, cells, axisKeyPack)
	effect, effectOK := axisPayloadForKey[*effectowner.SchemaFragment](state, cells, axisKeyEffect)
	if !valueOK || !staticTypeOK || !callOK || !heapOK || !placementOK || !contextOK || !evidenceOK || !packOK || !effectOK {
		return principals{}, false
	}
	set := principals{value: value, staticType: staticType, call: call, heap: heap, placement: placement, context: context, evidence: evidence, pack: pack, effect: effect}
	return set, set.available()
}

// hotPrincipals projects the bound axes into the rule surface's authority
// record. The Link inputs and the allocation catalog are the caller's; the
// factor authorities are the table's.
func (cells axisCells) hotPrincipals(state *catalog, inputs LinkInputs, allocations *allocationcatalog.Catalog) (authorities, bool) {
	value, valueOK := axisPayloadForKey[*valueowner.HotOwner](state, cells, axisKeyValue)
	staticType, staticTypeOK := axisPayloadForKey[*staticowner.HotOwner](state, cells, axisKeyStaticType)
	call, callOK := axisPayloadForKey[*callowner.HotOwner](state, cells, axisKeyCall)
	heap, heapOK := axisPayloadForKey[*heapowner.HotOwner](state, cells, axisKeyHeap)
	placement, placementOK := axisPayloadForKey[*placementowner.HotOwner](state, cells, axisKeyPlacement)
	context, contextOK := axisPayloadForKey[*contextowner.HotOwner](state, cells, axisKeyContext)
	evidence, evidenceOK := axisPayloadForKey[*placementsuspension.EvidenceOwner](state, cells, axisKeyPlacementEvidence)
	pack, packOK := axisPayloadForKey[*packowner.HotOwner](state, cells, axisKeyPack)
	effect, effectOK := axisPayloadForKey[*effectowner.HotOwner](state, cells, axisKeyEffect)
	if !valueOK || !staticTypeOK || !callOK || !heapOK || !placementOK || !contextOK || !evidenceOK || !packOK || !effectOK {
		return authorities{}, false
	}
	set := authorities{
		value: value, staticType: staticType, call: call, heap: heap, placement: placement, context: context, evidence: evidence, pack: pack, effect: effect,
		valueSchema: inputs.ValueSchema, heapSchema: inputs.HeapSchema, placementSchema: inputs.PlacementSchema, packSchema: inputs.PackSchema,
		contextSchema: inputs.contextSchema, composition: inputs.composition, topology: inputs.topology, keySelection: inputs.keySelection, allocations: allocations,
		targetContract: inputs.targetContract,
	}
	return set, set.available() && mountedActualsComplete(inputs.CallAlgebra, inputs.PackSchema)
}
