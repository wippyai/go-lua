package call

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// EntryContext is the caller-provided context used to key a callee entry.
// It is a pure value object: it records axes only and never resolves targets or
// reads summaries.
type EntryContext struct {
	ref      summary.FuncRef
	cells    flow.CaptureCells
	refs     flow.FunctionRefs
	closures flow.ClosureRefs
	values   summary.EntryValues
	facts    flow.BoundaryFacts
}

// NewEntryContext constructs an entry context from already-resolved entry axes.
func NewEntryContext(ref summary.FuncRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values summary.EntryValues) EntryContext {
	return NewEntryContextWithFacts(ref, cells, refs, closures, values, flow.BoundaryFactsDomain.Top())
}

// NewEntryContextWithFacts constructs an entry context from already-resolved
// entry axes, including parameter-relative path facts.
func NewEntryContextWithFacts(ref summary.FuncRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values summary.EntryValues, facts flow.BoundaryFacts) EntryContext {
	return EntryContext{
		ref:      ref,
		cells:    cells,
		refs:     cloneFunctionRefs(refs),
		closures: cloneClosureRefs(closures),
		values:   cloneEntryValues(values),
		facts:    cloneBoundaryFacts(facts),
	}
}

// EntryContextFromClosure constructs an entry context from a closure value's
// carried lexical environment.
func EntryContextFromClosure(ref summary.FuncRef, closure flow.ClosureRef, values summary.EntryValues) EntryContext {
	return NewEntryContext(ref, closure.EntryCells(), closure.EntryFunctionRefs(), closure.EntryClosureRefs(), values)
}

// EntryContextFromClosureWithLiveAxes constructs a closure entry context for an
// invocation point. ClosureRefs carry the lexical environment from allocation
// time; live caller axes override matching captured cells/paths because Lua
// closures capture mutable locations, not immutable value snapshots. Callers must
// pass axes already projected to the callee's captured symbols.
func EntryContextFromClosureWithLiveAxes(
	ref summary.FuncRef,
	closure flow.ClosureRef,
	liveCells flow.CaptureCells,
	liveRefs flow.FunctionRefs,
	liveClosures flow.ClosureRefs,
	values summary.EntryValues,
) EntryContext {
	return EntryContextFromClosureWithLiveAxesAndFacts(ref, closure, liveCells, liveRefs, liveClosures, values, flow.BoundaryFactsDomain.Top())
}

// EntryContextFromClosureWithLiveAxesAndFacts is EntryContextFromClosureWithLiveAxes
// plus caller-projected parameter-relative path facts.
func EntryContextFromClosureWithLiveAxesAndFacts(
	ref summary.FuncRef,
	closure flow.ClosureRef,
	liveCells flow.CaptureCells,
	liveRefs flow.FunctionRefs,
	liveClosures flow.ClosureRefs,
	values summary.EntryValues,
	facts flow.BoundaryFacts,
) EntryContext {
	return NewEntryContextWithFacts(
		ref,
		flow.OverlayCaptureCells(closure.EntryCells(), liveCells),
		flow.OverlayFunctionRefs(closure.EntryFunctionRefs(), liveRefs),
		flow.OverlayClosureRefs(closure.EntryClosureRefs(), liveClosures),
		values,
		facts,
	)
}

// EntryContextFromClosureWithLiveContext overlays a closure's captured
// environment with an already-projected live entry context for the same callee.
func EntryContextFromClosureWithLiveContext(closure flow.ClosureRef, live EntryContext) EntryContext {
	return EntryContextFromClosureWithLiveAxesAndFacts(
		live.ref,
		closure,
		live.cells,
		live.refs,
		live.closures,
		live.values,
		live.facts,
	)
}

// Ref returns the callee function identity for this entry context.
func (c EntryContext) Ref() summary.FuncRef { return c.ref }

// CaptureCells returns the captured-cell entry store.
func (c EntryContext) CaptureCells() flow.CaptureCells { return c.cells }

// FunctionRefs returns the function-identity entry store.
func (c EntryContext) FunctionRefs() flow.FunctionRefs { return cloneFunctionRefs(c.refs) }

// ClosureRefs returns the closure-value entry store.
func (c EntryContext) ClosureRefs() flow.ClosureRefs { return cloneClosureRefs(c.closures) }

// EntryValues returns caller-projected parameter values.
func (c EntryContext) EntryValues() summary.EntryValues { return cloneEntryValues(c.values) }

// EntryFacts returns caller-projected parameter-relative path facts.
func (c EntryContext) EntryFacts() flow.BoundaryFacts { return cloneBoundaryFacts(c.facts) }

// Key returns the canonical summary key for this exact entry context.
func (c EntryContext) Key() summary.Key {
	return summary.NewKeyWithEntryContextFacts(c.ref, c.cells, c.refs, c.closures, c.values, c.facts)
}

func cloneFunctionRefs(refs flow.FunctionRefs) flow.FunctionRefs {
	return flow.FunctionRefsDomain.Join(refs, nil)
}

func cloneClosureRefs(refs flow.ClosureRefs) flow.ClosureRefs {
	return flow.ClosureRefsDomain.Join(refs, nil)
}

func cloneEntryValues(in summary.EntryValues) summary.EntryValues {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]product.AbstractValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBoundaryFacts(in flow.BoundaryFacts) flow.BoundaryFacts {
	if in.IsBottom() || !in.HasProof() {
		return flow.BoundaryFactsDomain.Top()
	}
	return flow.BoundaryFactsOf(
		in.KeyPresence(),
		in.KeyArrays(),
		in.KeyArrayValues(),
		in.AppendKeys(),
		in.LengthLowerBounds(),
		in.IndexWrites(),
	).WithAppendElementFieldOrigins(in.AppendElementFieldOrigins())
}
