package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
)

type factsWriteStore interface {
	api.PostflowFunctionFactWriter
	api.PostflowCapturedFieldProjectionWriter
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool)
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

func (w projectionFactWriter) mergeParentCapturedFieldProjections(nestedSym cfg.SymbolID, fields map[cfg.SymbolID]postflow.FieldValues) bool {
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
