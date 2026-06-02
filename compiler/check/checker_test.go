package check

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/phase"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/compiler/stdlib"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewChecker(t *testing.T) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	if c == nil {
		t.Fatal("NewChecker returned nil")
	}
	if c.db != database {
		t.Error("db not set")
	}
}

func TestChecker_WithPass(t *testing.T) {
	pass := func(_ *Session, _ *ast.FunctionExpr, _ *api.FuncResult) []diag.Diagnostic {
		return nil
	}
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithPass(pass))
	if c == nil {
		t.Fatal("NewChecker with WithPass returned nil")
	}
	if len(c.passes) != 1 {
		t.Error("pass not registered")
	}
}

func TestChecker_WithMaxScopeDepth(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithMaxScopeDepth(4))
	if c.maxScopeDepth != 4 {
		t.Fatalf("maxScopeDepth = %d, want 4", c.maxScopeDepth)
	}
}

func TestChecker_ClearCacheBumpsRevision(t *testing.T) {
	database := db.New()
	c := NewChecker(database, Deps{Types: core.NewEngine()})
	before := database.Revision()

	c.ClearCache()

	if after := database.Revision(); after <= before {
		t.Fatalf("revision after ClearCache = %d, want > %d", after, before)
	}
}

func TestChecker_ScopeDepthDiagnostic(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithMaxScopeDepth(1), WithScopeDepthDiagnostics(true))
	source := `
do
  do
    local x = 1
  end
end
`
	sess := c.Check(source, "test.lua")
	found := false
	for _, d := range sess.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, "scope depth limit exceeded") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected scope depth limit diagnostic")
	}
}

func TestChecker_Check_Empty(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	sess := c.Check("", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.SourceName != "test.lua" {
		t.Error("SourceName not set")
	}
}

func TestChecker_Check_SimpleLocal(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	sess := c.Check("local x = 1", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Results) == 0 {
		t.Error("no results")
	}
}

func TestChecker_Check_ParseError(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	sess := c.Check("local x =", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Diagnostics) == 0 {
		t.Error("expected parse error diagnostic")
	}
}

func TestChecker_Check_NestedFunction(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	sess := c.Check("function foo() return 1 end", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(sess.Results))
	}
}

func TestChecker_Check_LocalFunction(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	sess := c.Check("local function foo() return 1 end", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if len(sess.Results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(sess.Results))
	}
}

func TestChecker_PassCalled(t *testing.T) {
	called := false
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithPass(func(_ *Session, _ *ast.FunctionExpr, _ *api.FuncResult) []diag.Diagnostic {
		called = true
		return nil
	}))
	c.Check("local x = 1", "test.lua")
	if !called {
		t.Error("pass was not called")
	}
}

func TestChecker_PassReturnsDiagnostics(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithPass(func(_ *Session, _ *ast.FunctionExpr, _ *api.FuncResult) []diag.Diagnostic {
		return []diag.Diagnostic{{Message: "test error"}}
	}))
	sess := c.Check("local x = 1", "test.lua")
	found := false
	for _, d := range sess.Diagnostics {
		if d.Message == "test error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pass diagnostic not in session")
	}
}

func TestChecker_CheckChunk(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	chunk := []ast.Stmt{
		&ast.LocalAssignStmt{
			Names: []string{"x"},
			Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
		},
	}
	sess := c.CheckChunk(chunk, "test.lua")
	if sess == nil {
		t.Fatal("CheckChunk returned nil")
	}
	if len(sess.Results) == 0 {
		t.Error("no results")
	}
}

