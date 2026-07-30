package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func stampSyntheticWIRPathSymbols(t *testing.T, body *wir.Body, bindings *bind.Result, paths ...path.Path) {
	t.Helper()
	if body == nil || bindings == nil {
		t.Fatal("synthetic WIR symbol stamping requires body and bindings")
	}
	seen := make(map[symbol.ID]bool)
	for _, p := range paths {
		if p.Symbol == 0 || seen[p.Symbol] {
			continue
		}
		seen[p.Symbol] = true
		kind, ok := bindings.Kind(p.Symbol)
		if !ok {
			t.Fatalf("missing binding kind for synthetic WIR symbol %d (%s)", p.Symbol, p.Root)
		}
		body.SetSymbolInfo(p.Symbol, wir.SymbolInfoConfig{
			Kind:           testWIRSymbolKind(kind),
			Name:           bindings.Name(p.Symbol),
			HasWrite:       bindings.HasWrite(p.Symbol),
			ImplicitGlobal: bindings.IsImplicitGlobalSymbol(p.Symbol),
		})
	}
}

func testWIRSymbolKind(kind symbol.Kind) wir.SymbolKind {
	switch kind {
	case symbol.Param:
		return wir.SymbolParam
	case symbol.Local:
		return wir.SymbolLocal
	case symbol.Global:
		return wir.SymbolGlobal
	case symbol.Upvalue:
		return wir.SymbolUpvalue
	case symbol.Function:
		return wir.SymbolFunction
	default:
		return wir.SymbolUnknown
	}
}
