package branchcond

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

func typeIsCall(receiver, arg ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Receiver: receiver, Method: "is", Args: []ast.Expr{arg}}
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
	assertLiteralTypeCheck(t, got, typ.LiteralString(wantLiteral))
	if got.LiteralString != wantLiteral {
		t.Fatalf("literal = %q, want %q", got.LiteralString, wantLiteral)
	}
}

func assertLiteralTypeCheck(t *testing.T, got Check, wantLiteral typ.Type) {
	t.Helper()
	lit, ok := got.LiteralValue()
	if !ok {
		t.Fatalf("literal = <none>, want %s", wantLiteral.String())
	}
	if !typ.TypeEquals(lit, wantLiteral) {
		t.Fatalf("literal = %s, want %s", lit.String(), wantLiteral.String())
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

func TestNormalizeTypeComparisonWithPathRhs(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		reversed bool
		wantKind CheckKind
	}{
		{name: "type equal variable rhs", operator: "==", wantKind: CheckTypeEqual},
		{name: "type equal variable lhs", operator: "==", reversed: true, wantKind: CheckTypeEqual},
		{name: "type not equal variable rhs", operator: "~=", wantKind: CheckTypeNot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject := ident("v")
			tag := ident("tag")
			var expr *ast.RelationalOpExpr
			if tt.reversed {
				expr = &ast.RelationalOpExpr{Operator: tt.operator, Lhs: tag, Rhs: typeCall(subject)}
			} else {
				expr = &ast.RelationalOpExpr{Operator: tt.operator, Lhs: typeCall(subject), Rhs: tag}
			}
			bindings := bindReturn(expr, "type")
			subjectPath := path.NewPath(mustIdentSymbol(t, bindings, subject), "v")
			tagPath := path.NewPath(mustIdentSymbol(t, bindings, tag), "tag")
			check := Normalize(expr, bindings)
			assertPathCheck(t, check, tt.wantKind, subjectPath, tagPath)
			if check.TypeName != "" {
				t.Fatalf("variable-rhs type comparison must carry no static type name, got %q", check.TypeName)
			}
		})
	}
}

func TestTypeCallShapeRecognition(t *testing.T) {
	root := ident("obj")
	plain := typeCall(dot(root, "kind"))
	wrapped := &ast.CastExpr{Expr: plain, Type: primitiveType("any"), Syntax: ast.CastSyntaxAs}

	if got, ok := TypeCall(plain); !ok || got != plain {
		t.Fatalf("TypeCall(plain) = %p, %v; want %p, true", got, ok, plain)
	}
	if got, ok := TypeCall(wrapped); !ok || got != plain {
		t.Fatalf("TypeCall(wrapped) = %p, %v; want %p, true", got, ok, plain)
	}
	if _, ok := TypeCall(typeIsCall(dot(root, "kind"), root)); ok {
		t.Fatalf("TypeCall accepted receiver method call shape")
	}
	if _, ok := TypeCall(&ast.FuncCallExpr{Func: ident("type"), Args: []ast.Expr{dot(root, "kind"), stringLit("extra")}}); ok {
		t.Fatalf("TypeCall accepted wrong-arity type call")
	}
}

func TestTypeIsCallShapeRecognition(t *testing.T) {
	root := ident("Point")
	arg := dot(ident("data"), "item")
	plain := typeIsCall(root, arg)
	wrapped := &ast.NonNilAssertExpr{Expr: plain}

	if got, ok := TypeIsCall(plain); !ok || got != plain {
		t.Fatalf("TypeIsCall(plain) = %p, %v; want %p, true", got, ok, plain)
	}
	if got, ok := TypeIsCall(wrapped); !ok || got != plain {
		t.Fatalf("TypeIsCall(wrapped) = %p, %v; want %p, true", got, ok, plain)
	}
	memberCallee := &ast.FuncCallExpr{Func: dot(root, "is"), Args: []ast.Expr{arg}}
	if got, ok := TypeIsCall(memberCallee); !ok || got != memberCallee {
		t.Fatalf("TypeIsCall(member callee) = %p, %v; want %p, true", got, ok, memberCallee)
	}
	if _, ok := TypeIsCall(typeCall(arg)); ok {
		t.Fatalf("TypeIsCall accepted direct function call shape")
	}
	if _, ok := TypeIsCall(&ast.FuncCallExpr{Receiver: root, Method: "is", Args: []ast.Expr{arg, stringLit("extra")}}); ok {
		t.Fatalf("TypeIsCall accepted wrong-arity method call")
	}
}

