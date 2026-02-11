package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
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
