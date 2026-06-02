package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeResolverCalleePrecedence(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "f"}
	sym := cfg.SymbolID(10)
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetName(sym, "f")

	resolver := TypeResolver{
		Bindings: bindings,
		ExprType: func(expr ast.Expr) typ.Type {
			if expr == ident {
				return typ.String
			}
			return nil
		},
		Static: StaticTypeLookup{
			FuncBySymbol: func(got cfg.SymbolID) (typ.Type, bool) {
				if got != sym {
					t.Fatalf("FuncBySymbol got %d, want %d", got, sym)
				}
				return typ.Boolean, true
			},
			GlobalByName: func(string) (typ.Type, bool) {
				return typ.Number, true
			},
		},
	}

	if got := resolver.ResolveCallee(ident); got != typ.String {
		t.Fatalf("ResolveCallee product precedence = %v, want string", got)
	}

	resolver.ExprType = nil
	if got := resolver.ResolveCallee(ident); got != typ.Boolean {
		t.Fatalf("ResolveCallee symbol fallback = %v, want boolean", got)
	}

	resolver.Static.FuncBySymbol = nil
	if got := resolver.ResolveCallee(ident); got != typ.Number {
		t.Fatalf("ResolveCallee global fallback = %v, want number", got)
	}
}

func TestTypeResolverGlobalSymbolBeatsNameLookup(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "print"}
	sym := cfg.SymbolID(11)
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetKind(sym, cfg.SymbolGlobal)
	bindings.SetName(sym, "print")

	resolver := TypeResolver{
		Bindings: bindings,
		Static: StaticTypeLookup{
			GlobalBySymbol: func(got cfg.SymbolID) (typ.Type, bool) {
				if got != sym {
					t.Fatalf("GlobalBySymbol got %d, want %d", got, sym)
				}
				return typ.String, true
			},
			GlobalByName: func(string) (typ.Type, bool) {
				return typ.Number, true
			},
		},
	}

	if got := resolver.ResolveCallee(ident); got != typ.String {
		t.Fatalf("ResolveCallee global symbol = %v, want string", got)
	}
}

func TestTypeResolverStaticCalleeReadsGlobalSymbolFallback(t *testing.T) {
	t.Parallel()

	ident := &ast.IdentExpr{Value: "up"}
	sym := cfg.SymbolID(12)
	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)
	bindings.SetKind(sym, cfg.SymbolGlobal)
	bindings.SetName(sym, "up")

	resolver := TypeResolver{
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
				return typ.String, true
			},
		},
	}

	if got := resolver.ResolveCallee(ident); got != typ.Any {
		t.Fatalf("ResolveCallee(any global) = %v, want live any", got)
	}
	if got := resolver.ResolveStaticCallee(ident); got != typ.String {
		t.Fatalf("ResolveStaticCallee(global) = %v, want symbol fallback", got)
	}
}

func TestTypeResolverFieldFallbackOrder(t *testing.T) {
	t.Parallel()

	expr, bindings, baseSym := testFieldExpr("mod", "run")
	field, ok := fieldkey.FromSegment(constraint.Segment{
		Kind: constraint.SegmentField,
		Name: "run",
	})
	if !ok {
		t.Fatal("failed to build field key")
	}
	importedBase := typ.NewRecord().Field("run", typ.Number).Build()

	resolver := TypeResolver{
		Bindings: bindings,
		Static: StaticTypeLookup{
			FieldFunc: func(sym cfg.SymbolID, got fieldkey.Key) (typ.Type, bool) {
				if sym != baseSym || got != field {
					t.Fatalf("FieldFunc got (%d,%+v), want (%d,%+v)", sym, got, baseSym, field)
				}
				return typ.String, true
			},
			ImportedBase: func(sym cfg.SymbolID) (typ.Type, bool) {
				if sym != baseSym {
					t.Fatalf("ImportedBase got %d, want %d", sym, baseSym)
				}
				return importedBase, true
			},
		},
	}

	if got := resolver.ResolveCallee(expr); got != typ.String {
		t.Fatalf("ResolveCallee field function = %v, want string", got)
	}

	resolver.Static.FieldFunc = nil
	if got := resolver.ResolveCallee(expr); got != typ.Number {
		t.Fatalf("ResolveCallee imported field = %v, want number", got)
	}
}

