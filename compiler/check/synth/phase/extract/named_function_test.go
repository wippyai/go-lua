package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
)

func newNamedFunctionSynth(localBindings, moduleBindings *bind.BindingTable) *Synthesizer {
	graph := ccfg.BuildWithBindings(&ast.FunctionExpr{}, localBindings)
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:    graph,
		Bindings: localBindings,
	})
	return NewSynthesizer(&Deps{
		CheckCtx:       checkCtx,
		ModuleBindings: moduleBindings,
		PreCache:       make(api.Cache),
		NarrowCache:    make(api.Cache),
	}, api.PhaseTypeResolution)
}

func TestFunctionLiteralForIdent_UsesModuleFallbackSymbolWhenPrimaryHasNoLiteral(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}

	localBindings := bind.NewBindingTable()
	moduleBindings := bind.NewBindingTable()

	const localSym ccfg.SymbolID = 11
	const moduleSym ccfg.SymbolID = 22
	localBindings.Bind(ident, localSym)
	moduleBindings.Bind(ident, moduleSym)

	want := &ast.FunctionExpr{}
	moduleBindings.SetFuncLitSymbol(want, moduleSym)

	synth := newNamedFunctionSynth(localBindings, moduleBindings)
	got := synth.functionLiteralForIdent(ident)
	if got != want {
		t.Fatalf("functionLiteralForIdent() = %v, want module literal %v", got, want)
	}
}

func TestFunctionLiteralForIdent_PrefersPrimaryBindingLiteral(t *testing.T) {
	ident := &ast.IdentExpr{Value: "f"}

	localBindings := bind.NewBindingTable()
	moduleBindings := bind.NewBindingTable()

	const localSym ccfg.SymbolID = 11
	const moduleSym ccfg.SymbolID = 22
	localBindings.Bind(ident, localSym)
	moduleBindings.Bind(ident, moduleSym)

	want := &ast.FunctionExpr{}
	moduleFn := &ast.FunctionExpr{}
	localBindings.SetFuncLitSymbol(want, localSym)
	moduleBindings.SetFuncLitSymbol(moduleFn, moduleSym)

	synth := newNamedFunctionSynth(localBindings, moduleBindings)
	got := synth.functionLiteralForIdent(ident)
	if got != want {
		t.Fatalf("functionLiteralForIdent() = %v, want primary literal %v", got, want)
	}
}

func TestFunctionLiteralForIdent_ResolvesAliasChainLiteral(t *testing.T) {
	stmts, err := parse.ParseString(`
		local function Target()
			return 1
		end
		local a = Target
		local b = a
		local f = b
		local y = f()
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	localBindings := bind.Bind(fn, nil)
	graph := ccfg.BuildWithBindings(fn, localBindings)
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:    graph,
		Bindings: localBindings,
	})
	synth := NewSynthesizer(&Deps{
		CheckCtx:       checkCtx,
		ModuleBindings: localBindings,
		PreCache:       make(api.Cache),
		NarrowCache:    make(api.Cache),
	}, api.PhaseTypeResolution)

	var (
		ident  *ast.IdentExpr
		target ccfg.SymbolID
	)
	graph.EachCallSite(func(p ccfg.Point, info *ccfg.CallInfo) {
		if info == nil || info.CalleeName != "f" {
			return
		}
		if callee, ok := info.Callee.(*ast.IdentExpr); ok {
			ident = callee
		}
		target, _ = graph.SymbolAt(p, "Target")
	})
	if ident == nil || target == 0 {
		t.Fatalf("expected f() call and Target symbol, got ident=%v target=%d", ident, target)
	}

	want, ok := localBindings.FuncLitBySymbol(target)
	if !ok || want == nil {
		t.Fatalf("expected function literal for Target symbol %d", target)
	}

	got := synth.functionLiteralForIdent(ident)
	if got != want {
		t.Fatalf("functionLiteralForIdent(alias chain) = %v, want %v", got, want)
	}
}