func TestChecker_TypeofNarrowing_UsesSolvedFlowProjection(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine(), GlobalTypes: stdlib.Library()})
	code := `
		local x: string | number = "hello"
		if type(x) == "string" then
			local s = x
		end
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.RootResult == nil {
		t.Fatal("RootResult is nil")
	}
	flowOps := sess.RootResult.SolvedFlow()
	if flowOps == nil {
		t.Fatal("SolvedFlow projection is nil")
	}

	found := false
	for _, point := range sess.RootResult.Graph.RPO() {
		symX, ok := sess.RootResult.Graph.SymbolAt(point, "x")
		if !ok || symX == 0 {
			continue
		}
		path := constraint.Path{Root: "x", Symbol: symX}
		if narrowed := flowOps.NarrowedTypeAt(point, path); typ.TypeEquals(narrowed, typ.String) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected canonical solved-flow projection to observe string-narrowed x in then branch")
	}
}

func TestChecker_FunctionLiteralsSummariesBySym_Nested(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	code := `
		local f = function() return 1 end
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.Store == nil || sess.Store.Module == nil || sess.Store.Module.Functions == nil {
		t.Fatal("Store or function registry is nil")
	}
	// Should have summary for nested function (f)
	if len(sess.Store.Module.Functions.BySym) < 1 {
		t.Errorf("expected at least 1 function symbol, got %d", len(sess.Store.Module.Functions.BySym))
	}
}

func TestChecker_FunctionLiteralsSummariesBySym_InTableField(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	code := `
		local t = {
			method = function(self)
				return self
			end
		}
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.Store == nil || sess.Store.Module == nil || sess.Store.Module.Functions == nil {
		t.Fatal("Store or function registry is nil")
	}
	// Should have summary for table field function (method)
	if len(sess.Store.Module.Functions.BySym) < 1 {
		t.Errorf("expected at least 1 function symbol, got %d", len(sess.Store.Module.Functions.BySym))
	}
}

func TestChecker_FunctionLiteralsSummariesBySym_InCallArg(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	code := `
		local function map(fn) return fn end
		local result = map(function(x) return x end)
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.Store == nil || sess.Store.Module == nil || sess.Store.Module.Functions == nil {
		t.Fatal("Store or function registry is nil")
	}
	// Should find: map function + callback
	if len(sess.Store.Module.Functions.BySym) < 2 {
		t.Errorf("expected at least 2 function symbols, got %d", len(sess.Store.Module.Functions.BySym))
	}
}

func TestChecker_FunctionLiteralsSummariesBySym_InReturn(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	code := `
		local function factory()
			return function(x) return x end
		end
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if sess.Store == nil || sess.Store.Module == nil || sess.Store.Module.Functions == nil {
		t.Fatal("Store or function registry is nil")
	}
	// Should find: factory function + returned function
	if len(sess.Store.Module.Functions.BySym) < 2 {
		t.Errorf("expected at least 2 function symbols, got %d", len(sess.Store.Module.Functions.BySym))
	}
}

func TestChecker_FunctionLiteralsAnalyzed(t *testing.T) {
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()})
	code := `
		local t = {
			method = function(self)
				return self
			end
		}
	`
	sess := c.Check(code, "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	// Function literals should be analyzed. After fixpoint convergence,
	// AnalyzedLiterals is cleared (iteration-local), but results persist.
	if sess.Store == nil {
		t.Fatal("missing store")
	}
	// At least the root + table method function should have results.
	if len(sess.Results) < 2 {
		t.Errorf("expected at least 2 function results (root + method), got %d", len(sess.Results))
	}
}

func TestBuildInitialSymbolTypes_GlobalsGetTyped(t *testing.T) {
	// Build a graph that uses a global variable
	code := `print("hello")`
	chunk, err := parse.Parse(strings.NewReader(code), "test.lua")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: chunk}
	graph := cfg.Build(fn, "print") // "print" is a global

	// Global types with "print" defined
	globalTypes := map[string]typ.Type{
		"print": &typ.Function{},
	}

	// Call BuildInitialSymbolTypes from resolve package
	result := phase.BuildInitialSymbolTypes(graph, globalenv.TypeOverlayFromMap(globalTypes), nil)

	// The global "print" should get its type
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	found := false
	for _, types := range result {
		for sym, tv := range types {
			name := graph.NameOf(sym)
			if name == "print" && tv.Type != nil {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected global 'print' to be typed")
	}
}