func TestTruthyChecksExtractSupportedConjuncts(t *testing.T) {
	value := ident("value")
	expr := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      &ast.RelationalOpExpr{Operator: "==", Lhs: typeCall(value), Rhs: stringLit("number")},
		Rhs:      &ast.RelationalOpExpr{Operator: ">", Lhs: value, Rhs: number("0")},
	}
	bindings := bindReturn(expr, "type")

	got := TruthyChecks(expr, bindings)

	if len(got) != 2 {
		t.Fatalf("TruthyChecks returned %d checks, want 2: %#v", len(got), got)
	}
	valuePath := path.NewPath(mustIdentSymbol(t, bindings, value), "value")
	assertCheck(t, got[0], CheckTypeEqual, valuePath, "number")
	if got[1].Kind != CheckNumGe || got[1].NumFloor != 1 {
		t.Fatalf("second conjunct = %#v, want CheckNumGe floor 1 for value > 0", got[1])
	}
}

func TestFalsyChecksExtractNegatedConjunctBounds(t *testing.T) {
	i := ident("i")
	xs := ident("xs")
	expr := &ast.UnaryNotOpExpr{
		Expr: &ast.LogicalOpExpr{
			Operator: "and",
			Lhs:      &ast.RelationalOpExpr{Operator: ">=", Lhs: i, Rhs: number("1")},
			Rhs:      &ast.RelationalOpExpr{Operator: "<=", Lhs: i, Rhs: lenOf(xs)},
		},
	}
	bindings := bindReturn(expr)
	iPath := path.NewPath(mustIdentSymbol(t, bindings, i), "i")
	xsPath := path.NewPath(mustIdentSymbol(t, bindings, xs), "xs")

	got := FalsyChecks(expr, bindings)

	if len(got) != 2 {
		t.Fatalf("FalsyChecks returned %d checks, want numeric floor and index range: %#v", len(got), got)
	}
	if got[0].Kind != CheckNumGe || !got[0].Path.Equal(iPath) || got[0].NumFloor != 1 {
		t.Fatalf("first check = %#v, want i >= 1", got[0])
	}
	if got[1].Kind != CheckIndexInRange || !got[1].Path.Equal(iPath) || !got[1].OtherPath.Equal(xsPath) {
		t.Fatalf("second check = %#v, want i <= #xs", got[1])
	}
}

func TestImpliedChecksOnEdgePreservesOuterEdgeAndLeafPolarity(t *testing.T) {
	i := ident("i")
	xs := ident("xs")
	expr := &ast.UnaryNotOpExpr{
		Expr: &ast.LogicalOpExpr{
			Operator: "and",
			Lhs:      &ast.RelationalOpExpr{Operator: ">=", Lhs: i, Rhs: number("1")},
			Rhs:      &ast.RelationalOpExpr{Operator: "<=", Lhs: i, Rhs: lenOf(xs)},
		},
	}
	bindings := bindReturn(expr)

	got := ImpliedChecksOnEdge(expr, bindings, false)

	if len(got) != 2 {
		t.Fatalf("ImpliedChecksOnEdge returned %d checks, want 2: %#v", len(got), got)
	}
	for idx, implied := range got {
		if implied.Edge {
			t.Fatalf("implied check %d edge = true, want false outer edge", idx)
		}
		if !implied.Polarity {
			t.Fatalf("implied check %d polarity = false, want true leaf condition", idx)
		}
	}
	if got[0].Check.Kind != CheckNumGe || got[1].Check.Kind != CheckIndexInRange {
		t.Fatalf("implied checks = %#v, want floor then range", got)
	}
}

func TestFalsyChecksExtractSupportedDisjuncts(t *testing.T) {
	pageRoot := ident("page")
	fieldRoot := ident("page")
	literalRoot := ident("page")
	fieldPathExpr := dot(fieldRoot, "data_func")
	literalPathExpr := dot(literalRoot, "data_func")
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.UnaryNotOpExpr{Expr: pageRoot},
		Rhs: &ast.LogicalOpExpr{
			Operator: "or",
			Lhs:      &ast.UnaryNotOpExpr{Expr: fieldPathExpr},
			Rhs: &ast.RelationalOpExpr{
				Operator: "==",
				Lhs:      literalPathExpr,
				Rhs:      stringLit(""),
			},
		},
	}
	bindings := bindReturn(expr)
	pageSym := mustIdentSymbol(t, bindings, pageRoot)
	pagePath := path.NewPath(pageSym, "page")
	fieldPath := pagePath.Field("data_func")

	got := FalsyChecks(expr, bindings)

	if len(got) != 3 {
		t.Fatalf("FalsyChecks returned %d checks, want 3: %#v", len(got), got)
	}
	assertCheck(t, got[0], CheckFalsy, pagePath, "")
	assertCheck(t, got[1], CheckFalsy, fieldPath, "")
	assertLiteralCheck(t, got[2], CheckLiteralEqual, fieldPath, "")
}

