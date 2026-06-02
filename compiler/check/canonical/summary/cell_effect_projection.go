package summary

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
)

// CellEffectLookup returns the caller-visible capture-cell effect of ref under
// the given direct-call entry-value context.
type CellEffectLookup func(ref FuncRef, entryValues EntryValues) flow.CaptureEffects

// CallbackArgResolver resolves one callback argument expression to the module
// functions it may denote, when known.
type CallbackArgResolver func(arg ast.Expr) ([]FuncRef, bool)

// CellEffectAggregation is the summary-owned call-site fold of direct callee
// effects plus callback effects. The driver supplies only resolution facts; this
// type owns callback parameter ordering, method-call argument indexing,
// cardinality weakening, and the cooccurring-effect algebra.
type CellEffectAggregation struct {
	DirectRefs []FuncRef
	// DirectEffects is the already-folded direct-callee contribution to the call
	// effects, avoiding duplicated direct-callee loops in drivers.
	DirectEffects     flow.CaptureEffects
	DirectEntryValues func(FuncRef) EntryValues
	CallbackSpec      *contract.Spec
	CallbackArgs      []ast.Expr
	MethodCall        bool
	ResolveCallback   CallbackArgResolver
	EffectOf          CellEffectLookup
}

// AggregateCellEffects combines the cell effects a call site may perform. Direct
// callee effects and callback effects have unknown relative runtime order, so the
// fold uses flow.CooccurringCaptureEffects. Callback specs are sorted by parameter
// index before projection so map iteration cannot influence the result.
func AggregateCellEffects(in CellEffectAggregation) flow.CaptureEffects {
	if in.EffectOf == nil {
		if len(in.DirectRefs) > 0 {
			return flow.CaptureEffectsDomain.Bottom()
		}
		return in.DirectEffects
	}
	out := in.DirectEffects
	add := func(next flow.CaptureEffects) {
		out = flow.CooccurringCaptureEffects(out, next)
	}

	for _, ref := range in.DirectRefs {
		var entryValues EntryValues
		if in.DirectEntryValues != nil {
			entryValues = in.DirectEntryValues(ref)
		}
		add(in.EffectOf(ref, entryValues))
	}

	if in.CallbackSpec == nil || len(in.CallbackSpec.Callbacks) == 0 || in.ResolveCallback == nil {
		return out
	}
	keys := make([]int, 0, len(in.CallbackSpec.Callbacks))
	for paramIdx := range in.CallbackSpec.Callbacks {
		keys = append(keys, paramIdx)
	}
	slices.Sort(keys)
	for _, paramIdx := range keys {
		cb := in.CallbackSpec.Callbacks[paramIdx]
		if cb == nil {
			continue
		}
		argIdx := callbackArgumentIndex(in.MethodCall, paramIdx)
		if argIdx < 0 || argIdx >= len(in.CallbackArgs) {
			continue
		}
		refs, ok := in.ResolveCallback(in.CallbackArgs[argIdx])
		if !ok || len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			effects := in.EffectOf(ref, nil)
			if cb.Cardinality != contract.CardExactlyOnce {
				effects = effects.May()
			}
			add(effects)
		}
	}
	return out
}

func callbackArgumentIndex(methodCall bool, paramIdx int) int {
	if methodCall {
		return paramIdx - 1
	}
	return paramIdx
}
