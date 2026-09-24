package captured

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// FromParentFacts computes captured types for a nested graph from parent facts.
func FromParentFacts(
	parentFacts flow.TypeFacts,
	childGraph *cfg.Graph,
	defPoint cfg.Point,
	bindingsOverride *bind.BindingTable,
) map[cfg.SymbolID]typ.Type {
	if parentFacts == nil || childGraph == nil || defPoint == 0 {
		return nil
	}
	bindings := bindingsOverride
	if bindings == nil {
		bindings = childGraph.Bindings()
	}
	if bindings == nil {
		return nil
	}
	fn := childGraph.Func()
	if fn == nil {
		return nil
	}
	capturedSyms := bindings.CapturedSymbols(fn)
	if len(capturedSyms) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(capturedSyms))
	for _, sym := range capturedSyms {
		if sym == 0 {
			continue
		}
		tv := parentFacts.EffectiveTypeAt(defPoint, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			out[sym] = tv.Type
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeCapturedTypes merges captured types into declared types as hints.
func MergeCapturedTypes(declared flow.DeclaredTypes, captured map[cfg.SymbolID]typ.Type) flow.DeclaredTypes {
	if len(captured) == 0 {
		return declared
	}
	if declared == nil {
		declared = make(flow.DeclaredTypes, len(captured))
	}
	for _, sym := range cfg.SortedSymbolIDs(captured) {
		t := captured[sym]
		if sym == 0 || t == nil {
			continue
		}
		if prev := declared[sym]; prev != nil {
			declared[sym] = typ.JoinPreferNonSoft(prev, t)
		} else {
			declared[sym] = t
		}
	}
	return declared
}

// HasUntypedSelf reports whether a recursive local function sees its own
// binding before that binding has a return type.
func HasUntypedSelf(bindings *bind.BindingTable, fn *ast.FunctionExpr, types map[cfg.SymbolID]typ.Type) bool {
	if bindings == nil || fn == nil {
		return false
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		if types[sym] != nil {
			continue
		}
		if own, ok := bindings.FuncLitBySymbol(sym); ok && own == fn {
			return true
		}
	}
	return false
}

// HasUntypedAliasCall reports a call through a captured local whose function
// type is still unavailable. Locally defined functions have their own return
// summaries; this detects callable aliases such as local sub = string.sub.
func HasUntypedAliasCall(graph *cfg.Graph, types map[cfg.SymbolID]typ.Type) bool {
	if graph == nil || graph.Bindings() == nil || graph.Func() == nil {
		return false
	}
	bindings := graph.Bindings()
	captured := make(map[cfg.SymbolID]bool)
	for _, sym := range bindings.CapturedSymbols(graph.Func()) {
		if types[sym] != nil {
			continue
		}
		if kind, ok := bindings.Kind(sym); ok && kind == cfg.SymbolGlobal {
			continue
		}
		if _, ok := bindings.FuncLitBySymbol(sym); ok {
			continue
		}
		captured[sym] = true
	}
	if len(captured) == 0 {
		return false
	}
	untyped := false
	graph.EachCallSite(func(_ cfg.Point, call *cfg.CallInfo) {
		if call != nil && captured[call.CalleeSymbol] {
			untyped = true
		}
	})
	return untyped
}
