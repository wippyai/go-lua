package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStore interface {
	MergeInterprocFactsNext(key api.GraphKey, delta api.Facts)
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

type functionSymbolLookup interface {
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
}

type interprocFactWriter struct {
	store factsWriteStore
}

func newInterprocFactWriter(store factsWriteStore) interprocFactWriter {
	return interprocFactWriter{store: store}
}

func (w interprocFactWriter) mergeParentFactsForSymbol(sym cfg.SymbolID, delta api.Facts) bool {
	if w.store == nil || sym == 0 {
		return false
	}
	parentKey, ok := w.store.ParentGraphKeyForSymbol(sym)
	if !ok {
		return false
	}
	w.store.MergeInterprocFactsNext(parentKey, delta)
	return true
}

func (w interprocFactWriter) writeLiteralSignatures(
	graph *cfg.Graph,
	parent *scope.State,
	sigs map[*ast.FunctionExpr]*typ.Function,
) {
	if w.store == nil || graph == nil || len(sigs) == 0 {
		return
	}
	if key, ok := w.store.GraphKeyFor(graph, parent); ok {
		delta := api.LiteralSigs{}
		for fnExpr, sig := range sigs {
			if fnExpr != nil && sig != nil && !w.isCanonicalFunction(fnExpr) {
				delta[fnExpr] = sig
			}
		}
		if len(delta) > 0 {
			w.store.MergeInterprocFactsNext(key, interprocdomain.LiteralSigsDelta(delta))
		}
	}
}

func (w interprocFactWriter) isCanonicalFunction(fn *ast.FunctionExpr) bool {
	lookup, ok := w.store.(functionSymbolLookup)
	if !ok || lookup == nil || fn == nil {
		return false
	}
	sym, ok := lookup.SymbolForFunc(fn)
	return ok && sym != 0
}
