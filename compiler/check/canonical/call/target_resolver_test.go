package call

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func TestTargetResolverHonorsFunctionAndClosureAuthority(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	resolver := TargetResolver{Bindings: bindings}
	path := callPathKey(bindings, call.Func)

	topFunctions := flow.WithFunctionRef(nil, path, flow.FunctionRefSetTop())
	finiteFunctions := flow.WithFunctionRef(nil, path, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 77}))
	absentFunctions := flow.FunctionRefsDomain.Bottom()
	topClosures := flow.WithClosureRef(nil, path, flow.ClosureRefSetTop())
	absentClosures := flow.ClosureRefsDomain.Bottom()

	withFunctionTop := resolver.Resolve(call, topFunctions, absentClosures)
	if !withFunctionTop.DirectAuthoritative() {
		t.Fatal("direct refs should be authoritative for top FunctionRefs")
	}
	if len(withFunctionTop.DirectRefs()) != 0 {
		t.Fatalf("top FunctionRefs should block static fallback; got %d refs", len(withFunctionTop.DirectRefs()))
	}

	withFunctionsAbsent := resolver.Resolve(call, absentFunctions, absentClosures)
	if withFunctionsAbsent.DirectAuthoritative() {
		t.Fatal("direct refs should be non-authoritative when FunctionRefs path is absent")
	}
	if len(withFunctionsAbsent.DirectRefs()) != 0 {
		t.Fatalf("absent FunctionRefs should not resolve direct refs without static fallback; got %d refs", len(withFunctionsAbsent.DirectRefs()))
	}

	withClosureTop := resolver.Resolve(call, absentFunctions, topClosures)
	if !withClosureTop.ClosureAuthoritative() {
		t.Fatal("closure refs should be authoritative for top ClosureRefs")
	}
	if len(withClosureTop.ClosureRefs()) != 0 {
		t.Fatalf("top ClosureRefs should have no concrete refs and stay authoritative; got %d", len(withClosureTop.ClosureRefs()))
	}

	withClosureTopAndFiniteDirect := resolver.Resolve(call, finiteFunctions, topClosures)
	if !withClosureTopAndFiniteDirect.ClosureAuthoritative() {
		t.Fatal("closure refs should stay authoritative for top ClosureRefs")
	}
	if withClosureTopAndFiniteDirect.UseClosureTargets() {
		t.Fatal("top ClosureRefs should not select closure targets")
	}
	if !withClosureTopAndFiniteDirect.UseDirectTargets() {
		t.Fatal("finite FunctionRefs should be the fallback when ClosureRefs is top/unknown")
	}
	if got := withClosureTopAndFiniteDirect.DirectRefs(); len(got) != 1 || got[0].GraphID != 77 {
		t.Fatalf("DirectRefs with top ClosureRefs = %+v, want graph 77", got)
	}

	withClosureAbsent := resolver.Resolve(call, absentFunctions, absentClosures)
	if withClosureAbsent.ClosureAuthoritative() {
		t.Fatal("closure refs should be non-authoritative when ClosureRefs path is absent")
	}
	if len(withClosureAbsent.ClosureRefs()) != 0 {
		t.Fatalf("absent ClosureRefs should not resolve closure refs; got %d", len(withClosureAbsent.ClosureRefs()))
	}
}

func TestTargetResolverStaticFallbackOnlyWhenProductAxisAbsent(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	path := callPathKey(bindings, call.Func)
	staticRef := summary.FuncRef{GraphID: 99}
	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(cfg.SymbolID) (summary.FuncRef, bool) {
				return staticRef, true
			},
		},
	}

	withAbsentProduct := resolver.Resolve(call, flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom())
	if got := withAbsentProduct.DirectRefs(); len(got) != 1 || got[0] != staticRef {
		t.Fatalf("static fallback refs = %+v, want %+v", got, staticRef)
	}

	withProductTop := resolver.Resolve(call, flow.WithFunctionRef(nil, path, flow.FunctionRefSetTop()), flow.ClosureRefsDomain.Bottom())
	if got := withProductTop.DirectRefs(); len(got) != 0 {
		t.Fatalf("authoritative product top should block static fallback; got %+v", got)
	}
}

