package composite

import (
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// axisPrincipalLimit bounds the principal-indexed projections below. Every
// axis declares exactly one writer principal and no two share it, so the
// artifact's factor lane catalog is the exact size of this index.
const axisPrincipalLimit = int(programartifact.RuleOutputEffect) + 1

type axisTemplate = axis.Template[LinkInputs]

// axisCells is one pass's per-axis payload, indexed by writer principal. The
// cold pass fills it with fragments and the hot pass with bound axes.
type axisCells [axisPrincipalLimit]axis.Cell

func (cells axisCells) available() bool {
	for principal := programartifact.RuleOutputInvalid + 1; int(principal) < axisPrincipalLimit; principal++ {
		if !cells[principal].Available() {
			return false
		}
	}
	return true
}

// axisPayload recovers one axis's cell at its declared type. It is the single
// typed recovery the composition performs; no pass reads a cell it did not
// index by the axis's own principal.
func axisPayload[T any](cells axisCells, principal programartifact.RuleOutputKind) (T, bool) {
	var absent T
	if int(principal) <= 0 || int(principal) >= axisPrincipalLimit {
		return absent, false
	}
	return axis.Payload[T](cells[principal])
}

// axisTemplates is the authored analyzer axis inventory. Each row instantiates
// one owning domain's own axis declaration with this composition's Link input
// record, which that domain admits through its own need interface.
//
// The declaration itself lives with the domain that owns the factor, so the
// inventory here states membership and order alone.
func axisTemplates() ([]*axisTemplate, bool) {
	var admitted []*axisTemplate
	rejected := false
	add := func(entry *axisTemplate, ok bool) {
		if !ok {
			rejected = true
			return
		}
		admitted = append(admitted, entry)
	}

	add(axis.New(valueowner.AxisEntry[LinkInputs]()))
	add(axis.New(packowner.AxisEntry[LinkInputs]()))
	add(axis.New(heapowner.AxisEntry[LinkInputs]()))
	add(axis.New(callowner.AxisEntry[LinkInputs]()))
	add(axis.New(effectowner.AxisEntry[LinkInputs]()))

	if rejected {
		return nil, false
	}
	return admitted, true
}

// DiagnosticAxis is the closed analyzer-owned classification of one axis. It
// is the axis's writer principal ordinal. Unknown covers empty, foreign, and
// generic lifecycle failures without a bound analyzer axis.
type DiagnosticAxis uint8

const DiagnosticAxisUnknown DiagnosticAxis = 0

// DiagnosticAxisForPrincipal projects one artifact factor lane into the closed
// analyzer axis classification.
func DiagnosticAxisForPrincipal(principal programartifact.RuleOutputKind) DiagnosticAxis {
	if _, ok := axisForPrincipal(principal); ok {
		return DiagnosticAxis(principal)
	}
	return DiagnosticAxisUnknown
}

func (diagnostic DiagnosticAxis) String() string {
	if entry, ok := axisForPrincipal(programartifact.RuleOutputKind(diagnostic)); ok {
		return string(entry.Key())
	}
	return "unknown"
}

func axisForPrincipal(principal programartifact.RuleOutputKind) (*axisTemplate, bool) {
	sealRegistry()
	if registry.sealed == nil || int(principal) <= 0 || int(principal) >= axisPrincipalLimit {
		return nil, false
	}
	entry := registry.axisByPrincipal[principal]
	return entry, entry != nil
}

// AxisCount is the size of the sealed axis inventory.
func AxisCount() int {
	sealRegistry()
	return len(registry.axes)
}

// AxisPrincipalAt returns the writer principal of one axis at its table
// position. The position is a traversal convenience; the principal is the
// identity.
func AxisPrincipalAt(position int) (programartifact.RuleOutputKind, bool) {
	sealRegistry()
	if position < 0 || position >= len(registry.axes) {
		return programartifact.RuleOutputInvalid, false
	}
	return registry.axes[position].Principal(), true
}

// AxisEntryID returns one axis's stable table identity.
func AxisEntryID(principal programartifact.RuleOutputKind) (schema.EntryID, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// AxisSemantic returns one axis's canonical Engine identity.
func AxisSemantic(principal programartifact.RuleOutputKind) (identity.SemanticKey, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return identity.SemanticKey{}, false
	}
	semantic := entry.Semantic(registry.bundle)
	return semantic, semantic.Available()
}

// AxisStorage returns where one axis's facts live.
func AxisStorage(principal programartifact.RuleOutputKind) (axis.Storage, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return axis.StorageInvalid, false
	}
	return entry.Storage(), true
}

