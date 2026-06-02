package facts

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// collectCallbackEnv derives callback-scoped global bindings as finite module
// facts. Contract EnvOverlay names are a boundary vocabulary; the returned facts
// are resolved to the callback body's graph symbols and never store name-keyed maps.
func collectCallbackEnv(p Program) []callbackEnvRow {
	out := computeCallbackEnvEntries(p)
	if len(out) == 0 {
		return nil
	}
	return out
}

type callbackEnvFactMap map[ref.FuncRef]map[cfg.SymbolID]typ.Type

// computeCallbackEnvEntries records, for every callback function literal, the
// callback-scoped globals its callee's spec injects, propagated into nested
// closures. String-keyed overlays are lowered immediately at the callback target
// graph; all internal propagation is ref/symbol keyed.
func computeCallbackEnvEntries(p Program) []callbackEnvRow {
	if len(p.Refs) == 0 || p.ResolveCallee == nil {
		return nil
	}
	facts := make(callbackEnvFactMap)

	refOverlays := make(map[ref.FuncRef]callbackenv.Overlays, len(p.Refs))
	for _, r := range p.Refs {
		if ov := inferRefCallbackOverlays(p, graphOf(p, r)); len(ov) > 0 {
			refOverlays[r] = ov
		}
	}

	for _, callerRef := range p.Refs {
		g := graphOf(p, callerRef)
		if g == nil {
			continue
		}
		g.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
			call := callInfoExpr(info)
			if call == nil {
				return
			}
			overlay := calleeCallbackOverlay(p, g, call, refOverlays)
			if len(overlay) == 0 {
				return
			}
			for argIdx := range call.Args {
				fnRef, ok := callbackArgFuncRef(p, info, argIdx)
				if !ok {
					continue
				}
				paramIdx := callbackParamIndexForArg(call, argIdx)
				cbOverlay, ok := overlay.ForParam(paramIdx)
				if !ok || len(cbOverlay) == 0 {
					continue
				}
				mergeEnvOverlayIntoFacts(p, facts, fnRef, cbOverlay)
			}
		})
	}
	propagateCallbackEnvFacts(p, facts)
	return callbackEnvRows(p, facts)
}

// inferRefCallbackOverlays runs the structural setup->param-call->cleanup
// recognizer over a module function and any closures it returns. Returned closures
// are additional sources for the same public function contract.
func inferRefCallbackOverlays(p Program, g *cfg.Graph) callbackenv.Overlays {
	if p.Evidence == nil || g == nil {
		return nil
	}
	paramSlots := g.ParamSlotsReadOnly()
	if len(paramSlots) == 0 {
		return nil
	}
	setupExpr := func(g *cfg.Graph) func(ast.Expr, cfg.Point) typ.Type {
		return func(expr ast.Expr, point cfg.Point) typ.Type {
			if p.SetupExprType != nil {
				if t := p.SetupExprType(g, expr, point); t != nil {
					return t
				}
			}
			return typ.Unknown
		}
	}
	sources := []callbackenv.Source{{
		Graph:     g,
		Evidence:  p.Evidence(g),
		SynthExpr: setupExpr(g),
	}}
	for _, returnedRef := range returnedClosureFuncRefs(p, g) {
		cg := graphOf(p, returnedRef)
		if cg == nil || cg == g {
			continue
		}
		sources = append(sources, callbackenv.Source{
			Graph:     cg,
			Evidence:  p.Evidence(cg),
			SynthExpr: setupExpr(cg),
		})
	}
	return callbackenv.InferFromSources(sources, paramSlots, g.Bindings())
}

func returnedClosureFuncRefs(p Program, g *cfg.Graph) []ref.FuncRef {
	if g == nil || p.RefForFuncSymbol == nil {
		return nil
	}
	seen := make(map[ref.FuncRef]bool)
	var out []ref.FuncRef
	g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for i, expr := range info.Exprs {
			fn, ok := expr.(*ast.FunctionExpr)
			if !ok || fn == nil {
				continue
			}
			r, ok := refForFuncSymbol(p, returnExprSymbol(info, i))
			if !ok || seen[r] {
				continue
			}
			seen[r] = true
			out = append(out, r)
		}
	})
	slices.SortFunc(out, compareFuncRef)
	return out
}

func returnExprSymbol(info *cfg.ReturnInfo, idx int) cfg.SymbolID {
	if info == nil || idx < 0 || idx >= len(info.Symbols) {
		return 0
	}
	return info.Symbols[idx]
}

func callInfoExpr(info *cfg.CallInfo) *ast.FuncCallExpr {
	if info == nil {
		return nil
	}
	return info.Call
}

func callbackArgFuncRef(p Program, info *cfg.CallInfo, argIdx int) (ref.FuncRef, bool) {
	if info == nil || argIdx < 0 || argIdx >= len(info.ArgSymbols) {
		return ref.FuncRef{}, false
	}
	return refForFuncSymbol(p, info.ArgSymbols[argIdx])
}

