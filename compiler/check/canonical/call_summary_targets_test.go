package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
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
	targetSet := ct.resolveCallTargets(
		call,
		prog,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
	)
	targets := targetSet.Select().Targets()
	if len(targets) != 1 {
		t.Fatalf("selected targets = %d, want one", len(targets))
	}

	arg := product.FromType(typ.LiteralString("test"))
	callCtx := transferlessProductCallContext([]product.AbstractValue{arg})
	site, ok := ct.productCallSiteFrame(call, callCtx)
	if !ok {
		t.Fatal("productCallSiteFrame failed")
	}
	projection := ct.productCallOutcomeProjection(site, callCtx, productCallOutcomeOptions{}, nil)
	returns := projection.signatureReturns(targets[0])
	if len(returns) != 1 || !typ.TypeEquals(returns[0], typ.String) {
		t.Fatalf("signature returns = %#v, want [string]; ctx=%v", returns, ctx)
	}
}

func TestSelectedTargetSignatureReturnsClosesNestedGenericRecordFromProductContext(t *testing.T) {
	chunk, err := parse.ParseString(`
type Container<T> = {
    value: T,
    get: fun(self: self): T
}

local function make_container<T>(v: T): Container<T>
    return {
        value = v,
        get = function(self): T return self.value end
    }
end

local c = make_container("hello")
`, "generic-record-signature-return.lua")
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
	targetSet := ct.resolveCallTargets(
		call,
		prog,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
	)
	targets := targetSet.Select().Targets()
	if len(targets) != 1 {
		t.Fatalf("selected targets = %d, want one", len(targets))
	}

	arg := product.FromType(typ.LiteralString("hello"))
	callCtx := transferlessProductCallContext([]product.AbstractValue{arg})
	site, ok := ct.productCallSiteFrame(call, callCtx)
	if !ok {
		t.Fatal("productCallSiteFrame failed")
	}
	projection := ct.productCallOutcomeProjection(site, callCtx, productCallOutcomeOptions{}, nil)
	returns := projection.signatureReturns(targets[0])
	if len(returns) != 1 {
		t.Fatalf("signature returns = %#v, want one return; ctx=%v", returns, ctx)
	}
	expanded := subst.ExpandInstantiated(returns[0])
	rec, ok := expanded.(*typ.Record)
	if !ok {
		t.Fatalf("signature return = %v expanded %v, want record; ctx=%v", returns[0], expanded, ctx)
	}
	value := rec.GetField("value")
	if value == nil || !typ.TypeEquals(value.Type, typ.String) {
		t.Fatalf("value field = %#v, want string; return=%v", value, expanded)
	}
	get := rec.GetField("get")
	getFn, ok := get.Type.(*typ.Function)
	if get == nil || !ok || len(getFn.Returns) != 1 || !typ.TypeEquals(getFn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string; return=%v", get, expanded)
	}
}

