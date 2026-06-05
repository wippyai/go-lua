package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// PostWidenObservationInput supplies the graph/topology facts needed to select
// exact post-widen summary observations. The selector is summary-layer policy:
// the driver owns scheduling the query calls, not deciding which summary shapes
// require an exact observer after lawful recursive widening.
type PostWidenObservationInput struct {
	Refs    []FuncRef
	Root    FuncRef
	Summary func(FuncRef) Summary
	Graph   func(FuncRef) *cfg.Graph

	IsMethod   func(FuncRef) bool
	Nested     func(FuncRef) []FuncRef
	Parent     func(FuncRef) (FuncRef, bool)
	CanObserve func(FuncRef) bool
}

// SelectPostWidenObservationRefs returns refs in the order they should be
// observed after the recursive summary fixed point has converged.
//
// Method receiver effects are observed in discovery order to preserve the
// historical receiver-effect frontier. Local factory/callback return precision is
// observed child-before-parent: when a parent returns a cell captured by a direct
// nested closure, the closure's exact cell effects must be visible before the
// parent factory summary is re-projected.
func SelectPostWidenObservationRefs(in PostWidenObservationInput) []FuncRef {
	if len(in.Refs) == 0 {
		return nil
	}
	var out []FuncRef
	seen := make(map[FuncRef]struct{})
	add := func(ref FuncRef) {
		if _, ok := seen[ref]; ok {
			return
		}
		if in.CanObserve != nil && !in.CanObserve(ref) {
			return
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	for _, ref := range in.Refs {
		if in.IsMethod != nil && in.IsMethod(ref) && summaryNeedsObservedReceiverEffects(in.summary(ref)) {
			add(ref)
		}
	}
	for i := len(in.Refs) - 1; i >= 0; i-- {
		ref := in.Refs[i]
		if ref == in.Root || (in.IsMethod != nil && in.IsMethod(ref)) {
			continue
		}
		if summaryNeedsObservedWidenedReturns(in.summary(ref)) ||
			returnsNestedCapturedSymbol(in, ref) ||
			isDirectChildOfReturnCapturedFunction(in, ref) {
			add(ref)
		}
	}
	return out
}

func (in PostWidenObservationInput) summary(ref FuncRef) Summary {
	if in.Summary == nil {
		return SummaryDomain.Bottom()
	}
	return in.Summary(ref)
}

func summaryNeedsObservedReceiverEffects(sum Summary) bool {
	for _, entry := range sum.ReceiverEffects.Entries() {
		if len(entry.Mutations) > 0 {
			return true
		}
	}
	return false
}

func summaryNeedsObservedWidenedReturns(sum Summary) bool {
	for _, ret := range sum.Returns {
		if ret.IsZero() {
			continue
		}
		if typ.ContainsRecursive(ret.ProjectValue()) {
			return true
		}
	}
	return false
}

func returnsNestedCapturedSymbol(in PostWidenObservationInput, ref FuncRef) bool {
	if in.Graph == nil || in.Nested == nil {
		return false
	}
	g := in.Graph(ref)
	if g == nil || g.Bindings() == nil {
		return false
	}
	returned := returnedSymbols(g)
	if len(returned) == 0 {
		return false
	}
	for _, child := range in.Nested(ref) {
		childGraph := in.Graph(child)
		if childGraph == nil || childGraph.Bindings() == nil || childGraph.Func() == nil {
			continue
		}
		for _, sym := range childGraph.Bindings().CapturedSymbols(childGraph.Func()) {
			if _, ok := returned[sym]; ok {
				return true
			}
		}
	}
	return false
}

func isDirectChildOfReturnCapturedFunction(in PostWidenObservationInput, ref FuncRef) bool {
	if in.Parent == nil {
		return false
	}
	parent, ok := in.Parent(ref)
	if !ok || parent == in.Root {
		return false
	}
	return returnsNestedCapturedSymbol(in, parent)
}

func returnedSymbols(g *cfg.Graph) map[cfg.SymbolID]struct{} {
	if g == nil || g.Bindings() == nil {
		return nil
	}
	returned := make(map[cfg.SymbolID]struct{})
	g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		n := len(info.Exprs)
		if len(info.Symbols) > n {
			n = len(info.Symbols)
		}
		for i := 0; i < n; i++ {
			var sym cfg.SymbolID
			if i < len(info.Symbols) {
				sym = info.Symbols[i]
			}
			if sym == 0 && i < len(info.Exprs) {
				if ident, ok := info.Exprs[i].(*ast.IdentExpr); ok {
					sym, _ = g.Bindings().SymbolOf(ident)
				}
			}
			if sym != 0 {
				returned[sym] = struct{}{}
			}
		}
	})
	return returned
}
