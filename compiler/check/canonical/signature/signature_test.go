package signature

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildFunctionUnannotatedParamIsOptionalGradualAny(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("x")
	sig := (Input{
		Function:    fn,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
		ReturnMode:  ReturnDeclaredOnly,
	}).Build()

	if sig == nil || len(sig.Params) != 1 {
		t.Fatalf("params = %#v, want one", sig)
	}
	if sig.Params[0].Name != "x" || !sig.Params[0].Optional || !typ.IsAny(sig.Params[0].Type) {
		t.Fatalf("param = %#v, want optional gradual any x", sig.Params[0])
	}
}

func TestBuildFunctionDeclaredGenericReturnBeatsInferred(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("x")
	fn.TypeParams = []ast.TypeParamExpr{{Name: "T"}}
	fn.ReturnTypes = []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}}
	inferredUsed := false

	sig := (Input{
		Function:    fn,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
		InferredReturns: func(*ast.FunctionExpr) []typ.Type {
			inferredUsed = true
			return []typ.Type{typ.String}
		},
		ReturnMode: ReturnDeclaredThenInferred,
	}).Build()

	if sig == nil || len(sig.TypeParams) != 1 || sig.TypeParams[0].Name != "T" {
		t.Fatalf("type params = %#v, want T", sig.TypeParams)
	}
	if len(sig.Returns) != 1 {
		t.Fatalf("returns = %#v, want one T return", sig.Returns)
	}
	if tp, ok := sig.Returns[0].(*typ.TypeParam); !ok || tp.Name != "T" {
		t.Fatalf("return = %#v, want type param T", sig.Returns[0])
	}
	if inferredUsed {
		t.Fatal("inferred returns ran despite declared generic return")
	}
}

func TestBuildGenericCallbackSignatureClosesExpectedParam(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("x", "fn")
	fn.TypeParams = []ast.TypeParamExpr{{Name: "T"}, {Name: "U"}}
	fn.ParList.Types = []ast.TypeExpr{
		&ast.TypeRefExpr{Path: []string{"T"}},
		&ast.FunctionTypeExpr{
			Params:  []ast.FunctionParamExpr{{Name: "value", Type: &ast.TypeRefExpr{Path: []string{"T"}}}},
			Returns: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"U"}}},
		},
	}
	fn.ReturnTypes = []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"U"}}}

	sig := (Input{
		Function:    fn,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
		ReturnMode:  ReturnDeclaredOnly,
	}).Build()
	callback := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	inferred := ops.InferCall(db.NewQueryContext(db.New()), ops.CallDef{
		Callee: sig,
		Args:   []typ.Type{typ.String, callback},
	})
	expected := inferred.ExpectedArgType(1)
	expectedFn, ok := expected.(*typ.Function)
	if !ok || expectedFn == nil || len(expectedFn.Params) != 1 {
		t.Fatalf("expected callback type = %v, want unary function", expected)
	}
	if !typ.TypeEquals(expectedFn.Params[0].Type, typ.String) {
		t.Fatalf("expected callback param = %v, want string; signature=%v", expectedFn.Params[0].Type, sig)
	}
}

func TestBuildFunctionUnannotatedReturnSplicesInferred(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("x")
	sig := (Input{
		Function:    fn,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
		InferredReturns: func(got *ast.FunctionExpr) []typ.Type {
			if got != fn {
				t.Fatalf("inferred returns got %#v, want fn", got)
			}
			return []typ.Type{typ.Number}
		},
		ReturnMode: ReturnDeclaredThenInferred,
	}).Build()

	if sig == nil || len(sig.Returns) != 1 || sig.Returns[0] != typ.Number {
		t.Fatalf("returns = %#v, want inferred number", sig.Returns)
	}
}

func TestResolvableDeclaredReturnFallsBackWhenNoAnnotationResolves(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("x")
	fn.ReturnTypes = []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"Missing"}}}

	got := (ReturnInput{
		Function: fn,
		Scope:    scope.NewWithBuiltins(),
		ResolveType: func(ast.TypeExpr, *scope.State) typ.Type {
			return nil
		},
		InferredReturns: func(*ast.FunctionExpr) []typ.Type {
			return []typ.Type{typ.Boolean}
		},
		Mode: ReturnResolvableDeclaredThenInferred,
	}).Types()

	if len(got) != 1 || got[0] != typ.Boolean {
		t.Fatalf("returns = %#v, want inferred boolean fallback", got)
	}
}

func TestBuildMethodPrependsNamedReceiverSelf(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("arg")
	info := &cfg.FuncDefInfo{
		ReceiverName: "Service",
		FuncExpr:     fn,
	}
	self := typ.NewRecord().Field("run", typ.Func().Build()).Build()
	sig := (Input{
		Method:      info,
		Base:        scope.NewWithBuiltins().WithType("Service", self),
		ResolveType: testResolveType,
		ReturnMode:  ReturnDeclaredOnly,
	}).Build()

	if sig == nil || len(sig.Params) != 2 {
		t.Fatalf("params = %#v, want self + arg", sig)
	}
	if sig.Params[0].Name != "self" || sig.Params[0].Optional || !typ.TypeEquals(sig.Params[0].Type, self) {
		t.Fatalf("self param = %#v, want required Service", sig.Params[0])
	}
}