func TestProductCallOutcomeRepairsNestedGenericRecordSummary(t *testing.T) {
	chunk, err := parse.ParseString(`
type Container<T> = {
    value: T,
    get: fun(self: self): T
}

local function make_container<T>(v: T): Container<T>
    return {
        value = v,
        get = function(self): T return self.value end
    }
end

local c = make_container("hello")
`, "generic-record-product-outcome.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	call := onlyCallExpr(t, chunk)
	driver, _, rootGraph, ctx := testDriverProgram(t, chunk)
	defer func() {
		driver.activeProgram = nil
		driver.activeCtx = nil
		driver.activeQueries = nil
	}()

	ct := callTyper{d: driver, g: rootGraph}
	arg := product.FromType(typ.LiteralString("hello"))
	callCtx := transferlessProductCallContext([]product.AbstractValue{arg})
	site, ok := ct.productCallSiteFrame(call, callCtx)
	if !ok {
		t.Fatal("productCallSiteFrame failed")
	}
	outcome := ct.productCallOutcomeProjection(site, callCtx, productCallOutcomeOptions{}, nil).outcome()
	targets := outcome.Targets()
	if len(targets) != 1 || len(targets[0].Summary.Returns) != 1 {
		t.Fatalf("product outcome targets = %#v, want one target with one summary return", targets)
	}
	summaryRec, ok := targets[0].Summary.Returns[0].ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("summary return = %v, want record", targets[0].Summary.Returns[0].ProjectValue())
	}
	summaryGet := summaryRec.GetField("get")
	if summaryGet == nil {
		t.Fatal("summary get field missing")
	}
	summaryGetFn, ok := summaryGet.Type.(*typ.Function)
	if !ok || len(summaryGetFn.Returns) != 1 {
		t.Fatalf("summary get field = %#v, want function return", summaryGet)
	}
	if _, isRef := summaryGetFn.Returns[0].(*typ.Ref); isRef {
		t.Fatalf("summary get return leaked unresolved ref: %v", summaryGetFn.Returns[0])
	}
	values := outcome.InferredReturnValues()
	if len(values) != 1 {
		t.Fatalf("product outcome values = %#v, want one value; targets=%#v ctx=%v", values, targets, ctx)
	}
	rec, ok := values[0].ProjectValue().(*typ.Record)
	if !ok {
		t.Fatalf("product outcome = %v, want record; targets=%#v ctx=%v", values[0].ProjectValue(), targets, ctx)
	}
	get := rec.GetField("get")
	getFn, ok := get.Type.(*typ.Function)
	if get == nil || !ok || len(getFn.Returns) != 1 || !typ.TypeEquals(getFn.Returns[0], typ.String) {
		t.Fatalf("get field = %#v, want function returning string; outcome=%v targets=%#v", get, rec, targets)
	}
}

func TestDeclaredTupleClosedTreatsInstantiatedGenericAliasAsClosed(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp},
		typ.NewRecord().Field("value", tp).Build(),
	)
	user := typ.NewRecord().Field("id", typ.String).Build()

	if !declaredTupleClosed([]typ.Type{typ.Instantiate(result, user)}) {
		t.Fatal("closed instantiated declared return was treated as an open generic binder")
	}
	if declaredTupleClosed([]typ.Type{tp}) {
		t.Fatal("open type-parameter declared return was treated as closed")
	}
	if declaredTupleClosed([]typ.Type{typ.Instantiate(result, tp)}) {
		t.Fatal("instantiated declared return with open type argument was treated as closed")
	}
}

func TestDeclaredTupleClosedKeepsResolvedFunctionGenericAliasReturnsOpen(t *testing.T) {
	chunk, err := parse.ParseString(`
type Failure = {code: string, message: string}
type Result<T> = {ok: true, value: T} | {ok: false, error: Failure}
type Envelope = {id: string}

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
	if result.ok then
		return fn(result.value)
	end
	return { ok = false, error = result.error }
end

local function decode(raw: any): Result<Envelope>
	return ok({ id = "evt" })
end
`, "generic-result-declared-return-closed.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	driver, prog, _, _ := testDriverProgram(t, chunk)
	defer func() {
		driver.activeProgram = nil
		driver.activeCtx = nil
		driver.activeQueries = nil
	}()

	genericDeclared := 0
	closedNonGeneric := 0
	for _, ref := range prog.refs {
		fn := prog.funcExpr(ref)
		if fn == nil || len(fn.ReturnTypes) == 0 {
			continue
		}
		if len(fn.TypeParams) > 0 {
			genericDeclared++
			if prog.refHasClosedDeclaredReturns(ref) {
				t.Fatalf("generic declared return %v was treated as a closed caller-visible tuple", prog.declaredReturns[ref])
			}
			continue
		}
		if prog.refHasClosedDeclaredReturns(ref) {
			closedNonGeneric++
		}
	}
	if genericDeclared != 2 {
		t.Fatalf("generic declared function count = %d, want 2", genericDeclared)
	}
	if closedNonGeneric == 0 {
		t.Fatal("non-generic Result<Envelope> return was not treated as closed")
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
	if store := sess.CanonicalStoreHandle(); store != nil {
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