func TestTargetResolverFunctionRefsAtExprAreAuthoritative(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	path := callPathKey(bindings, call.Func)
	staticRef := summary.FuncRef{GraphID: 99}
	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(cfg.SymbolID) (summary.FuncRef, bool) {
				return staticRef, true
			},
		},
	}

	liveRefs := flow.WithFunctionRef(nil, path, flow.FunctionRefSetOf(
		flow.FunctionRef{GraphID: 20},
		flow.FunctionRef{GraphID: 10},
	))
	got, ok := resolver.ResolveFunctionRefsAtExpr(call.Func, liveRefs)
	if !ok || len(got) != 2 || got[0].GraphID != 10 || got[1].GraphID != 20 {
		t.Fatalf("ResolveFunctionRefsAtExpr finite = %+v/%v, want sorted live refs 10,20", got, ok)
	}

	topRefs := flow.WithFunctionRef(nil, path, flow.FunctionRefSetTop())
	got, ok = resolver.ResolveFunctionRefsAtExpr(call.Func, topRefs)
	if !ok || len(got) != 0 {
		t.Fatalf("ResolveFunctionRefsAtExpr top = %+v/%v, want authoritative unknown", got, ok)
	}
}

func TestTargetResolverFunctionRefsFallbackToRawSymbol(t *testing.T) {
	t.Parallel()

	const rawSym cfg.SymbolID = 88
	arg := &ast.IdentExpr{Value: "cb"}
	resolver := TargetResolver{}
	liveRefs := flow.WithFunctionRef(nil, flow.SymbolPathKey(rawSym, nil), flow.FunctionRefSetOf(
		flow.FunctionRef{GraphID: 30},
		flow.FunctionRef{GraphID: 10},
	))

	got, ok := resolver.ResolveFunctionRefsAtExprOrSymbol(arg, liveRefs, rawSym)
	if !ok || len(got) != 2 || got[0].GraphID != 10 || got[1].GraphID != 30 {
		t.Fatalf("ResolveFunctionRefsAtExprOrSymbol = %+v/%v, want sorted raw-symbol refs 10,30", got, ok)
	}
}

func TestTargetResolverClosureRefSetAtExprIsAuthoritative(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	path := callPathKey(bindings, call.Func)
	resolver := TargetResolver{Bindings: bindings}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 20}, flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom())
	liveRefs := flow.WithClosureRef(nil, path, flow.ClosureRefSetOf(closure))

	got, ok := resolver.ResolveClosureRefSetAtExpr(call.Func, liveRefs)
	if !ok || got.IsBottom() || got.IsTop() {
		t.Fatalf("ResolveClosureRefSetAtExpr finite = %v/%v, want finite set", got.Format(), ok)
	}
	refs := got.Refs()
	if len(refs) != 1 || refs[0].Ref.GraphID != 20 {
		t.Fatalf("ResolveClosureRefSetAtExpr refs = %+v, want graph 20", refs)
	}

	topRefs := flow.WithClosureRef(nil, path, flow.ClosureRefSetTop())
	got, ok = resolver.ResolveClosureRefSetAtExpr(call.Func, topRefs)
	if !ok || !got.IsTop() {
		t.Fatalf("ResolveClosureRefSetAtExpr top = %v/%v, want authoritative top", got.Format(), ok)
	}
}

func TestTargetResolverCallbackArgRefsUseLiveAxisBeforeStaticFallback(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	path := callPathKey(bindings, call.Func)
	staticUsed := false
	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(cfg.SymbolID) (summary.FuncRef, bool) {
				staticUsed = true
				return summary.FuncRef{GraphID: 99}, true
			},
		},
	}
	liveRefs := flow.WithFunctionRef(nil, path, flow.FunctionRefSetOf(
		flow.FunctionRef{GraphID: 20},
		flow.FunctionRef{GraphID: 10},
	))

	got, ok := resolver.ResolveCallbackArgRefs(call.Func, liveRefs, nil)
	if !ok || len(got) != 2 || got[0].GraphID != 10 || got[1].GraphID != 20 {
		t.Fatalf("ResolveCallbackArgRefs = %+v/%v, want sorted live refs 10,20", got, ok)
	}
	if staticUsed {
		t.Fatal("static fallback ran despite authoritative live callback refs")
	}
}

func TestTargetResolverCallbackArgRefsFunctionLiteralBeatsStaticFallback(t *testing.T) {
	t.Parallel()

	arg := &ast.FunctionExpr{}
	literalRef := summary.FuncRef{GraphID: 7}
	staticUsed := false
	resolver := TargetResolver{
		Static: StaticTargetLookup{
			FuncBySymbol: func(cfg.SymbolID) (summary.FuncRef, bool) {
				staticUsed = true
				return summary.FuncRef{GraphID: 8}, true
			},
		},
	}

	got, ok := resolver.ResolveCallbackArgRefs(arg, flow.FunctionRefsDomain.Bottom(), func(fn *ast.FunctionExpr) (summary.FuncRef, bool) {
		if fn != arg {
			t.Fatalf("function literal resolver got %#v, want arg", fn)
		}
		return literalRef, true
	})

	if !ok || len(got) != 1 || got[0] != literalRef {
		t.Fatalf("ResolveCallbackArgRefs = %+v/%v; want literal ref", got, ok)
	}
	if staticUsed {
		t.Fatal("static fallback ran despite function literal ref")
	}
}