func TestBuildMethodUnnamedReceiverSelfIsGradualAny(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("arg")
	sig := (Input{
		Method:      &cfg.FuncDefInfo{FuncExpr: fn},
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
		ReturnMode:  ReturnDeclaredOnly,
	}).Build()

	if sig == nil || len(sig.Params) != 2 {
		t.Fatalf("params = %#v, want self + arg", sig)
	}
	if sig.Params[0].Name != "self" || sig.Params[0].Optional || !typ.IsAny(sig.Params[0].Type) {
		t.Fatalf("self param = %#v, want required gradual any", sig.Params[0])
	}
}

func TestLiteralSignaturesResolveEnclosingGenericScope(t *testing.T) {
	t.Parallel()

	inner := functionWithParams("x")
	inner.ParList.Types = []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}}
	inner.ReturnTypes = []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}}
	outer := &ast.FunctionExpr{
		TypeParams: []ast.TypeParamExpr{{Name: "T"}},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"inner"},
				Exprs: []ast.Expr{inner},
			},
		},
	}
	g := cfg.Build(outer)
	if g == nil {
		t.Fatal("cfg.Build returned nil")
	}

	got := (LiteralInput{
		Graph:       g,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
	}).Signatures()
	sig := got[inner]
	if sig == nil {
		t.Fatalf("literal signatures = %#v, missing inner", got)
	}
	if len(sig.Params) != 1 {
		t.Fatalf("params = %#v, want one", sig.Params)
	}
	param, ok := sig.Params[0].Type.(*typ.TypeParam)
	if !ok || param.Name != "T" {
		t.Fatalf("param type = %#v, want enclosing type param T", sig.Params[0].Type)
	}
	ret, ok := sig.Returns[0].(*typ.TypeParam)
	if !ok || ret.Name != "T" {
		t.Fatalf("return type = %#v, want enclosing type param T", sig.Returns[0])
	}
}

func TestFunctionContextScopeCarriesGenericVariadicType(t *testing.T) {
	t.Parallel()

	fn := functionWithParams("head")
	fn.TypeParams = []ast.TypeParamExpr{{Name: "T"}}
	fn.ParList.HasVargs = true
	fn.ParList.VarargType = &ast.TypeRefExpr{Path: []string{"T"}}

	sc := (ScopeInput{
		Function:    fn,
		Base:        scope.NewWithBuiltins(),
		ResolveType: testResolveType,
	}).FunctionContext()
	if sc == nil {
		t.Fatal("FunctionContextScope returned nil")
	}
	variadic := sc.VariadicType()
	if variadic == nil {
		t.Fatal("variadic type is nil")
	}
	tp, ok := variadic.(*typ.TypeParam)
	if !ok || tp.Name != "T" {
		t.Fatalf("variadic type = %#v, want type param T", variadic)
	}
	if !sc.IsLocal("head") {
		t.Fatal("parameter name was not marked local")
	}
}

func TestLiteralSignaturesUseMethodResolver(t *testing.T) {
	t.Parallel()

	methodBody := functionWithParams("arg")
	owner := &ast.IdentExpr{Value: "Service"}
	outer := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.FuncDefStmt{
				Name: &ast.FuncName{
					Func:     owner,
					Receiver: owner,
					Method:   "run",
				},
				Func: methodBody,
			},
		},
	}
	g := cfg.Build(outer)
	if g == nil {
		t.Fatal("cfg.Build returned nil")
	}
	var method *cfg.FuncDefInfo
	g.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if info != nil && info.FuncExpr == methodBody {
			method = info
		}
	})
	if method == nil {
		t.Fatal("test CFG did not expose method info")
	}
	self := typ.NewRecord().Field("run", typ.Func().Build()).Build()
	got := (LiteralInput{
		Graph:       g,
		Base:        scope.NewWithBuiltins().WithType("Service", self),
		ResolveType: testResolveType,
		MethodFor: func(fn *ast.FunctionExpr) *cfg.FuncDefInfo {
			if fn == methodBody {
				return method
			}
			return nil
		},
	}).Signatures()
	sig := got[methodBody]
	if sig == nil || len(sig.Params) != 2 {
		t.Fatalf("method literal signature = %#v, want self + arg", sig)
	}
	if sig.Params[0].Name != "self" || sig.Params[0].Optional || !typ.TypeEquals(sig.Params[0].Type, self) {
		t.Fatalf("self param = %#v, want required Service", sig.Params[0])
	}
}

func functionWithParams(names ...string) *ast.FunctionExpr {
	return &ast.FunctionExpr{ParList: &ast.ParList{Names: names}}
}

func testResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	switch e := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		switch e.Name {
		case "string":
			return typ.String
		case "number":
			return typ.Number
		case "boolean":
			return typ.Boolean
		case "any":
			return typ.Any
		}
	case *ast.TypeRefExpr:
		if len(e.Path) == 1 && sc != nil {
			if t, ok := sc.LookupTypeParam(e.Path[0]); ok {
				return t
			}
			if t, ok := sc.LookupType(e.Path[0]); ok {
				return t
			}
		}
	case *ast.FunctionTypeExpr:
		builder := typ.Func()
		resolveScope := sc
		for _, tp := range e.TypeParams {
			param := typ.NewTypeParam(tp.Name, nil)
			builder.TypeParamRef(param)
			if resolveScope != nil {
				resolveScope = resolveScope.WithTypeParams(map[string]typ.Type{tp.Name: param})
			}
		}
		for _, p := range e.Params {
			builder.Param(p.Name, testResolveType(p.Type, resolveScope))
		}
		returns := make([]typ.Type, 0, len(e.Returns))
		for _, ret := range e.Returns {
			returns = append(returns, testResolveType(ret, resolveScope))
		}
		if len(returns) > 0 {
			builder.Returns(returns...)
		}
		return builder.Build()
	}
	return nil
}
