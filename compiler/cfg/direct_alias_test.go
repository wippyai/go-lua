package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func buildGraphForDirectAliasTest(t *testing.T, code string) *Graph {
	t.Helper()

	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	g := Build(&ast.FunctionExpr{Stmts: stmts})
	if g == nil {
		t.Fatal("expected graph")
	}
	return g
}

func symbolAtExit(t *testing.T, g *Graph, name string) SymbolID {
	t.Helper()

	sym, ok := g.SymbolAt(g.Exit(), name)
	if !ok || sym == 0 {
		t.Fatalf("expected symbol for %q at exit", name)
	}
	return sym
}

func TestDirectAliasSymbol_LocalAlias(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local f = B
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	bSym := symbolAtExit(t, g, "B")

	if got := g.DirectAliasSymbol(fSym); got != bSym {
		t.Fatalf("DirectAliasSymbol(f) = %d, want %d", got, bSym)
	}
}

func TestDirectAliasSymbol_ReassignmentDifferentSourceInvalidates(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local function C()
			return 2
		end
		local f = B
		f = C
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	if got := g.DirectAliasSymbol(fSym); got != 0 {
		t.Fatalf("DirectAliasSymbol(f) = %d, want 0 after reassignment", got)
	}
}

func TestDirectAliasSymbol_NonIdentifierSourceInvalidates(t *testing.T) {
	g := buildGraphForDirectAliasTest(t, `
		local function B()
			return 1
		end
		local f = B
		f = get_func()
		return f()
	`)

	fSym := symbolAtExit(t, g, "f")
	if got := g.DirectAliasSymbol(fSym); got != 0 {
		t.Fatalf("DirectAliasSymbol(f) = %d, want 0 for non-ident source", got)
	}
}