func refForFuncSymbol(p Program, sym cfg.SymbolID) (ref.FuncRef, bool) {
	if p.RefForFuncSymbol == nil || sym == 0 {
		return ref.FuncRef{}, false
	}
	return p.RefForFuncSymbol(sym)
}

func calleeCallbackOverlay(
	p Program,
	g *cfg.Graph,
	call *ast.FuncCallExpr,
	refOverlays map[ref.FuncRef]callbackenv.Overlays,
) callbackenv.Overlays {
	if call == nil {
		return nil
	}
	if p.ResolveCallee != nil {
		if r, ok := p.ResolveCallee(g, call); ok {
			if ov, ok := refOverlays[r]; ok && len(ov) > 0 {
				return ov
			}
			if p.CallbackOverlaysForRef != nil {
				if ov := p.CallbackOverlaysForRef(r); len(ov) > 0 {
					return ov
				}
			}
		}
	}
	if p.CalleeCallbackOverlays == nil {
		return nil
	}
	return p.CalleeCallbackOverlays(g, call)
}

func callbackParamIndexForArg(call *ast.FuncCallExpr, argIdx int) int {
	if call != nil && call.Method != "" {
		return argIdx + 1
	}
	return argIdx
}

func mergeEnvOverlayIntoFacts(p Program, facts callbackEnvFactMap, r ref.FuncRef, overlay callbackenv.Overlay) {
	if len(overlay) == 0 {
		return
	}
	g := graphOf(p, r)
	if g == nil {
		return
	}
	for _, entry := range overlay {
		if entry.Name == "" || entry.Type == nil {
			continue
		}
		sym, ok := callbackEnvGlobalSymbol(g, entry.Name.String())
		if !ok {
			continue
		}
		mergeCallbackEnvFact(facts, r, sym, entry.Type)
	}
}

func callbackEnvGlobalSymbol(g *cfg.Graph, name string) (cfg.SymbolID, bool) {
	if g == nil || name == "" {
		return 0, false
	}
	sym, ok := g.GlobalSymbol(name)
	if !ok || sym == 0 {
		return 0, false
	}
	if bindings := g.Bindings(); bindings != nil {
		if k, ok := bindings.Kind(sym); ok && k != cfg.SymbolGlobal {
			return 0, false
		}
	}
	return sym, true
}

func mergeCallbackEnvFact(facts callbackEnvFactMap, r ref.FuncRef, sym cfg.SymbolID, t typ.Type) bool {
	if sym == 0 || t == nil {
		return false
	}
	bySym := facts[r]
	if bySym == nil {
		bySym = make(map[cfg.SymbolID]typ.Type)
		facts[r] = bySym
	}
	if existing := bySym[sym]; existing != nil {
		joined := value.JoinPrecise(existing, t)
		if typ.TypeEquals(existing, joined) {
			return false
		}
		bySym[sym] = joined
		return true
	}
	bySym[sym] = t
	return true
}

func propagateCallbackEnvFacts(p Program, facts callbackEnvFactMap) {
	if len(facts) == 0 || p.NestedFuncRefs == nil {
		return
	}
	for changed := true; changed; {
		changed = false
		for _, parentRef := range p.Refs {
			parentFacts := facts[parentRef]
			if len(parentFacts) == 0 {
				continue
			}
			parentGraph := graphOf(p, parentRef)
			if parentGraph == nil {
				continue
			}
			children := p.NestedFuncRefs(parentRef)
			slices.SortFunc(children, compareFuncRef)
			for _, childRef := range children {
				childGraph := graphOf(p, childRef)
				if childGraph == nil {
					continue
				}
				for parentSym, t := range parentFacts {
					name := parentGraph.NameOf(parentSym)
					if name == "" {
						continue
					}
					childSym, ok := callbackEnvGlobalSymbol(childGraph, name)
					if !ok {
						continue
					}
					if mergeCallbackEnvFact(facts, childRef, childSym, t) {
						changed = true
					}
				}
			}
		}
	}
}

func callbackEnvRows(p Program, facts callbackEnvFactMap) []callbackEnvRow {
	if len(facts) == 0 {
		return nil
	}
	var out []callbackEnvRow
	for _, r := range p.Refs {
		bySym := facts[r]
		if len(bySym) == 0 {
			continue
		}
		syms := make([]cfg.SymbolID, 0, len(bySym))
		for sym := range bySym {
			syms = append(syms, sym)
		}
		slices.Sort(syms)
		for _, sym := range syms {
			t := bySym[sym]
			if sym == 0 || t == nil {
				continue
			}
			out = append(out, callbackEnvRow{
				FuncRef: r,
				Binding: callbackenv.GlobalBinding{
					Symbol: sym,
					Type:   t,
				},
			})
		}
	}
	return out
}