func TestTargetResolverCallbackArgRefsStaticFallback(t *testing.T) {
	t.Parallel()

	arg := &ast.IdentExpr{Value: "cb"}
	staticRef := summary.FuncRef{GraphID: 9}
	bindings := bind.NewBindingTable()
	bindings.Bind(arg, 90)
	bindings.SetName(90, "cb")
	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
				if sym != 90 {
					t.Fatalf("FuncBySymbol sym = %d, want 90", sym)
				}
				return staticRef, true
			},
		},
	}

	got, ok := resolver.ResolveCallbackArgRefs(arg, flow.FunctionRefsDomain.Bottom(), func(*ast.FunctionExpr) (summary.FuncRef, bool) {
		t.Fatal("function literal resolver ran for ident")
		return summary.FuncRef{}, false
	})

	if !ok || len(got) != 1 || got[0] != staticRef {
		t.Fatalf("ResolveCallbackArgRefs = %+v/%v; want static ref", got, ok)
	}
}

func TestTargetResolverCallbackArgRefsLiveTopBlocksStaticFallback(t *testing.T) {
	t.Parallel()

	call, bindings := testCallForPath()
	path := callPathKey(bindings, call.Func)
	staticUsed := false
	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(cfg.SymbolID) (summary.FuncRef, bool) {
				staticUsed = true
				return summary.FuncRef{GraphID: 30}, true
			},
		},
	}
	liveRefs := flow.WithFunctionRef(nil, path, flow.FunctionRefSetTop())

	got, ok := resolver.ResolveCallbackArgRefs(call.Func, liveRefs, nil)
	if !ok || len(got) != 0 {
		t.Fatalf("ResolveCallbackArgRefs = %+v/%v, want authoritative unknown", got, ok)
	}
	if staticUsed {
		t.Fatal("static fallback ran despite authoritative unknown FunctionRefs")
	}
}

func TestTargetResolverStaticExprOrSymbolExpandsAliases(t *testing.T) {
	t.Parallel()

	stmts, err := parse.ParseString(`
		local function Target()
			return 1
		end
		local a = Target
		local b = a
		local use = b
		use()
	`, "target_resolver_alias.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}
	var (
		useExpr *ast.IdentExpr
		target  cfg.SymbolID
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "use" {
			return
		}
		if ident, ok := info.Callee.(*ast.IdentExpr); ok {
			useExpr = ident
		}
		target, _ = graph.SymbolAt(p, "Target")
	})
	if useExpr == nil || target == 0 {
		t.Fatalf("expected use ident and Target symbol, got use=%v target=%d", useExpr, target)
	}
	raw, ok := bindings.SymbolOf(useExpr)
	if !ok || raw == 0 {
		t.Fatalf("expected raw symbol for use, got %d/%v", raw, ok)
	}
	want := summary.FuncRef{GraphID: 77}
	resolver := TargetResolver{
		Graph:    graph,
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
				return want, sym == target
			},
		},
	}

	got, ok := resolver.ResolveStaticExprOrSymbol(useExpr, raw)
	if !ok || got != want {
		t.Fatalf("ResolveStaticExprOrSymbol = %+v/%v, want %+v/true", got, ok, want)
	}
}

func TestTargetResolverCallbackArgRefsExpandsStaticAliases(t *testing.T) {
	t.Parallel()

	stmts, err := parse.ParseString(`
		local function Target()
			return 1
		end
		local a = Target
		local b = a
		local use = b
		consume(use)
	`, "target_resolver_callback_alias.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: stmts})
	if graph == nil {
		t.Fatal("expected graph")
	}
	bindings := graph.Bindings()
	if bindings == nil {
		t.Fatal("expected bindings")
	}
	var (
		arg    ast.Expr
		target cfg.SymbolID
	)
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || info.CalleeName != "consume" || len(info.Args) == 0 {
			return
		}
		arg = info.Args[0]
		target, _ = graph.SymbolAt(p, "Target")
	})
	if arg == nil || target == 0 {
		t.Fatalf("expected callback arg and Target symbol, got arg=%v target=%d", arg, target)
	}
	argIdent, ok := arg.(*ast.IdentExpr)
	if !ok || argIdent == nil {
		t.Fatalf("expected ident callback arg, got %T", arg)
	}
	raw, ok := bindings.SymbolOf(argIdent)
	if !ok || raw == 0 {
		t.Fatalf("expected raw symbol for callback arg, got %d/%v", raw, ok)
	}
	want := summary.FuncRef{GraphID: 77}
	resolver := TargetResolver{
		Graph:    graph,
		Bindings: bindings,
		Static: StaticTargetLookup{
			FuncBySymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
				return want, sym == target
			},
		},
	}

	got, ok := resolver.ResolveCallbackArgRefsOrSymbol(arg, flow.FunctionRefsDomain.Bottom(), raw, nil)
	if !ok || len(got) != 1 || got[0] != want {
		t.Fatalf("ResolveCallbackArgRefsOrSymbol = %+v/%v, want [%+v]/true", got, ok, want)
	}
}

