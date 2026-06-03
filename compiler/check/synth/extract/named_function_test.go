package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	ccfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
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
	}, api.SynthModeResolve)
}

func TestFunctionLiteralForIdent_UsesModuleSymbolWhenPrimaryHasNoLiteral(t *testing.T) {
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
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
	}, api.SynthModeResolve)

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

func TestGraphLocalFunctionLiteralForExpr_ResolvesFieldDefinitionAttr(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {}
		function M.run()
			return 1
		end
		local f = M.run
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
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
	}, api.SynthModeResolve)

	var (
		attr *ast.AttrGetExpr
		want *ast.FunctionExpr
	)
	graph.EachAssign(func(_ ccfg.Point, info *ccfg.AssignInfo) {
		if attr != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, _ ccfg.AssignTarget, source ast.Expr) {
			if attr != nil {
				return
			}
			if candidate, ok := source.(*ast.AttrGetExpr); ok {
				attr = candidate
			}
		})
	})
	graph.EachFuncDef(func(_ ccfg.Point, info *ccfg.FuncDefInfo) {
		if want != nil || info == nil || info.Name != "run" {
			return
		}
		want = info.FuncExpr
	})
	if attr == nil || want == nil {
		t.Fatalf("expected alias assignment attr and run func def, got attr=%v want=%v", attr, want)
	}

	got := synth.graphLocalFunctionLiteralForExpr(attr)
	if got != want {
		t.Fatalf("graphLocalFunctionLiteralForExpr(M.run) = %v, want %v", got, want)
	}
}

func TestGraphLocalFunctionLiteralForExpr_IgnoresMutableFieldPathAttr(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		M.dep = {
			get = function()
				return 1
			end,
		}
		local f = M.dep.get
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
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
	}, api.SynthModeResolve)

	var attr *ast.AttrGetExpr
	graph.EachAssign(func(_ ccfg.Point, info *ccfg.AssignInfo) {
		if attr != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, _ ccfg.AssignTarget, source ast.Expr) {
			if attr != nil {
				return
			}
			if candidate, ok := source.(*ast.AttrGetExpr); ok {
				attr = candidate
			}
		})
	})
	if attr == nil {
		t.Fatal("expected alias assignment attr source")
	}

	if got := synth.graphLocalFunctionLiteralForExpr(attr); got != nil {
		t.Fatalf("graphLocalFunctionLiteralForExpr(M.dep.get) = %v, want nil", got)
	}
}

func TestHasDominatingDirectFunctionRebind_FalseWhenOnlyCapturedFieldChanges(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
		local f = M.run
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
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
	}, api.SynthModeResolve)

	var (
		attr *ast.AttrGetExpr
		at   ccfg.Point
	)
	graph.EachAssign(func(p ccfg.Point, info *ccfg.AssignInfo) {
		if attr != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, _ ccfg.AssignTarget, source ast.Expr) {
			if attr != nil {
				return
			}
			if candidate, ok := source.(*ast.AttrGetExpr); ok {
				attr = candidate
				at = p
			}
		})
	})
	if attr == nil {
		t.Fatal("expected alias assignment attr source")
	}

	sym, stableFn, _ := synth.graphLocalFunctionForExpr(attr)
	if stableFn == nil || sym == 0 {
		t.Fatalf("expected stable graph-local function for attr, got sym=%d fn=%v", sym, stableFn)
	}
	if synth.hasDominatingDirectFunctionRebind(sym, stableFn, at) {
		t.Fatal("captured field mutation should not invalidate field-defined wrapper value")
	}
}

func TestHasDominatingDirectFunctionRebind_TrueWhenFieldIsReassigned(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.run = function()
			return nil
		end
		local f = M.run
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
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
	}, api.SynthModeResolve)

	var (
		attr *ast.AttrGetExpr
		at   ccfg.Point
	)
	graph.EachAssign(func(p ccfg.Point, info *ccfg.AssignInfo) {
		if attr != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, _ ccfg.AssignTarget, source ast.Expr) {
			if attr != nil {
				return
			}
			if candidate, ok := source.(*ast.AttrGetExpr); ok {
				attr = candidate
				at = p
			}
		})
	})
	if attr == nil {
		t.Fatal("expected alias assignment attr source")
	}

	sym, stableFn, _ := synth.graphLocalFunctionForExpr(attr)
	if stableFn == nil || sym == 0 {
		t.Fatalf("expected stable graph-local function for attr, got sym=%d fn=%v", sym, stableFn)
	}
	if !synth.hasDominatingDirectFunctionRebind(sym, stableFn, at) {
		t.Fatal("direct dominating field reassignment should invalidate field-defined wrapper value")
	}
}

func TestSynthExprWithSpec_ProjectsFieldDefinedFunctionWithCapturedPathOverlay(t *testing.T) {
	stmts, err := parse.ParseString(`
		local M = {
			dep = {
				get = function()
					return nil
				end,
			},
		}
		function M.run()
			return M.dep.get()
		end
		M.dep = {
			get = function()
				return { answer = "ok" }
			end,
		}
		local f = M.run
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	localBindings := bind.Bind(fn, nil)
	graph := ccfg.BuildWithBindings(fn, localBindings)

	var (
		attr  *ast.AttrGetExpr
		at    ccfg.Point
		runFn *ast.FunctionExpr
	)
	graph.EachAssign(func(p ccfg.Point, info *ccfg.AssignInfo) {
		if attr != nil || info == nil {
			return
		}
		info.EachTargetSource(func(_ int, target ccfg.AssignTarget, source ast.Expr) {
			if attr != nil || target.Name != "f" {
				return
			}
			if candidate, ok := source.(*ast.AttrGetExpr); ok {
				attr = candidate
				at = p
			}
		})
	})
	graph.EachFuncDef(func(_ ccfg.Point, info *ccfg.FuncDefInfo) {
		if info != nil && info.Name == "run" {
			runFn = info.FuncExpr
		}
	})
	mSym, _ := graph.SymbolAt(at, "M")
	if attr == nil || at == 0 || runFn == nil || mSym == 0 {
		t.Fatalf("missing test coordinates attr=%v at=%d runFn=%v mSym=%d", attr, at, runFn, mSym)
	}

	res := typ.NewRecord().Field("answer", typ.String).Build()
	staleRun := typ.Func().Returns(typ.Nil).Build()
	currentGet := typ.Func().Returns(res).Build()
	currentM := typ.NewRecord().
		Field("dep", typ.NewRecord().Field("get", currentGet).Build()).
		Field("run", staleRun).
		Build()

	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:    graph,
		Bindings: localBindings,
	})
	baseScope := scope.New()
	graphs := newTestGraphProvider()
	childGraph := ccfg.BuildWithBindings(runFn, localBindings)
	graphs.cache[runFn] = childGraph
	synth := NewSynthesizer(&Deps{
		Ctx:            db.NewQueryContext(db.New()),
		Types:          querycore.NewEngine(),
		Scopes:         api.ScopeMap{at: baseScope},
		DefaultScope:   baseScope,
		CheckCtx:       checkCtx,
		Evidence:       trace.GraphEvidence(graph, localBindings),
		ModuleBindings: localBindings,
		Graphs:         graphs,
	}, api.SynthModeDeclared)

	got := unwrap.Function(synth.synthExprWithSpec(attr, at, api.SpecTypes{mSym: currentM}))
	if got == nil || len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], res) {
		t.Fatalf("synthExprWithSpec(M.run) = %v, want callable returning %v; stmts=%d returns=%d", got, res, len(runFn.Stmts), len(trace.GraphEvidence(childGraph, localBindings).Returns))
	}
}