func TestSufficientChecksOnEdgeExtractsTruthyDisjunctCases(t *testing.T) {
	choice := ident("choice")
	choiceAuto := ident("choice")
	choiceAny := ident("choice")
	expr := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      &ast.UnaryNotOpExpr{Expr: choice},
		Rhs: &ast.LogicalOpExpr{
			Operator: "or",
			Lhs:      &ast.RelationalOpExpr{Operator: "==", Lhs: choiceAuto, Rhs: stringLit("auto")},
			Rhs:      &ast.RelationalOpExpr{Operator: "==", Lhs: choiceAny, Rhs: stringLit("any")},
		},
	}
	bindings := bindReturn(expr)
	choicePath := path.NewPath(mustIdentSymbol(t, bindings, choice), "choice")

	got := SufficientChecksOnEdge(expr, bindings, true)

	if len(got) != 3 {
		t.Fatalf("SufficientChecksOnEdge returned %d checks, want falsy + two literal cases: %#v", len(got), got)
	}
	if !got[0].Polarity || !got[0].Edge || got[0].Check.Kind != CheckFalsy || !got[0].Check.Path.Equal(choicePath) {
		t.Fatalf("first sufficient case = %#v, want truthy outer edge from not choice", got[0])
	}
	assertLiteralCheck(t, got[1].Check, CheckLiteralEqual, choicePath, "auto")
	assertLiteralCheck(t, got[2].Check, CheckLiteralEqual, choicePath, "any")
	for i, sufficient := range got {
		if !sufficient.Edge || !sufficient.Polarity {
			t.Fatalf("case %d = %#v, want true outer edge and true leaf polarity", i, sufficient)
		}
	}
}

func TestNormalizeLiteralComparisons(t *testing.T) {
	tests := []struct {
		name     string
		expr     func(*ast.IdentExpr) ast.Expr
		wantKind CheckKind
		literal  typ.Type
		field    string
	}{
		{
			name: "field equal literal",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "kind"), Rhs: stringLit("dog")}
			},
			wantKind: CheckLiteralEqual,
			literal:  typ.LiteralString("dog"),
			field:    "kind",
		},
		{
			name: "literal not equal field",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: stringLit("cat"), Rhs: dot(root, "kind")}
			},
			wantKind: CheckLiteralNot,
			literal:  typ.LiteralString("cat"),
			field:    "kind",
		},
		{
			name: "field equal true",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "ok"), Rhs: &ast.TrueExpr{}}
			},
			wantKind: CheckLiteralEqual,
			literal:  typ.LiteralBool(true),
			field:    "ok",
		},
		{
			name: "field not equal false",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: dot(root, "ok"), Rhs: &ast.FalseExpr{}}
			},
			wantKind: CheckLiteralNot,
			literal:  typ.LiteralBool(false),
			field:    "ok",
		},
		{
			name: "field equal integer",
			expr: func(root *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: dot(root, "code"), Rhs: number("1")}
			},
			wantKind: CheckLiteralEqual,
			literal:  typ.LiteralInt(1),
			field:    "code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := ident("obj")
			expr := tt.expr(root)
			bindings := bindReturn(expr)
			got := Normalize(expr, bindings)
			wantPath := path.NewPath(mustIdentSymbol(t, bindings, root), "obj").Field(tt.field)
			assertCheck(t, got, tt.wantKind, wantPath, "")
			assertLiteralTypeCheck(t, got, tt.literal)
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

func lenOf(expr ast.Expr) *ast.UnaryLenOpExpr {
	return &ast.UnaryLenOpExpr{Expr: expr}
}

