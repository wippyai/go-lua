package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

type factsWriteStore interface {
	MergeLegacyFactsNext(key api.GraphKey, delta api.Facts)
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

type functionSymbolLookup interface {
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
}

type legacyFactWriter struct {
	store factsWriteStore
}

func newLegacyFactWriter(store factsWriteStore) legacyFactWriter {
	return legacyFactWriter{store: store}
}

func (w legacyFactWriter) mergeParentFactsForSymbol(sym cfg.SymbolID, delta api.Facts) bool {
	if w.store == nil || sym == 0 {
		return false
	}
	parentKey, ok := w.store.ParentGraphKeyForSymbol(sym)
	if !ok {
		return false
	}
	w.store.MergeLegacyFactsNext(parentKey, delta)
	return true
}

func (w legacyFactWriter) writeLiteralSignatures(
	graph *cfg.Graph,
	parent *scope.State,
	sigs api.LiteralSignatureLookup,
) {
	if w.store == nil || graph == nil || sigs == nil {
		return
	}
	if key, ok := w.store.GraphKeyFor(graph, parent); ok {
		delta := api.LiteralSigs{}
		for _, nested := range graph.NestedFunctions() {
			fnExpr := nested.Func
			sig := sigs.Lookup(fnExpr)
			if fnExpr != nil && sig != nil && !w.isCanonicalFunction(fnExpr) {
				delta[fnExpr] = sig
			}
		}
		if len(delta) > 0 {
			w.store.MergeLegacyFactsNext(key, interprocdomain.LiteralSigsDelta(delta))
		}
	}
}

func (w legacyFactWriter) isCanonicalFunction(fn *ast.FunctionExpr) bool {
	lookup, ok := w.store.(functionSymbolLookup)
	if !ok || lookup == nil || fn == nil {
		return false
	}
	sym, ok := lookup.SymbolForFunc(fn)
	return ok && sym != 0
}
