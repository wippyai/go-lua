package branchcond

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func stringLit(value string) *ast.StringExpr {
	return &ast.StringExpr{Value: value}
}

func primitiveType(name string) *ast.PrimitiveTypeExpr {
	return &ast.PrimitiveTypeExpr{Name: name}
}

func cast(expr ast.Expr, typeName string, syntax ast.CastSyntax) *ast.CastExpr {
	return &ast.CastExpr{Expr: expr, Type: primitiveType(typeName), Syntax: syntax}
}

func dot(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       stringLit(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func typeCall(arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{arg}}
}

func call(name string) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name)}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func bindReturn(expr ast.Expr, globals ...string) *bind.Result {
	return bind.BindChunk([]ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{expr}}}, bind.Options{Globals: globals})
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		t.Fatalf("missing symbol for %q", ident.Value)
	}
	return id
}

func assertCheck(t *testing.T, got Check, wantKind CheckKind, wantPath path.Path, wantTypeName string) {
	t.Helper()
	if got.Kind != wantKind {
		t.Fatalf("check kind = %v, want %v", got.Kind, wantKind)
	}
	if got.TypeName != wantTypeName {
		t.Fatalf("type name = %q, want %q", got.TypeName, wantTypeName)
	}
	if !got.Path.Equal(wantPath) {
		t.Fatalf("path = %#v, want %#v", got.Path, wantPath)
	}
}

func assertLiteralCheck(t *testing.T, got Check, wantKind CheckKind, wantPath path.Path, wantLiteral string) {
	t.Helper()
	assertCheck(t, got, wantKind, wantPath, "")
	if got.LiteralString != wantLiteral {
		t.Fatalf("literal = %q, want %q", got.LiteralString, wantLiteral)
	}
}

func assertPathCheck(t *testing.T, got Check, wantKind CheckKind, wantPath, wantOtherPath path.Path) {
	t.Helper()
	assertCheck(t, got, wantKind, wantPath, "")
	if !got.OtherPath.Equal(wantOtherPath) {
		t.Fatalf("other path = %#v, want %#v", got.OtherPath, wantOtherPath)
	}
}

func assertCheckNone(t *testing.T, got Check) {
	t.Helper()
	if got.Kind != CheckNone || !got.Path.IsEmpty() || !got.OtherPath.IsEmpty() || got.TypeName != "" {
		t.Fatalf("check = %#v, want empty CheckNone", got)
	}
}

func TestNormalizePathChecks(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		wantKind CheckKind
		wantPath func(symbol.ID) path.Path
	}{
		{
			name: "plain path truthy",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return dot(root, "ready")
			},
			wantKind: CheckTruthy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("ready")
			},
		},
		{
			name: "not path falsy",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: dot(root, "ready")}
			},
			wantKind: CheckFalsy,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("ready")
			},
		},
		{
			name: "path equal nil",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "child"), Rhs: &ast.NilExpr{}}
			},
			wantKind: CheckNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("child")
			},
		},
		{
			name: "nil not equal path",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: &ast.NilExpr{}, Rhs: dot(root, "child")}
			},
			wantKind: CheckNotNil,
			wantPath: func(root symbol.ID) path.Path {
				return path.NewPath(root, "obj").Field("child")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := ident("obj")
			expr := tt.expr(root)
			bindings := bindReturn(expr)
			assertCheck(t, Normalize(expr, bindings), tt.wantKind, tt.wantPath(mustIdentSymbol(t, bindings, root)), "")
		})
	}
}

func TestNormalizePathComparisons(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		wantKind CheckKind
	}{
		{
			name:     "result channel equals channel",
			operator: "==",
			wantKind: CheckPathEqual,
		},
		{
			name:     "result channel not equals channel",
			operator: "~=",
			wantKind: CheckPathNot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ident("result")
			ch := ident("ch")
			expr := &ast.RelationalOpExpr{Operator: tt.operator, Lhs: dot(result, "channel"), Rhs: ch}
			bindings := bindReturn(expr)
			resultPath := path.NewPath(mustIdentSymbol(t, bindings, result), "result").Field("channel")
			chPath := path.NewPath(mustIdentSymbol(t, bindings, ch), "ch")
			assertPathCheck(t, Normalize(expr, bindings), tt.wantKind, resultPath, chPath)
		})
	}
}

