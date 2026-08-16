package grammar

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/analysis/domain/heap/allocation/catalog"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
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

// axisTemplates is the authored analyzer axis inventory. Each row is one
// domain's coordinate space: its cold factor shape, the typed factor binding
// that instantiates it, and the algebra that binding publishes.
//
// The domain imports above are this cut's registration mechanism: an axis is
// declared where the table is composed. The surface record itself is blind to
// every domain, so the same rows move into generated per-domain registration
// without changing the interface, and these imports leave with them.
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

	add(axis.New(axis.Spec[LinkInputs, *valueowner.SchemaFragment, *valueowner.HotOwner, valuedomain.Value]{
		Key:         "value",
		Principal:   programartifact.RuleOutputValue,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.ValueFactor },
		Declare: func(context axis.Declaration) (*valueowner.SchemaFragment, bool) {
			return valueowner.DeclareSchema(context.Builder, context.Bundle.ValueFactor, context.Bundle.ValueSummary, context.Bundle.ValueSummaryFold)
		},
		Bind: func(context axis.Binding[LinkInputs, *valueowner.SchemaFragment]) (*valueowner.HotOwner, bool) {
			return valueowner.BindHot(context.Binding, context.Fragment, context.Inputs.ValueSchema)
		},
		Algebra: func(owner *valueowner.HotOwner) (axis.Algebra[valuedomain.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[valuedomain.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}))

	add(axis.New(axis.Spec[LinkInputs, *packowner.SchemaFragment, *packowner.HotOwner, packdomain.Value]{
		Key:         "pack",
		Principal:   programartifact.RuleOutputPack,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.PackFactor },
		Declare: func(context axis.Declaration) (*packowner.SchemaFragment, bool) {
			return packowner.DeclareSchema(context.Builder, context.Bundle.PackFactor)
		},
		Bind: func(context axis.Binding[LinkInputs, *packowner.SchemaFragment]) (*packowner.HotOwner, bool) {
			return packowner.BindHot(context.Binding, context.Fragment, context.Inputs.PackSchema)
		},
		Algebra: func(owner *packowner.HotOwner) (axis.Algebra[packdomain.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[packdomain.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}))

	add(axis.New(axis.Spec[LinkInputs, *heapowner.SchemaFragment, *heapowner.HotOwner, heapdomain.Value]{
		Key:         "heap",
		Principal:   programartifact.RuleOutputHeap,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.HeapFactor },
		Declare: func(context axis.Declaration) (*heapowner.SchemaFragment, bool) {
			return heapowner.DeclareSchema(context.Builder, context.Bundle.HeapFactor)
		},
		Bind: func(context axis.Binding[LinkInputs, *heapowner.SchemaFragment]) (*heapowner.HotOwner, bool) {
			return heapowner.BindHot(context.Binding, context.Fragment, context.Inputs.HeapSchema)
		},
		Algebra: func(owner *heapowner.HotOwner) (axis.Algebra[heapdomain.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[heapdomain.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}))

	add(axis.New(axis.Spec[LinkInputs, *callowner.SchemaFragment, *callowner.HotOwner, calldomain.Value]{
		Key:         "call",
		Principal:   programartifact.RuleOutputCall,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.CallFactor },
		Declare: func(context axis.Declaration) (*callowner.SchemaFragment, bool) {
			return callowner.DeclareSchema(context.Builder, context.Bundle.CallFactor)
		},
		Bind: func(context axis.Binding[LinkInputs, *callowner.SchemaFragment]) (*callowner.HotOwner, bool) {
			return callowner.BindHot(context.Binding, context.Fragment, context.Inputs.CallAlgebra)
		},
		Algebra: func(owner *callowner.HotOwner) (axis.Algebra[calldomain.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[calldomain.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}))

	add(axis.New(axis.Spec[LinkInputs, *effectowner.SchemaFragment, *effectowner.HotOwner, effectfactor.Value]{
		Key:         "effect",
		Principal:   programartifact.RuleOutputEffect,
		Storage:     axis.StorageFactor,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilitySolve,
		Concurrency: axis.ConcurrencySingleWriter,
		Semantic:    func(bundle vocabulary.Bundle) engine.SemanticKey { return bundle.EffectFactor },
		Declare: func(context axis.Declaration) (*effectowner.SchemaFragment, bool) {
			return effectowner.DeclareSchema(context.Builder, context.Bundle.EffectFactor)
		},
		Bind: func(context axis.Binding[LinkInputs, *effectowner.SchemaFragment]) (*effectowner.HotOwner, bool) {
			return effectowner.BindHot(context.Binding, context.Fragment, context.Inputs.EffectAlgebra)
		},
		Algebra: func(owner *effectowner.HotOwner) (axis.Algebra[effectfactor.Value], bool) {
			spec, ok := owner.FactorSpec()
			if !ok {
				return axis.Algebra[effectfactor.Value]{}, false
			}
			return axis.Adopt(spec)
		},
	}))

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
func AxisSemantic(principal programartifact.RuleOutputKind) (engine.SemanticKey, bool) {
	entry, ok := axisForPrincipal(principal)
	if !ok {
		return engine.SemanticKey{}, false
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
