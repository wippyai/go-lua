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
	ref        summary.FuncRef
	references flow.ReferenceContext
	values     summary.EntryValues
	facts      flow.BoundaryFacts
}

// NewEntryContext constructs an entry context from already-resolved entry axes.
func NewEntryContext(ref summary.FuncRef, references flow.ReferenceContext, values summary.EntryValues, facts flow.BoundaryFacts) EntryContext {
	return EntryContext{
		ref:        ref,
		references: flow.ReferenceContextOf(references.CaptureCells(), references.FunctionRefs(), references.ClosureRefs()),
		values:     cloneEntryValues(values),
		facts:      cloneBoundaryFacts(facts),
	}
}

// EntryContextFromClosureWithLiveContext overlays a closure's captured
// environment with an already-projected live entry context for the same callee.
// Lua closures capture mutable locations, so live axes win over the allocation
// snapshot for matching cells and reference paths.
func EntryContextFromClosureWithLiveContext(closure flow.ClosureRef, live EntryContext) EntryContext {
	return NewEntryContext(
		live.ref,
		flow.OverlayReferenceContext(
			flow.ReferenceContextOf(closure.EntryCells(), closure.EntryFunctionRefs(), closure.EntryClosureRefs()),
			live.references,
		),
		live.values,
		live.facts,
	)
}

// Ref returns the callee function identity for this entry context.
func (c EntryContext) Ref() summary.FuncRef { return c.ref }

// CaptureCells returns the captured-cell entry store.
func (c EntryContext) CaptureCells() flow.CaptureCells { return c.references.CaptureCells() }

// FunctionRefs returns the function-identity entry store.
func (c EntryContext) FunctionRefs() flow.FunctionRefs { return c.references.FunctionRefs() }

// ClosureRefs returns the closure-value entry store.
func (c EntryContext) ClosureRefs() flow.ClosureRefs { return c.references.ClosureRefs() }

// References returns the full callee-entry reference environment.
func (c EntryContext) References() flow.ReferenceContext {
	return flow.ReferenceContextOf(c.CaptureCells(), c.FunctionRefs(), c.ClosureRefs())
}

// EntryValues returns caller-projected parameter values.
func (c EntryContext) EntryValues() summary.EntryValues { return cloneEntryValues(c.values) }

// EntryFacts returns caller-projected parameter-relative path facts.
func (c EntryContext) EntryFacts() flow.BoundaryFacts { return cloneBoundaryFacts(c.facts) }

// Key returns the canonical summary key for this exact entry context.
func (c EntryContext) Key() summary.Key {
	return summary.NewKeyWithReferenceContext(c.ref, c.references, c.values, c.facts)
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
