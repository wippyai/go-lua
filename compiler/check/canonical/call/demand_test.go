package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeFallbackOutcomeFunctionShapeSummaryWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	summaryFn := typ.Func().Param("x", typ.String).Build()
	resolved := false

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(ast.Expr) typ.Type {
					resolved = true
					return typ.Func().Param("x", typ.Number).Build()
				},
			},
		},
		SummarySignature: summaryFn,
	}).FunctionShape()
	if got != summaryFn {
		t.Fatalf("FunctionShape summary = %v, want summary function", got)
	}
	if resolved {
		t.Fatal("callee resolver ran despite summary function")
	}
}

func TestTypeFallbackOutcomeFunctionShapeResolvesPlainCallee(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	fn := typ.Func().Param("x", typ.String).Build()

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(expr ast.Expr) typ.Type {
					if expr != call.Func {
						t.Fatalf("ExprType got %#v, want call func", expr)
					}
					return fn
				},
			},
		},
		UseResolvedSignature: true,
	}).FunctionShape()
	if got != fn {
		t.Fatalf("FunctionShape plain = %v, want fn", got)
	}
}

func TestTypeFallbackOutcomeFunctionShapeUsesStaticGlobalWhenLiveCalleeIsAny(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "up"}
	call := &ast.FuncCallExpr{Func: ident}
	sym := cfg.SymbolID(7)
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetKind(sym, cfg.SymbolGlobal)
	bindings.SetName(sym, "up")
	fn := typ.Func().Param("callback", typ.Func().Build()).Build()

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
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
		},
		UseResolvedSignature: true,
	}).FunctionShape()
	if got != fn {
		t.Fatalf("FunctionShape(any live global) = %v, want static function", got)
	}
}

func TestTypeFallbackOutcomeFunctionShapeResolvesMethodMember(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "obj"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "run"}
	fn := typ.Func().Param("x", typ.String).Build()
	rec := typ.NewRecord().Field("run", fn).Build()

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(expr ast.Expr) typ.Type {
					if expr != receiver {
						t.Fatalf("ExprType got %#v, want receiver", expr)
					}
					return rec
				},
			},
		},
		UseResolvedSignature: true,
	}).FunctionShape()
	if got != fn {
		t.Fatalf("FunctionShape method = %v, want fn", got)
	}
}

func TestTypeFallbackOutcomeFunctionShapeResolvesInterfaceMethod(t *testing.T) {
	t.Parallel()

	receiver := &ast.IdentExpr{Value: "now"}
	call := &ast.FuncCallExpr{Receiver: receiver, Method: "sub"}
	fn := typ.Func().Param("self", typ.Self).Param("other", typ.Self).Build()
	timeType := typ.NewInterface("time.Time", []typ.Method{{Name: "sub", Type: fn}})

	got := NewTypeFallbackOutcome(TypeFallbackInput{
		Return: ReturnInput{
			Call: call,
			Resolver: TypeResolver{
				ExprType: func(expr ast.Expr) typ.Type {
					if expr != receiver {
						t.Fatalf("ExprType got %#v, want receiver", expr)
					}
					return timeType
				},
			},
		},
		UseResolvedSignature: true,
	}).FunctionShape()
	if got != fn {
		t.Fatalf("FunctionShape interface method = %v, want %v", got, fn)
	}
}

func TestTypeFallbackOutcomeFunctionShapeRejectsDynamicAndNonFunction(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}
	for name, callee := range map[string]typ.Type{
		"any":    typ.Any,
		"number": typ.Number,
		"nil":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			got := NewTypeFallbackOutcome(TypeFallbackInput{
				Return: ReturnInput{
					Call: call,
					Resolver: TypeResolver{
						ExprType: func(ast.Expr) typ.Type {
							return callee
						},
					},
				},
				UseResolvedSignature: true,
			}).FunctionShape()
			if got != nil {
				t.Fatalf("FunctionShape(%s) = %v, want nil", name, got)
			}
		})
	}
}