func TestNormalizeLengthFloorGuards(t *testing.T) {
	tests := []struct {
		name      string
		build     func(arr *ast.IdentExpr) ast.Expr
		wantFloor int64
	}{
		{
			name: "greater than zero",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: ">", Lhs: lenOf(arr), Rhs: number("0")}
			},
			wantFloor: 1,
		},
		{
			name: "greater equal one",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: ">=", Lhs: lenOf(arr), Rhs: number("1")}
			},
			wantFloor: 1,
		},
		{
			name: "not equal zero",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "~=", Lhs: lenOf(arr), Rhs: number("0")}
			},
			wantFloor: 1,
		},
		{
			name: "reversed zero less than len",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "<", Lhs: number("0"), Rhs: lenOf(arr)}
			},
			wantFloor: 1,
		},
		{
			name: "greater than two",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: ">", Lhs: lenOf(arr), Rhs: number("2")}
			},
			wantFloor: 3,
		},
		{
			name: "equal two",
			build: func(arr *ast.IdentExpr) ast.Expr {
				return &ast.RelationalOpExpr{Operator: "==", Lhs: lenOf(arr), Rhs: number("2")}
			},
			wantFloor: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arr := ident("xs")
			expr := tt.build(arr)
			bindings := bindReturn(expr)
			check := Normalize(expr, bindings)
			if check.Kind != CheckLenGe {
				t.Fatalf("want CheckLenGe, got kind %d", check.Kind)
			}
			if check.LenFloor != tt.wantFloor {
				t.Fatalf("want floor %d, got %d", tt.wantFloor, check.LenFloor)
			}
			wantPath := path.NewPath(mustIdentSymbol(t, bindings, arr), "xs")
			if check.Path.Key() != wantPath.Key() {
				t.Fatalf("want array path %v, got %v", wantPath, check.Path)
			}
		})
	}
}

func TestNormalizeLengthFloorNotEqualZeroHasNoFalseEdgeFloor(t *testing.T) {
	// `len ~= 0` proves non-empty on its true edge, but its false edge does not
	// establish a positive length floor.
	for _, op := range []string{"~="} {
		arr := ident("xs")
		expr := &ast.RelationalOpExpr{Operator: op, Lhs: lenOf(arr), Rhs: number("0")}
		bindings := bindReturn(expr)
		if check := Normalize(expr, bindings); check.Kind == CheckLenGe {
			if check.Negated {
				t.Fatalf("operator %q must not produce a negated length floor for zero", op)
			}
		}
	}
}

func TestNormalizeLengthFloorNegatedFalseEdge(t *testing.T) {
	// `#xs < c` / `#xs <= c` establish a length floor on the FALSE edge (the
	// `if #xs < lo then error end` guard form): not(#xs<5) is #xs>=5, and
	// not(#xs<=5) is #xs>=6.
	for _, tc := range []struct {
		op    string
		floor int64
	}{{"<", 5}, {"<=", 6}} {
		arr := ident("xs")
		expr := &ast.RelationalOpExpr{Operator: tc.op, Lhs: lenOf(arr), Rhs: number("5")}
		bindings := bindReturn(expr)
		check := Normalize(expr, bindings)
		if check.Kind != CheckLenGe || !check.Negated || check.LenFloor != tc.floor {
			t.Fatalf("operator %q: got kind=%v negated=%v floor=%d, want CheckLenGe negated floor=%d",
				tc.op, check.Kind, check.Negated, check.LenFloor, tc.floor)
		}
	}
}

func TestNormalizeLengthFloorEqualZeroFalseEdge(t *testing.T) {
	arr := ident("xs")
	expr := &ast.RelationalOpExpr{Operator: "==", Lhs: lenOf(arr), Rhs: number("0")}
	bindings := bindReturn(expr)
	check := Normalize(expr, bindings)
	if check.Kind != CheckLenGe || !check.Negated || check.LenFloor != 1 {
		t.Fatalf("got kind=%v negated=%v floor=%d, want CheckLenGe negated floor=1",
			check.Kind, check.Negated, check.LenFloor)
	}
}

func TestNormalizeLengthFloorNotEqualPositiveFalseEdge(t *testing.T) {
	arr := ident("xs")
	expr := &ast.RelationalOpExpr{Operator: "~=", Lhs: lenOf(arr), Rhs: number("2")}
	bindings := bindReturn(expr)
	check := Normalize(expr, bindings)
	if check.Kind != CheckLenGe || !check.Negated || check.LenFloor != 2 {
		t.Fatalf("got kind=%v negated=%v floor=%d, want CheckLenGe negated floor=2",
			check.Kind, check.Negated, check.LenFloor)
	}
}
