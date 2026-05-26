package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRunNarrow_NilExtractInputs(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	input := NarrowInput{
		PhaseEnv: PhaseEnv{
			Graph: graph,
			Fn:    fn,
		},
		Scope: ScopeOutput{
			BaseScope: scope.New(),
			Scopes:    map[cfg.Point]*scope.State{graph.Entry(): scope.New()},
		},
		Extract: FlowExtractOutput{Inputs: nil},
		Solve:   FlowSolveOutput{Solution: nil},
	}

	output := RunNarrow(input)
	if output.Synth == nil {
		t.Error("expected non-nil Synth engine")
	}
}

func TestRunNarrow_WithSolution(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	base := scope.New()
	scopes := map[cfg.Point]*scope.State{graph.Entry(): base}

	input := NarrowInput{
		PhaseEnv: PhaseEnv{
			Graph: graph,
			Fn:    fn,
		},
		Scope: ScopeOutput{
			BaseScope: base,
			Scopes:    scopes,
		},
		Extract: FlowExtractOutput{
			Inputs: &flow.Inputs{},
			Params: []flow.ParamInfo{},
		},
		Solve: FlowSolveOutput{
			Solution: &flow.Solution{},
		},
	}

	output := RunNarrow(input)
	if output.Synth == nil {
		t.Error("expected non-nil Synth engine")
	}
}

func TestRunNarrow_MergesAnnotatedVars(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}}}
	graph := cfg.Build(fn)
	base := scope.New()
	scopes := map[cfg.Point]*scope.State{graph.Entry(): base}

	paramSyms := graph.ParamSymbols()
	if len(paramSyms) == 0 || paramSyms[0] == 0 {
		t.Fatal("expected parameter symbol")
	}
	sym := paramSyms[0]

	input := NarrowInput{
		PhaseEnv: PhaseEnv{
			Graph: graph,
			Fn:    fn,
		},
		Scope: ScopeOutput{
			BaseScope:     base,
			Scopes:        scopes,
			AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
		},
		Extract: FlowExtractOutput{
			Inputs: &flow.Inputs{
				DeclaredTypes: make(map[cfg.SymbolID]typ.Type),
			},
		},
		Solve: FlowSolveOutput{Solution: &flow.Solution{}},
	}

	output := RunNarrow(input)
	if output.Facts == nil {
		t.Fatal("expected non-nil Facts")
	}
	if !output.Facts.IsAnnotated(sym) {
		t.Errorf("expected annotated vars to include param symbol %d", sym)
	}
}

func TestNewPathFromExprFunc_NilSolution(t *testing.T) {
	pathFunc := newPathFromExprFunc(nil, nil, nil)
	if pathFunc == nil {
		t.Fatal("expected non-nil PathFromExprFunc")
	}
	result := pathFunc(0, nil, nil)
	if !result.IsEmpty() {
		t.Errorf("expected empty path for nil solution, got %v", result)
	}
}

func TestNewPathFromExprFunc_ValidSolution(t *testing.T) {
	solution := &flow.Solution{}
	pathFunc := newPathFromExprFunc(solution, nil, nil)
	if pathFunc == nil {
		t.Fatal("expected non-nil PathFromExprFunc")
	}
	result := pathFunc(0, nil, nil)
	// Empty path is valid for nil expr
	if result.Root != "" && result.Symbol != 0 {
		t.Error("expected empty path for nil expr")
	}
}

func TestNewPathFromExprFunc_WithIdent(t *testing.T) {
	solution := &flow.Solution{}
	pathFunc := newPathFromExprFunc(solution, nil, nil)
	ident := &ast.IdentExpr{Value: "x"}
	result := pathFunc(0, ident, nil)
	// Without bindings, path may be empty or have root "x"
	if result.Root != "" && result.Root != "x" {
		t.Errorf("path root should be empty or 'x', got %v", result.Root)
	}
}

func TestNewPathFromExprFunc_AttachesVisibleSSAVersion(t *testing.T) {
	body, err := parse.ParseString(`
		local x = 1
		return x
	`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: body}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected graph")
	}
	var retPoint cfg.Point
	var retExpr ast.Expr
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if len(info.Exprs) > 0 {
			retPoint = p
			retExpr = info.Exprs[0]
		}
	})
	ident, ok := retExpr.(*ast.IdentExpr)
	if !ok || ident == nil {
		t.Fatalf("expected return identifier, got %T", retExpr)
	}
	sym, ok := graph.Bindings().SymbolOf(ident)
	if !ok || sym == 0 {
		t.Fatal("expected return identifier to have symbol")
	}
	want := graph.VisibleVersion(retPoint, sym)
	if want.IsZero() {
		t.Fatalf("expected visible version for symbol %d at return point %d", sym, retPoint)
	}

	pathFunc := newPathFromExprFunc(&flow.Solution{}, graph.Bindings(), graph)
	got := pathFunc(retPoint, ident, nil)
	if got.Symbol != sym {
		t.Fatalf("path symbol = %d, want %d", got.Symbol, sym)
	}
	if got.Version != want.ID {
		t.Fatalf("path version = %d, want %d", got.Version, want.ID)
	}
}