func TestNormalizeTypeComparisons(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		wantKind CheckKind
		typeName string
	}{
		{
			name: "type equal",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("table")}
			},
			wantKind: CheckTypeEqual,
			typeName: "table",
		},
		{
			name: "type equal reversed operands",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: stringLit("table"), Rhs: typeCall(dot(root, "kind"))}
			},
			wantKind: CheckTypeEqual,
			typeName: "table",
		},
		{
			name: "type not equal",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("function")}
			},
			wantKind: CheckTypeNot,
			typeName: "function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := ident("obj")
			expr := tt.expr(root)
			bindings := bindReturn(expr, "type")
			wantPath := path.NewPath(mustIdentSymbol(t, bindings, root), "obj").Field("kind")
			assertCheck(t, Normalize(expr, bindings), tt.wantKind, wantPath, tt.typeName)
		})
	}
}

func TestNormalizeStringLiteralComparisons(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		wantKind CheckKind
		literal  string
	}{
		{
			name: "field equal literal",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "kind"), Rhs: stringLit("dog")}
			},
			wantKind: CheckLiteralEqual,
			literal:  "dog",
		},
		{
			name: "literal not equal field",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: stringLit("cat"), Rhs: dot(root, "kind")}
			},
			wantKind: CheckLiteralNot,
			literal:  "cat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := ident("obj")
			expr := tt.expr(root)
			bindings := bindReturn(expr)
			wantPath := path.NewPath(mustIdentSymbol(t, bindings, root), "obj").Field("kind")
			assertLiteralCheck(t, Normalize(expr, bindings), tt.wantKind, wantPath, tt.literal)
		})
	}
}

func TestNormalizeAssertionWrappedPathsDoesNotResolve(t *testing.T) {
	tests := []struct {
		name    string
		expr    func(*ast.IdentExpr) ast.Expr
		globals []string
	}{
		{
			name: "as cast truthy",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return cast(root, "number", ast.CastSyntaxAs)
			},
		},
		{
			name: "colon cast truthy",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return cast(root, "any", ast.CastSyntaxColonColon)
			},
		},
		{
			name: "not as cast",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.UnaryNotOpExpr{Expr: cast(root, "number", ast.CastSyntaxAs)}
			},
		},
		{
			name: "colon cast equal nil",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{
					Operator: "==",
					Lhs:      cast(root, "string", ast.CastSyntaxColonColon),
					Rhs:      &ast.NilExpr{},
				}
			},
		},
		{
			name: "type of as cast equal table",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{
					Operator: "==",
					Lhs:      typeCall(cast(root, "table", ast.CastSyntaxAs)),
					Rhs:      stringLit("table"),
				}
			},
			globals: []string{"type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := ident("x")
			expr := tt.expr(root)
			assertCheckNone(t, Normalize(expr, bindReturn(expr, tt.globals...)))
		})
	}
}

func TestNormalizeUnsupportedConditions(t *testing.T) {
	t.Run("unsupported relop", func(t *testing.T) {
		root := ident("obj")
		expr := &ast.RelationalOpExpr{Operator: "<", Lhs: dot(root, "kind"), Rhs: &ast.NilExpr{}}
		assertCheckNone(t, Normalize(expr, bindReturn(expr)))
	})

	t.Run("non-global type", func(t *testing.T) {
		root := ident("obj")
		expr := &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("table")}
		stmts := []ast.Stmt{
			localAssign([]string{"type"}, number("0")),
			&ast.ReturnStmt{Exprs: []ast.Expr{expr}},
		}
		bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
		assertCheckNone(t, Normalize(expr, bindings))
	})

	t.Run("non-path subject", func(t *testing.T) {
		expr := &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(call("make")), Rhs: stringLit("table")}
		assertCheckNone(t, Normalize(expr, bindReturn(expr, "type", "make")))
	})
}

func TestSupportsTypeComparison(t *testing.T) {
	t.Run("global type path comparison", func(t *testing.T) {
		root := ident("obj")
		expr := &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(dot(root, "kind")), Rhs: stringLit("table")}
		if !SupportsTypeComparison(expr, bindReturn(expr, "type")) {
			t.Fatalf("SupportsTypeComparison rejected global type(path) comparison")
		}
	})

	t.Run("nil comparison", func(t *testing.T) {
		root := ident("obj")
		expr := &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "kind"), Rhs: &ast.NilExpr{}}
		if SupportsTypeComparison(expr, bindReturn(expr)) {
			t.Fatalf("SupportsTypeComparison accepted nil comparison")
		}
	})

	t.Run("unsupported type call shape", func(t *testing.T) {
		root := ident("obj")
		expr := &ast.RelationalOpExpr{
			Operator: "==",
			Lhs:      &ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{dot(root, "kind"), stringLit("extra")}},
			Rhs:      stringLit("table"),
		}
		if SupportsTypeComparison(expr, bindReturn(expr, "type")) {
			t.Fatalf("SupportsTypeComparison accepted wrong-arity type call")
		}
	})
}
