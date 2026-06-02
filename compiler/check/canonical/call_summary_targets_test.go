package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSelectedTargetSignatureReturnsClosesGenericFromProductContext(t *testing.T) {
	chunk, err := parse.ParseString(`
local function identity<T>(x: T): T
    return x
end
local s: string = identity("test")
`, "generic-signature-return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	call := onlyCallExpr(t, chunk)
	driver, prog, rootGraph, ctx := testDriverProgram(t, chunk)
	defer func() {
		driver.activeProgram = nil
		driver.activeCtx = nil
		driver.activeQueries = nil
	}()

	ct := callTyper{d: driver, g: rootGraph}
	targets := canonicalcall.SelectedTargets(ct.resolveCallTargets(
		call,
		prog,
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
	))
	if len(targets) != 1 {
		t.Fatalf("selected targets = %d, want one", len(targets))
	}

	arg := product.FromType(typ.LiteralString("test"))
	callCtx := transferlessProductCallContext([]product.AbstractValue{arg})
	returns := ct.selectedTargetSignatureReturns(
		prog,
		targets[0],
		call,
		callCtx.ArgTypes(),
		callCtx.ExprType,
		flow.CaptureCellsDomain.Bottom(),
		flow.FunctionRefsDomain.Bottom(),
		nil,
	)
	if len(returns) != 1 || !typ.TypeEquals(returns[0], typ.String) {
		t.Fatalf("signature returns = %#v, want [string]; ctx=%v", returns, ctx)
	}
}

func testDriverProgram(t *testing.T, chunk []ast.Stmt) (*Driver, *program, *cfg.Graph, *db.QueryContext) {
	t.Helper()

	driver := NewDriver(Config{
		Types:  core.NewEngine(),
		Stdlib: scope.NewWithBuiltins(),
	})
	sess := newCanonicalTestSession("generic-signature-return.lua")
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
	sess.SetRootFuncNode(root)
	moduleBindings := bind.Bind(root, driver.globalTypes.Names())
	if store := sess.StoreHandle(); store != nil {
		store.SetModuleBindings(moduleBindings)
	}
	rootGraph := sess.GetOrBuildCFG(root)
	if rootGraph == nil {
		t.Fatal("root graph not built")
	}
	sess.RegisterGraphHierarchy(rootGraph)
	moduleAliases := topology.DiscoverModuleAliases(topology.ModuleAliasDiscoveryInput{
		Root:         rootGraph,
		GraphForFunc: sess.GetOrBuildCFG,
		AliasesForGraph: func(g *cfg.Graph) map[cfg.SymbolID]string {
			evidence := sess.EvidenceForGraph(g)
			return modules.AliasesFromAssignments(evidence.Assignments, g)
		},
	})
	driver.resolver = resolve.New(resolve.Config{
		ModuleBindings: moduleBindings,
		ModuleAliases:  moduleAliases,
	})
	driver.typedefCache = make(map[ast.TypeExpr]typ.Type)
	driver.moduleScope = driver.buildModuleScope(sess, rootGraph)
	driver.pointScopes = driver.buildHierarchyScopes(sess, rootGraph)
	prog := driver.buildProgram(sess, rootGraph, topology.ResolveModuleAliases(moduleAliases, driver.cfg.Manifests))
	queries := summary.New(prog)
	driver.activeProgram = prog
	driver.activeCtx = sess.Context()
	driver.activeQueries = queries
	return driver, prog, rootGraph, sess.Context()
}

func onlyCallExpr(t *testing.T, chunk []ast.Stmt) *ast.FuncCallExpr {
	t.Helper()
	for _, stmt := range chunk {
		local, ok := stmt.(*ast.LocalAssignStmt)
		if !ok {
			continue
		}
		for _, expr := range local.Exprs {
			if call, ok := expr.(*ast.FuncCallExpr); ok {
				return call
			}
		}
	}
	t.Fatal("call expression not found")
	return nil
}

func transferlessProductCallContext(args []product.AbstractValue) transfer.ProductCallContext {
	return transfer.ProductCallContext{
		ArgValues:        args,
		RuntimeArgValues: args,
		ExprValue: func(e ast.Expr) (product.AbstractValue, bool) {
			if lit, ok := e.(*ast.StringExpr); ok {
				return product.FromType(typ.LiteralString(lit.Value)), true
			}
			return product.AbstractValue{}, false
		},
	}
}