func TestTargetResolverStaticMethodFallbackOrder(t *testing.T) {
	t.Parallel()

	recv := &ast.IdentExpr{Value: "obj"}
	sym := cfg.SymbolID(43)
	bindings := bind.NewBindingTable()
	bindings.Bind(recv, sym)
	bindings.SetName(sym, "obj")
	call := &ast.FuncCallExpr{Receiver: recv, Method: "run"}
	fieldRef := summary.FuncRef{GraphID: 10}
	selfRef := summary.FuncRef{GraphID: 20}

	resolver := TargetResolver{
		Bindings: bindings,
		Static: StaticTargetLookup{
			FieldFunc: func(gotSym cfg.SymbolID, field fieldkey.Key) (summary.FuncRef, bool) {
				if gotSym != sym || field.Name != "run" {
					t.Fatalf("FieldFunc got (%d,%+v), want (%d,run)", gotSym, field, sym)
				}
				return fieldRef, true
			},
			SelfMethodRef: func(gotSym cfg.SymbolID, field fieldkey.Key) (summary.FuncRef, bool) {
				if gotSym != sym || field.Name != "run" {
					t.Fatalf("SelfMethodRef got (%d,%+v), want (%d,run)", gotSym, field, sym)
				}
				return selfRef, true
			},
		},
	}

	ref, ok := resolver.ResolveStaticMethod(call)
	if !ok || ref != fieldRef {
		t.Fatalf("ResolveStaticMethod = %+v,%v; want field ref %+v,true", ref, ok, fieldRef)
	}

	resolver.Static.FieldFunc = nil
	ref, ok = resolver.ResolveStaticMethod(call)
	if !ok || ref != selfRef {
		t.Fatalf("ResolveStaticMethod self fallback = %+v,%v; want %+v,true", ref, ok, selfRef)
	}
}

func TestTargetResolverStaticMethodRequiresIdentReceiver(t *testing.T) {
	t.Parallel()

	called := false
	resolver := TargetResolver{
		Bindings: bind.NewBindingTable(),
		Static: StaticTargetLookup{
			FieldFunc: func(cfg.SymbolID, fieldkey.Key) (summary.FuncRef, bool) {
				called = true
				return summary.FuncRef{GraphID: 10}, true
			},
			SelfMethodRef: func(cfg.SymbolID, fieldkey.Key) (summary.FuncRef, bool) {
				called = true
				return summary.FuncRef{GraphID: 20}, true
			},
		},
	}
	call := &ast.FuncCallExpr{
		Receiver: &ast.AttrGetExpr{
			Object: &ast.IdentExpr{Value: "obj"},
			Key:    &ast.StringExpr{Value: "nested"},
		},
		Method: "run",
	}

	if ref, ok := resolver.ResolveStaticMethod(call); ok || ref != (summary.FuncRef{}) {
		t.Fatalf("ResolveStaticMethod non-ident receiver = %+v,%v; want zero,false", ref, ok)
	}
	if called {
		t.Fatal("static method callbacks ran for non-ident receiver")
	}
}

func testCallForPath() (*ast.FuncCallExpr, *bind.BindingTable) {
	ident := &ast.IdentExpr{Value: "callee"}
	symbol := cfg.SymbolID(42)
	b := bind.NewBindingTable()
	b.Bind(ident, symbol)
	b.SetName(symbol, "callee")
	return &ast.FuncCallExpr{Func: ident}, b
}

func callPathKey(bindings *bind.BindingTable, expr ast.Expr) constraint.PathKey {
	path := flowpath.FromExprWithBindings(expr, nil, bindings)
	if path.IsEmpty() {
		panic(fmt.Sprintf("unexpected empty path for %T", expr))
	}
	return path.Key()
}
