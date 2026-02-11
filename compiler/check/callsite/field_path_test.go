package callsite

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestFieldPathWithBaseSymbol_Nested(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(77)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: &ast.AttrGetExpr{
			Object: base,
			Key:    &ast.IdentExpr{Value: "a"},
		},
		Key: &ast.IdentExpr{Value: "b"},
	}

	sym, path, ok := FieldPathWithBaseSymbol(bindings, expr)
	if !ok {
		t.Fatal("expected static nested path resolution")
	}
	if sym != baseSym {
		t.Fatalf("base symbol = %d, want %d", sym, baseSym)
	}
	if path != "a.b" {
		t.Fatalf("path = %q, want a.b", path)
	}
}

func TestFieldPathWithBaseSymbol_RejectsDynamicKey(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(88)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.NumberExpr{Value: "1"},
	}

	if _, _, ok := FieldPathWithBaseSymbol(bindings, expr); ok {
		t.Fatal("expected dynamic numeric key to be rejected")
	}
}

func TestFieldPathWithBaseSymbol_RejectsAmbiguousStringSegment(t *testing.T) {
	bindings := bind.NewBindingTable()
	base := &ast.IdentExpr{Value: "obj"}
	baseSym := cfg.SymbolID(99)
	bindings.Bind(base, baseSym)

	expr := &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: "a.b"},
	}
	if _, _, ok := FieldPathWithBaseSymbol(bindings, expr); ok {
		t.Fatal("expected non-identifier string segment to be rejected")
	}
}