// AxisCardinality returns the shape of one axis's key space. A later inventory
// that materializes its own coordinates reads this rather than assuming a
// dense ordinal range.
func AxisCardinality(principal programartifact.RuleOutputKind) (axis.Cardinality, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return axis.CardinalityInvalid, false
	}
	return entry.Cardinality(), true
}

// AxisLifetime returns the scope one axis's facts are valid for.
func AxisLifetime(principal programartifact.RuleOutputKind) (axis.Lifetime, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return axis.LifetimeInvalid, false
	}
	return entry.Lifetime(), true
}

// declareAxes runs the table's cold axis pass and returns each axis's fragment
// cell at its principal. It is the only place a factor's Schema shape is
// recorded, and it runs before the rule pass because a rule declares against
// the principals produced here.
func declareAxes(builder *engine.SchemaBuilder, bundle vocabulary.Bundle) (axisCells, DiagnosticAxis, bool) {
	var fragments axisCells
	sealRegistry()
	if registry.sealed == nil || builder == nil {
		return fragments, DiagnosticAxisUnknown, false
	}
	context := axis.Declaration{Builder: builder, Bundle: bundle}
	for _, entry := range registry.axes {
		fragment, ok := entry.Declare(context)
		if !ok {
			return fragments, DiagnosticAxis(entry.Principal()), false
		}
		fragments[entry.Principal()] = fragment
	}
	if !fragments.available() {
		return fragments, DiagnosticAxisUnknown, false
	}
	return fragments, DiagnosticAxisUnknown, true
}

// bindAxes runs the table's hot axis pass: every axis instantiates its own
// typed factor binding and publishes the algebra of that binding. It is the
// first pass of the binding transaction, so every later pass binds against a
// declared axis rather than a hand-ordered owner sequence.
func bindAxes(binding *engine.SchemaBinding, fragments axisCells, inputs LinkInputs) (axisCells, DiagnosticAxis, bool) {
	var bound axisCells
	sealRegistry()
	if registry.sealed == nil || binding == nil || !fragments.available() {
		return bound, DiagnosticAxisUnknown, false
	}
	for _, entry := range registry.axes {
		principal := entry.Principal()
		hot, ok := entry.Bind(binding, inputs, fragments[principal])
		if !ok {
			return bound, DiagnosticAxis(principal), false
		}
		bound[principal] = hot
	}
	if !bound.available() {
		return bound, DiagnosticAxisUnknown, false
	}
	return bound, DiagnosticAxisUnknown, true
}

// coldPrincipals projects the declared axis fragments into the rule surface's
// principal record.
func (cells axisCells) coldPrincipals() (principals, bool) {
	value, valueOK := axisPayload[*valueowner.SchemaFragment](cells, programartifact.RuleOutputValue)
	call, callOK := axisPayload[*callowner.SchemaFragment](cells, programartifact.RuleOutputCall)
	heap, heapOK := axisPayload[*heapowner.SchemaFragment](cells, programartifact.RuleOutputHeap)
	pack, packOK := axisPayload[*packowner.SchemaFragment](cells, programartifact.RuleOutputPack)
	effect, effectOK := axisPayload[*effectowner.SchemaFragment](cells, programartifact.RuleOutputEffect)
	if !valueOK || !callOK || !heapOK || !packOK || !effectOK {
		return principals{}, false
	}
	set := principals{value: value, call: call, heap: heap, pack: pack, effect: effect}
	return set, set.available()
}

// hotPrincipals projects the bound axes into the rule surface's authority
// record. The Link inputs and the allocation catalog are the caller's; the
// factor authorities are the table's.
func (cells axisCells) hotPrincipals(inputs LinkInputs, allocations *allocationcatalog.Catalog) (authorities, bool) {
	value, valueOK := axisPayload[*valueowner.HotOwner](cells, programartifact.RuleOutputValue)
	call, callOK := axisPayload[*callowner.HotOwner](cells, programartifact.RuleOutputCall)
	heap, heapOK := axisPayload[*heapowner.HotOwner](cells, programartifact.RuleOutputHeap)
	pack, packOK := axisPayload[*packowner.HotOwner](cells, programartifact.RuleOutputPack)
	effect, effectOK := axisPayload[*effectowner.HotOwner](cells, programartifact.RuleOutputEffect)
	if !valueOK || !callOK || !heapOK || !packOK || !effectOK {
		return authorities{}, false
	}
	set := authorities{
		value: value, call: call, heap: heap, pack: pack, effect: effect,
		valueSchema: inputs.ValueSchema, heapSchema: inputs.HeapSchema, packSchema: inputs.PackSchema,
		topology: inputs.Topology, allocations: allocations, activation: inputs.ActivationCatalog,
	}
	return set, set.available()
}
