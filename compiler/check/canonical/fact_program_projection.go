package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/facts"
	canonicalsig "github.com/wippyai/go-lua/compiler/check/canonical/signature"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// factProgramProjection is the declaration/static view consumed by module fact
// extraction. It is intentionally pre-solve: facts may read source annotations,
// bindings, manifests, and static callback contracts, but not converged summaries.
type factProgramProjection struct {
	driver        *Driver
	session       api.AnalysisSession
	program       *program
	moduleAliases []topology.ModuleAlias
}

func (d *Driver) factProgramProjection(sess api.AnalysisSession, prog *program, moduleAliases []topology.ModuleAlias) factProgramProjection {
	return factProgramProjection{
		driver:        d,
		session:       sess,
		program:       prog,
		moduleAliases: moduleAliases,
	}
}

func (p factProgramProjection) programView() facts.Program {
	prog := p.program
	return facts.Program{
		Refs:          prog.refs,
		ModuleAliases: p.moduleAliases,
		Graph: func(ref summary.FuncRef) *cfg.Graph {
			return prog.Graph(ref)
		},
		Evidence: func(g *cfg.Graph) api.FlowEvidence {
			return p.session.EvidenceForGraph(g)
		},
		ResolveCallee: func(g *cfg.Graph, call *ast.FuncCallExpr) (summary.FuncRef, bool) {
			ct := callTyper{d: p.driver, g: g}
			return ct.resolveCalleeRef(call, prog)
		},
		RefForFuncSymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
			return prog.refBySymbol(sym)
		},
		DeclaredReturnTypes: func(ref summary.FuncRef) []typ.Type {
			return append([]typ.Type(nil), prog.declaredReturns[ref]...)
		},
		NestedFuncRefs: func(ref summary.FuncRef) []summary.FuncRef {
			return prog.funcTopology.NestedRefs(ref)
		},
		CallbackOverlaysForRef: func(ref summary.FuncRef) callbackenv.Overlays {
			return p.driver.declaredCallbackOverlaysForRef(prog, ref)
		},
		CalleeCallbackOverlays: p.calleeCallbackOverlays,
		TypeByName:             p.typeByName,
		SetupExprType:          p.setupExprType,
	}
}

func (p factProgramProjection) calleeCallbackOverlays(g *cfg.Graph, call *ast.FuncCallExpr) callbackenv.Overlays {
	resolver := callTyper{d: p.driver, g: g}.callTypeResolver(nil)
	return canonicalcall.StaticCallbackOverlaysForCall(canonicalcall.StaticCallbackOverlayInput{
		Call:     call,
		Resolver: resolver,
	})
}

func (p factProgramProjection) typeByName(name string) typ.Type {
	if name == "" || p.driver == nil {
		return nil
	}
	sc := p.driver.baseScope()
	if sc == nil {
		return nil
	}
	t, ok := sc.LookupType(name)
	if !ok || t == nil || typ.IsAbsentOrUnknown(t) {
		return nil
	}
	return t
}

func (p factProgramProjection) setupExprType(g *cfg.Graph, expr ast.Expr, point cfg.Point) typ.Type {
	if p.driver == nil || expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.FunctionExpr:
		base := p.driver.baseScope()
		if scopes := p.driver.buildPointScopes(g); scopes != nil {
			if sc := scopes[point]; sc != nil {
				base = sc
			}
		}
		return canonicalsig.Build(canonicalsig.Input{
			Function:    e,
			Base:        base,
			ResolveType: p.driver.resolveType,
			ReturnMode:  canonicalsig.ReturnDeclaredOnly,
		})
	case *ast.FuncCallExpr:
		if g == nil {
			return nil
		}
		callee := callTyper{d: p.driver, g: g}.callTypeResolver(nil).ResolveCallee(e.Func)
		fn := unwrap.Function(callee)
		if fn == nil || len(fn.Returns) == 0 {
			return nil
		}
		return fn.Returns[0]
	default:
		return nil
	}
}
