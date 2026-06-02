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
}

// NewEntryContext constructs an entry context from already-resolved entry axes.
func NewEntryContext(ref summary.FuncRef, cells flow.CaptureCells, refs flow.FunctionRefs, closures flow.ClosureRefs, values summary.EntryValues) EntryContext {
	return EntryContext{
		ref:      ref,
		cells:    cells,
		refs:     cloneFunctionRefs(refs),
		closures: cloneClosureRefs(closures),
		values:   cloneEntryValues(values),
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
	return NewEntryContext(
		ref,
		overlayCaptureCells(closure.EntryCells(), liveCells),
		overlayFunctionRefs(closure.EntryFunctionRefs(), liveRefs),
		overlayClosureRefs(closure.EntryClosureRefs(), liveClosures),
		values,
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

// Key returns the canonical summary key for this exact entry context.
func (c EntryContext) Key() summary.Key {
	return summary.NewKeyWithEntryContext(c.ref, c.cells, c.refs, c.closures, c.values)
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

func overlayCaptureCells(base, live flow.CaptureCells) flow.CaptureCells {
	if base.IsTop() || live.IsTop() {
		if live.IsTop() {
			return live
		}
		return base
	}
	out := base
	for _, entry := range live.Entries() {
		out = out.With(entry.Symbol, entry.Value)
	}
	return out
}

func overlayFunctionRefs(base, live flow.FunctionRefs) flow.FunctionRefs {
	if flow.FunctionRefsDomain.Equal(base, flow.FunctionRefsDomain.Top()) ||
		flow.FunctionRefsDomain.Equal(live, flow.FunctionRefsDomain.Top()) {
		if flow.FunctionRefsDomain.Equal(live, flow.FunctionRefsDomain.Top()) {
			return live
		}
		return base
	}
	out := flow.FunctionRefsDomain.Join(base, nil)
	for path, set := range live {
		if set.IsBottom() {
			continue
		}
		out = flow.WithFunctionRef(out, path, set)
	}
	return out
}

func overlayClosureRefs(base, live flow.ClosureRefs) flow.ClosureRefs {
	if flow.ClosureRefsDomain.Equal(base, flow.ClosureRefsDomain.Top()) ||
		flow.ClosureRefsDomain.Equal(live, flow.ClosureRefsDomain.Top()) {
		if flow.ClosureRefsDomain.Equal(live, flow.ClosureRefsDomain.Top()) {
			return live
		}
		return base
	}
	out := flow.ClosureRefsDomain.Join(base, nil)
	for path, set := range live {
		if set.IsBottom() {
			continue
		}
		out = flow.WithClosureRef(out, path, set)
	}
	return out
}
