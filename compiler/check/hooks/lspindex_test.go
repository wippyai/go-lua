package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/lsp/index"
)

func TestLSPIndexer_EmptyFunction(t *testing.T) {
	symbols := index.NewSymbolIndex()
	indexer := NewLSPIndexer(symbols, nil)

	// Test with nil result
	indexer.extractFromFunction(nil, nil, nil)

	// Test with empty result
	sess := &check.Session{SourceName: "test.lua"}
	result := &api.FuncResult{}
	indexer.extractFromFunction(sess, nil, result)

	// Should not panic and should handle gracefully
	if len(symbols.SymbolsInFile("test.lua")) != 0 {
		t.Error("Expected no symbols for empty function")
	}
}

func TestAstSpan_NilNode(t *testing.T) {
	span := astSpan(nil)
	if span.Valid() {
		t.Error("Expected invalid span for nil node")
	}
}
