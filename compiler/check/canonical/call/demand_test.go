package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDemandFunctionProjectionSummaryWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	summaryFn := typ.Func().Param("x", typ.String).Build()
	resolved := false

	got := (DemandFunctionProjection{
		Call: call,
		SummaryFunction: func(*ast.FuncCallExpr) *typ.Function {
			return summaryFn
		},
		Resolver: TypeResolver{
			ExprType: func(ast.Expr) typ.Type {
				resolved = true
				return typ.Func().Param("x", typ.Number).Build()
			},
		},
	}).Function()
	if got != summaryFn {
		t.Fatalf("DemandFunctionProjection summary = %v, want summary function", got)
	}
	if resolved {
		t.Fatal("callee resolver ran despite summary function")
	}
}

func TestDemandFunctionProjectionResolvesPlainCallee(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	fn := typ.Func().Param("x", typ.String).Build()

	got := (DemandFunctionProjection{
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr != call.Func {
					t.Fatalf("ExprType got %#v, want call func", expr)
				}
				return fn
			},
		},
	}).Function()
	if got != fn {
		t.Fatalf("DemandFunctionProjection plain = %v, want fn", got)
	}
}

func TestDemandFunctionProjectionUsesStaticGlobalWhenLiveCalleeIsAny(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "up"}
	call := &ast.FuncCallExpr{Func: ident}
	sym := cfg.SymbolID(7)
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetKind(sym, cfg.SymbolGlobal)
	bindings.SetName(sym, "up")
	fn := typ.Func().Param("callback", typ.Func().Build()).Build()

	got := (DemandFunctionProjection{
		Call: call,
		Resolver: TypeResolver{
			Bindings: bindings,
			ExprType: func(expr ast.Expr) typ.Type {
				if expr == ident {
					return typ.Any
				}
				return nil
			},
			Static: StaticTypeLookup{
				GlobalBySymbol: func(got cfg.SymbolID) (typ.Type, bool) {
					if got != sym {
						t.Fatalf("GlobalBySymbol got %d, want %d", got, sym)
					}
					return fn, true
				},
			},
		},
	}).Function()
	if got != fn {
		t.Fatalf("DemandFunctionProjection(any live global) = %v, want static function", got)
	}
}

func TestDemandFunctionProjectionResolvesMethodMember(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "obj"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "run"}
	fn := typ.Func().Param("x", typ.String).Build()
	rec := typ.NewRecord().Field("run", fn).Build()

	got := (DemandFunctionProjection{
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr != receiver {
					t.Fatalf("ExprType got %#v, want receiver", expr)
				}
				return rec
			},
		},
	}).Function()
	if got != fn {
		t.Fatalf("DemandFunctionProjection method = %v, want fn", got)
	}
}

func TestDemandFunctionProjectionResolvesInterfaceMethod(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "now"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "sub"}
	fn := typ.Func().Param("self", typ.Self).Param("other", typ.Self).Build()
	timeType := typ.NewInterface("time.Time", []typ.Method{{Name: "sub", Type: fn}})

	got := (DemandFunctionProjection{
		Call: call,
		Resolver: TypeResolver{
			ExprType: func(expr ast.Expr) typ.Type {
				if expr != receiver {
					t.Fatalf("ExprType got %#v, want receiver", expr)
				}
				return timeType
			},
		},
	}).Function()
	if got != fn {
		t.Fatalf("DemandFunctionProjection interface method = %v, want %v", got, fn)
	}
}

func TestDemandFunctionProjectionRejectsDynamicAndNonFunction(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	for name, callee := range map[string]typ.Type{
		"any":    typ.Any,
		"number": typ.Number,
		"nil":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			got := (DemandFunctionProjection{
				Call: call,
				Resolver: TypeResolver{
					ExprType: func(ast.Expr) typ.Type {
						return callee
					},
				},
			}).Function()
			if got != nil {
				t.Fatalf("DemandFunctionProjection(%s) = %v, want nil", name, got)
			}
		})
	}
}
