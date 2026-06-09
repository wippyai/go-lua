package interproc

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStore interface {
	MergeFunctionFactProjection(key api.GraphKey, sym cfg.SymbolID, fact api.FunctionFact)
	MergeLiteralSignatureProjection(key api.GraphKey, fn *ast.FunctionExpr, sig *typ.Function)
	MergeCapturedFieldProjection(key api.GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields api.FieldValues)
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
}

type functionSymbolLookup interface {
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
}

type projectionFactWriter struct {
	store factsWriteStore
}

func newPostflowProjectionWriter(store factsWriteStore) projectionFactWriter {
	return projectionFactWriter{store: store}
}

func (w projectionFactWriter) mergeParentFunctionFacts(facts api.FunctionFacts) bool {
	if w.store == nil || len(facts) == 0 {
		return false
	}
	updated := false
	for _, sym := range cfg.SortedSymbolIDs(facts) {
		parentKey, ok := w.store.ParentGraphKeyForSymbol(sym)
		if !ok {
			continue
		}
		w.store.MergeFunctionFactProjection(parentKey, sym, facts[sym])
		updated = true
	}
	return updated
}

func (w projectionFactWriter) mergeParentCapturedFieldProjections(nestedSym cfg.SymbolID, fields map[cfg.SymbolID]api.FieldValues) bool {
	if w.store == nil || nestedSym == 0 || len(fields) == 0 {
		return false
	}
	parentKey, ok := w.store.ParentGraphKeyForSymbol(nestedSym)
	if !ok {
		return false
	}
	updated := false
	for _, capturedSym := range cfg.SortedSymbolIDs(fields) {
		fieldValues := fields[capturedSym]
		if capturedSym == 0 || len(fieldValues) == 0 {
			continue
		}
		w.store.MergeCapturedFieldProjection(parentKey, nestedSym, capturedSym, fieldValues)
		updated = true
	}
	return updated
}

func (w projectionFactWriter) writeLiteralSignatures(
	graph *cfg.Graph,
	parent *scope.State,
	sigs api.LiteralSignatureLookup,
) {
	if w.store == nil || graph == nil || sigs == nil {
		return
	}
	key, ok := w.store.GraphKeyFor(graph, parent)
	if !ok {
		return
	}
	for _, nested := range graph.NestedFunctions() {
		fnExpr := nested.Func
		sig := sigs.Lookup(fnExpr)
		if fnExpr != nil && sig != nil && !w.isCanonicalFunction(fnExpr) {
			w.store.MergeLiteralSignatureProjection(key, fnExpr, sig)
		}
	}
}

func (w projectionFactWriter) isCanonicalFunction(fn *ast.FunctionExpr) bool {
	lookup, ok := w.store.(functionSymbolLookup)
	if !ok || lookup == nil || fn == nil {
		return false
	}
	sym, ok := lookup.SymbolForFunc(fn)
	return ok && sym != 0
}