func TestTypeResolverStaticFieldFallbackReadsGlobalBaseMembers(t *testing.T) {
	t.Parallel()

	expr, bindings, baseSym := testFieldExpr("migration", "define")
	globalBase := typ.NewRecord().Field("define", typ.Boolean).Build()
	importedBase := typ.NewRecord().Field("define", typ.Number).Build()

	resolver := TypeResolver{
		Bindings: bindings,
		Static: StaticTypeLookup{
			GlobalBySymbol: func(sym cfg.SymbolID) (typ.Type, bool) {
				if sym != baseSym {
					t.Fatalf("GlobalBySymbol got %d, want %d", sym, baseSym)
				}
				return globalBase, true
			},
			ImportedBase: func(sym cfg.SymbolID) (typ.Type, bool) {
				if sym != baseSym {
					t.Fatalf("ImportedBase got %d, want %d", sym, baseSym)
				}
				return importedBase, true
			},
		},
	}

	if got := resolver.ResolveCallee(expr); got != typ.Boolean {
		t.Fatalf("ResolveCallee global field = %v, want boolean", got)
	}
}

func TestTypeResolverStaticFieldFallbackIsOneSegmentOnly(t *testing.T) {
	t.Parallel()

	base := &ast.IdentExpr{Value: "mod"}
	mid := &ast.AttrGetExpr{Object: base, Key: &ast.StringExpr{Value: "nested"}}
	expr := &ast.AttrGetExpr{Object: mid, Key: &ast.StringExpr{Value: "run"}}
	bindings := bind.NewBindingTable()
	bindings.Bind(base, cfg.SymbolID(12))
	bindings.SetName(cfg.SymbolID(12), "mod")

	called := false
	resolver := TypeResolver{
		Bindings: bindings,
		Static: StaticTypeLookup{
			FieldFunc: func(cfg.SymbolID, fieldkey.Key) (typ.Type, bool) {
				called = true
				return typ.String, true
			},
			ImportedBase: func(cfg.SymbolID) (typ.Type, bool) {
				called = true
				return typ.NewRecord().Field("run", typ.String).Build(), true
			},
		},
	}

	if got := resolver.ResolveCallee(expr); got != nil {
		t.Fatalf("ResolveCallee deep static field = %v, want nil", got)
	}
	if called {
		t.Fatal("static fallback callback ran for deeper-than-one-segment path")
	}
}

func TestTypeResolverReceiverPrecedence(t *testing.T) {
	t.Parallel()

	recv := &ast.IdentExpr{Value: "r"}
	sym := cfg.SymbolID(13)
	bindings := bind.NewBindingTable()
	bindings.Bind(recv, sym)
	bindings.SetName(sym, "r")

	resolver := TypeResolver{
		Bindings: bindings,
		Static: StaticTypeLookup{
			FuncBySymbol: func(cfg.SymbolID) (typ.Type, bool) {
				return typ.String, true
			},
			GlobalByName: func(string) (typ.Type, bool) {
				return typ.Number, true
			},
		},
	}

	if got := resolver.ResolveReceiver(recv); got != typ.String {
		t.Fatalf("ResolveReceiver symbol fallback = %v, want string", got)
	}
	resolver.Static.FuncBySymbol = nil
	if got := resolver.ResolveReceiver(recv); got != typ.Number {
		t.Fatalf("ResolveReceiver global fallback = %v, want number", got)
	}
}

func testFieldExpr(baseName, fieldName string) (ast.Expr, *bind.BindingTable, cfg.SymbolID) {
	base := &ast.IdentExpr{Value: baseName}
	sym := cfg.SymbolID(99)
	bindings := bind.NewBindingTable()
	bindings.Bind(base, sym)
	bindings.SetName(sym, baseName)
	return &ast.AttrGetExpr{
		Object: base,
		Key:    &ast.StringExpr{Value: fieldName},
	}, bindings, sym
}
