package extract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	compcfg "github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
)

// functionLiteralForIdent resolves an identifier to its underlying function
// literal when the symbol is bound to a local function definition/literal.
func (s *Synthesizer) functionLiteralForIdent(ident *ast.IdentExpr) *ast.FunctionExpr {
	if ident == nil {
		return nil
	}

	var graph *compcfg.Graph
	if s.deps.CheckCtx != nil {
		if g, ok := s.deps.CheckCtx.Graph().(*compcfg.Graph); ok {
			graph = g
		}
	}

	bindings := s.deps.ModuleBindings
	if graph != nil && graph.Bindings() != nil {
		bindings = graph.Bindings()
	}
	moduleBindings := s.deps.ModuleBindings

	hasFunctionLiteral := func(sym compcfg.SymbolID) bool {
		if sym == 0 {
			return false
		}
		if fn := callsite.FunctionLiteralForSymbol(graph, bindings, sym); fn != nil {
			return true
		}
		if moduleBindings != nil && moduleBindings != bindings {
			return callsite.FunctionLiteralForSymbol(graph, moduleBindings, sym) != nil
		}
		return false
	}

	sym := callsite.CanonicalSymbolFromExprWithAliases(ident, 0, graph, bindings, moduleBindings, hasFunctionLiteral)
	if sym == 0 {
		return nil
	}
	if fn := callsite.FunctionLiteralForSymbol(graph, bindings, sym); fn != nil {
		return fn
	}
	if moduleBindings != nil && moduleBindings != bindings {
		if fn := callsite.FunctionLiteralForSymbol(graph, moduleBindings, sym); fn != nil {
			return fn
		}
	}

	return nil
}
