package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExpectedArgProjectionMasksCallbackBodyDemandDuringExpectationProjection(t *testing.T) {
	t.Parallel()

	tp := typ.NewTypeParam("T", nil)
	up := typ.NewTypeParam("U", nil)
	callee := typ.Func().
		TypeParamRef(tp).
		TypeParamRef(up).
		Param("x", tp).
		Param("fn", typ.Func().Param("value", tp).Returns(up).Build()).
		Returns(up).
		Build()
	callbackArg := &ast.FunctionExpr{}
	call := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "map"},
		Args: []ast.Expr{&ast.StringExpr{Value: "seed"}, callbackArg},
	}
	bodyDemandCallback := typ.Func().Param("value", typ.Number).Returns(typ.Integer).Build()

	got := (ExpectedArgProjection{
		Call: call,
		ArgTypes: []typ.Type{
			typ.String,
			bodyDemandCallback,
		},
		CallbackArg: func(arg ast.Expr) bool {
			return arg == callbackArg
		},
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr == call.Func {
					return callee
				}
				return nil
			},
		},
		Ctx:   db.NewQueryContext(db.New()),
		Query: noopTypeOps{},
	}).ExpectedTypes()

	if len(got) != 2 {
		t.Fatalf("expected args = %v, want two args", got)
	}
	expectedFn, ok := got[1].(*typ.Function)
	if !ok || expectedFn == nil || len(expectedFn.Params) != 1 {
		t.Fatalf("callback expected arg = %v, want unary function", got[1])
	}
	if !typ.TypeEquals(expectedFn.Params[0].Type, typ.String) {
		t.Fatalf("callback param = %v, want string", expectedFn.Params[0].Type)
	}
}

func TestExpectedArgProjectionKeepsDirectFunctionLiteralShallow(t *testing.T) {
	t.Parallel()

	callbackArg := &ast.FunctionExpr{}
	otherArg := &ast.IdentExpr{Value: "value"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{callbackArg, otherArg}}
	got := callArgTypesForExpectedArgProjection(ExpectedArgProjection{
		Call:                call,
		ArgTypes:            []typ.Type{typ.Func().Param("x", typ.String).Returns(typ.Number).Build(), typ.Integer},
		ExprType:            func(ast.Expr) typ.Type { return typ.Boolean },
		ShallowFuncLiterals: true,
	})

	if len(got) != 2 {
		t.Fatalf("shallow args = %v, want two args", got)
	}
	fn, ok := got[0].(*typ.Function)
	if !ok || fn == nil || len(fn.Params) != 0 || len(fn.Returns) != 0 {
		t.Fatalf("callback arg = %v, want shallow function literal arity without return proof", got[0])
	}
	if !typ.TypeEquals(got[1], typ.Integer) {
		t.Fatalf("non-callback arg = %v, want projected integer", got[1])
	}
}

type noopTypeOps struct{}

func (noopTypeOps) Field(*db.QueryContext, typ.Type, string) (typ.Type, bool) { return nil, false }
func (noopTypeOps) Index(*db.QueryContext, typ.Type, typ.Type) (typ.Type, bool) {
	return nil, false
}
func (noopTypeOps) Method(*db.QueryContext, typ.Type, string) (typ.Type, bool) { return nil, false }
func (noopTypeOps) BinaryOp(*db.QueryContext, typ.Type, string, typ.Type) typ.Type {
	return typ.Unknown
}
func (noopTypeOps) UnaryOp(*db.QueryContext, string, typ.Type) typ.Type { return typ.Unknown }
func (noopTypeOps) IsSubtype(*db.QueryContext, typ.Type, typ.Type) bool { return false }
func (noopTypeOps) ExpandInstantiated(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}
func (noopTypeOps) Widen(_ *db.QueryContext, t typ.Type) typ.Type { return t }
func (noopTypeOps) WidenForInference(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}
