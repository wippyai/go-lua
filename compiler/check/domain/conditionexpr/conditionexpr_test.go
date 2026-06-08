package conditionexpr_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/domain/conditionexpr"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
)

func TestConditions_TruthyAttrEmitsCanonicalPathAndFieldPresence(t *testing.T) {
	event := &ast.IdentExpr{Value: "event"}
	payload := &ast.AttrGetExpr{
		Object:    event,
		Key:       &ast.StringExpr{Value: "payload"},
		KeySyntax: ast.AttrKeyDot,
	}
	from := &ast.AttrGetExpr{
		Object:    payload,
		Key:       &ast.StringExpr{Value: "from"},
		KeySyntax: ast.AttrKeyDot,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"event"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: from, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	sym, ok := bindings.SymbolOf(event)
	if !ok || sym == 0 {
		t.Fatal("expected event symbol")
	}

	branches := conditionexpr.Extractor{Bindings: bindings}.Conditions(from)
	wantPath := constraint.NewPath(sym, "event").
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "payload"}).
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "from"})
	wantBase := constraint.NewPath(sym, "event").
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "payload"})

	if !conditionContains(branches.OnTrue, constraint.Truthy{Path: wantPath}) {
		t.Fatalf("true branch did not contain truthy(%s): %#v", wantPath.String(), branches.OnTrue)
	}
	if !conditionContains(branches.OnTrue, constraint.HasField{Path: wantBase, Field: "from"}) {
		t.Fatalf("true branch did not contain has-field(%s.from): %#v", wantBase.String(), branches.OnTrue)
	}
	if !conditionContains(branches.OnFalse, constraint.Falsy{Path: wantPath}) {
		t.Fatalf("false branch did not contain falsy(%s): %#v", wantPath.String(), branches.OnFalse)
	}
}

func TestConditions_StaticIndexStringDoesNotCollapseToDotField(t *testing.T) {
	payload := &ast.IdentExpr{Value: "payload"}
	field := &ast.AttrGetExpr{
		Object:    payload,
		Key:       &ast.StringExpr{Value: "x-y"},
		KeySyntax: ast.AttrKeyIndex,
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"payload"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: field, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	sym, ok := bindings.SymbolOf(payload)
	if !ok || sym == 0 {
		t.Fatal("expected payload symbol")
	}

	branches := conditionexpr.Extractor{Bindings: bindings}.Conditions(field)
	want := constraint.NewPath(sym, "payload").
		Append(constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"})
	wrong := constraint.NewPath(sym, "payload").
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "x-y"})

	if !conditionContains(branches.OnTrue, constraint.Truthy{Path: want}) {
		t.Fatalf("true branch did not contain truthy(%s): %#v", want.String(), branches.OnTrue)
	}
	if conditionContains(branches.OnTrue, constraint.Truthy{Path: wrong}) {
		t.Fatalf("bracket string index collapsed to dot field: %#v", branches.OnTrue)
	}
}

func TestConditions_TypeProbeEmitsCanonicalPathPredicates(t *testing.T) {
	page := &ast.IdentExpr{Value: "page"}
	placement := &ast.AttrGetExpr{
		Object:    page,
		Key:       &ast.StringExpr{Value: "placement"},
		KeySyntax: ast.AttrKeyDot,
	}
	cond := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "type"},
			Args: []ast.Expr{placement},
		},
		Rhs: &ast.StringExpr{Value: "string"},
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"page"}},
		Stmts: []ast.Stmt{
			&ast.IfStmt{Condition: cond, Then: []ast.Stmt{&ast.ReturnStmt{}}},
		},
	}
	bindings := bind.Bind(fn, nil)
	sym, ok := bindings.SymbolOf(page)
	if !ok || sym == 0 {
		t.Fatal("expected page symbol")
	}

	branches := conditionexpr.Extractor{Bindings: bindings}.Conditions(cond)
	path := constraint.NewPath(sym, "page").
		Append(constraint.Segment{Kind: constraint.SegmentField, Name: "placement"})
	key := narrow.BuiltinTypeKey("string")

	if !conditionContains(branches.OnTrue, constraint.HasType{Path: path, Type: key}) {
		t.Fatalf("true branch did not contain hastype(%s): %#v", path.String(), branches.OnTrue)
	}
	if !conditionContains(branches.OnFalse, constraint.NotHasType{Path: path, Type: key}) {
		t.Fatalf("false branch did not contain not-hastype(%s): %#v", path.String(), branches.OnFalse)
	}
}

func conditionContains(cond constraint.Condition, want constraint.Constraint) bool {
	for _, got := range cond.AllConstraints() {
		if got.Equals(want) {
			return true
		}
	}
	return false
}
